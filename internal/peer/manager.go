package peer

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"

	"mycelium/internal/database"
	"mycelium/internal/identity"
	"mycelium/internal/metagraph"
	"mycelium/internal/protocol"
	"mycelium/internal/util"

	"github.com/centrifuge/go-substrate-rpc-client/v4/signature"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"github.com/gorilla/websocket"
)

// Package peer provides peer-to-peer networking functionality for the Mycelium network.

// PeerManager handles peer-to-peer networking functionality for the Mycelium network.
// It maintains a list of active peers, manages validator verification,
// and coordinates database synchronization between nodes.
type PeerManager struct {
	mu sync.RWMutex

	// Core identity and components
	identity  *identity.Identity
	meta      *metagraph.Metagraph
	querier   metagraph.ChainQuerier
	syncMgr   *database.SyncManager
	blacklist *BlacklistManager

	// Peer tracking
	peers    map[*Peer]struct{}
	outbound map[string]*Peer // peerID -> peer

	// Port management
	portRange          [2]uint16
	usedPorts          map[uint16]bool
	portRotationJitter int64
	lastPortRotation   time.Time

	// Connection settings
	gossipTick time.Duration
	syncTick   time.Duration
}

// NewPeerManager creates a new peer manager instance.
// It initializes the peer tracking structures and starts the sync manager.
// Parameters:
//   - identity: The identity of the node
//   - meta: The metagraph for validator verification
//   - querier: The chain querier for validator verification
//   - syncMgr: The sync manager for database synchronization
//   - verifyURL: The URL for validator verification
//   - minStake: The minimum stake required for validator verification
//   - portRange: The range of ports to use for peer connections
func NewPeerManager(
	identity *identity.Identity,
	meta *metagraph.Metagraph,
	querier metagraph.ChainQuerier,
	syncMgr *database.SyncManager,
	verifyURL string,
	minStake float64,
	portRange [2]uint16,
) (*PeerManager, error) {
	validators, err := NewValidatorRegistry(verifyURL, minStake)
	if err != nil {
		return nil, fmt.Errorf("failed to create validator registry: %w", err)
	}

	blacklist := NewBlacklistManager(validators)

	return &PeerManager{
		identity:   identity,
		meta:       meta,
		querier:    querier,
		syncMgr:    syncMgr,
		blacklist:  blacklist,
		peers:      make(map[*Peer]struct{}),
		outbound:   make(map[string]*Peer),
		portRange:  portRange,
		usedPorts:  make(map[uint16]bool),
		gossipTick: time.Second * 10,
		syncTick:   time.Second * 30,
	}, nil
}

// AddPeer adds a new peer to the manager if they are a valid validator.
// Invalid peers are rejected and logged.
func (pm *PeerManager) AddPeer(p *Peer) {
	if !p.Identity().IsValidator() {
		log.Printf("Rejecting non-validator peer: %s", p.ID())
		return
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.peers[p] = struct{}{}
}

// RemovePeer removes a peer from all tracking maps
func (pm *PeerManager) RemovePeer(p *Peer) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.peers, p)
	delete(pm.outbound, p.ID())
}

// StartDiscovery begins the peer discovery and synchronization process.
func (pm *PeerManager) StartDiscovery(ctx context.Context) {
	syncTicker := time.NewTicker(time.Minute)
	meshTicker := time.NewTicker(time.Minute * 5)
	cleanupTicker := time.NewTicker(time.Hour)

	for {
		select {
		case <-ctx.Done():
			return
		case <-syncTicker.C:
			// Sync chain state
			if err := pm.meta.SyncFromChain(ctx, pm.querier); err != nil {
				log.Printf("Error syncing chain state: %v", err)
			}
		case <-meshTicker.C:
			// Ensure connections to all active validators
			validators := pm.meta.GetValidators()
			for _, hotkey := range validators {
				if pm.meta.IsActive(hotkey) {
					// Create identity from validator info
					ip, port, _ := pm.meta.GetAxonInfo(hotkey)
					stake := float64(pm.meta.GetStake(hotkey))
					id := identity.New(string(hotkey[:]), nil, pm.identity.NetUID)
					id.SetNetworkInfo(ip, port)
					id.SetValidatorStatus(true, stake, 0)

					if err := pm.connectToPeer(ctx, id); err != nil {
						log.Printf("Error connecting to peer %s: %v", id.ID, err)
					}
				}
			}
		case <-cleanupTicker.C:
			// Cleanup inactive peers
			pm.mu.Lock()
			for peer := range pm.peers {
				if !peer.Identity().IsActive {
					peer.conn.Close()
					delete(pm.peers, peer)
					delete(pm.outbound, peer.Identity().ID)
				}
			}
			pm.mu.Unlock()
		}
	}
}

