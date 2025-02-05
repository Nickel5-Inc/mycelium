// Package substrate provides a bridge between Go and Python for substrate functionality.
package substrate

/*
#include <stdbool.h>
#include <stdint.h>
#include <stdlib.h>
*/
import "C"
import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"sync"
	"time"
	"unsafe"

	"github.com/centrifuge/go-substrate-rpc-client/v4/signature"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"github.com/decred/base58"
)

var clients sync.Map

//export NewSubstrateClient
func NewSubstrateClient(url *C.char) unsafe.Pointer {
	client, err := NewClient(Config{
		Endpoint:   C.GoString(url),
		SS58Format: 42,       // Bittensor format
		ChainSpec:  "finney", // Default to finney network
	})
	if err != nil {
		return nil
	}
	ptr := unsafe.Pointer(client)
	clients.Store(ptr, client)
	return ptr
}

//export GetStake
func GetStake(client unsafe.Pointer, hotkey, coldkey *C.char) C.double {
	c := (*Client)(client)
	if c == nil {
		return 0
	}

	hotkeyBytes := decodeAddress(C.GoString(hotkey))
	coldkeyBytes := decodeAddress(C.GoString(coldkey))
	if hotkeyBytes == (types.AccountID{}) || coldkeyBytes == (types.AccountID{}) {
		return 0
	}

	stake, err := c.GetStake(nil, hotkeyBytes, coldkeyBytes)
	if err != nil {
		return 0
	}

	// Convert U128 to float64 via string to avoid precision loss
	stakeFloat, _ := stake.Float64()
	return C.double(stakeFloat / 1e9) // Convert RAO to TAO
}

//export AddStake
func AddStake(client unsafe.Pointer, amount C.double, hotkey, coldkeypair *C.char, waitForInclusion, waitForFinalization C.bool) C.bool {
	c, ok := clients.Load(client)
	if !ok {
		return false
	}
	cli := c.(*Client)

	hotkeyID := decodeAddress(C.GoString(hotkey))
	coldkeyID := decodeAddress(C.GoString(coldkeypair))

	// Convert amount from TAO to RAO (1 TAO = 1e9 RAO)
	amountRao := big.NewInt(int64(amount * 1e9))
	stake := types.NewU128(*amountRao)

	config := DefaultStakeConfig()
	config.MaxAttempts = 3
	config.RetryDelay = 1 * time.Second

	kp := signature.KeyringPair{
		URI:       C.GoString(coldkeypair),
		Address:   C.GoString(coldkeypair),
		PublicKey: coldkeyID[:],
	}

	err := cli.AddStake(context.Background(), stake, hotkeyID, kp, config)
	return C.bool(err == nil)
}

//export RemoveStake
func RemoveStake(client unsafe.Pointer, amount C.double, hotkey, coldkeypair *C.char, waitForInclusion, waitForFinalization C.bool) C.bool {
	c, ok := clients.Load(client)
	if !ok {
		return false
	}
	cli := c.(*Client)

	hotkeyID := decodeAddress(C.GoString(hotkey))
	coldkeyID := decodeAddress(C.GoString(coldkeypair))

	// Convert amount from TAO to RAO (1 TAO = 1e9 RAO)
	amountRao := big.NewInt(int64(amount * 1e9))
	stake := types.NewU128(*amountRao)

	config := DefaultStakeConfig()
	config.MaxAttempts = 3
	config.RetryDelay = 1 * time.Second

	kp := signature.KeyringPair{
		URI:       C.GoString(coldkeypair),
		Address:   C.GoString(coldkeypair),
		PublicKey: coldkeyID[:],
	}

	err := cli.RemoveStake(context.Background(), stake, hotkeyID, kp, config)
	return C.bool(err == nil)
}

