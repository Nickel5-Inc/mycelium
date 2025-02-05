package substrate

import (
	"fmt"

	gsrpc "github.com/centrifuge/go-substrate-rpc-client/v4"
	"github.com/centrifuge/go-substrate-rpc-client/v4/signature"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

// ExtrinsicBuilder helps build and sign substrate extrinsics
type ExtrinsicBuilder struct {
	api      *gsrpc.SubstrateAPI
	metadata *types.Metadata
	call     types.Call
}

// NewExtrinsicBuilder creates a new extrinsic builder
func NewExtrinsicBuilder(api *gsrpc.SubstrateAPI, metadata *types.Metadata) *ExtrinsicBuilder {
	return &ExtrinsicBuilder{
		api:      api,
		metadata: metadata,
	}
}

// WithCall sets the call for the extrinsic
func (b *ExtrinsicBuilder) WithCall(module, function string, args ...interface{}) (*ExtrinsicBuilder, error) {
	call, err := types.NewCall(b.metadata, fmt.Sprintf("%s.%s", module, function), args...)
	if err != nil {
		return nil, fmt.Errorf("creating call: %w", err)
	}
	b.call = call
	return b, nil
}

// Build creates and signs the extrinsic
func (b *ExtrinsicBuilder) Build(keypair signature.KeyringPair) (types.Extrinsic, error) {
	// Create extrinsic
	ext := types.NewExtrinsic(b.call)

	// Get genesis hash
	genesisHash, err := b.api.RPC.Chain.GetBlockHash(0)
	if err != nil {
		return types.Extrinsic{}, fmt.Errorf("getting genesis hash: %w", err)
	}

	// Get runtime version
	rv, err := b.api.RPC.State.GetRuntimeVersionLatest()
	if err != nil {
		return types.Extrinsic{}, fmt.Errorf("getting runtime version: %w", err)
	}

	// Sign the extrinsic
	era := types.ExtrinsicEra{IsMortalEra: false}
	err = ext.Sign(keypair, types.SignatureOptions{
		BlockHash:          genesisHash,
		Era:                era,
		GenesisHash:        genesisHash,
		Nonce:              types.NewUCompactFromUInt(0),
		SpecVersion:        rv.SpecVersion,
		Tip:                types.NewUCompactFromUInt(0),
		TransactionVersion: rv.TransactionVersion,
	})
	if err != nil {
		return types.Extrinsic{}, fmt.Errorf("signing extrinsic: %w", err)
	}

	return ext, nil
}
