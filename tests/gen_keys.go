//go:build genkeys
// +build genkeys

package main

import (
	"fmt"
	"log"
	"os"

	"github.com/bitkarrot/higher/keyderivation"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env from project root (script is under tests/)
	if err := godotenv.Load(".env"); err != nil {
		log.Fatalf("failed to load .env: %v", err)
	}

	mnemonic, ok := os.LookupEnv("RELAY_MNEMONIC")
	if !ok || mnemonic == "" {
		log.Fatalf("RELAY_MNEMONIC not set in .env")
	}

	der, err := keyderivation.NewNostrKeyDeriver(mnemonic)
	if err != nil {
		log.Fatalf("failed to initialize deriver: %v", err)
	}

	fmt.Println("Generated keys (BIP32, path m/44'/1237'/0'/0/index):")
	for i := uint32(0); i < 5; i++ {
		kp, err := der.DeriveKeyBIP32(i)
		if err != nil {
			log.Fatalf("failed to derive key at index %d: %v", i, err)
		}
		fmt.Printf("\nIndex: %d\n", i)
		fmt.Printf("  Public (hex): %s\n", kp.PublicKey)
		fmt.Printf("  Private (hex): %s\n", kp.PrivateKey)
		fmt.Printf("  npub: %s\n", kp.PublicKeyNIP)
		fmt.Printf("  nsec: %s\n", kp.PrivateKeyNIP)
	}
}
