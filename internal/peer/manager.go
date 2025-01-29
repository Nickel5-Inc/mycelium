package peer

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"

	"mycelium/internal/database"
	"mycelium/internal/protocol"

	"github.com/gorilla/websocket"
)

// Package peer provides peer-to-peer networking functionality for the Mycelium network.

// PeerManager handles peer connections, discovery, and synchronization.
// It maintains a list of active peers, manages validator verification,
// and coordinates database synchronization between nodes.
type PeerManager struct {
	nodeID                string
	peers                 map[*Peer]bool
	peerInfo              map[string]protocol.PeerInfo
	outbound              map[string]*Peer
	mu                    sync.RWMutex
	gossipTick            time.Duration
	syncTick              time.Duration
	maxPeers              int
	listenAddr            string
	portRange             [2]uint16
	usedPorts             map[uint16]bool
	db                    database.Database
	syncMgr               *database.SyncManager
	validators            *ValidatorRegistry
	dialer                *websocket.Dialer
	portRotationEnabled   bool
	portRotationInterval  time.Duration
	portRotationJitter    int64
	portRotationBatchSize int
	lastPortRotation      time.Time
	blacklist             *BlacklistManager
}

// NewPeerManager creates a new peer manager instance.
// It initializes the peer tracking structures and starts the sync manager.
// Parameters:
//   - listenAddr: The address to listen for incoming peer connections
//   - portRange: The range of ports to use for outbound connections
//   - db: The database interface for synchronization
//   - verifyURL: The URL endpoint for validator verification
//   - minStake: Minimum stake required for validators
//   - portRotation: Configuration for port rotation
func NewPeerManager(listenAddr string, portRange [2]uint16, db database.Database, verifyURL string, minStake float64, portRotation struct {
	Enabled   bool
	Interval  time.Duration
	JitterMs  int64
	BatchSize int
}) *PeerManager {
	nodeID := generateNodeID()
	validators := NewValidatorRegistry(verifyURL, minStake)

	pm := &PeerManager{
		nodeID:     nodeID,
		peers:      make(map[*Peer]bool),
		peerInfo:   make(map[string]protocol.PeerInfo),
		outbound:   make(map[string]*Peer),
		usedPorts:  make(map[uint16]bool),
		gossipTick: time.Second * 10,
		syncTick:   time.Second * 30,
		maxPeers:   50,
		listenAddr: listenAddr,
		portRange:  portRange,
		db:         db,
		validators: validators,
		dialer: &websocket.Dialer{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		portRotationEnabled:   portRotation.Enabled,
		portRotationInterval:  portRotation.Interval,
		portRotationJitter:    portRotation.JitterMs,
		portRotationBatchSize: portRotation.BatchSize,
		lastPortRotation:      time.Now(),
	}

	pm.blacklist = NewBlacklistManager(validators)

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

// RemovePeer removes a peer from all tracking maps
func (pm *PeerManager) RemovePeer(p *Peer) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.peers, p)
	delete(pm.outbound, p.info.ID)
}

