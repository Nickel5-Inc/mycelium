package crypto

import (
	"github.com/ChainSafe/go-schnorrkel"
	"github.com/gtank/merlin"
)

// SigningContext is used to create a domain-specific signing context
const SigningContext = "MyceliumMessage"

// VerifySr25519 verifies an sr25519 signature
func VerifySr25519(message, signature, publicKey []byte) (bool, error) {
	// Convert public key
	var pubKeyBytes [32]byte
	copy(pubKeyBytes[:], publicKey)

	pubKey, err := schnorrkel.NewPublicKey(pubKeyBytes)
	if err != nil {
		return false, err
	}

	// Convert signature
	var sigBytes [64]byte
	copy(sigBytes[:], signature)
	sig := new(schnorrkel.Signature)
	err = sig.Decode(sigBytes)
	if err != nil {
		return false, err
	}

	// Create transcript
	t := merlin.NewTranscript(SigningContext)
	t.AppendMessage([]byte("sign-bytes"), message)

	// Verify using sr25519
	verified, err := pubKey.Verify(sig, t)
	return verified, err
}
