package substrate

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/centrifuge/go-substrate-rpc-client/v4/scale"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

// Balance represents a substrate balance value
type Balance types.U128

func (b Balance) Encode(encoder scale.Encoder) error {
	return types.U128(b).Encode(encoder)
}

func (b *Balance) Decode(decoder scale.Decoder) error {
	var u types.U128
	err := u.Decode(decoder)
	if err != nil {
		return err
	}
	*b = Balance(u)
	return nil
}

// Stake represents a validator's stake amount
type Stake struct {
	// The amount staked
	Amount Balance
	// When the stake was last updated
	LastUpdate types.BlockNumber
}

func (s Stake) Encode(encoder scale.Encoder) error {
	if err := s.Amount.Encode(encoder); err != nil {
		return fmt.Errorf("encoding stake amount: %w", err)
	}
	if err := s.LastUpdate.Encode(encoder); err != nil {
		return fmt.Errorf("encoding stake last update: %w", err)
	}
	return nil
}

func (s *Stake) Decode(decoder scale.Decoder) error {
	if err := s.Amount.Decode(decoder); err != nil {
		return fmt.Errorf("decoding stake amount: %w", err)
	}
	if err := s.LastUpdate.Decode(decoder); err != nil {
		return fmt.Errorf("decoding stake last update: %w", err)
	}
	return nil
}

// ValidatorInfo represents a validator's information
type ValidatorInfo struct {
	// Hotkey is the validator's active key
	Hotkey types.AccountID
	// Coldkey is the validator's storage key
	Coldkey types.AccountID
	// Stake information
	Stake Stake
	// Active status
	Active bool
	// Last update block
	LastUpdate types.BlockNumber
}

func (v ValidatorInfo) Encode(encoder scale.Encoder) error {
	if err := encoder.Encode(v.Hotkey[:]); err != nil {
		return fmt.Errorf("encoding hotkey: %w", err)
	}
	if err := encoder.Encode(v.Coldkey[:]); err != nil {
		return fmt.Errorf("encoding coldkey: %w", err)
	}
	if err := v.Stake.Encode(encoder); err != nil {
		return fmt.Errorf("encoding stake: %w", err)
	}
	if err := encoder.Encode(v.Active); err != nil {
		return fmt.Errorf("encoding active status: %w", err)
	}
	if err := v.LastUpdate.Encode(encoder); err != nil {
		return fmt.Errorf("encoding last update: %w", err)
	}
	return nil
}

func (v *ValidatorInfo) Decode(decoder scale.Decoder) error {
	var hotkeyBytes, coldkeyBytes [32]byte
	if err := decoder.Decode(&hotkeyBytes); err != nil {
		return fmt.Errorf("decoding hotkey: %w", err)
	}
	if err := decoder.Decode(&coldkeyBytes); err != nil {
		return fmt.Errorf("decoding coldkey: %w", err)
	}

	copy(v.Hotkey[:], hotkeyBytes[:])
	copy(v.Coldkey[:], coldkeyBytes[:])

	if err := v.Stake.Decode(decoder); err != nil {
		return fmt.Errorf("decoding stake: %w", err)
	}
	if err := decoder.Decode(&v.Active); err != nil {
		return fmt.Errorf("decoding active status: %w", err)
	}
	if err := v.LastUpdate.Decode(decoder); err != nil {
		return fmt.Errorf("decoding last update: %w", err)
	}
	return nil
}

// EncodeToBytes encodes a validator info to bytes
func (v *ValidatorInfo) EncodeToBytes() ([]byte, error) {
	var buffer bytes.Buffer

	// Encode AccountIDs
	buffer.Write(v.Hotkey[:])
	buffer.Write(v.Coldkey[:])

	// Encode Stake Amount (U128)
	amount := types.U128(v.Stake.Amount)
	amountBytes := amount.Bytes()
	if len(amountBytes) < 16 {
		// Pad to 16 bytes if needed
		padded := make([]byte, 16)
		copy(padded[16-len(amountBytes):], amountBytes)
		amountBytes = padded
	}
	buffer.Write(amountBytes)

	// Encode Stake LastUpdate (BlockNumber)
	lastUpdateBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(lastUpdateBytes, uint32(v.Stake.LastUpdate))
	buffer.Write(lastUpdateBytes)

	// Encode Active status
	if v.Active {
		buffer.WriteByte(1)
	} else {
		buffer.WriteByte(0)
	}

	// Encode LastUpdate (BlockNumber)
	lastUpdateBytes = make([]byte, 4)
	binary.LittleEndian.PutUint32(lastUpdateBytes, uint32(v.LastUpdate))
	buffer.Write(lastUpdateBytes)

	return buffer.Bytes(), nil
}

// DecodeFromBytes decodes a validator info from raw storage data
func (v *ValidatorInfo) DecodeFromBytes(data []byte) error {
	if len(data) < 32*2+16+1+4 { // 2 AccountIDs + U128 + bool + BlockNumber
		return fmt.Errorf("insufficient data length for ValidatorInfo")
	}

	// Decode AccountIDs
	copy(v.Hotkey[:], data[:32])
	copy(v.Coldkey[:], data[32:64])

	// Decode Stake Amount (U128)
	amountBig := new(big.Int).SetBytes(data[64:80])
	v.Stake.Amount = Balance(types.NewU128(*amountBig))

	// Decode Stake LastUpdate (BlockNumber)
	v.Stake.LastUpdate = types.BlockNumber(binary.LittleEndian.Uint32(data[80:84]))

	// Decode Active status
	v.Active = data[84] != 0

	// Decode LastUpdate (BlockNumber)
	v.LastUpdate = types.BlockNumber(binary.LittleEndian.Uint32(data[85:89]))

	return nil
}
