package protocol

import (
	"testing"
)

func TestEncodeRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		msg  Message
		want string
	}{
		{"JOIN", Message{Type: TypeJoin, Username: "alice"}, "JOIN|alice"},
		{"SEND", Message{Type: TypeSend, Body: "hello world"}, "SEND|hello world"},
		{"LEAVE", Message{Type: TypeLeave}, "LEAVE"},
		{"OK", Message{Type: TypeOK}, "OK"},
		{"ERR", Message{Type: TypeErr, Body: "username taken"}, "ERR|username taken"},
		{"MSG", Message{Type: TypeMsg, Username: "bob", Body: "hi there"}, "MSG|bob|hi there"},
		{"JOINED", Message{Type: TypeJoined, Username: "charlie"}, "JOINED|charlie"},
		{"LEFT", Message{Type: TypeLeft, Username: "dave"}, "LEFT|dave"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := Encode(tt.msg)
			if encoded != tt.want {
				t.Errorf("Encode() = %q, want %q", encoded, tt.want)
			}

			decoded, err := Decode(encoded)
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			if decoded.Type != tt.msg.Type {
				t.Errorf("Decode().Type = %q, want %q", decoded.Type, tt.msg.Type)
			}
			if decoded.Username != tt.msg.Username {
				t.Errorf("Decode().Username = %q, want %q", decoded.Username, tt.msg.Username)
			}
			if decoded.Body != tt.msg.Body {
				t.Errorf("Decode().Body = %q, want %q", decoded.Body, tt.msg.Body)
			}
		})
	}
}

func TestDecodeValid(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  Message
	}{
		{"JOIN", "JOIN|alice", Message{Type: TypeJoin, Username: "alice"}},
		{"SEND", "SEND|hello", Message{Type: TypeSend, Body: "hello"}},
		{"LEAVE", "LEAVE", Message{Type: TypeLeave}},
		{"OK", "OK", Message{Type: TypeOK}},
		{"ERR", "ERR|bad", Message{Type: TypeErr, Body: "bad"}},
		{"MSG", "MSG|bob|hello", Message{Type: TypeMsg, Username: "bob", Body: "hello"}},
		{"JOINED", "JOINED|eve", Message{Type: TypeJoined, Username: "eve"}},
		{"LEFT", "LEFT|frank", Message{Type: TypeLeft, Username: "frank"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Decode(tt.input)
			if err != nil {
				t.Fatalf("Decode(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("Decode(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}

func TestDecodeInvalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"unknown type", "UNKNOWN|data"},
		{"JOIN without username", "JOIN|"},
		{"JOIN no payload", "JOIN"},
		{"SEND without body", "SEND|"},
		{"SEND no payload", "SEND"},
		{"ERR without body", "ERR|"},
		{"ERR no payload", "ERR"},
		{"MSG missing body", "MSG|bob"},
		{"MSG empty body", "MSG|bob|"},
		{"MSG empty username", "MSG||hello"},
		{"MSG no payload", "MSG"},
		{"JOINED without username", "JOINED|"},
		{"JOINED no payload", "JOINED"},
		{"LEFT without username", "LEFT|"},
		{"LEFT no payload", "LEFT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Decode(tt.input)
			if err == nil {
				t.Errorf("Decode(%q) expected error, got nil", tt.input)
			}
		})
	}
}

func TestDecodeMessageBodyWithPipes(t *testing.T) {
	input := "MSG|alice|hello|world|foo"
	got, err := Decode(input)
	if err != nil {
		t.Fatalf("Decode(%q) error = %v", input, err)
	}
	if got.Type != TypeMsg {
		t.Errorf("Type = %q, want %q", got.Type, TypeMsg)
	}
	if got.Username != "alice" {
		t.Errorf("Username = %q, want %q", got.Username, "alice")
	}
	if got.Body != "hello|world|foo" {
		t.Errorf("Body = %q, want %q", got.Body, "hello|world|foo")
	}
}

func TestDecodeSendBodyWithPipes(t *testing.T) {
	input := "SEND|hello|world|test"
	got, err := Decode(input)
	if err != nil {
		t.Fatalf("Decode(%q) error = %v", input, err)
	}
	if got.Type != TypeSend {
		t.Errorf("Type = %q, want %q", got.Type, TypeSend)
	}
	if got.Body != "hello|world|test" {
		t.Errorf("Body = %q, want %q", got.Body, "hello|world|test")
	}
}

func TestEncodeUnknownType(t *testing.T) {
	encoded := Encode(Message{Type: "UNKNOWN"})
	if encoded != "" {
		t.Errorf("Encode(unknown) = %q, want empty string", encoded)
	}
}
