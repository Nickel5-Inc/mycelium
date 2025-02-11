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
	"mycelium/internal/identity"
	"mycelium/internal/metagraph"
	"mycelium/internal/protocol"
	"mycelium/internal/substrate"
	"mycelium/internal/util"

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
	outbound map[string]*Peer // hotkey -> peer (changed from nodeID -> peer)

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
	validators, err := NewValidatorRegistry(querier, minStake)
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

// AddPeer adds a peer to the manager
func (pm *PeerManager) AddPeer(p *Peer) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.peers[p] = struct{}{}
	if id := p.Identity().GetID(); id != "" {
		pm.outbound[id] = p
	}
}

// RemovePeer removes a peer from the manager
func (pm *PeerManager) RemovePeer(p *Peer) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	delete(pm.peers, p)
	if id := p.Identity().GetID(); id != "" {
		delete(pm.outbound, id)
	}
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
			go pm.ensureFullMesh(ctx)
		case <-cleanupTicker.C:
			// Cleanup inactive peers
			pm.mu.Lock()
			for peer := range pm.peers {
				if !peer.Identity().IsActive {
					peer.conn.Close()
					delete(pm.peers, peer)
					delete(pm.outbound, peer.Identity().GetID())
				}
			}
			pm.mu.Unlock()
			// Cleanup invalid validators
			go pm.cleanupInvalidPeers()
		}
	}
}

// gossipPeers broadcasts information about known peers
func (pm *PeerManager) gossipPeers() {
	// Create gossip payload
	payload := map[string]interface{}{
		"peer_info": map[string]interface{}{
			"hotkey":  pm.identity.GetID(),
			"ip":      pm.identity.IP,
			"port":    pm.identity.Port,
			"version": pm.identity.Version,
		},
	}

	msg, err := protocol.NewMessage(protocol.MessageTypeGossip, pm.identity.GetID(), payload)
	if err != nil {
		log.Printf("Failed to create gossip message: %v", err)
		return
	}

	encoded, err := msg.Encode()
	if err != nil {
		log.Printf("Failed to encode gossip message: %v", err)
		return
	}

	// Send to random subset of peers
	pm.mu.RLock()
	peers := pm.selectRandomPeerConnections(pm.GetPeers(), 3)
	pm.mu.RUnlock()

	for _, peer := range peers {
		select {
		case peer.send <- encoded:
		default:
			log.Printf("Failed to send gossip to peer %s: buffer full", peer.ID())
		}
	}
}

// syncPeers initiates both peer information and database synchronization.
// It sends sync requests to peers and handles database change propagation.
func (pm *PeerManager) syncPeers() {
	// Create sync request payload
	payload := map[string]interface{}{
		"request_full_sync": true,
	}

	msg, err := protocol.NewMessage(protocol.MessageTypeSync, pm.identity.GetID(), payload)
	if err != nil {
		log.Printf("Failed to create sync message: %v", err)
		return
	}

	encoded, err := msg.Encode()
	if err != nil {
		log.Printf("Failed to encode sync message: %v", err)
		return
	}

	// Send to all peers
	pm.mu.RLock()
	peers := pm.GetPeers()
	pm.mu.RUnlock()

	for _, peer := range peers {
		select {
		case peer.send <- encoded:
		default:
			log.Printf("Failed to send sync to peer %s: buffer full", peer.ID())
		}
	}
}

