# Key Derivation Example (Nostr HD Keys)

This example (`examples/keyderivation/main.go`) demonstrates how to deterministically derive Nostr keypairs from a single master secret using two approaches:

- BIP32/BIP44-style derivation with `btcd/btcutil` (recommended)
- A simple HMAC-SHA256 scheme (for comparison/testing)

It also shows how to:

- Display the generated BIP-39 mnemonic and master/root keypair
- Derive multiple child keys and print them in both hex and NIP-19 formats (`nsec`/`npub`)
- Check whether a given public key (hex or `npub`) belongs to the same master key
- Create and sign a Nostr event using a derived private key via `go-nostr`

## Derivation Details

Implemented in `keyderivation/hdkey.go`:

- Path: `m/44'/1237'/0'/0/index`
  - `44'` — BIP44 purpose
  - `1237'` — Nostr coin type
  - `0'` — account 0
  - `0` — external chain
  - `index` — non-hardened address index starting at 0
- Formats:
  - Private/Public keys are output in hex.
  - NIP-19 encodings (`nsec` for private, `npub` for public) are produced with `go-nostr`.

## Prerequisites

- Go 1.20+ installed

## How to Run

From the repository root (`higher/`):

```bash
# Run directly
go run ./examples/keyderivation

# Or build and run
go build -o key-derivation ./examples/keyderivation
./key-derivation
```

By default, the example generates a fresh 12-word BIP-39 mnemonic on each run and derives keys from it. The generated mnemonic is printed at the top of the output so you can save/reuse it.

## CLI Usage and Flags

The example now supports flags so you don't have to edit code to supply inputs:

- `--mnemonic` — Provide a BIP-39 mnemonic directly.
- `--mnemonic-file` — Path to a file containing a BIP-39 mnemonic (one line).
- `--start` — Start index for derivation (default: 0).
- `--count` — Number of keys to derive with BIP32 (default: 3).
- `--simple-count` — Number of keys to derive with the simple HMAC method (default: 2).
- `--check-max` — Max index to scan when checking key ownership (default: 100).
- `--event-index` — Index used to sign the sample Nostr event (default: 0).
 - `--message` — Content for the sample Nostr event (default: "Hello Nostr! This message was signed with a derived key.")

Examples:

```bash
# Run with a specific mnemonic string
go run ./examples/keyderivation \
  --mnemonic "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about" \
  --start 0 --count 5 --simple-count 2 --check-max 200 --event-index 0 \
  --message "Signing from index 0 with my custom message"

# Run with a mnemonic from a file
echo "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about" > /tmp/m.txt
go run ./examples/keyderivation --mnemonic-file /tmp/m.txt --count 4

# Build then run with flags
go build -o key-derivation ./examples/keyderivation
./key-derivation --count 10 --simple-count 3
```

## What You’ll See

The program walks through five steps:

1. Create a deriver from a BIP-39 mnemonic (auto-generated if empty).
2. Show the master/root keypair derived from the mnemonic/seed.
3. Derive several child keys using BIP32 and print them (hex and `nsec`/`npub`).
4. Derive a couple of keys using the simple HMAC method and print them (`nsec`/`npub`).
5. Verify whether a target key belongs to the same master using both hex and `npub` inputs.
6. Create and sign a simple Nostr text note with a derived key, then verify its signature.

Example output sections include:

- The 12-word mnemonic used
- Master keypair (hex + NIP-19)
- Derived child keys at indices 0..N
- Key-ownership checks like:
  - `✅ Hex key abcd... found at index X: true`
  - `✅ NIP-19 key npub1... found at index X: true`
- A signed event with fields `ID`, `PubKey`, `Content`, `Sig`, and signature verification status

## Use Your Own Mnemonic

You can pass a mnemonic without editing code by using flags. Either pass it directly:

```bash
go run ./examples/keyderivation --mnemonic "<your 12 or 24 words>"
```

Or place it in a file and reference it:

```bash
go run ./examples/keyderivation --mnemonic-file /path/to/mnemonic.txt
```

## Safety Notes

- Private keys are printed to stdout for demonstration. Do not use this example output in production.
- Always keep your mnemonic and private keys secret. Anyone who knows them can sign as you.
- The simple HMAC derivation is for educational comparison only. For real applications, use the BIP32/BIP44 method.

## Relevant Files

- `examples/keyderivation/main.go` — the runnable example
- `keyderivation/hdkey.go` — implementation of derivation and helpers

## Troubleshooting

- If you see "invalid mnemonic" after editing the code, ensure your mnemonic is valid BIP-39 words and spacing.
- If `go run` fails, ensure you’re running from the repo root and that modules are downloaded: `go mod download`.
