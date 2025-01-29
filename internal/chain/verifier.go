// Package chain provides functionality for interacting with the validator chain
package chain

import (
	"context"

	// Import the generated protobuf code
	chain "mycelium/proto/chain"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// VerifyResult contains the verification response data from the chain
type VerifyResult struct {
	Valid    bool              // Whether the signature is valid
	Stake    float64           // The validator's stake amount
	Metadata map[string]string // Additional metadata about the validator
}

// ValidatorInfo contains detailed information about a validator node
type ValidatorInfo struct {
	PeerID   string            // Unique identifier of the validator
	Stake    float64           // Validator's stake amount
	Rank     int64             // Validator's rank in the network
	IP       string            // Validator's IP address
	Port     int32             // Validator's port number
	Metadata map[string]string // Additional validator metadata
}

// Verifier defines the interface for chain verification operations
type Verifier interface {
	// VerifyPeer validates a peer's signature and returns their verification status
	VerifyPeer(ctx context.Context, peerID string, sig, msg []byte) (*VerifyResult, error)

	// GetStake retrieves the current stake amount for a peer
	GetStake(ctx context.Context, peerID string) (float64, error)

	// GetValidators returns the list of validators in a subnet
	GetValidators(ctx context.Context, subnet string) ([]ValidatorInfo, error)
}

// grpcVerifier implements the Verifier interface using gRPC
type grpcVerifier struct {
	client chain.ChainVerifierClient
	conn   *grpc.ClientConn
}

// NewVerifier creates a new gRPC-based verifier client
func NewVerifier(addr string) (Verifier, error) {
	// Create an insecure connection (TODO: Add TLS support)
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	return &grpcVerifier{
		client: chain.NewChainVerifierClient(conn),
		conn:   conn,
	}, nil
}

// Close closes the gRPC connection
func (v *grpcVerifier) Close() error {
	return v.conn.Close()
}

// VerifyPeer implements the Verifier interface
func (v *grpcVerifier) VerifyPeer(ctx context.Context, peerID string, sig, msg []byte) (*VerifyResult, error) {
	resp, err := v.client.VerifyPeer(ctx, &chain.VerifyRequest{
		PeerId:    peerID,
		Signature: sig,
		Message:   msg,
	})
	if err != nil {
		return nil, err
	}

	return &VerifyResult{
		Valid:    resp.Valid,
		Stake:    resp.Stake,
		Metadata: resp.Metadata,
	}, nil
}

// GetStake implements the Verifier interface
func (v *grpcVerifier) GetStake(ctx context.Context, peerID string) (float64, error) {
	resp, err := v.client.GetStake(ctx, &chain.StakeRequest{
		PeerId: peerID,
	})
	if err != nil {
		return 0, err
	}

	return resp.Stake, nil
}

// GetValidators implements the Verifier interface
func (v *grpcVerifier) GetValidators(ctx context.Context, subnet string) ([]ValidatorInfo, error) {
	resp, err := v.client.GetRegistry(ctx, &chain.RegistryRequest{
		Subnet: subnet,
	})
	if err != nil {
		return nil, err
	}

	validators := make([]ValidatorInfo, len(resp.Validators))
	for i, v := range resp.Validators {
		validators[i] = ValidatorInfo{
			PeerID:   v.PeerId,
			Stake:    v.Stake,
			Rank:     v.Rank,
			IP:       v.Ip,
			Port:     v.Port,
			Metadata: v.Metadata,
		}
	}

	return validators, nil
}