// gossipPeers broadcasts information about known peers
func (pm *PeerManager) gossipPeers() {
	pm.mu.RLock()
	peers := make([]*Peer, 0, len(pm.peers))
	for peer := range pm.peers {
		peers = append(peers, peer)
	}
	pm.mu.RUnlock()

	// Convert identities to protocol format
	peerInfoList := make([]protocol.PeerInfo, 0, len(peers))
	for _, peer := range peers {
		id := peer.Identity()
		if id.IsValidator() {
			peerInfoList = append(peerInfoList, protocol.PeerInfo{
				ID:       id.ID,
				Address:  fmt.Sprintf("%s:%d", id.IP, id.Port),
				LastSeen: id.LastSeen,
				Version:  id.Version,
				Metadata: id.Metadata,
			})
		}
	}

	// Select random subset of peers to gossip about
	subset := pm.selectRandomPeerInfos(peerInfoList, 10)

	payload := map[string]any{
		"peers": subset,
	}

	msg := protocol.NewMessage(protocol.MessageTypeGossip, pm.identity.ID, payload)
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
	msg := protocol.NewMessage(protocol.MessageTypeSync, pm.identity.ID, payload)
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
			LastSync: time.Now().Add(-time.Hour), // Get last hour of changes
			Tables:   []string{},                 // Empty means all tables
		}
		payload := map[string]any{"request": req}
		msg := protocol.NewMessage(protocol.MessageTypeDBSyncReq, pm.identity.ID, payload)
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

// handleGossip processes a gossip message from a peer
func (pm *PeerManager) handleGossip(msg *protocol.Message) {
	// Extract peer info from gossip message
	peerInfoMap, ok := msg.Payload["peer_info"].(map[string]any)
	if !ok {
		log.Printf("Invalid gossip message format")
		return
	}

	// Convert map to identity
	var id identity.Identity
	if err := util.ConvertMapToStruct(peerInfoMap, &id); err != nil {
		log.Printf("Failed to parse peer info: %v", err)
		return
	}

	// Process the peer info
	pm.processPeerInfo(&id)
}

