package protocol

import (
	"errors"
	"strings"
)

// Message types sent from client to server.
const (
	TypeJoin  = "JOIN"
	TypeSend  = "SEND"
	TypeLeave = "LEAVE"
)

// Message types sent from server to client.
const (
	TypeOK     = "OK"
	TypeErr    = "ERR"
	TypeMsg    = "MSG"
	TypeJoined = "JOINED"
	TypeLeft   = "LEFT"
)

// Message represents a parsed protocol message.
type Message struct {
	Type     string // One of the Type* constants
	Username string // Populated for JOIN, MSG, JOINED, LEFT
	Body     string // Populated for SEND, MSG, ERR
}

// ErrInvalidMessage is returned when a message cannot be parsed.
var ErrInvalidMessage = errors.New("invalid message format")

// Encode serializes a Message into a wire-format string (without trailing newline).
func Encode(m Message) string {
	switch m.Type {
	case TypeJoin:
		return TypeJoin + "|" + m.Username
	case TypeSend:
		return TypeSend + "|" + m.Body
	case TypeLeave:
		return TypeLeave
	case TypeOK:
		return TypeOK
	case TypeErr:
		return TypeErr + "|" + m.Body
	case TypeMsg:
		return TypeMsg + "|" + m.Username + "|" + m.Body
	case TypeJoined:
		return TypeJoined + "|" + m.Username
	case TypeLeft:
		return TypeLeft + "|" + m.Username
	default:
		return ""
	}
}

// Decode parses a single wire-format line (without trailing newline) into a Message.
func Decode(line string) (Message, error) {
	if line == "" {
		return Message{}, ErrInvalidMessage
	}

	parts := strings.SplitN(line, "|", 2)
	msgType := parts[0]

	switch msgType {
	case TypeJoin:
		if len(parts) < 2 || parts[1] == "" {
			return Message{}, ErrInvalidMessage
		}
		return Message{Type: TypeJoin, Username: parts[1]}, nil

	case TypeSend:
		if len(parts) < 2 || parts[1] == "" {
			return Message{}, ErrInvalidMessage
		}
		return Message{Type: TypeSend, Body: parts[1]}, nil

	case TypeLeave:
		return Message{Type: TypeLeave}, nil

	case TypeOK:
		return Message{Type: TypeOK}, nil

	case TypeErr:
		if len(parts) < 2 || parts[1] == "" {
			return Message{}, ErrInvalidMessage
		}
		return Message{Type: TypeErr, Body: parts[1]}, nil

	case TypeMsg:
		if len(parts) < 2 {
			return Message{}, ErrInvalidMessage
		}
		// Split the payload further to get username and body
		subParts := strings.SplitN(parts[1], "|", 2)
		if len(subParts) < 2 || subParts[0] == "" || subParts[1] == "" {
			return Message{}, ErrInvalidMessage
		}
		return Message{Type: TypeMsg, Username: subParts[0], Body: subParts[1]}, nil

	case TypeJoined:
		if len(parts) < 2 || parts[1] == "" {
			return Message{}, ErrInvalidMessage
		}
		return Message{Type: TypeJoined, Username: parts[1]}, nil

	case TypeLeft:
		if len(parts) < 2 || parts[1] == "" {
			return Message{}, ErrInvalidMessage
		}
		return Message{Type: TypeLeft, Username: parts[1]}, nil

	default:
		return Message{}, ErrInvalidMessage
	}
}
