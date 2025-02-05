package protocol

import (
	"time"
)

// CreateHandshakeMessage creates a new handshake message
func CreateHandshakeMessage(senderID string, version string) (*Message, error) {
	payload := map[string]interface{}{
		"version": version,
	}
	return NewMessage(MessageTypeHandshake, senderID, payload)
}

// CreatePingMessage creates a new ping message
func CreatePingMessage(senderID string) (*Message, error) {
	payload := map[string]interface{}{
		"timestamp": time.Now().UnixNano(),
	}
	return NewMessage(MessageTypePing, senderID, payload)
}

// CreatePongMessage creates a new pong message
func CreatePongMessage(senderID string, pingTimestamp int64) (*Message, error) {
	payload := map[string]interface{}{
		"ping_timestamp": pingTimestamp,
		"timestamp":      time.Now().UnixNano(),
	}
	return NewMessage(MessageTypePong, senderID, payload)
}

// CreateSyncMessage creates a new sync message
func CreateSyncMessage(senderID string, lastSync int64) (*Message, error) {
	payload := map[string]interface{}{
		"last_sync": lastSync,
	}
	return NewMessage(MessageTypeSync, senderID, payload)
}
