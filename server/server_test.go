package server

import (
	"bufio"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/pankaj/simple-chat/protocol"
)

// helper: connect a raw TCP client, send JOIN, wait for OK.
func connectClient(t *testing.T, addr, username string) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	fmt.Fprintf(conn, "%s\n", protocol.Encode(protocol.Message{Type: protocol.TypeJoin, Username: username}))
	line := readLine(t, conn, 2*time.Second)
	msg, err := protocol.Decode(line)
	if err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if msg.Type != protocol.TypeOK {
		t.Fatalf("expected OK, got %s: %s", msg.Type, msg.Body)
	}
	return conn
}

// helper: read one line from a connection with a timeout.
func readLine(t *testing.T, conn net.Conn, timeout time.Duration) string {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(timeout))
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		t.Fatalf("failed to read line: %v", scanner.Err())
	}
	conn.SetReadDeadline(time.Time{})
	return scanner.Text()
}

func startServer(t *testing.T) *ChatServer {
	t.Helper()
	srv := New()
	if err := srv.Listen(":0"); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	t.Cleanup(func() { srv.Shutdown() })
	return srv
}

func TestAddClientUniqueness(t *testing.T) {
	srv := New()
	c1 := &ConnectedClient{username: "alice", outbox: make(chan string, 1)}
	c2 := &ConnectedClient{username: "alice", outbox: make(chan string, 1)}

	if !srv.addClient(c1) {
		t.Fatal("first addClient should succeed")
	}
	if srv.addClient(c2) {
		t.Fatal("second addClient with same username should fail")
	}
}

func TestRemoveClient(t *testing.T) {
	srv := New()
	c := &ConnectedClient{username: "alice", outbox: make(chan string, 1)}
	srv.addClient(c)
	srv.removeClient("alice")

	srv.mu.RLock()
	_, exists := srv.clients["alice"]
	srv.mu.RUnlock()
	if exists {
		t.Fatal("client should have been removed")
	}
}

func TestBroadcastExcludesSender(t *testing.T) {
	srv := New()
	c1 := &ConnectedClient{username: "alice", outbox: make(chan string, 10)}
	c2 := &ConnectedClient{username: "bob", outbox: make(chan string, 10)}
	c3 := &ConnectedClient{username: "charlie", outbox: make(chan string, 10)}

	srv.addClient(c1)
	srv.addClient(c2)
	srv.addClient(c3)

	srv.broadcast("alice", "MSG|alice|hello")

	// alice should NOT receive
	select {
	case <-c1.outbox:
		t.Fatal("sender should not receive their own broadcast")
	default:
	}

	// bob and charlie should receive
	for _, c := range []*ConnectedClient{c2, c3} {
		select {
		case msg := <-c.outbox:
			if msg != "MSG|alice|hello" {
				t.Errorf("expected MSG|alice|hello, got %s", msg)
			}
		default:
			t.Errorf("client %s should have received the broadcast", c.username)
		}
	}
}

func TestSendNonBlocking(t *testing.T) {
	c := &ConnectedClient{username: "alice", outbox: make(chan string, 1)}
	c.Send("msg1")
	c.Send("msg2") // should not block, msg2 gets dropped

	select {
	case msg := <-c.outbox:
		if msg != "msg1" {
			t.Errorf("expected msg1, got %s", msg)
		}
	default:
		t.Fatal("outbox should have msg1")
	}
}

func TestHandleConnectionBadFirstMessage(t *testing.T) {
	srv := startServer(t)
	addr := srv.Addr().String()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	// Send a SEND message as the first message (should get ERR).
	fmt.Fprintf(conn, "%s\n", protocol.Encode(protocol.Message{Type: protocol.TypeSend, Body: "hello"}))
	line := readLine(t, conn, 2*time.Second)
	msg, err := protocol.Decode(line)
	if err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if msg.Type != protocol.TypeErr {
		t.Fatalf("expected ERR, got %s", msg.Type)
	}
}

func TestHandleConnectionDuplicateUsername(t *testing.T) {
	srv := startServer(t)
	addr := srv.Addr().String()

	conn1 := connectClient(t, addr, "alice")
	defer conn1.Close()

	// Second connection with the same username.
	conn2, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn2.Close()

	fmt.Fprintf(conn2, "%s\n", protocol.Encode(protocol.Message{Type: protocol.TypeJoin, Username: "alice"}))
	line := readLine(t, conn2, 2*time.Second)
	msg, err := protocol.Decode(line)
	if err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if msg.Type != protocol.TypeErr {
		t.Fatalf("expected ERR for duplicate username, got %s", msg.Type)
	}
	if msg.Body != "username taken" {
		t.Errorf("expected 'username taken', got %q", msg.Body)
	}
}

func TestMessageBroadcast(t *testing.T) {
	srv := startServer(t)
	addr := srv.Addr().String()

	alice := connectClient(t, addr, "alice")
	defer alice.Close()

	bob := connectClient(t, addr, "bob")
	defer bob.Close()

	// Drain the JOINED notification that alice receives when bob joins.
	readLine(t, alice, 2*time.Second)

	// Alice sends a message.
	fmt.Fprintf(alice, "%s\n", protocol.Encode(protocol.Message{Type: protocol.TypeSend, Body: "hello bob"}))

	// Bob should receive it.
	line := readLine(t, bob, 2*time.Second)
	msg, err := protocol.Decode(line)
	if err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if msg.Type != protocol.TypeMsg {
		t.Fatalf("expected MSG, got %s", msg.Type)
	}
	if msg.Username != "alice" {
		t.Errorf("expected username 'alice', got %q", msg.Username)
	}
	if msg.Body != "hello bob" {
		t.Errorf("expected body 'hello bob', got %q", msg.Body)
	}
}

func TestJoinNotification(t *testing.T) {
	srv := startServer(t)
	addr := srv.Addr().String()

	alice := connectClient(t, addr, "alice")
	defer alice.Close()

	bob := connectClient(t, addr, "bob")
	defer bob.Close()

	// Alice should receive a JOINED notification for bob.
	line := readLine(t, alice, 2*time.Second)
	msg, err := protocol.Decode(line)
	if err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if msg.Type != protocol.TypeJoined {
		t.Fatalf("expected JOINED, got %s", msg.Type)
	}
	if msg.Username != "bob" {
		t.Errorf("expected username 'bob', got %q", msg.Username)
	}
}

func TestLeaveNotification(t *testing.T) {
	srv := startServer(t)
	addr := srv.Addr().String()

	alice := connectClient(t, addr, "alice")
	defer alice.Close()

	bob := connectClient(t, addr, "bob")

	// Drain the JOINED notification.
	readLine(t, alice, 2*time.Second)

	// Bob sends LEAVE.
	fmt.Fprintf(bob, "%s\n", protocol.Encode(protocol.Message{Type: protocol.TypeLeave}))
	bob.Close()

	// Alice should receive a LEFT notification.
	line := readLine(t, alice, 2*time.Second)
	msg, err := protocol.Decode(line)
	if err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if msg.Type != protocol.TypeLeft {
		t.Fatalf("expected LEFT, got %s", msg.Type)
	}
	if msg.Username != "bob" {
		t.Errorf("expected username 'bob', got %q", msg.Username)
	}
}
