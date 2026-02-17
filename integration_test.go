package main_test

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pankaj/simple-chat/protocol"
	"github.com/pankaj/simple-chat/server"
)

// testClient wraps a connection with a persistent buffered reader.
type testClient struct {
	conn   net.Conn
	reader *bufio.Reader
}

func startTestServer(t *testing.T) string {
	t.Helper()
	srv := server.New()
	if err := srv.Listen(":0"); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	t.Cleanup(func() { srv.Shutdown() })
	return srv.Addr().String()
}

func joinTestClient(t *testing.T, addr, username string) *testClient {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	tc := &testClient{conn: conn, reader: bufio.NewReader(conn)}

	fmt.Fprintf(conn, "%s\n", protocol.Encode(protocol.Message{
		Type:     protocol.TypeJoin,
		Username: username,
	}))

	line := tc.readLine(t, 2*time.Second)
	msg, err := protocol.Decode(line)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if msg.Type != protocol.TypeOK {
		t.Fatalf("expected OK for %s, got %s: %s", username, msg.Type, msg.Body)
	}
	return tc
}

func (tc *testClient) readLine(t *testing.T, timeout time.Duration) string {
	t.Helper()
	tc.conn.SetReadDeadline(time.Now().Add(timeout))
	line, err := tc.reader.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read line: %v", err)
	}
	tc.conn.SetReadDeadline(time.Time{})
	return strings.TrimRight(line, "\n")
}

func (tc *testClient) sendMsg(t *testing.T, body string) {
	t.Helper()
	fmt.Fprintf(tc.conn, "%s\n", protocol.Encode(protocol.Message{
		Type: protocol.TypeSend,
		Body: body,
	}))
}

func (tc *testClient) sendLeave(t *testing.T) {
	t.Helper()
	fmt.Fprintf(tc.conn, "%s\n", protocol.Encode(protocol.Message{Type: protocol.TypeLeave}))
}

func TestIntegrationSingleClientJoinAndLeave(t *testing.T) {
	addr := startTestServer(t)
	tc := joinTestClient(t, addr, "alice")
	tc.sendLeave(t)
}

func TestIntegrationDuplicateUsername(t *testing.T) {
	addr := startTestServer(t)
	_ = joinTestClient(t, addr, "alice")

	// Second client with the same username.
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	tc := &testClient{conn: conn, reader: bufio.NewReader(conn)}
	fmt.Fprintf(conn, "%s\n", protocol.Encode(protocol.Message{
		Type:     protocol.TypeJoin,
		Username: "alice",
	}))

	line := tc.readLine(t, 2*time.Second)
	msg, err := protocol.Decode(line)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if msg.Type != protocol.TypeErr {
		t.Fatalf("expected ERR, got %s", msg.Type)
	}
}

func TestIntegrationMessageBroadcast(t *testing.T) {
	addr := startTestServer(t)

	alice := joinTestClient(t, addr, "alice")
	bob := joinTestClient(t, addr, "bob")

	// Drain JOINED notification on alice.
	alice.readLine(t, 2*time.Second)

	// Alice sends a message.
	alice.sendMsg(t, "hello bob")

	// Bob receives it.
	line := bob.readLine(t, 2*time.Second)
	msg, err := protocol.Decode(line)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if msg.Type != protocol.TypeMsg || msg.Username != "alice" || msg.Body != "hello bob" {
		t.Errorf("unexpected message: %+v", msg)
	}
}

func TestIntegrationJoinNotification(t *testing.T) {
	addr := startTestServer(t)

	alice := joinTestClient(t, addr, "alice")
	_ = joinTestClient(t, addr, "bob")

	line := alice.readLine(t, 2*time.Second)
	msg, err := protocol.Decode(line)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if msg.Type != protocol.TypeJoined || msg.Username != "bob" {
		t.Errorf("expected JOINED|bob, got %+v", msg)
	}
}

func TestIntegrationLeaveNotification(t *testing.T) {
	addr := startTestServer(t)

	alice := joinTestClient(t, addr, "alice")
	bob := joinTestClient(t, addr, "bob")

	// Drain JOINED notification on alice.
	alice.readLine(t, 2*time.Second)

	// Bob leaves.
	bob.sendLeave(t)
	bob.conn.Close()

	line := alice.readLine(t, 2*time.Second)
	msg, err := protocol.Decode(line)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if msg.Type != protocol.TypeLeft || msg.Username != "bob" {
		t.Errorf("expected LEFT|bob, got %+v", msg)
	}
}

func TestIntegrationDisconnectCleanup(t *testing.T) {
	addr := startTestServer(t)

	alice := joinTestClient(t, addr, "alice")
	bob := joinTestClient(t, addr, "bob")

	// Drain JOINED notification on alice.
	alice.readLine(t, 2*time.Second)

	// Abruptly close bob's connection.
	bob.conn.Close()

	// Alice should receive LEFT|bob.
	line := alice.readLine(t, 2*time.Second)
	msg, err := protocol.Decode(line)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if msg.Type != protocol.TypeLeft || msg.Username != "bob" {
		t.Errorf("expected LEFT|bob, got %+v", msg)
	}

	// Wait for server cleanup.
	time.Sleep(100 * time.Millisecond)

	// A new "bob" should be able to join.
	newBob := joinTestClient(t, addr, "bob")
	newBob.sendLeave(t)
}

func TestIntegrationManyConcurrentClients(t *testing.T) {
	addr := startTestServer(t)

	const numClients = 50
	clients := make([]*testClient, numClients)

	// Connect all clients sequentially to avoid race on JOINED notifications.
	for i := 0; i < numClients; i++ {
		clients[i] = joinTestClient(t, addr, fmt.Sprintf("user%d", i))
	}

	// Drain all JOINED notifications.
	// Client i receives notifications for clients i+1..numClients-1.
	for i := 0; i < numClients; i++ {
		for j := i + 1; j < numClients; j++ {
			clients[i].readLine(t, 2*time.Second)
		}
	}

	// Each client sends one message.
	for i := 0; i < numClients; i++ {
		clients[i].sendMsg(t, fmt.Sprintf("hello from user%d", i))
	}

	// Each client should receive numClients-1 MSG messages.
	var wg sync.WaitGroup
	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			received := 0
			for received < numClients-1 {
				clients[idx].conn.SetReadDeadline(time.Now().Add(5 * time.Second))
				line, err := clients[idx].reader.ReadString('\n')
				if err != nil {
					t.Errorf("user%d: failed to read after %d messages: %v", idx, received, err)
					return
				}
				line = strings.TrimRight(line, "\n")
				msg, err := protocol.Decode(line)
				if err != nil {
					t.Errorf("user%d: decode error: %v", idx, err)
					return
				}
				if msg.Type == protocol.TypeMsg {
					received++
				}
			}
		}(i)
	}
	wg.Wait()

	// Disconnect all.
	for i := 0; i < numClients; i++ {
		clients[i].sendLeave(t)
	}
}
