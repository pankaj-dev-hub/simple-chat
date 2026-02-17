package client

import (
	"bufio"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/pankaj/simple-chat/protocol"
)

// mockServer creates a TCP listener that handles one connection with a
// custom handler function. Returns the listener address.
func mockServer(t *testing.T, handler func(net.Conn)) string {
	t.Helper()
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		handler(conn)
	}()

	return ln.Addr().String()
}

func TestNewConnectsAndJoins(t *testing.T) {
	addr := mockServer(t, func(conn net.Conn) {
		scanner := bufio.NewScanner(conn)
		if !scanner.Scan() {
			return
		}
		msg, err := protocol.Decode(scanner.Text())
		if err != nil {
			t.Errorf("mock server decode error: %v", err)
			return
		}
		if msg.Type != protocol.TypeJoin {
			t.Errorf("expected JOIN, got %s", msg.Type)
			return
		}
		if msg.Username != "testuser" {
			t.Errorf("expected username 'testuser', got %q", msg.Username)
		}
		fmt.Fprintf(conn, "%s\n", protocol.Encode(protocol.Message{Type: protocol.TypeOK}))
		// Keep connection alive briefly.
		time.Sleep(100 * time.Millisecond)
	})

	c, err := New(addr, "testuser")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	c.conn.Close()
}

func TestNewRejectsOnError(t *testing.T) {
	addr := mockServer(t, func(conn net.Conn) {
		scanner := bufio.NewScanner(conn)
		scanner.Scan()
		fmt.Fprintf(conn, "%s\n", protocol.Encode(protocol.Message{
			Type: protocol.TypeErr,
			Body: "username taken",
		}))
	})

	_, err := New(addr, "testuser")
	if err == nil {
		t.Fatal("New() expected error, got nil")
	}
	if got := err.Error(); got != "server rejected join: username taken" {
		t.Errorf("unexpected error: %s", got)
	}
}

func TestCloseSendsLeave(t *testing.T) {
	received := make(chan string, 1)

	addr := mockServer(t, func(conn net.Conn) {
		scanner := bufio.NewScanner(conn)
		// Read JOIN.
		scanner.Scan()
		fmt.Fprintf(conn, "%s\n", protocol.Encode(protocol.Message{Type: protocol.TypeOK}))
		// Read LEAVE.
		if scanner.Scan() {
			received <- scanner.Text()
		}
	})

	c, err := New(addr, "testuser")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	c.Close()

	select {
	case line := <-received:
		msg, err := protocol.Decode(line)
		if err != nil {
			t.Fatalf("decode error: %v", err)
		}
		if msg.Type != protocol.TypeLeave {
			t.Errorf("expected LEAVE, got %s", msg.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for LEAVE message")
	}
}
