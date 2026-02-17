package client

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/pankaj/simple-chat/protocol"
)

// ChatClient manages the connection to the chat server.
type ChatClient struct {
	username string
	conn     net.Conn
	reader   *bufio.Reader
	done     chan struct{}
}

// New creates a ChatClient and connects to the server at addr.
// It sends a JOIN message and waits for OK or ERR.
func New(addr, username string) (*ChatClient, error) {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connecting to server: %w", err)
	}

	// Send JOIN.
	_, err = fmt.Fprintf(conn, "%s\n", protocol.Encode(protocol.Message{
		Type:     protocol.TypeJoin,
		Username: username,
	}))
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("sending JOIN: %w", err)
	}

	// Wait for response.
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	reader := bufio.NewReader(conn)
	line, err := reader.ReadString('\n')
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("reading server response: %v", err)
	}
	conn.SetReadDeadline(time.Time{})

	msg, err := protocol.Decode(strings.TrimRight(line, "\n"))
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("decoding server response: %w", err)
	}

	if msg.Type == protocol.TypeErr {
		conn.Close()
		return nil, fmt.Errorf("server rejected join: %s", msg.Body)
	}

	if msg.Type != protocol.TypeOK {
		conn.Close()
		return nil, fmt.Errorf("unexpected response: %s", msg.Type)
	}

	return &ChatClient{
		username: username,
		conn:     conn,
		reader:   reader,
		done:     make(chan struct{}),
	}, nil
}

// Run starts the interactive REPL. Blocks until the user types "leave"
// or the server disconnects.
func (c *ChatClient) Run() {
	go c.receiveLoop()

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("> ")
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			fmt.Print("> ")
			continue
		}

		if line == "leave" {
			c.Close()
			return
		}

		if strings.HasPrefix(line, "send ") {
			msg := strings.TrimPrefix(line, "send ")
			encoded := protocol.Encode(protocol.Message{Type: protocol.TypeSend, Body: msg})
			fmt.Fprintf(c.conn, "%s\n", encoded)
		} else {
			fmt.Println("Unknown command. Use 'send <message>' or 'leave'.")
		}

		fmt.Print("> ")
	}
}

// Close sends a LEAVE message and closes the connection.
func (c *ChatClient) Close() {
	fmt.Fprintf(c.conn, "%s\n", protocol.Encode(protocol.Message{Type: protocol.TypeLeave}))
	c.conn.Close()
}

// receiveLoop reads messages from the server and prints them.
func (c *ChatClient) receiveLoop() {
	for {
		line, err := c.reader.ReadString('\n')
		if err != nil {
			break
		}
		msg, err := protocol.Decode(strings.TrimRight(line, "\n"))
		if err != nil {
			continue
		}
		switch msg.Type {
		case protocol.TypeMsg:
			fmt.Printf("\n[%s]: %s\n> ", msg.Username, msg.Body)
		case protocol.TypeJoined:
			fmt.Printf("\n* %s has joined the chat *\n> ", msg.Username)
		case protocol.TypeLeft:
			fmt.Printf("\n* %s has left the chat *\n> ", msg.Username)
		case protocol.TypeErr:
			fmt.Printf("\nError: %s\n> ", msg.Body)
		}
	}

	// Server disconnected.
	close(c.done)
	fmt.Println("\nDisconnected from server.")
	os.Exit(0)
}