// StartDiscovery begins the peer discovery and synchronization process.
// It runs three periodic tasks:
//   - Peer gossip: Shares known peer information
//   - Database sync: Synchronizes database changes between peers
//   - Cleanup: Removes inactive validators and invalid peers
//   - Port rotation: Rotates active connections
//   - Blacklist sync: Synchronizes blacklist with other peers
func (pm *PeerManager) StartDiscovery(ctx context.Context) {
	ticker := time.NewTicker(pm.gossipTick)
	syncTicker := time.NewTicker(pm.syncTick)
	meshTicker := time.NewTicker(time.Second * 30) // Check mesh connectivity every 30s
	cleanupTicker := time.NewTicker(time.Minute * 10)
	var rotationTicker *time.Ticker
	if pm.portRotationEnabled {
		rotationTicker = time.NewTicker(pm.portRotationInterval)
	}
	blacklistSyncTicker := time.NewTicker(time.Minute * 5) // Sync blacklist every 5 minutes
	blacklistCleanupTicker := time.NewTicker(time.Hour)    // Cleanup expired entries hourly

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pm.gossipPeers()
		case <-syncTicker.C:
			pm.syncPeers()
		case <-meshTicker.C:
			pm.ensureFullMesh(ctx)
		case <-cleanupTicker.C:
			pm.validators.CleanupInactive()
			pm.cleanupInvalidPeers()
		case <-blacklistSyncTicker.C:
			pm.syncBlacklist()
		case <-blacklistCleanupTicker.C:
			pm.blacklist.Cleanup()
			// Check if any blacklisted IPs now have sufficient stake
			pm.checkBlacklistedStakes()
		}
		if pm.portRotationEnabled && rotationTicker != nil {
			select {
			case <-rotationTicker.C:
				pm.rotateActivePorts(ctx)
			default:
			}
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

// handleGossip processes a gossip message from a peer.
// It updates the local peer registry with any new or updated peer information.
func (pm *PeerManager) handleGossip(msg *protocol.Message) error {
	peers, ok := msg.Payload["peers"].([]interface{})
	if !ok {
		return fmt.Errorf("invalid gossip payload")
	}

	for _, peer := range peers {
		if peerInfo, ok := peer.(map[string]interface{}); ok {
			pm.processPeerInfo(peerInfo)
		}
	}
	return nil
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

// ensureFullMesh ensures connections with all known validators
func (pm *PeerManager) ensureFullMesh(ctx context.Context) {
	pm.mu.RLock()
	validators := pm.validators.GetValidators()
	pm.mu.RUnlock()

	for _, validator := range validators {
		// Skip self
		if validator.PeerID == pm.nodeID {
			continue
		}

		// Skip if we already have an outbound connection
		pm.mu.RLock()
		_, hasOutbound := pm.outbound[validator.PeerID]
		pm.mu.RUnlock()
		if hasOutbound {
			continue
		}

		// Establish new connection
		go pm.connectToPeer(ctx, validator)
	}
}

// getAvailablePort returns an available port from the configured range
func (pm *PeerManager) getAvailablePort() (uint16, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Try ports in range
	for port := pm.portRange[0]; port <= pm.portRange[1]; port++ {
		if !pm.usedPorts[port] {
			// Test if port is actually available
			listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
			if err == nil {
				listener.Close()
				pm.usedPorts[port] = true
				return port, nil
			}
		}
	}
	return 0, fmt.Errorf("no available ports in range %d-%d", pm.portRange[0], pm.portRange[1])
}

// releasePort marks a port as available
func (pm *PeerManager) releasePort(port uint16) {
	pm.mu.Lock()
	delete(pm.usedPorts, port)
	pm.mu.Unlock()
}

// connectToPeer establishes an outbound connection to a peer
func (pm *PeerManager) connectToPeer(ctx context.Context, validator ValidatorStatus) {
	// Get an available port for this connection
	localPort, err := pm.getAvailablePort()
	if err != nil {
		log.Printf("Failed to get available port: %v", err)
		return
	}
	defer pm.releasePort(localPort)

	// Set up local address for outbound connection
	localAddr := &net.TCPAddr{
		IP:   net.ParseIP(validator.IP),
		Port: int(localPort),
	}

	// Configure dialer with local address
	dialer := websocket.Dialer{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		NetDial: (&net.Dialer{
			LocalAddr: localAddr,
		}).Dial,
	}

	addr := fmt.Sprintf("ws://%s:%d/ws", validator.IP, validator.Port)
	conn, _, err := dialer.DialContext(ctx, addr, nil)
	if err != nil {
		log.Printf("Failed to connect to peer %s: %v", validator.PeerID, err)
		return
	}

	peer := NewPeer(conn, pm)
	peer.info = protocol.PeerInfo{
		ID:      validator.PeerID,
		Address: addr,
	}

	pm.mu.Lock()
	pm.outbound[validator.PeerID] = peer
	pm.peers[peer] = true
	pm.mu.Unlock()

	go peer.Handle(ctx)
}

// rotateActivePorts changes the ports for all active connections
func (pm *PeerManager) rotateActivePorts(ctx context.Context) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	log.Printf("Starting port rotation for %d active connections", len(pm.outbound))

	// Store old connections that need to be migrated
	oldConnections := make(map[string]ValidatorStatus)
	count := 0
	for peerID, peer := range pm.outbound {
		if count >= pm.portRotationBatchSize {
			break
		}
		if validator, exists := pm.validators.GetValidator(peerID); exists {
			oldConnections[peerID] = validator
			// Close old connection gracefully
			peer.conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseServiceRestart, "Port rotation"))
			peer.conn.Close()
			delete(pm.outbound, peerID)
			delete(pm.peers, peer)
			count++
		}
	}

	// Clear used ports for the rotated connections
	for port := range pm.usedPorts {
		if count <= 0 {
			break
		}
		delete(pm.usedPorts, port)
		count--
	}

	// Establish new connections on different ports
	for peerID, validator := range oldConnections {
		go func(id string, v ValidatorStatus) {
			// Add configurable random jitter
			if pm.portRotationJitter > 0 {
				jitter := time.Duration(rand.Int63n(pm.portRotationJitter)) * time.Millisecond
				time.Sleep(jitter)
			}
			pm.connectToPeer(ctx, v)
		}(peerID, validator)
	}

	pm.lastPortRotation = time.Now()
	log.Printf("Completed port rotation initiation for %d connections", len(oldConnections))
}

