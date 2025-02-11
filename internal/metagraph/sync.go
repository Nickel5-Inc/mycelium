package metagraph

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/btcsuite/btcutil/base58"
	"github.com/centrifuge/go-substrate-rpc-client/v4/scale"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"golang.org/x/crypto/blake2b"
)

// ChainQuerier defines the interface for querying chain state
type ChainQuerier interface {
	// QueryNeuronCount returns the total number of neurons in a subnet
	QueryNeuronCount(ctx context.Context, netuid types.U16) (types.U16, error)

	// QueryValidatorSet returns the list of validator hotkeys in a subnet
	QueryValidatorSet(ctx context.Context, netuid types.U16) ([]types.AccountID, error)

	// QueryStake returns the stake amount for a validator
	QueryStake(ctx context.Context, hotkey types.AccountID) (types.U64, error)

	// QueryWeights returns the weights set by a validator
	QueryWeights(ctx context.Context, netuid types.U16, source types.AccountID) (map[types.AccountID]types.U16, error)

	// QueryAxonInfo returns the IP, port and version for a validator
	QueryAxonInfo(ctx context.Context, netuid types.U16, hotkey types.AccountID) (string, uint16, string, error)

	// QueryBlock returns the current block number
	QueryBlock(ctx context.Context) (types.U64, error)

	// GetNeuronsLite returns all neuron info for a subnet in a single state call
	GetNeuronsLite(ctx context.Context, netuid types.U16) ([]byte, error)
}

// normalizeU16Float normalizes a U16 value to a float between 0 and 1
func normalizeU16Float(x types.U16) float64 {
	return float64(x) / float64(65535) // U16_MAX
}

// raoToTao converts RAO (10^-9 TAO) to TAO
func raoToTao(rao types.U64) float64 {
	return float64(rao) / 1e9
}

// AxonInfo represents the axon endpoint information
type AxonInfo struct {
	Block        types.U64
	Version      types.U32
	IP           types.U128
	Port         types.U16
	IPType       types.U8
	Protocol     types.U8
	Placeholder1 types.U8
	Placeholder2 types.U8
}

// PrometheusInfo represents the prometheus endpoint information
type PrometheusInfo struct {
	Block   types.U64
	Version types.U32
	IP      types.U128
	Port    types.U16
	IPType  types.U8
}

// NeuronInfo represents the decoded neuron information from the state call
type NeuronInfo struct {
	Hotkey          types.AccountID
	Coldkey         types.AccountID
	UID             types.U16
	NetUID          types.U16
	Active          bool
	AxonInfo        AxonInfo
	PrometheusInfo  PrometheusInfo
	Stake           map[types.AccountID]types.U64
	Rank            types.U16
	Emission        types.U64
	Incentive       types.U16
	Consensus       types.U16
	Trust           types.U16
	ValidatorTrust  types.U16
	Dividends       types.U16
	LastUpdate      types.U64
	ValidatorPermit bool
	PruningScore    types.U16
}

// formatIP formats a U128 IP address based on its type (v4 or v6)
func formatIP(ip types.U128, ipType types.U8) string {
	// Convert U128 to big.Int for proper handling
	ipInt := ip.Int

	// Convert to bytes
	ipBytes := make([]byte, 16)
	ipInt.FillBytes(ipBytes) // This will zero-pad from the left

	if ipType == 4 {
		// For IPv4, use just the last 4 bytes
		return net.IPv4(ipBytes[12], ipBytes[13], ipBytes[14], ipBytes[15]).String()
	}
	// For IPv6, use all 16 bytes
	return net.IP(ipBytes).String()
}

// encodeSS58 encodes a 32-byte public key to an SS58 address using the provided ss58Format.
func encodeSS58(pubKey []byte, ss58Format uint8) (string, error) {
	if len(pubKey) != 32 {
		return "", errors.New("invalid public key length")
	}

	// For ss58Format < 64, the prefix is a single byte representing the format
	prefix := []byte{ss58Format}

	// Data is prefix followed by the 32-byte public key
	data := append(prefix, pubKey...)

	// Compute the checksum over the concatenation of "SS58PRE" and data
	checksumInput := append([]byte("SS58PRE"), data...)
	hash, err := blake2b.New512(nil)
	if err != nil {
		return "", err
	}
	if _, err := hash.Write(checksumInput); err != nil {
		return "", err
	}
	hashSum := hash.Sum(nil)
	// Take first 2 bytes of the hash as checksum
	checksum := hashSum[:2]

	// Append checksum to data
	finalData := append(data, checksum...)

	// Base58 encode the final data
	encoded := base58.Encode(finalData)
	return encoded, nil
}