//export SetWeights
func SetWeights(client unsafe.Pointer, netuid C.ushort, weightsJSON *C.char, keypair *C.char, waitForInclusion, waitForFinalization C.bool) C.bool {
	c := (*Client)(client)
	if c == nil {
		return false
	}

	// Parse weights from JSON
	var weights map[string]float64
	if err := json.Unmarshal([]byte(C.GoString(weightsJSON)), &weights); err != nil {
		return false
	}

	// Convert weights to u16 format
	weightMap := make(map[types.AccountID]types.U16)
	for addr, weight := range weights {
		if bytes := decodeAddress(addr); bytes != (types.AccountID{}) {
			weightMap[bytes] = types.U16(weight * float64(^uint16(0)))
		}
	}

	keypairBytes := decodeAddress(C.GoString(keypair))
	if keypairBytes == (types.AccountID{}) {
		return false
	}

	// Create keypair from bytes
	kp := signature.KeyringPair{
		URI:       string(keypairBytes[:]),
		Address:   C.GoString(keypair),
		PublicKey: keypairBytes[:],
	}

	config := DefaultSetWeightsConfig()
	config.MaxAttempts = 3
	config.RetryDelay = 30 * time.Second

	err := c.SetWeights(nil, types.U16(netuid), weightMap, kp, config)
	return C.bool(err == nil)
}

//export ServeAxon
func ServeAxon(client unsafe.Pointer, netuid C.ushort, ip *C.char, port C.ushort, keypair *C.char, version *C.char) C.bool {
	c := (*Client)(client)
	if c == nil {
		return false
	}

	keypairBytes := decodeAddress(C.GoString(keypair))
	if keypairBytes == (types.AccountID{}) {
		return false
	}

	// Create keypair from bytes
	kp := signature.KeyringPair{
		URI:       string(keypairBytes[:]),
		Address:   C.GoString(keypair),
		PublicKey: keypairBytes[:],
	}

	config := DefaultServeAxonConfig()
	err := c.ServeAxon(nil, types.U16(netuid), C.GoString(ip), uint16(port), kp, C.GoString(version), config)
	return C.bool(err == nil)
}

//export IsRegistered
func IsRegistered(client unsafe.Pointer, hotkey *C.char, netuid C.ushort) C.bool {
	c := (*Client)(client)
	if c == nil {
		return false
	}

	hotkeyBytes := decodeAddress(C.GoString(hotkey))
	if hotkeyBytes == (types.AccountID{}) {
		return false
	}

	registered, err := c.CanSetWeights(nil, types.U16(netuid), hotkeyBytes[:])
	return C.bool(err == nil && registered)
}

//export GetPrometheusInfo
func GetPrometheusInfo(client unsafe.Pointer, hotkey *C.char, netuid C.ushort) *C.char {
	c, ok := clients.Load(client)
	if !ok {
		return nil
	}
	cli := c.(*Client)

	hotkeyID := decodeAddress(C.GoString(hotkey))
	if hotkeyID == (types.AccountID{}) {
		return nil
	}

	// Query prometheus info
	ip, port, version, err := cli.QueryAxonInfo(context.Background(), types.U16(netuid), hotkeyID)
	if err != nil {
		return nil
	}

	// Create metrics info map
	info := map[string]string{
		"ip":      ip,
		"port":    fmt.Sprintf("%d", port+1), // Prometheus port is axon port + 1
		"version": version,
	}

	// Convert to JSON
	jsonBytes, err := json.Marshal(info)
	if err != nil {
		return nil
	}

	return C.CString(string(jsonBytes))
}

//export FreeString
func FreeString(str *C.char) {
	C.free(unsafe.Pointer(str))
}

//export FreeClient
func FreeClient(client unsafe.Pointer) {
	if c, ok := clients.Load(client); ok {
		cli := c.(*Client)
		cli.Close()
		clients.Delete(client)
	}
}

// Helper function to decode SS58 addresses
func decodeAddress(addr string) types.AccountID {
	var id types.AccountID

	// Decode base58 string
	decoded := base58.Decode(addr)
	if len(decoded) != 35 { // 1 byte version + 32 bytes public key + 2 bytes checksum
		return id
	}

	// Skip version byte and checksum
	copy(id[:], decoded[1:33])
	return id
}