// handleDBSync processes a database sync message
func (pm *PeerManager) handleDBSync(msg *protocol.Message) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	switch msg.Type {
	case protocol.MessageTypeDBSyncReq:
		var req protocol.DBSyncRequest
		if reqData, ok := msg.Payload["request"].(map[string]interface{}); ok {
			if err := util.ConvertMapToStruct(reqData, &req); err != nil {
				log.Printf("Failed to decode DB sync request: %v", err)
				return
			}
		} else {
			log.Printf("Invalid DB sync request format")
			return
		}

		// Get changes since last sync
		changes, err := pm.syncMgr.GetUnsynced(context.Background(), time.Unix(0, req.LastSync))
		if err != nil {
			log.Printf("Failed to get unsynced changes: %v", err)
			return
		}

		// Convert to protocol format
		dbChanges := make([]protocol.DBChange, len(changes))
		for i, c := range changes {
			// Convert map to JSON bytes for transport
			dataBytes, err := json.Marshal(c.Data)
			if err != nil {
				log.Printf("Failed to marshal change data: %v", err)
				continue
			}

			dbChanges[i] = protocol.DBChange{
				ID:        c.ID,
				TableName: c.TableName,
				Operation: c.Operation,
				RecordID:  c.RecordID,
				Data:      dataBytes,
				Timestamp: c.Timestamp.UnixNano(),
				NodeID:    c.NodeID,
			}
		}

		// Send response
		resp := protocol.DBSyncResponse{Changes: dbChanges}
		payload := map[string]interface{}{"response": resp}

		msg, err := protocol.NewMessage(protocol.MessageTypeDBSyncResp, pm.identity.GetID(), payload)
		if err != nil {
			log.Printf("Failed to create DB sync response: %v", err)
			return
		}

		encoded, err := msg.Encode()
		if err != nil {
			log.Printf("Failed to encode DB sync response: %v", err)
			return
		}

		// Find requesting peer
		for peer := range pm.peers {
			if peer.ID() == msg.SenderID {
				select {
				case peer.send <- encoded:
				default:
					log.Printf("Failed to send DB sync response: buffer full")
				}
				break
			}
		}

	case protocol.MessageTypeDBSyncResp:
		var resp protocol.DBSyncResponse
		if respData, ok := msg.Payload["response"].(map[string]interface{}); ok {
			if err := util.ConvertMapToStruct(respData, &resp); err != nil {
				log.Printf("Failed to decode DB sync response: %v", err)
				return
			}
		} else {
			log.Printf("Invalid DB sync response format")
			return
		}

		// Convert to database format
		changes := make([]database.ChangeRecord, len(resp.Changes))
		for i, c := range resp.Changes {
			// Convert JSON bytes back to map
			var data map[string]interface{}
			if err := json.Unmarshal(c.Data, &data); err != nil {
				log.Printf("Failed to unmarshal change data: %v", err)
				continue
			}

			changes[i] = database.ChangeRecord{
				ID:        c.ID,
				TableName: c.TableName,
				Operation: c.Operation,
				RecordID:  c.RecordID,
				Data:      data,
				Timestamp: time.Unix(0, c.Timestamp),
				NodeID:    c.NodeID,
			}
		}

		// Apply changes
		if err := pm.syncMgr.ApplyChanges(context.Background(), changes); err != nil {
			log.Printf("Failed to apply DB changes: %v", err)
			return
		}
	}
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
			ip, port, version := pm.meta.GetAxonInfo(hotkey)
			stake := float64(pm.meta.GetStake(hotkey))

			// Create wallet from hotkey
			wallet, err := substrate.NewWallet(string(hotkey[:]), "")
			if err != nil {
				log.Printf("Failed to create wallet for %s: %v", string(hotkey[:]), err)
				continue
			}

			id := identity.New(wallet, pm.identity.NetUID)
			id.SetNetworkInfo(ip, port)
			id.Version = version
			id.SetValidatorStatus(true, stake, 0)

			// Skip self
			if id.GetID() == pm.identity.GetID() {
				continue
			}

			// Skip if we already have an outbound connection
			pm.mu.RLock()
			_, hasOutbound := pm.outbound[id.GetID()]
			pm.mu.RUnlock()
			if hasOutbound {
				continue
			}

			// Establish new connection
			if err := pm.connectToPeer(ctx, id); err != nil {
				log.Printf("Failed to connect to peer %s: %v", id.GetID(), err)
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

// connectToPeer establishes a connection to a peer
func (pm *PeerManager) connectToPeer(ctx context.Context, id *identity.Identity) error {
	// Skip if we already have a connection
	if pm.hasConnection(id.GetID()) {
		return nil
	}

	// Get available port
	_, err := pm.getAvailablePort()
	if err != nil {
		return fmt.Errorf("no available ports: %w", err)
	}

	// Create WebSocket connection
	url := fmt.Sprintf("ws://%s:%d/ws", id.IP, id.Port)
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return fmt.Errorf("dialing peer: %w", err)
	}

	// Create peer
	peer := NewPeer(conn, pm, id)
	pm.AddPeer(peer)

	// Start handling connection
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
	for hotkey, peer := range pm.outbound {
		// Close old connection gracefully
		peer.conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseServiceRestart, "Port rotation"))
		peer.conn.Close()
		delete(pm.outbound, hotkey)
		delete(pm.peers, peer)
		oldConnections[hotkey] = peer.Identity()
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
				log.Printf("Failed to reconnect to peer %s: %v", identity.GetID(), err)
			}
		}(id)
	}

	pm.lastPortRotation = time.Now()
	log.Printf("Completed port rotation initiation for %d connections", len(oldConnections))
}

