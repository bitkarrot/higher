package keyderivation

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/tyler-smith/go-bip39"
)

// NostrKeyPair represents a Nostr public/private key pair with additional formats
type NostrKeyPair struct {
	PrivateKey    string `json:"private_key"`     // hex encoded
	PublicKey     string `json:"public_key"`      // hex encoded
	PrivateKeyNIP string `json:"private_key_nip"` // nsec format
	PublicKeyNIP  string `json:"public_key_nip"`  // npub format
	Index         uint32 `json:"index"`
}

// GetMasterKeyPair returns the master key (root) as a NostrKeyPair
// This is the raw master private/public key derived from the BIP32 master extended key.
func (nkd *NostrKeyDeriver) GetMasterKeyPair() (*NostrKeyPair, error) {
    // Obtain EC private key from master extended key
    privKey, err := nkd.masterKey.ECPrivKey()
    if err != nil {
        return nil, fmt.Errorf("failed to get master EC private key: %v", err)
    }

    // Serialize private key bytes
    privKeyBytes := privKey.Serialize()
    // Nostr expects 32-byte x-only pubkey; use compressed pubkey and drop prefix byte
    pubKeyBytes := privKey.PubKey().SerializeCompressed()[1:]

    // Hex encodings
    privKeyHex := hex.EncodeToString(privKeyBytes)
    pubKeyHex := hex.EncodeToString(pubKeyBytes)

    // NIP-19 encodings
    privKeyNIP, err := nip19.EncodePrivateKey(privKeyHex)
    if err != nil {
        return nil, fmt.Errorf("failed to encode master private key to NIP-19: %v", err)
    }
    pubKeyNIP, err := nip19.EncodePublicKey(pubKeyHex)
    if err != nil {
        return nil, fmt.Errorf("failed to encode master public key to NIP-19: %v", err)
    }

    return &NostrKeyPair{
        PrivateKey:    privKeyHex,
        PublicKey:     pubKeyHex,
        PrivateKeyNIP: privKeyNIP,
        PublicKeyNIP:  pubKeyNIP,
        Index:         0,
    }, nil
}

// GetMasterKeyPairNostr returns the master key pair in Nostr formats
func (nkd *NostrKeyDeriver) GetMasterKeyPairNostr() (*NostrKeyPair, error) {
    masterKeyPair, err := nkd.GetMasterKeyPair()
    if err != nil {
        return nil, err
    }

    return masterKeyPair, nil
}

// NostrKeyDeriver handles deterministic key derivation for Nostr
type NostrKeyDeriver struct {
	masterKey  *hdkeychain.ExtendedKey
	mnemonic   string
	masterSeed []byte
	network    *chaincfg.Params
}

// NewNostrKeyDeriver creates a new key deriver from a mnemonic
func NewNostrKeyDeriver(mnemonic string) (*NostrKeyDeriver, error) {
	if mnemonic == "" {
		// Generate a new mnemonic if none provided
		entropy, err := bip39.NewEntropy(128) // 12 words
		if err != nil {
			return nil, fmt.Errorf("failed to generate entropy: %v", err)
		}
		mnemonic, err = bip39.NewMnemonic(entropy)
		if err != nil {
			return nil, fmt.Errorf("failed to generate mnemonic: %v", err)
		}
		fmt.Printf("Generated mnemonic: %s\n", mnemonic)
	}

	// Validate mnemonic
	if !bip39.IsMnemonicValid(mnemonic) {
		return nil, fmt.Errorf("invalid mnemonic")
	}

	// Generate seed from mnemonic (with empty passphrase)
	seed := bip39.NewSeed(mnemonic, "")

	// Use mainnet parameters (standard for most applications)
	network := &chaincfg.MainNetParams

	// Create master key from seed using btcutil
	masterKey, err := hdkeychain.NewMaster(seed, network)
	if err != nil {
		return nil, fmt.Errorf("failed to create master key: %v", err)
	}

	return &NostrKeyDeriver{
		masterKey:  masterKey,
		mnemonic:   mnemonic,
		masterSeed: seed,
		network:    network,
	}, nil
}

