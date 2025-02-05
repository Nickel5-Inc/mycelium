package crypto

// Signer defines the interface for message signing operations
type Signer interface {
	// Sign signs the provided message and returns the signature
	Sign(message []byte) ([]byte, error)

	// Verify verifies the signature against the message
	Verify(message, signature []byte) bool

	// PublicKey returns the public key bytes
	PublicKey() []byte
}

// SignerFromWallet adapts a substrate.Wallet to implement the Signer interface
type SignerFromWallet struct {
	wallet interface {
		Sign(message []byte) ([]byte, error)
		Verify(message, signature []byte) bool
		GetAddress() string
	}
	publicKey []byte
}

// NewSignerFromWallet creates a new signer from a wallet
func NewSignerFromWallet(wallet interface {
	Sign(message []byte) ([]byte, error)
	Verify(message, signature []byte) bool
	GetAddress() string
}) *SignerFromWallet {
	return &SignerFromWallet{
		wallet: wallet,
	}
}

func (s *SignerFromWallet) Sign(message []byte) ([]byte, error) {
	return s.wallet.Sign(message)
}

func (s *SignerFromWallet) Verify(message, signature []byte) bool {
	return s.wallet.Verify(message, signature)
}

func (s *SignerFromWallet) PublicKey() []byte {
	return s.publicKey
}