// handleDBSync processes a database sync message
func (pm *PeerManager) handleDBSync(msg *protocol.Message) {
	if pm.syncMgr == nil {
		return
	}

	switch msg.Type {
	case protocol.MessageTypeDBSyncReq:
		var req protocol.DBSyncRequest
		if reqData, ok := msg.Payload["request"].(map[string]any); ok {
			if err := util.ConvertMapToStruct(reqData, &req); err != nil {
				log.Printf("Failed to decode DB sync request: %v", err)
				return
			}
		} else {
			log.Printf("Invalid DB sync request payload")
			return
		}

		// Get changes since last sync
		changes, err := pm.syncMgr.GetUnsynced(context.Background(), req.LastSync)
		if err != nil {
			log.Printf("Failed to get DB changes: %v", err)
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

		// Send response
		resp := protocol.DBSyncResponse{Changes: dbChanges}
		payload := map[string]any{"response": resp}
		msg := protocol.NewMessage(protocol.MessageTypeDBSyncResp, pm.identity.ID, payload)
		encoded, err := msg.Encode()
		if err != nil {
			log.Printf("Failed to encode DB sync response: %v", err)
			return
		}

		// Find the requesting peer
		pm.mu.RLock()
		for peer := range pm.peers {
			if peer.Identity().ID == msg.SenderID {
				select {
				case peer.send <- encoded:
				default:
					log.Printf("Failed to send DB sync response: buffer full")
				}
				break
			}
		}
		pm.mu.RUnlock()

	case protocol.MessageTypeDBSyncResp:
		var resp protocol.DBSyncResponse
		if respData, ok := msg.Payload["response"].(map[string]any); ok {
			if err := util.ConvertMapToStruct(respData, &resp); err != nil {
				log.Printf("Failed to decode DB sync response: %v", err)
				return
			}
		} else {
			log.Printf("Invalid DB sync response payload")
			return
		}

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

		// Apply changes
		if err := pm.syncMgr.ApplyChanges(context.Background(), changes); err != nil {
			log.Printf("Failed to apply DB changes: %v", err)
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

// cleanupInvalidPeers removes any peers that are no longer valid validators
func (pm *PeerManager) cleanupInvalidPeers() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for peer := range pm.peers {
		if !peer.Identity().IsValidator() {
			delete(pm.peers, peer)
			peer.conn.Close()
		}
	}
}

// ensureFullMesh ensures connections with all known validators
func (pm *PeerManager) ensureFullMesh(ctx context.Context) {
	validators := pm.meta.GetValidators()
	for _, hotkey := range validators {
		if pm.meta.IsActive(hotkey) {
			// Create identity from validator info
			ip, port, _ := pm.meta.GetAxonInfo(hotkey)
			stake := float64(pm.meta.GetStake(hotkey))
			id := identity.New(string(hotkey[:]), nil, pm.identity.NetUID)
			id.SetNetworkInfo(ip, port)
			id.SetValidatorStatus(true, stake, 0)

			// Skip self
			if id.ID == pm.identity.ID {
				continue
			}

			// Skip if we already have an outbound connection
			pm.mu.RLock()
			_, hasOutbound := pm.outbound[id.ID]
			pm.mu.RUnlock()
			if hasOutbound {
				continue
			}

			// Establish new connection
			if err := pm.connectToPeer(ctx, id); err != nil {
				log.Printf("Failed to connect to peer %s: %v", id.ID, err)
			}
		}
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

// connectToPeer establishes a connection to a peer
func (pm *PeerManager) connectToPeer(ctx context.Context, id *identity.Identity) error {
	if id.IP == "" || id.Port == 0 {
		return fmt.Errorf("peer %s has no valid network info", id.ID)
	}

	// Check blacklist
	if pm.blacklist.IsBlocked(id.IP) {
		return fmt.Errorf("peer IP %s is blacklisted", id.IP)
	}

	// Get an available port
	localPort, err := pm.getAvailablePort()
	if err != nil {
		return fmt.Errorf("failed to get available port: %w", err)
	}
	defer pm.releasePort(localPort)

	// Set up local address
	localAddr := &net.TCPAddr{
		IP:   net.ParseIP("0.0.0.0"),
		Port: int(localPort),
	}

	// Configure dialer
	dialer := websocket.Dialer{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		NetDial: (&net.Dialer{
			LocalAddr: localAddr,
		}).Dial,
	}

	// Connect
	addr := fmt.Sprintf("ws://%s:%d", id.IP, id.Port)
	conn, _, err := dialer.DialContext(ctx, addr, nil)
	if err != nil {
		return fmt.Errorf("dialing peer: %w", err)
	}

	peer := NewPeer(conn, pm, id)
	pm.mu.Lock()
	pm.peers[peer] = struct{}{}
	pm.outbound[id.ID] = peer
	pm.mu.Unlock()

	go peer.Handle(ctx)

	return nil
}

// rotateActivePorts changes the ports for all active connections
func (pm *PeerManager) rotateActivePorts(ctx context.Context) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	log.Printf("Starting port rotation for %d active connections", len(pm.outbound))

	// Store old connections that need to be migrated
	oldConnections := make(map[string]*identity.Identity)
	for peerID, peer := range pm.outbound {
		// Close old connection gracefully
		peer.conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseServiceRestart, "Port rotation"))
		peer.conn.Close()
		delete(pm.outbound, peerID)
		delete(pm.peers, peer)
		oldConnections[peerID] = peer.Identity()
	}

	// Clear used ports
	pm.usedPorts = make(map[uint16]bool)

	// Establish new connections on different ports
	for _, id := range oldConnections {
		go func(identity *identity.Identity) {
			// Add configurable random jitter
			if pm.portRotationJitter > 0 {
				jitter := time.Duration(rand.Int63n(pm.portRotationJitter)) * time.Millisecond
				time.Sleep(jitter)
			}
			if err := pm.connectToPeer(ctx, identity); err != nil {
				log.Printf("Failed to reconnect to peer %s: %v", identity.ID, err)
			}
		}(id)
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

	msg := protocol.NewMessage(protocol.MessageTypeBlacklistSync, pm.identity.ID, payload)
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
	// Get active validators from metagraph
	validators := pm.meta.GetValidators()

	// Group validators by IP and calculate total stake
	stakeByIP := make(map[string]float64)
	for _, hotkey := range validators {
		if pm.meta.IsActive(hotkey) {
			ip, _, _ := pm.meta.GetAxonInfo(hotkey)
			stake := float64(pm.meta.GetStake(hotkey))
			stakeByIP[ip] += stake
		}
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
func (pm *PeerManager) processPeerInfo(id *identity.Identity) {
	// Skip self
	if id.ID == pm.identity.ID {
		return
	}

	// Check if we need to establish a connection
	if !pm.hasConnection(id.ID) {
		if err := pm.connectToPeer(context.Background(), id); err != nil {
			log.Printf("Failed to connect to peer %s: %v", id.ID, err)
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

// Start starts the peer manager
func (pm *PeerManager) Start(ctx context.Context) error {
	// Start periodic tasks
	go pm.periodicTasks(ctx)

	// Initial sync with chain
	if err := pm.meta.SyncFromChain(ctx, pm.querier); err != nil {
		return fmt.Errorf("initial chain sync: %w", err)
	}

	// Connect to initial peers from metagraph
	validators := pm.meta.GetValidators()
	for _, hotkey := range validators {
		if pm.meta.IsActive(hotkey) {
			// Create identity from validator info
			ip, port, _ := pm.meta.GetAxonInfo(hotkey)
			stake := float64(pm.meta.GetStake(hotkey))
			id := identity.New(string(hotkey[:]), nil, pm.identity.NetUID)
			id.SetNetworkInfo(ip, port)
			id.SetValidatorStatus(true, stake, 0)

			if err := pm.connectToPeer(ctx, id); err != nil {
				log.Printf("Error connecting to peer %s: %v", id.ID, err)
			}
		}
	}

	return nil
}

// periodicTasks runs periodic maintenance tasks
func (pm *PeerManager) periodicTasks(ctx context.Context) {
	syncTicker := time.NewTicker(time.Minute)
	meshTicker := time.NewTicker(time.Minute * 5)
	cleanupTicker := time.NewTicker(time.Hour)
	portRotationTicker := time.NewTicker(time.Hour * 4)     // Rotate ports every 4 hours
	blacklistSyncTicker := time.NewTicker(time.Minute * 15) // Sync blacklist every 15 minutes
	stakeCheckTicker := time.NewTicker(time.Hour)           // Check stakes every hour

	for {
		select {
		case <-ctx.Done():
			return
		case <-syncTicker.C:
			// Sync chain state
			if err := pm.meta.SyncFromChain(ctx, pm.querier); err != nil {
				log.Printf("Error syncing chain state: %v", err)
			}
		case <-meshTicker.C:
			// Ensure connections to all active validators
			validators := pm.meta.GetValidators()
			for _, hotkey := range validators {
				if pm.meta.IsActive(hotkey) {
					// Create identity from validator info
					ip, port, _ := pm.meta.GetAxonInfo(hotkey)
					stake := float64(pm.meta.GetStake(hotkey))
					id := identity.New(string(hotkey[:]), nil, pm.identity.NetUID)
					id.SetNetworkInfo(ip, port)
					id.SetValidatorStatus(true, stake, 0)

					if !pm.hasConnection(id.ID) {
						if err := pm.connectToPeer(ctx, id); err != nil {
							log.Printf("Error connecting to peer %s: %v", id.ID, err)
						}
					}
				}
			}
		case <-cleanupTicker.C:
			// Cleanup inactive peers
			pm.mu.Lock()
			for peer := range pm.peers {
				if !peer.Identity().IsActive {
					peer.conn.Close()
					delete(pm.peers, peer)
					delete(pm.outbound, peer.Identity().ID)
				}
			}
			pm.mu.Unlock()
		case <-portRotationTicker.C:
			// Rotate ports for security
			go pm.rotateActivePorts(ctx)
		case <-blacklistSyncTicker.C:
			// Sync blacklist with peers
			go pm.syncBlacklist()
		case <-stakeCheckTicker.C:
			// Check stakes of blacklisted IPs
			go pm.checkBlacklistedStakes()
		}
	}
}

// handleHandshakeMsg processes a handshake message
func (p *Peer) handleHandshakeMsg(msg *protocol.Message) error {
	peerID, ok := msg.Payload["peer_id"].(string)
	if !ok {
		return fmt.Errorf("invalid handshake: missing peer_id")
	}

	signatureBytes, ok := msg.Payload["signature"].([]byte)
	if !ok {
		return fmt.Errorf("invalid handshake: missing signature")
	}

	messageBytes, ok := msg.Payload["message"].([]byte)
	if !ok {
		return fmt.Errorf("invalid handshake: missing message")
	}

	// Create identity for verification
	id := identity.New(peerID, nil, p.manager.identity.NetUID)

	// Verify the peer using metagraph
	var hotkey types.AccountID
	copy(hotkey[:], []byte(peerID))
	if !p.manager.meta.IsActive(hotkey) {
		return fmt.Errorf("peer is not active")
	}

	// Verify signature using substrate
	pubKeyHex := hex.EncodeToString(hotkey[:])
	ok, err := signature.Verify(messageBytes, signatureBytes, pubKeyHex)
	if err != nil || !ok {
		return fmt.Errorf("invalid signature: %w", err)
	}

	// Get stake from metagraph
	stake := float64(p.manager.meta.GetStake(hotkey))
	if stake == 0 {
		return fmt.Errorf("peer has no stake")
	}

	// Get network info from metagraph
	ip, port, _ := p.manager.meta.GetAxonInfo(hotkey)
	id.SetNetworkInfo(ip, port)
	id.SetValidatorStatus(true, stake, 0)

	// Update peer identity
	p.mu.Lock()
	p.identity = id
	p.identity.UpdateLastSeen()
	p.mu.Unlock()

	return nil
}