// NewNostrKeyDeriverFromSeed creates a deriver directly from seed bytes
func NewNostrKeyDeriverFromSeed(seed []byte) (*NostrKeyDeriver, error) {
	network := &chaincfg.MainNetParams
	masterKey, err := hdkeychain.NewMaster(seed, network)
	if err != nil {
		return nil, fmt.Errorf("failed to create master key: %v", err)
	}

	return &NostrKeyDeriver{
		masterKey:  masterKey,
		masterSeed: seed,
		network:    network,
	}, nil
}

// DeriveKeyBIP32 derives a Nostr key using BIP32 hierarchical derivation
// Uses path: m/44'/1237'/0'/0/index (standard Nostr derivation path)
func (nkd *NostrKeyDeriver) DeriveKeyBIP32(index uint32) (*NostrKeyPair, error) {
	// BIP44 derivation path: m/44'/1237'/0'/0/index
	// 44' = BIP44 purpose (hardened)
	purposeKey, err := nkd.masterKey.Derive(hdkeychain.HardenedKeyStart + 44)
	if err != nil {
		return nil, fmt.Errorf("failed to derive purpose key: %v", err)
	}

	// 1237' = Nostr coin type (hardened) - officially registered
	coinTypeKey, err := purposeKey.Derive(hdkeychain.HardenedKeyStart + 1237)
	if err != nil {
		return nil, fmt.Errorf("failed to derive coin type key: %v", err)
	}

	// 0' = Account (hardened)
	accountKey, err := coinTypeKey.Derive(hdkeychain.HardenedKeyStart + 0)
	if err != nil {
		return nil, fmt.Errorf("failed to derive account key: %v", err)
	}

	// 0 = External chain (non-hardened)
	chainKey, err := accountKey.Derive(0)
	if err != nil {
		return nil, fmt.Errorf("failed to derive chain key: %v", err)
	}

	// index = Address index (non-hardened)
	childKey, err := chainKey.Derive(index)
	if err != nil {
		return nil, fmt.Errorf("failed to derive child key at index %d: %v", index, err)
	}

	// Get the private key
	privKey, err := childKey.ECPrivKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get EC private key: %v", err)
	}

	// Convert to bytes for Nostr format
	privKeyBytes := privKey.Serialize()
	pubKeyBytes := privKey.PubKey().SerializeCompressed()[1:] // Remove 0x02/0x03 prefix for Nostr

	// Create Nostr-compatible hex strings
	privKeyHex := hex.EncodeToString(privKeyBytes)
	pubKeyHex := hex.EncodeToString(pubKeyBytes)

	// Create NIP-19 encoded versions using go-nostr
	privKeyNIP, err := nip19.EncodePrivateKey(privKeyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to encode private key to NIP-19: %v", err)
	}

	pubKeyNIP, err := nip19.EncodePublicKey(pubKeyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to encode public key to NIP-19: %v", err)
	}

	return &NostrKeyPair{
		PrivateKey:    privKeyHex,
		PublicKey:     pubKeyHex,
		PrivateKeyNIP: privKeyNIP,
		PublicKeyNIP:  pubKeyNIP,
		Index:         index,
	}, nil
}

