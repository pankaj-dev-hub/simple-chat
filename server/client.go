package server

import (
	"bufio"
	"fmt"
	"log"
	"net"

	"github.com/pankaj/simple-chat/protocol"
)

const outboxSize = 256

// ConnectedClient represents a single TCP connection after a successful JOIN.
type ConnectedClient struct {
	username string
	conn     net.Conn
	server   *ChatServer
	outbox   chan string
	done     chan struct{}
}

func newConnectedClient(username string, conn net.Conn, srv *ChatServer) *ConnectedClient {
	return &ConnectedClient{
		username: username,
		conn:     conn,
		server:   srv,
		outbox:   make(chan string, outboxSize),
		done:     make(chan struct{}),
	}
}

// Send enqueues a message to the client's outbox. Non-blocking: drops
// the message if the buffer is full (protects against slow clients).
func (c *ConnectedClient) Send(line string) {
	select {
	case c.outbox <- line:
	default:
		log.Printf("dropping message for slow client %s", c.username)
	}
}

// readLoop reads lines from the TCP connection and dispatches them.
func (c *ConnectedClient) readLoop() {
	scanner := bufio.NewScanner(c.conn)
	scanner.Buffer(make([]byte, 4096), 4096)

	for scanner.Scan() {
		msg, err := protocol.Decode(scanner.Text())
		if err != nil {
			continue
		}

		switch msg.Type {
		case protocol.TypeSend:
			line := protocol.Encode(protocol.Message{
				Type:     protocol.TypeMsg,
				Username: c.username,
				Body:     msg.Body,
			})
			c.server.broadcast(c.username, line)

		case protocol.TypeLeave:
			return
		}
	}
}

// writeLoop drains the outbox channel and writes each message to the connection.
func (c *ConnectedClient) writeLoop() {
	for {
		select {
		case msg := <-c.outbox:
			_, err := fmt.Fprintf(c.conn, "%s\n", msg)
			if err != nil {
				return
			}
		case <-c.done:
			// Drain remaining messages
			for {
				select {
				case msg := <-c.outbox:
					fmt.Fprintf(c.conn, "%s\n", msg)
				default:
					return
				}
			}
		}
	}
}