// accountIDToSS58 transforms an AccountID into its SS58 string representation
func accountIDToSS58(account types.AccountID, ss58Format uint8) string {
	addr, err := encodeSS58(account[:], ss58Format)
	if err != nil {
		return ""
	}
	return addr
}

// SyncFromChain updates the metagraph state by querying the chain
func (m *Metagraph) SyncFromChain(ctx context.Context, querier ChainQuerier) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get current block
	block, err := querier.QueryBlock(ctx)
	if err != nil {
		return fmt.Errorf("querying block: %w", err)
	}
	m.Block = block

	// Get all neuron info in a single state call
	neuronsData, err := querier.GetNeuronsLite(ctx, m.NetUID)
	if err != nil {
		return fmt.Errorf("getting neurons lite: %w", err)
	}

	// Decode the neuron info
	neurons, err := DecodeNeuronsLite(neuronsData)
	if err != nil {
		return fmt.Errorf("decoding neurons lite: %w", err)
	}

	// Log summary of neurons with properly formatted values
	for _, neuron := range neurons {
		axonIP := formatIP(neuron.AxonInfo.IP, neuron.AxonInfo.IPType)
		var totalStake types.U64 = 0
		for _, amt := range neuron.Stake {
			totalStake += amt
		}
		stakeTAO := raoToTao(totalStake)
		fmt.Printf("Neuron - Hotkey: %s, Coldkey: %s, Axon: %s:%d, Total Stake: %.2f TAO\n",
			accountIDToSS58(neuron.Hotkey, 42),
			accountIDToSS58(neuron.Coldkey, 42),
			axonIP,
			neuron.AxonInfo.Port,
			stakeTAO)
	}

	// Reset maps
	m.Stakes = make(map[types.AccountID]types.U64)
	m.Active = make(map[types.AccountID]bool)
	m.Weights = make(map[types.AccountID]map[types.AccountID]types.U16)
	m.IPs = make(map[types.AccountID]string)
	m.Ports = make(map[types.AccountID]uint16)
	m.Versions = make(map[types.AccountID]string)
	m.LastUpdate = make(map[types.AccountID]time.Time)

	// Update metagraph state from decoded neuron info
	m.Hotkeys = make([]types.AccountID, 0, len(neurons))
	for _, neuron := range neurons {
		m.Hotkeys = append(m.Hotkeys, neuron.Hotkey)
		var totalStake types.U64 = 0
		for _, stake := range neuron.Stake {
			totalStake += stake
		}
		m.Stakes[neuron.Hotkey] = totalStake
		m.Weights[neuron.Hotkey] = map[types.AccountID]types.U16{neuron.Hotkey: neuron.Incentive}
		m.IPs[neuron.Hotkey] = formatIP(neuron.AxonInfo.IP, neuron.AxonInfo.IPType)
		m.Ports[neuron.Hotkey] = uint16(neuron.AxonInfo.Port)
		m.Versions[neuron.Hotkey] = fmt.Sprintf("%d", neuron.AxonInfo.Version)
		m.Active[neuron.Hotkey] = neuron.Active
		m.LastUpdate[neuron.Hotkey] = time.Unix(int64(neuron.LastUpdate), 0)
	}

	// Update total neurons count
	m.N = types.U16(len(neurons))

	return nil
}