// DeriveKeySimple derives a key using simple HMAC-SHA256 approach
func (nkd *NostrKeyDeriver) DeriveKeySimple(index uint32) (*NostrKeyPair, error) {
	// Create HMAC with master seed as key
	h := hmac.New(sha256.New, nkd.masterSeed)

	// Write "nostr" + index as data
	indexBytes := make([]byte, 4)
	indexBytes[0] = byte(index >> 24)
	indexBytes[1] = byte(index >> 16)
	indexBytes[2] = byte(index >> 8)
	indexBytes[3] = byte(index)

	h.Write([]byte("nostr"))
	h.Write(indexBytes)

	// Get the derived key (32 bytes)
	derivedKey := h.Sum(nil)

	// Create private/public key from bytes using btcec
	_, pubKey := btcec.PrivKeyFromBytes(derivedKey)

	// Format for Nostr (remove compression prefix from public key)
	pubKeyBytes := pubKey.SerializeCompressed()[1:]

	// Create hex strings
	privKeyHex := hex.EncodeToString(derivedKey)
	pubKeyHex := hex.EncodeToString(pubKeyBytes)

	// Create NIP-19 encoded versions
	privKeyNIP, err := nip19.EncodePrivateKey(privKeyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to encode private key to NIP-19: %v", err)
	}

	pubKeyNIP, err := nip19.EncodePublicKey(pubKeyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to encode public key to NIP-19: %v", err)
	}

	return &NostrKeyPair{
		PrivateKey:    privKeyHex,
		PublicKey:     pubKeyHex,
		PrivateKeyNIP: privKeyNIP,
		PublicKeyNIP:  pubKeyNIP,
		Index:         index,
	}, nil
}

// DeriveMultipleKeys derives multiple keys at once
func (nkd *NostrKeyDeriver) DeriveMultipleKeys(start, count uint32, useBIP32 bool) ([]*NostrKeyPair, error) {
	keys := make([]*NostrKeyPair, 0, count)

	for i := uint32(0); i < count; i++ {
		index := start + i
		var keyPair *NostrKeyPair
		var err error

		if useBIP32 {
			keyPair, err = nkd.DeriveKeyBIP32(index)
		} else {
			keyPair, err = nkd.DeriveKeySimple(index)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to derive key at index %d: %v", index, err)
		}

		keys = append(keys, keyPair)
	}

	return keys, nil
}

// CheckKeyBelongsToMaster checks if a given key belongs to this master key
// Supports both hex format and NIP-19 format inputs
func (nkd *NostrKeyDeriver) CheckKeyBelongsToMaster(targetKey string, maxIndex uint32, useBIP32 bool) (bool, uint32, error) {
	// Try to decode if it's NIP-19 format
	var targetPubKey string
	if prefix, decoded, err := nip19.Decode(targetKey); err == nil {
		if prefix == "npub" {
			targetPubKey = decoded.(string)
		} else {
			return false, 0, fmt.Errorf("unsupported NIP-19 format: %s", prefix)
		}
	} else {
		// Assume it's hex format
		targetPubKey = targetKey
	}

	// Search through derivation indices
	for i := uint32(0); i <= maxIndex; i++ {
		var keyPair *NostrKeyPair
		var err error

		if useBIP32 {
			keyPair, err = nkd.DeriveKeyBIP32(i)
		} else {
			keyPair, err = nkd.DeriveKeySimple(i)
		}

		if err != nil {
			return false, 0, fmt.Errorf("failed to derive key at index %d: %v", i, err)
		}

		if keyPair.PublicKey == targetPubKey {
			return true, i, nil
		}
	}

	return false, 0, nil
}

// CreateNostrEvent creates a sample Nostr event using go-nostr
func (nkd *NostrKeyDeriver) CreateNostrEvent(keyIndex uint32, content string) (*nostr.Event, error) {
	keyPair, err := nkd.DeriveKeyBIP32(keyIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to derive key: %v", err)
	}

	// Create a new event
	event := &nostr.Event{
		Kind:      nostr.KindTextNote,
		Content:   content,
		CreatedAt: nostr.Now(),
		Tags:      nostr.Tags{},
	}

	// Sign the event using the derived private key
	err = event.Sign(keyPair.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign event: %v", err)
	}

	return event, nil
}

// GetMnemonic returns the mnemonic phrase (if available)
func (nkd *NostrKeyDeriver) GetMnemonic() string {
	return nkd.mnemonic
}

// GenerateRandomSeed generates a random 32-byte seed
func GenerateRandomSeed() ([]byte, error) {
	seed := make([]byte, 32)
	_, err := rand.Read(seed)
	if err != nil {
		return nil, fmt.Errorf("failed to generate random seed: %v", err)
	}
	return seed, nil
}
