# Tests and Utilities in `tests/`

This directory contains:

- `relay_events_test.go` — integration test that verifies access control for master-derived keys vs. random keys.
- `gen_keys.go` — a small helper program to derive and print 5 keys from `RELAY_MNEMONIC` in your `.env`.

## Run the integration test

From the project root (`/higher`):

```bash
go test ./tests -v
```

What it does:

- Spawns the relay via `go run .` (no need to start the relay yourself).
- Connects to `ws://localhost:3334`.
- Uses a freshly generated mnemonic for the relay and derives keys on the client side to publish kind-1 events.
- Verifies that derived keys are accepted and random keys are rejected when `TEAM_DOMAIN` is set.

Requirements:

- Ensure no other process is already bound to port `3334`.
- Go toolchain installed.

## Run the key generator (`gen_keys.go`)

This helper prints 5 keypairs (indices `0..4`) using the same BIP32 path as the app: `m/44'/1237'/0'/0/index`.

1) Set `RELAY_MNEMONIC` in your project `.env` file at the repo root (same file used by the app).

2) From the project root, run:

```bash
go run -tags genkeys ./tests/gen_keys.go
```

Notes:

- The file has a build tag to avoid conflicting with the test package (`package tests`). That’s why `-tags genkeys` is required.
- Output includes public/private in hex and NIP-19 encodings (npub/nsec).
