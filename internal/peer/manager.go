package peer

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"log"
	"math/rand"
	"sync"
	"time"

	"mycelium/internal/database"
	"mycelium/internal/protocol"
)

// Package peer provides peer-to-peer networking functionality for the Mycelium network.

// PeerManager handles peer connections, discovery, and synchronization.
// It maintains a list of active peers, manages validator verification,
// and coordinates database synchronization between nodes.
type PeerManager struct {
	nodeID     string
	peers      map[*Peer]bool
	peerInfo   map[string]protocol.PeerInfo
	mu         sync.RWMutex
	gossipTick time.Duration
	syncTick   time.Duration
	maxPeers   int
	listenAddr string
	db         database.Database
	syncMgr    *database.SyncManager
	validators *ValidatorRegistry
}

// NewPeerManager creates a new peer manager instance.
// It initializes the peer tracking structures and starts the sync manager.
// Parameters:
//   - listenAddr: The address to listen for incoming peer connections
//   - db: The database interface for synchronization
//   - verifyURL: The URL endpoint for validator verification
func NewPeerManager(listenAddr string, db database.Database, verifyURL string) *PeerManager {
	nodeID := generateNodeID()
	pm := &PeerManager{
		nodeID:     nodeID,
		peers:      make(map[*Peer]bool),
		peerInfo:   make(map[string]protocol.PeerInfo),
		gossipTick: time.Second * 10,
		syncTick:   time.Second * 30,
		maxPeers:   50,
		listenAddr: listenAddr,
		db:         db,
		validators: NewValidatorRegistry(verifyURL),
	}

	// Initialize sync manager
	syncMgr, err := database.NewSyncManager(db, nodeID)
	if err != nil {
		log.Printf("Failed to initialize sync manager: %v", err)
	} else {
		pm.syncMgr = syncMgr
	}

	return pm
}

// AddPeer adds a new peer to the manager if they are a valid validator.
// Invalid peers are rejected and logged.
func (pm *PeerManager) AddPeer(p *Peer) {
	if !pm.validators.IsValidator(p.info.ID) {
		log.Printf("Rejecting non-validator peer: %s", p.info.ID)
		return
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.peers[p] = true
}

// RemovePeer removes a peer from the manager.
func (pm *PeerManager) RemovePeer(p *Peer) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.peers, p)
}

// StartDiscovery begins the peer discovery and synchronization process.
// It runs three periodic tasks:
//   - Peer gossip: Shares known peer information
//   - Database sync: Synchronizes database changes between peers
//   - Cleanup: Removes inactive validators and invalid peers
func (pm *PeerManager) StartDiscovery(ctx context.Context) {
	ticker := time.NewTicker(pm.gossipTick)
	syncTicker := time.NewTicker(pm.syncTick)
	cleanupTicker := time.NewTicker(time.Minute * 10)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pm.gossipPeers()
		case <-syncTicker.C:
			pm.syncPeers()
		case <-cleanupTicker.C:
			pm.validators.CleanupInactive()
			pm.cleanupInvalidPeers()
		}
	}
}

// gossipPeers broadcasts information about known peers to a random subset of connected peers.
// It only shares information about validated peers to prevent propagation of invalid peers.
func (pm *PeerManager) gossipPeers() {
	pm.mu.RLock()
	peers := make([]*Peer, 0, len(pm.peers))
	for peer := range pm.peers {
		peers = append(peers, peer)
	}

	peerInfoList := make([]protocol.PeerInfo, 0, len(pm.peerInfo))
	for _, info := range pm.peerInfo {
		if pm.validators.IsValidator(info.ID) {
			peerInfoList = append(peerInfoList, info)
		}
	}
	pm.mu.RUnlock()

	// Select random subset of peers to gossip about
	subset := pm.selectRandomPeerInfos(peerInfoList, 10)

	payload := map[string]any{
		"peers": subset,
	}

	msg := protocol.NewMessage(protocol.MessageTypeGossip, pm.nodeID, payload)
	encoded, err := msg.Encode()
	if err != nil {
		log.Printf("Failed to encode gossip message: %v", err)
		return
	}

	// Send to random subset of connected peers
	targets := pm.selectRandomPeerConnections(peers, 3)
	for _, peer := range targets {
		select {
		case peer.send <- encoded:
		default:
			log.Printf("Failed to send gossip message to peer: buffer full")
		}
	}
}

// syncPeers initiates both peer information and database synchronization.
// It sends sync requests to peers and handles database change propagation.
func (pm *PeerManager) syncPeers() {
	pm.mu.RLock()
	peers := make([]*Peer, 0, len(pm.peers))
	for peer := range pm.peers {
		peers = append(peers, peer)
	}
	pm.mu.RUnlock()

	// Regular peer sync
	payload := map[string]any{
		"request_full_sync": true,
	}
	msg := protocol.NewMessage(protocol.MessageTypeSync, pm.nodeID, payload)
	encoded, err := msg.Encode()
	if err != nil {
		log.Printf("Failed to encode sync message: %v", err)
		return
	}

	// Send sync request to all peers
	for _, peer := range peers {
		select {
		case peer.send <- encoded:
		default:
			log.Printf("Failed to send sync message to peer: buffer full")
		}
	}

	// Database sync
	if pm.syncMgr != nil {
		req := protocol.DBSyncRequest{
			LastSync: time.Now().Add(-time.Hour), // Sync last hour's changes
			Tables:   []string{},                 // Empty means all tables
		}
		payload := map[string]any{"request": req}
		msg := protocol.NewMessage(protocol.MessageTypeDBSyncReq, pm.nodeID, payload)
		encoded, err := msg.Encode()
		if err != nil {
			log.Printf("Failed to encode DB sync message: %v", err)
			return
		}

		// Send to random subset of peers
		targets := pm.selectRandomPeerConnections(peers, 3)
		for _, peer := range targets {
			select {
			case peer.send <- encoded:
			default:
				log.Printf("Failed to send DB sync message to peer: buffer full")
			}
		}
	}
}