// syncBlacklist synchronizes the blacklist with other peers
func (pm *PeerManager) syncBlacklist() {
	update := pm.blacklist.GetSyncUpdate()
	payload := map[string]any{
		"blacklist_sync": update,
	}

	msg := protocol.NewMessage(protocol.MessageTypeBlacklistSync, pm.nodeID, payload)
	encoded, err := msg.Encode()
	if err != nil {
		log.Printf("Failed to encode blacklist sync message: %v", err)
		return
	}

	// Broadcast to all peers
	pm.mu.RLock()
	peers := make([]*Peer, 0, len(pm.peers))
	for peer := range pm.peers {
		peers = append(peers, peer)
	}
	pm.mu.RUnlock()

	for _, peer := range peers {
		select {
		case peer.send <- encoded:
		default:
			log.Printf("Failed to send blacklist sync to peer: buffer full")
		}
	}
}

// checkBlacklistedStakes checks if any blacklisted IPs now have sufficient stake
func (pm *PeerManager) checkBlacklistedStakes() {
	pm.mu.RLock()
	validators := pm.validators.GetValidators()
	pm.mu.RUnlock()

	// Group validators by IP and calculate total stake
	stakeByIP := make(map[string]float64)
	for _, v := range validators {
		stakeByIP[v.IP] += v.Stake
	}

	// Check each IP
	for ip, stake := range stakeByIP {
		if stake >= pm.blacklist.minStakeToUnblock {
			if pm.blacklist.CheckStakeAndUnblock(ip) {
				log.Printf("Unblocked IP %s due to sufficient stake: %f", ip, stake)
			}
		}
	}
}

// processPeerInfo processes peer information received from gossip
func (pm *PeerManager) processPeerInfo(peerInfo map[string]any) {
	// Convert the map to PeerInfo struct
	var info protocol.PeerInfo
	if err := mapToStruct(peerInfo, &info); err != nil {
		log.Printf("Error converting peer info: %v", err)
		return
	}

	// Skip self
	if info.ID == pm.nodeID {
		return
	}

	// Update peer info
	pm.mu.Lock()
	pm.peerInfo[info.ID] = info
	pm.mu.Unlock()

	// Check if we need to establish a connection
	if !pm.hasConnection(info.ID) {
		if validator, exists := pm.validators.GetValidator(info.ID); exists {
			go pm.connectToPeer(context.Background(), validator)
		}
	}
}

// hasConnection checks if we have an active connection to a peer
func (pm *PeerManager) hasConnection(peerID string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	_, hasOutbound := pm.outbound[peerID]
	return hasOutbound
}

// IsIPBlocked checks if an IP is blocked by the blacklist
func (pm *PeerManager) IsIPBlocked(ip string) bool {
	return pm.blacklist.IsBlocked(ip)
}