// DecodeNeuronsLite decodes the raw bytes from the state call into NeuronInfo structs
func DecodeNeuronsLite(data []byte) ([]NeuronInfo, error) {
	// First dump the raw bytes for inspection
	fmt.Printf("Raw bytes length: %d\n", len(data))
	if len(data) > 64 {
		fmt.Printf("First 64 bytes: %02x\n", data[:64])
		fmt.Printf("Last 64 bytes: %02x\n", data[len(data)-64:])
	}

	// Print first few bytes in detail
	fmt.Println("First 10 bytes in detail:")
	for i := 0; i < min(10, len(data)); i++ {
		fmt.Printf("Byte %d: %02x\n", i, data[i])
	}

	// Instead of decoding the neuron vector directly from data, first decode a Bytes wrapper
	decoder := scale.NewDecoder(bytes.NewReader(data))
	var outer types.Bytes
	if err := decoder.Decode(&outer); err != nil {
		return nil, fmt.Errorf("decoding outer bytes: %w", err)
	}
	fmt.Printf("Decoded outer bytes length: %d\n", len(outer))

	// Now create a new decoder for the actual neuron data
	innerDecoder := scale.NewDecoder(bytes.NewReader(outer))

	// Decode the length of the neuron vector as a UCompact
	var length types.UCompact
	if err := innerDecoder.Decode(&length); err != nil {
		return nil, fmt.Errorf("decoding neuron vector length: %w", err)
	}

	neuronCount := uint32(length.Int64())
	fmt.Printf("Decoded neuron count: %d\n", neuronCount)

	if neuronCount == 0 {
		return nil, fmt.Errorf("zero neuron count")
	}
	if neuronCount > 1000 { // Sanity check - no subnet should have more than 1000 neurons
		return nil, fmt.Errorf("unreasonable neuron count: %d", neuronCount)
	}

	// Decode each neuron
	neurons := make([]NeuronInfo, 0, neuronCount)
	for i := uint32(0); i < neuronCount; i++ {
		// Suppress detailed logging for each neuron
		var neuron NeuronInfo
		if err := innerDecoder.Decode(&neuron.Hotkey); err != nil {
			return nil, fmt.Errorf("decoding hotkey for neuron %d: %w", i, err)
		}
		if err := innerDecoder.Decode(&neuron.Coldkey); err != nil {
			return nil, fmt.Errorf("decoding coldkey for neuron %d: %w", i, err)
		}

		// Validate account IDs are not zero
		if isZeroAccount(neuron.Hotkey) {
			return nil, fmt.Errorf("zero hotkey for neuron %d", i)
		}
		if isZeroAccount(neuron.Coldkey) {
			return nil, fmt.Errorf("zero coldkey for neuron %d", i)
		}

		// Decode UID and NetUID as compact integers
		var uid, netuid types.UCompact
		if err := innerDecoder.Decode(&uid); err != nil {
			return nil, fmt.Errorf("decoding UID for neuron %d: %w", i, err)
		}
		if err := innerDecoder.Decode(&netuid); err != nil {
			return nil, fmt.Errorf("decoding NetUID for neuron %d: %w", i, err)
		}
		neuron.UID = types.U16(uid.Int64())
		neuron.NetUID = types.U16(netuid.Int64())

		// Decode active flag
		if err := innerDecoder.Decode(&neuron.Active); err != nil {
			return nil, fmt.Errorf("decoding Active for neuron %d: %w", i, err)
		}

		// Decode axon_info struct
		if err := innerDecoder.Decode(&neuron.AxonInfo); err != nil {
			return nil, fmt.Errorf("decoding AxonInfo for neuron %d: %w", i, err)
		}

		// Decode prometheus_info struct
		if err := innerDecoder.Decode(&neuron.PrometheusInfo); err != nil {
			return nil, fmt.Errorf("decoding PrometheusInfo for neuron %d: %w", i, err)
		}

		// Decode stake vector length as compact
		var stakeLen types.UCompact
		if err := innerDecoder.Decode(&stakeLen); err != nil {
			return nil, fmt.Errorf("decoding stake length for neuron %d: %w", i, err)
		}
		stakeLenVal := uint64(stakeLen.Int64())

		// Decode stake entries
		neuron.Stake = make(map[types.AccountID]types.U64)
		for j := uint64(0); j < stakeLenVal; j++ {
			var account types.AccountID
			var amount types.UCompact
			if err := innerDecoder.Decode(&account); err != nil {
				return nil, fmt.Errorf("decoding stake account %d for neuron %d: %w", j, i, err)
			}
			if err := innerDecoder.Decode(&amount); err != nil {
				return nil, fmt.Errorf("decoding stake amount %d for neuron %d: %w", j, i, err)
			}
			if isZeroAccount(account) {
				return nil, fmt.Errorf("zero account in stake entry %d for neuron %d", j, i)
			}
			neuron.Stake[account] = types.U64(amount.Int64())
		}

		// Decode remaining compact fields
		var rank, emission, incentive, consensus, trust, validatorTrust, dividends, lastUpdate, pruningScore types.UCompact
		if err := innerDecoder.Decode(&rank); err != nil {
			return nil, fmt.Errorf("decoding Rank for neuron %d: %w", i, err)
		}
		if err := innerDecoder.Decode(&emission); err != nil {
			return nil, fmt.Errorf("decoding Emission for neuron %d: %w", i, err)
		}
		if err := innerDecoder.Decode(&incentive); err != nil {
			return nil, fmt.Errorf("decoding Incentive for neuron %d: %w", i, err)
		}
		if err := innerDecoder.Decode(&consensus); err != nil {
			return nil, fmt.Errorf("decoding Consensus for neuron %d: %w", i, err)
		}
		if err := innerDecoder.Decode(&trust); err != nil {
			return nil, fmt.Errorf("decoding Trust for neuron %d: %w", i, err)
		}
		if err := innerDecoder.Decode(&validatorTrust); err != nil {
			return nil, fmt.Errorf("decoding ValidatorTrust for neuron %d: %w", i, err)
		}
		if err := innerDecoder.Decode(&dividends); err != nil {
			return nil, fmt.Errorf("decoding Dividends for neuron %d: %w", i, err)
		}
		if err := innerDecoder.Decode(&lastUpdate); err != nil {
			return nil, fmt.Errorf("decoding LastUpdate for neuron %d: %w", i, err)
		}

		// Decode validator permit bool
		if err := innerDecoder.Decode(&neuron.ValidatorPermit); err != nil {
			return nil, fmt.Errorf("decoding ValidatorPermit for neuron %d: %w", i, err)
		}

		if err := innerDecoder.Decode(&pruningScore); err != nil {
			return nil, fmt.Errorf("decoding PruningScore for neuron %d: %w", i, err)
		}

		// Convert compact values with validation
		rankVal := rank.Int64()
		if rankVal > 65535 {
			return nil, fmt.Errorf("invalid rank value %d for neuron %d", rankVal, i)
		}
		neuron.Rank = types.U16(rankVal)

		neuron.Emission = types.U64(emission.Int64())

		incentiveVal := incentive.Int64()
		if incentiveVal > 65535 {
			return nil, fmt.Errorf("invalid incentive value %d for neuron %d", incentiveVal, i)
		}
		neuron.Incentive = types.U16(incentiveVal)

		consensusVal := consensus.Int64()
		if consensusVal > 65535 {
			return nil, fmt.Errorf("invalid consensus value %d for neuron %d", consensusVal, i)
		}
		neuron.Consensus = types.U16(consensusVal)

		trustVal := trust.Int64()
		if trustVal > 65535 {
			return nil, fmt.Errorf("invalid trust value %d for neuron %d", trustVal, i)
		}
		neuron.Trust = types.U16(trustVal)

		validatorTrustVal := validatorTrust.Int64()
		if validatorTrustVal > 65535 {
			return nil, fmt.Errorf("invalid validator trust value %d for neuron %d", validatorTrustVal, i)
		}
		neuron.ValidatorTrust = types.U16(validatorTrustVal)

		dividendsVal := dividends.Int64()
		if dividendsVal > 65535 {
			return nil, fmt.Errorf("invalid dividends value %d for neuron %d", dividendsVal, i)
		}
		neuron.Dividends = types.U16(dividendsVal)

		neuron.LastUpdate = types.U64(lastUpdate.Int64())

		pruningScoreVal := pruningScore.Int64()
		if pruningScoreVal > 65535 {
			return nil, fmt.Errorf("invalid pruning score value %d for neuron %d", pruningScoreVal, i)
		}
		neuron.PruningScore = types.U16(pruningScoreVal)

		neurons = append(neurons, neuron)
	}

	// Log a summary of the decoding process
	fmt.Printf("Completed decoding %d neurons\n", len(neurons))

	return neurons, nil
}

// isZeroAccount checks if an account ID is zero
func isZeroAccount(account types.AccountID) bool {
	for _, b := range account[:] {
		if b != 0 {
			return false
		}
	}
	return true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
