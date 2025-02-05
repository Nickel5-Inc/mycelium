package protocol

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"

	fb "mycelium/internal/protocol/fb"

	flatbuffers "github.com/google/flatbuffers/go"
)

// Encoding type constants
const (
	EncodingJSON       = 0x1
	EncodingFlatBuffer = 0x2

	// Size threshold in bytes above which we use FlatBuffers
	FlatBufferThreshold = 1024
)

// Message represents a protocol message
type Message struct {
	// Core message fields
	Version   string      `json:"version"`
	Type      MessageType `json:"type"`
	SenderID  string      `json:"sender_id"` // SS58 address of sender
	Timestamp time.Time   `json:"timestamp"`
	Nonce     uint64      `json:"nonce"` // Prevent replay attacks

	// Message content
	Payload map[string]interface{} `json:"payload"` // Type-specific payload

	// Security fields
	Signature string `json:"signature"` // Hex-encoded signature with 0x prefix
}

// Encode serializes the message using either JSON or FlatBuffers based on size
func (m *Message) Encode() ([]byte, error) {
	// First try JSON encoding to check size
	jsonData, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("encoding message to JSON: %w", err)
	}

	// If message is small enough, use JSON
	if len(jsonData) <= FlatBufferThreshold {
		// Prepend encoding type
		data := make([]byte, len(jsonData)+1)
		data[0] = EncodingJSON
		copy(data[1:], jsonData)
		return data, nil
	}

	// Otherwise use FlatBuffers
	builder := flatbuffers.NewBuilder(1024)

	// Create strings
	version := builder.CreateString(m.Version)
	senderID := builder.CreateString(m.SenderID)
	signature := builder.CreateString(m.Signature)

	// Serialize payload to JSON
	payloadBytes, err := json.Marshal(m.Payload)
	if err != nil {
		return nil, fmt.Errorf("encoding payload: %w", err)
	}
	payload := builder.CreateByteVector(payloadBytes)

	// Start building message
	fb.MessageStart(builder)
	fb.MessageAddVersion(builder, version)
	fb.MessageAddType(builder, int8(m.Type))
	fb.MessageAddSenderId(builder, senderID)
	fb.MessageAddTimestamp(builder, m.Timestamp.UnixNano())
	fb.MessageAddNonce(builder, m.Nonce)
	fb.MessageAddPayload(builder, payload)
	fb.MessageAddSignature(builder, signature)
	msg := fb.MessageEnd(builder)

	builder.Finish(msg)

	// Get the finished bytes with encoding type prefix
	buf := builder.FinishedBytes()
	data := make([]byte, len(buf)+1)
	data[0] = EncodingFlatBuffer
	copy(data[1:], buf)

	return data, nil
}

// DecodeMessage deserializes a message from either JSON or FlatBuffers format
func DecodeMessage(data []byte) (*Message, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty message data")
	}

	// Check encoding type
	encodingType := data[0]
	msgData := data[1:]

	switch encodingType {
	case EncodingJSON:
		var msg Message
		if err := json.Unmarshal(msgData, &msg); err != nil {
			return nil, fmt.Errorf("decoding JSON message: %w", err)
		}
		return &msg, nil

	case EncodingFlatBuffer:
		msg := fb.GetRootAsMessage(msgData, 0)

		// Convert FlatBuffer message to Message struct
		m := &Message{
			Version:   string(msg.Version()),
			Type:      MessageType(msg.Type()),
			SenderID:  string(msg.SenderId()),
			Timestamp: time.Unix(0, msg.Timestamp()),
			Nonce:     msg.Nonce(),
			Signature: string(msg.Signature()),
		}

		// Get payload bytes
		payloadLen := msg.PayloadLength()
		payloadBytes := make([]byte, payloadLen)
		for i := 0; i < payloadLen; i++ {
			payloadBytes[i] = msg.Payload(i)
		}

		// Decode payload from JSON
		if err := json.Unmarshal(payloadBytes, &m.Payload); err != nil {
			return nil, fmt.Errorf("decoding payload: %w", err)
		}

		return m, nil

	default:
		return nil, fmt.Errorf("unknown encoding type: %d", encodingType)
	}
}

// NewMessage creates a new message with the given type and payload
func NewMessage(msgType MessageType, senderID string, payload map[string]interface{}) (*Message, error) {
	msg := &Message{
		Version:   "1.0.0",
		Type:      msgType,
		SenderID:  senderID,
		Timestamp: time.Now(),
		Payload:   payload,
	}

	// Generate random nonce
	nonceBytes := make([]byte, 8)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}
	msg.Nonce = binary.BigEndian.Uint64(nonceBytes)

	return msg, nil
}
