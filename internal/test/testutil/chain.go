package testutil

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"github.com/stretchr/testify/require"
)

// TestChain represents a test substrate chain
type TestChain struct {
	t      *testing.T
	server *TestHTTPServer
	blocks map[types.Hash]types.Block
	height types.U64
}

// NewTestChain creates a new test chain
func NewTestChain(t *testing.T) *TestChain {
	chain := &TestChain{
		t:      t,
		blocks: make(map[types.Hash]types.Block),
		height: 0,
	}

	// Create HTTP server for chain RPC
	server := NewTestHTTPServer(t, chain.handleRPC())
	chain.server = server

	return chain
}

// URL returns the chain RPC URL
func (c *TestChain) URL() string {
	return c.server.URL()
}

// Height returns the current chain height
func (c *TestChain) Height() types.U64 {
	return c.height
}

// AddBlock adds a block to the chain
func (c *TestChain) AddBlock(block types.Block) types.Hash {
	hash := types.NewHash([]byte(fmt.Sprintf("block-%d", c.height)))
	c.blocks[hash] = block
	c.height++
	return hash
}

// GetBlock returns a block by hash
func (c *TestChain) GetBlock(hash types.Hash) (types.Block, bool) {
	block, ok := c.blocks[hash]
	return block, ok
}

// GetBlockByNumber returns a block by its number
func (c *TestChain) GetBlockByNumber(number types.U64) (types.Block, types.Hash, bool) {
	for hash, block := range c.blocks {
		if uint64(block.Header.Number) == uint64(number) {
			return block, hash, true
		}
	}
	return types.Block{}, types.Hash{}, false
}

// handleRPC handles RPC requests
func (c *TestChain) handleRPC() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID      int           `json:"id"`
			JSONRPC string        `json:"jsonrpc"`
			Method  string        `json:"method"`
			Params  []interface{} `json:"params"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var result interface{}
		var err error

		switch req.Method {
		case "chain_getBlock":
			var hash types.Hash
			if len(req.Params) > 0 {
				hashStr := req.Params[0].(string)
				hashBytes, err := hex.DecodeString(hashStr)
				if err != nil {
					break
				}
				copy(hash[:], hashBytes)
			} else {
				hash = c.GetFinalizedHead()
			}
			block, ok := c.blocks[hash]
			if !ok {
				err = fmt.Errorf("block not found")
				break
			}
			result = block
		case "chain_getBlockHash":
			number := types.U64(req.Params[0].(float64))
			_, hash, ok := c.GetBlockByNumber(number)
			if !ok {
				err = fmt.Errorf("block not found")
				break
			}
			result = hash
		default:
			err = fmt.Errorf("unknown method: %s", req.Method)
		}

		resp := struct {
			ID      int         `json:"id"`
			JSONRPC string      `json:"jsonrpc"`
			Result  interface{} `json:"result,omitempty"`
			Error   string      `json:"error,omitempty"`
		}{
			ID:      req.ID,
			JSONRPC: "2.0",
		}

		if err != nil {
			resp.Error = err.Error()
		} else {
			resp.Result = result
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
}

// GetFinalizedHead returns the hash of the finalized head
func (c *TestChain) GetFinalizedHead() types.Hash {
	if c.height == 0 {
		return types.Hash{}
	}
	_, hash, _ := c.GetBlockByNumber(c.height - 1)
	return hash
}

// RequireBlockExists asserts that a block exists
func (c *TestChain) RequireBlockExists(t *testing.T, hash types.Hash) {
	_, ok := c.GetBlock(hash)
	require.True(t, ok)
}

// RequireBlockNotExists asserts that a block does not exist
func (c *TestChain) RequireBlockNotExists(t *testing.T, hash types.Hash) {
	_, ok := c.GetBlock(hash)
	require.False(t, ok)
}

// RequireHeight asserts the current chain height
func (c *TestChain) RequireHeight(t *testing.T, expected types.U64) {
	require.Equal(t, expected, c.Height())
}

// WithTestChain runs a test with a chain
func WithTestChain(t *testing.T, fn func(*TestChain)) {
	chain := NewTestChain(t)
	fn(chain)
}