// handleGossip processes incoming gossip messages containing peer information.
// It updates the local peer registry with any new or updated peer information.
func (pm *PeerManager) handleGossip(msg *protocol.Message) {
	var peers []protocol.PeerInfo
	if peersRaw, ok := msg.Payload["peers"].([]interface{}); ok {
		for _, peerRaw := range peersRaw {
			if peerMap, ok := peerRaw.(map[string]interface{}); ok {
				peer := protocol.PeerInfo{
					ID:       peerMap["id"].(string),
					Address:  peerMap["address"].(string),
					LastSeen: peerMap["last_seen"].(time.Time),
					Version:  peerMap["version"].(string),
					Metadata: peerMap["metadata"].(map[string]string),
				}
				peers = append(peers, peer)
			}
		}
	}

	pm.mu.Lock()
	for _, peer := range peers {
		if existing, ok := pm.peerInfo[peer.ID]; !ok || existing.LastSeen.Before(peer.LastSeen) {
			pm.peerInfo[peer.ID] = peer
		}
	}
	pm.mu.Unlock()
}

// handleDBSync processes database synchronization messages.
// It handles both sync requests and responses, applying changes as needed.
func (pm *PeerManager) handleDBSync(msg *protocol.Message) {
	if pm.syncMgr == nil {
		log.Printf("Sync manager not initialized")
		return
	}

	switch msg.Type {
	case protocol.MessageTypeDBSyncReq:
		var req protocol.DBSyncRequest
		if data, err := json.Marshal(msg.Payload); err == nil {
			if err := json.Unmarshal(data, &req); err == nil {
				changes, err := pm.syncMgr.GetUnsynced(context.Background(), req.LastSync)
				if err != nil {
					log.Printf("Failed to get unsynced changes: %v", err)
					return
				}

				// Convert to protocol format
				dbChanges := make([]protocol.DBChange, len(changes))
				for i, c := range changes {
					dbChanges[i] = protocol.DBChange{
						ID:        c.ID,
						TableName: c.TableName,
						Operation: c.Operation,
						RecordID:  c.RecordID,
						Data:      c.Data,
						Timestamp: c.Timestamp,
						NodeID:    c.NodeID,
					}
				}

				resp := protocol.DBSyncResponse{Changes: dbChanges}
				payload := map[string]any{"response": resp}
				respMsg := protocol.NewMessage(protocol.MessageTypeDBSyncResp, pm.nodeID, payload)

				// Send to requesting peer
				for peer := range pm.peers {
					if peer.info.ID == msg.SenderID {
						if data, err := respMsg.Encode(); err == nil {
							select {
							case peer.send <- data:
							default:
								log.Printf("Failed to send DB sync response: buffer full")
							}
						}
						break
					}
				}
			}
		}

	case protocol.MessageTypeDBSyncResp:
		var resp protocol.DBSyncResponse
		if data, err := json.Marshal(msg.Payload); err == nil {
			if err := json.Unmarshal(data, &resp); err == nil {
				// Convert to database format
				changes := make([]database.ChangeRecord, len(resp.Changes))
				for i, c := range resp.Changes {
					changes[i] = database.ChangeRecord{
						ID:        c.ID,
						TableName: c.TableName,
						Operation: c.Operation,
						RecordID:  c.RecordID,
						Data:      c.Data,
						Timestamp: c.Timestamp,
						NodeID:    c.NodeID,
					}
				}

				if err := pm.syncMgr.ApplyChanges(context.Background(), changes); err != nil {
					log.Printf("Failed to apply changes: %v", err)
					return
				}

				// Mark changes as synced
				ids := make([]int64, len(changes))
				for i, c := range changes {
					ids[i] = c.ID
				}
				if err := pm.syncMgr.MarkSynced(context.Background(), ids); err != nil {
					log.Printf("Failed to mark changes as synced: %v", err)
				}
			}
		}
	}
}

// generateNodeID creates a random 16-byte node identifier encoded as a hex string.
func generateNodeID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// selectRandomPeerInfos randomly selects n peers from the provided list.
// If the input list has fewer than n peers, returns the entire list.
func (pm *PeerManager) selectRandomPeerInfos(peers []protocol.PeerInfo, n int) []protocol.PeerInfo {
	if len(peers) <= n {
		return peers
	}
	result := make([]protocol.PeerInfo, len(peers))
	copy(result, peers)
	for i := len(result) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		result[i], result[j] = result[j], result[i]
	}
	return result[:n]
}

// selectRandomPeerConnections randomly selects n connected peers.
// If there are fewer than n connected peers, returns all connected peers.
func (pm *PeerManager) selectRandomPeerConnections(peers []*Peer, n int) []*Peer {
	if len(peers) <= n {
		return peers
	}
	result := make([]*Peer, len(peers))
	copy(result, peers)
	for i := len(result) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		result[i], result[j] = result[j], result[i]
	}
	return result[:n]
}

// cleanupInvalidPeers removes any peers that are no longer valid validators.
// This includes closing their connections and removing them from the peer list.
func (pm *PeerManager) cleanupInvalidPeers() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for peer := range pm.peers {
		if !pm.validators.IsValidator(peer.info.ID) {
			delete(pm.peers, peer)
			peer.conn.Close()
		}
	}
}
