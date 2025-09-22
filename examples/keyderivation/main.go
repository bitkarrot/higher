package main

import (
    "flag"
    "fmt"
    "log"
    "os"
    "strings"

    "github.com/bitkarrot/higher/keyderivation"
)

func main() {
    fmt.Println("=== Nostr Deterministic Key Derivation with btcd/btcutil and go-nostr ===")

    // CLI flags
    mnemonicFlag := flag.String("mnemonic", "", "BIP-39 mnemonic to use; if empty a new one is generated")
    mnemonicFile := flag.String("mnemonic-file", "", "Path to a file containing a BIP-39 mnemonic")
    start := flag.Uint("start", 0, "Start index for derivation (BIP32 and simple)")
    count := flag.Uint("count", 3, "Number of keys to derive with BIP32 method")
    simpleCount := flag.Uint("simple-count", 2, "Number of keys to derive with simple HMAC method")
    checkMax := flag.Uint("check-max", 100, "Max index to scan when checking key ownership")
    eventIndex := flag.Uint("event-index", 0, "Index to use when creating the sample Nostr event")
    message := flag.String("message", "Hello Nostr! This message was signed with a derived key.", "Content for the sample Nostr event")
    flag.Parse()

    // Basic validation and sane minimums
    if *count == 0 {
        *count = 1
    }
    if *simpleCount == 0 {
        *simpleCount = 1
    }
    // checkMax can be zero (meaning only index 0)

    // Resolve mnemonic: direct flag > file > empty (auto-generate)
    mnemonic := strings.TrimSpace(*mnemonicFlag)
    if mnemonic == "" && *mnemonicFile != "" {
        data, err := os.ReadFile(*mnemonicFile)
        if err != nil {
            log.Fatal("Failed to read mnemonic file:", err)
        }
        mnemonic = strings.TrimSpace(string(data))
    }

    // Method 1: Using BIP39 mnemonic with btcutil
    fmt.Println("\n1. Creating deriver from mnemonic using btcutil...")
    deriver, err := keyderivation.NewNostrKeyDeriver(mnemonic)
    if err != nil {
        log.Fatal("Failed to create key deriver:", err)
    }

    fmt.Printf("Mnemonic: %s\n", deriver.GetMnemonic())

    // Show master (root) keypair for clarity
    fmt.Println("\n1a. Master keypair (root derived from mnemonic/seed):")
    masterKP, err := deriver.GetMasterKeyPair()
    if err != nil {
        log.Fatal("Failed to get master keypair:", err)
    }
    fmt.Printf("  Master Private (hex): %s\n", masterKP.PrivateKey)
    fmt.Printf("  Master Public  (hex): %s\n", masterKP.PublicKey)
    fmt.Printf("  Master Private (nsec): %s\n", masterKP.PrivateKeyNIP)
    fmt.Printf("  Master Public  (npub): %s\n", masterKP.PublicKeyNIP)

    // Derive keys using BIP32 with btcutil
    fmt.Println("\n2. Deriving keys using BIP32 method with btcutil...")
    keys, err := deriver.DeriveMultipleKeys(uint32(*start), uint32(*count), true)
    if err != nil {
        log.Fatal("Failed to derive keys:", err)
    }

    for _, key := range keys {
        fmt.Printf("Index %d:\n", key.Index)
        fmt.Printf("  Private (hex): %s\n", key.PrivateKey)
        fmt.Printf("  Public (hex):  %s\n", key.PublicKey)
        fmt.Printf("  Private (nsec): %s\n", key.PrivateKeyNIP)
        fmt.Printf("  Public (npub):  %s\n", key.PublicKeyNIP)
        fmt.Println()
    }

    // Test with simple HMAC method
    fmt.Println("\n3. Deriving keys using simple HMAC method...")
    simpleKeys, err := deriver.DeriveMultipleKeys(uint32(*start), uint32(*simpleCount), false)
    if err != nil {
        log.Fatal("Failed to derive simple keys:", err)
    }

    for _, key := range simpleKeys {
        fmt.Printf("Index %d:\n", key.Index)
        fmt.Printf("  Private (nsec): %s\n", key.PrivateKeyNIP)
        fmt.Printf("  Public (npub):  %s\n", key.PublicKeyNIP)
        fmt.Println()
    }

    // Test key identification with both hex and NIP-19 formats
    fmt.Println("\n4. Testing key identification...")
    // Prefer index 1 if present, else fall back to 0
    target := keys[0]
    if len(keys) > 1 {
        target = keys[1]
    }
    targetKeyHex := target.PublicKey
    targetKeyNIP := target.PublicKeyNIP

    // Test with hex format
    found, index, err := deriver.CheckKeyBelongsToMaster(targetKeyHex, uint32(*checkMax), true)
    if err != nil {
        log.Fatal("Failed to check key (hex):", err)
    }
    fmt.Printf("✅ Hex key %s found at index %d: %v\n", targetKeyHex[:16]+"...", index, found)

    // Test with NIP-19 format
    found, index, err = deriver.CheckKeyBelongsToMaster(targetKeyNIP, uint32(*checkMax), true)
    if err != nil {
        log.Fatal("Failed to check key (NIP-19):", err)
    }
    fmt.Printf("✅ NIP-19 key %s found at index %d: %v\n", targetKeyNIP[:16]+"...", index, found)

    // Create a sample Nostr event using go-nostr
    fmt.Println("\n5. Creating Nostr event with go-nostr...")
    event, err := deriver.CreateNostrEvent(uint32(*eventIndex), *message)
    if err != nil {
        log.Fatal("Failed to create event:", err)
    }

    fmt.Printf("Event ID: %s\n", event.ID)
    fmt.Printf("Author: %s\n", event.PubKey)
    fmt.Printf("Content: %s\n", event.Content)
    fmt.Printf("Signature: %s\n", event.Sig)
    ok, sigErr := event.CheckSignature()
    fmt.Printf("Valid signature: %v (err: %v)\n", ok, sigErr)

    // Verify the event author matches our derived key
    firstKey := keys[0]
    fmt.Printf("Matches derived key: %v\n", event.PubKey == firstKey.PublicKey)

    fmt.Println("\n=== Demo Complete ===")
}
