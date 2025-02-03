package substrate

import (
	"math/big"
	"testing"

	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatorInfoEncodeDecode(t *testing.T) {
	// Create test data with proper 32-byte arrays
	hotkeyBytes := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	coldkeyBytes := [32]byte{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}

	hotkey, err := types.NewAccountID(hotkeyBytes[:])
	require.NoError(t, err)

	coldkey, err := types.NewAccountID(coldkeyBytes[:])
	require.NoError(t, err)

	amount := types.NewU128(*big.NewInt(1000))

	original := ValidatorInfo{
		Hotkey:  *hotkey,
		Coldkey: *coldkey,
		Stake: Stake{
			Amount:     Balance(amount),
			LastUpdate: 12345,
		},
		Active:     true,
		LastUpdate: 67890,
	}

	// Test encoding
	encoded, err := original.EncodeToBytes()
	require.NoError(t, err)
	require.NotEmpty(t, encoded)

	// Test decoding
	var decoded ValidatorInfo
	err = decoded.DecodeFromBytes(encoded)
	require.NoError(t, err)

	// Verify all fields match
	assert.Equal(t, original.Hotkey, decoded.Hotkey)
	assert.Equal(t, original.Coldkey, decoded.Coldkey)
	assert.Equal(t, original.Stake.Amount, decoded.Stake.Amount)
	assert.Equal(t, original.Stake.LastUpdate, decoded.Stake.LastUpdate)
	assert.Equal(t, original.Active, decoded.Active)
	assert.Equal(t, original.LastUpdate, decoded.LastUpdate)
}