// syncBlacklist synchronizes the blacklist with other peers
func (pm *PeerManager) syncBlacklist() {
	// Get current blacklist state
	blacklist := pm.blacklist.GetSyncUpdate()

	// Create sync message
	payload := map[string]interface{}{
		"blacklist_sync": blacklist,
	}

	msg, err := protocol.NewMessage(protocol.MessageTypeBlacklistSync, pm.identity.GetID(), payload)
	if err != nil {
		log.Printf("Failed to create blacklist sync message: %v", err)
		return
	}

	encoded, err := msg.Encode()
	if err != nil {
		log.Printf("Failed to encode blacklist sync message: %v", err)
		return
	}

	// Send to all peers
	pm.mu.RLock()
	peers := pm.GetPeers()
	pm.mu.RUnlock()

	for _, peer := range peers {
		select {
		case peer.send <- encoded:
		default:
			log.Printf("Failed to send blacklist sync to peer %s: buffer full", peer.ID())
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
	if id.GetID() == pm.identity.GetID() {
		return
	}

	// Check if we need to establish a connection
	if !pm.hasConnection(id.GetID()) {
		if err := pm.connectToPeer(context.Background(), id); err != nil {
			log.Printf("Failed to connect to peer %s: %v", id.GetID(), err)
		}
	}
}

// hasConnection checks if we have an active connection to a peer
func (pm *PeerManager) hasConnection(hotkey string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	for peer := range pm.peers {
		if peer.Identity().GetID() == hotkey {
			return true
		}
	}
	return false
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

			// Create wallet from hotkey
			wallet, err := substrate.NewWallet(string(hotkey[:]), "")
			if err != nil {
				log.Printf("Failed to create wallet for %s: %v", string(hotkey[:]), err)
				continue
			}

			id := identity.New(wallet, pm.identity.NetUID)
			id.SetNetworkInfo(ip, port)
			id.SetValidatorStatus(true, stake, 0)

			if err := pm.connectToPeer(ctx, id); err != nil {
				log.Printf("Error connecting to peer %s: %v", id.GetID(), err)
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
	portRotationTicker := time.NewTicker(time.Hour * 4)
	blacklistSyncTicker := time.NewTicker(time.Minute * 15)
	stakeCheckTicker := time.NewTicker(time.Hour)
	gossipTicker := time.NewTicker(pm.gossipTick)
	peerSyncTicker := time.NewTicker(pm.syncTick)

	for {
		select {
		case <-ctx.Done():
			return
		case <-syncTicker.C:
			// Sync chain state
			if err := pm.meta.SyncFromChain(ctx, pm.querier); err != nil {
				log.Printf("Error syncing chain state: %v", err)
			}
		case <-peerSyncTicker.C:
			// Sync peer information and database state
			go pm.syncPeers()
		case <-meshTicker.C:
			// Ensure connections to all active validators
			go pm.ensureFullMesh(ctx)
		case <-cleanupTicker.C:
			// Cleanup inactive peers
			pm.mu.Lock()
			for peer := range pm.peers {
				if !peer.Identity().IsActive {
					peer.conn.Close()
					delete(pm.peers, peer)
					delete(pm.outbound, peer.Identity().GetID())
				}
			}
			pm.mu.Unlock()
			// Cleanup invalid validators
			go pm.cleanupInvalidPeers()
		case <-portRotationTicker.C:
			// Rotate ports for security
			go pm.rotateActivePorts(ctx)
		case <-blacklistSyncTicker.C:
			// Sync blacklist with peers
			go pm.syncBlacklist()
		case <-stakeCheckTicker.C:
			// Check stakes of blacklisted IPs
			go pm.checkBlacklistedStakes()
		case <-gossipTicker.C:
			// Gossip peer information
			go pm.gossipPeers()
		}
	}
}

// handleHandshakeMsg processes a handshake message from a peer
func (p *Peer) handleHandshakeMsg(msg *protocol.Message) error {
	// Create temporary wallet for handshake
	tempWallet, err := substrate.NewWallet("", "")
	if err != nil {
		return fmt.Errorf("failed to create temporary wallet: %w", err)
	}

	id := identity.New(tempWallet, uint16(p.manager.identity.NetUID))
	id.SetNetworkInfo(p.conn.RemoteAddr().String(), 0)

	// Verify handshake signature
	signature, ok := msg.Payload["signature"].(string)
	if !ok {
		return fmt.Errorf("missing signature in handshake")
	}

	challenge, ok := msg.Payload["challenge"].(string)
	if !ok {
		return fmt.Errorf("missing challenge in handshake")
	}

	// Convert hex signature to bytes
	signatureBytes, err := hex.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("invalid signature format: %w", err)
	}

	valid := tempWallet.Verify([]byte(challenge), signatureBytes)
	if !valid {
		return fmt.Errorf("invalid signature")
	}

	p.identity = id
	return nil
}

// GetPeers returns a list of all connected peers
func (pm *PeerManager) GetPeers() []*Peer {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	peers := make([]*Peer, 0, len(pm.peers))
	for peer := range pm.peers {
		peers = append(peers, peer)
	}
	return peers
}

// handlePeerMsg processes a message from a peer
func (pm *PeerManager) handlePeerMsg(peer *Peer, msg *protocol.Message) {
	switch msg.Type {
	case protocol.MessageTypeHandshake:
		if err := peer.handleHandshakeMsg(msg); err != nil {
			log.Printf("Handshake failed: %v", err)
			peer.conn.Close()
			return
		}

		// Send our handshake response
		resp := protocol.Message{
			Type: protocol.MessageTypeHandshakeResponse,
			Payload: map[string]interface{}{
				"peer_id": pm.identity.GetID(),
				"version": pm.identity.Version,
			},
		}
		if err := peer.conn.WriteJSON(resp); err != nil {
			log.Printf("Failed to send handshake response: %v", err)
			peer.conn.Close()
			return
		}

	case protocol.MessageTypeHandshakeResponse:
		// Update peer version if provided
		if version, ok := msg.Payload["version"].(string); ok {
			peer.Identity().Version = version
		}

	case protocol.MessageTypePing:
		// Send pong response
		resp := protocol.Message{
			Type: protocol.MessageTypePong,
			Payload: map[string]interface{}{
				"peer_id": pm.identity.GetID(),
			},
		}
		if err := peer.conn.WriteJSON(resp); err != nil {
			log.Printf("Failed to send pong: %v", err)
		}

	case protocol.MessageTypePong:
		peer.Identity().UpdateLastSeen()

	case protocol.MessageTypeGossip:
		// Process gossip message
		var gossipData struct {
			HotkeyAddress string `json:"hotkey_address"`
			IP            string `json:"ip"`
			Port          uint16 `json:"port"`
			Version       string `json:"version"`
		}
		if err := util.ConvertMapToStruct(msg.Payload, &gossipData); err != nil {
			log.Printf("Failed to decode gossip message: %v", err)
			return
		}

		// Create wallet from gossip data
		wallet, err := substrate.NewWallet(gossipData.HotkeyAddress, "")
		if err != nil {
			log.Printf("Failed to create wallet from gossip: %v", err)
			return
		}

		id := identity.New(wallet, uint16(pm.meta.NetUID))
		id.SetNetworkInfo(gossipData.IP, gossipData.Port)
		id.Version = gossipData.Version

		// Process peer info
		pm.processPeerInfo(id)

	case protocol.MessageTypeDBSyncReq, protocol.MessageTypeDBSyncResp:
		pm.handleDBSync(msg)
	}
}
