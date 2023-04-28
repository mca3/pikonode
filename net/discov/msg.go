package discov

import (
	"encoding/binary"
)

// Discovery message commands.
const (
	// 0x00 - Reserved for future revisions.
	_ byte = 0x00

	// 0x01 - Hello
	// Broadcasts your existance on the network.
	//
	// Payload:
	// - uint16: Listening port for WireGuard
	// - [44]byte: Base64 WireGuard public key.
	Hello = 0x01

	// 0x02 - Hello Reply
	// Broadcasts your existance on the network.
	//
	// The payload of this is the same as 0x01 Hello; this type explicitly
	// marks this message as a reply to an earlier Hello message.
	// Send these as a reply to 0x01, and not in any other case.
	//
	// Payload:
	// - uint16: Listening port for WireGuard
	// - [44]byte: Base64 WireGuard public key.
	HelloReply = 0x02
)

// Message holds a decoded message and all associated data with it.
type Message struct {
	Type byte

	Port uint16
	Key  string
}

func NewHello(port uint16, key string, reply bool) []byte {
	buf := [51]byte{}

	copy(buf[:4], "PIKO")

	if reply {
		// The semantics of Hello Reply is the same, but Hello Reply
		// must not be sent as a result of another Hello Reply.
		// Only as a result of a Hello.
		buf[4] = HelloReply
	} else {
		buf[4] = Hello
	}

	binary.BigEndian.PutUint16(buf[5:7], uint16(port))
	copy(buf[7:], key)

	return buf[:]
}

// Parse parses a message.
//
// The "PIKO" header is not checked for its existance or validity.
func Parse(buf []byte) Message {
	switch buf[4] {
	case Hello, HelloReply:
		port := binary.BigEndian.Uint16(buf[5:7])
		key := string(buf[7:51])
		return Message{
			Type: buf[4],
			Port: port,
			Key:  key,
		}
	default:
		return Message{
			Type: buf[4],
		}
	}
}
