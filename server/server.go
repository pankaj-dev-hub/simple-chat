package server

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/pankaj/simple-chat/protocol"
)

// ChatServer manages all connected clients in a single chat room.
type ChatServer struct {
	listener net.Listener
	mu       sync.RWMutex
	clients  map[string]*ConnectedClient
	quit     chan struct{}
	wg       sync.WaitGroup
}

// New creates a new ChatServer.
func New() *ChatServer {
	return &ChatServer{
		clients: make(map[string]*ConnectedClient),
		quit:    make(chan struct{}),
	}
}

// Listen binds to the given address and starts accepting connections.
func (s *ChatServer) Listen(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.listener = ln
	s.wg.Add(1)
	go s.serve()
	return nil
}

// Addr returns the listener's address (useful in tests with ":0" port).
func (s *ChatServer) Addr() net.Addr {
	return s.listener.Addr()
}

// Shutdown gracefully stops the server.
func (s *ChatServer) Shutdown() {
	close(s.quit)
	s.listener.Close()

	s.mu.Lock()
	for _, c := range s.clients {
		c.conn.Close()
	}
	s.mu.Unlock()

	s.wg.Wait()
}

// serve runs the accept loop.
func (s *ChatServer) serve() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.quit:
				return
			default:
				log.Printf("accept error: %v", err)
				continue
			}
		}
		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

// handleConnection manages a single TCP connection from accept to close.
func (s *ChatServer) handleConnection(conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()

	// Set a deadline for the initial JOIN message.
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 4096), 4096)

	if !scanner.Scan() {
		return
	}

	msg, err := protocol.Decode(scanner.Text())
	if err != nil || msg.Type != protocol.TypeJoin {
		fmt.Fprintf(conn, "%s\n", protocol.Encode(protocol.Message{
			Type: protocol.TypeErr,
			Body: "expected JOIN message",
		}))
		return
	}

	username := msg.Username
	if username == "" {
		fmt.Fprintf(conn, "%s\n", protocol.Encode(protocol.Message{
			Type: protocol.TypeErr,
			Body: "username cannot be empty",
		}))
		return
	}

	client := newConnectedClient(username, conn, s)
	if !s.addClient(client) {
		fmt.Fprintf(conn, "%s\n", protocol.Encode(protocol.Message{
			Type: protocol.TypeErr,
			Body: "username taken",
		}))
		return
	}

	// Clear the deadline for normal operation.
	conn.SetReadDeadline(time.Time{})

	// Send OK to the new client.
	fmt.Fprintf(conn, "%s\n", protocol.Encode(protocol.Message{Type: protocol.TypeOK}))

	// Notify others that this user joined.
	s.broadcast(username, protocol.Encode(protocol.Message{
		Type:     protocol.TypeJoined,
		Username: username,
	}))

	// Start read and write loops.
	go client.writeLoop()
	client.readLoop()

	// readLoop returned: the client disconnected or sent LEAVE.
	close(client.done)
	s.removeClient(username)
}

// addClient registers a client. Returns false if the username is taken.
func (s *ChatServer) addClient(c *ConnectedClient) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.clients[c.username]; exists {
		return false
	}
	s.clients[c.username] = c
	return true
}

// removeClient unregisters a client and broadcasts a LEFT message.
func (s *ChatServer) removeClient(username string) {
	s.mu.Lock()
	_, exists := s.clients[username]
	delete(s.clients, username)
	s.mu.Unlock()

	if exists {
		s.broadcast(username, protocol.Encode(protocol.Message{
			Type:     protocol.TypeLeft,
			Username: username,
		}))
	}
}

// broadcast sends a message to all connected clients except the sender.
func (s *ChatServer) broadcast(sender string, line string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for name, c := range s.clients {
		if name != sender {
			c.Send(line)
		}
	}
}
