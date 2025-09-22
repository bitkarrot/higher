# Higher: Nostr Relay for Hierarchical determinstic keys

This relay software specializes in providing a Nostr relay with access to keys derived from a master key. Any keys which are not derived from the master key will be rejected for write events. Optional: Allow nostr.json from a domain to have read/write access to the relay. This Relay is based on the khatru library framework.

In the .env file, the pubkey in the nostr.json located at the domain specified in the TEAM_DOMAIN variable is used to reject events from pubkeys that are not derived from the master pubkey.

Additional features we added for production use:
- Blossom
   - added read and write timeouts
   - prevent slow header attacks, max header size
   - max size upload
   - added /mirror endpoint to allow for syncing content with other relays
   - added /list endpoint to allow for listing content for a specific user
- Relay Kinds - add support to limit kinds allowed, kinds specified in .env file
- Frontend
   - added front page with relay and blossom information


## Hierarchical Deterministic (HD) Keys in Higher Relay

This relay implements Hierarchical Deterministic (HD) key authorization using BIP-32/BIP-44-style derivation. It validates incoming keys for event write/read and Blossom uploads against a configured HD master.

**Key implementation files**
- `keyderivation/hdkey.go`
- `main.go` (authorization logic in `RejectEvent`, `RejectFilter`, and Blossom `RejectUpload`)

**Master key configuration**
- Exactly one of the following must be set in `.env` (validated in `LoadConfig()`):
  - `RELAY_MNEMONIC` — BIP-39 mnemonic
  - `RELAY_SEED_HEX` — hex-encoded 32-byte seed
- The relay initializes the HD master in `initDeriver()` and keeps the deriver in a global `deriver` for access checks.

**Derivation scheme**
- Nostr BIP44 coin type `1237`, path: `m/44'/1237'/0'/0/index`
  - `44'` — BIP44 purpose
  - `1237'` — Nostr coin type
  - `0'` — account 0
  - `0` — external chain
  - `index` — address index (non-hardened), starting at 0

**Implemented in `keyderivation/hdkey.go`**
- `NewNostrKeyDeriver(...)` — builds a deriver from mnemonic or seed
- `DeriveKeyBIP32(index)` — derives a key pair at the path above
- `GetMasterKeyPair()` — returns the root (master) key

### See more at 
- [HD Keys Implementation](./HD_KEYS.md)
- [Access Control Flow](./ACCESS_CONTROL.md)

## Table of Contents

- [Documentation](#documentation)
- [Prerequisites](#prerequisites)
- [Setting Environment Variables](#setting-environment-variables)
- [Running Docker](#running-docker)
- [Installing Go](#installing-go)
- [Compiling the Application](#compiling-the-application)
- [Running the Application as a Service](#running-the-application-as-a-service)

## Prerequisites

- A Linux-based operating system
- Go installed on your system
- A Webserver (like nginx) if blossom is enabled

## Setting Environment Variables

1.  Create a `.env` file in the root directory of your project.

2.  Add your environment variables to the `.env` file. For example:

    ```env

    RELAY_NAME="Higher"
    RELAY_PUBKEY="72e2d6ea......."
    RELAY_DESCRIPTION="Nostr Relay for Hierarchical determinstic keys"

    DB_ENGINE="lmdb" # lmdb, badger, postgres
    DB_PATH="db/" # only needed for lmdb, badger

   # only needed for postgres
    POSTGRES_USER=higher
    POSTGRES_PASSWORD=password
    POSTGRES_DB=relay
    POSTGRES_HOST=localhost
    POSTGRES_PORT=5437

    TEAM_DOMAIN="higher.bitkarrot.co"
    BLOSSOM_ENABLED="true"
    BLOSSOM_PATH="blossom/"
    BLOSSOM_URL="http://localhost:3334"

    ```

## Compiling the Application

1. Clone the repository:

   ```bash
   git clone https://github.com/bitkarrot/higher.git
   cd higher
   ```

2. Build the application:

   ```bash
   go build -o higher-relay
   ```

## Running the Application as a Service

1. Create a systemd service file:

   ```bash
   sudo nano /etc/systemd/system/higher-relay.service
   ```

2. Add the following content to the service file: (update paths and usernames as needed)

   ```ini
   [Unit]
   Description=Higher Relay
   After=network.target

   [Service]
   ExecStart=/path/to/yourappname
   WorkingDirectory=/path/to/higher-relay
   EnvironmentFile=/path/to/higher-relay/.env
   Restart=always
   User=ubuntu

   [Install]
   WantedBy=multi-user.target
   ```

3. Reload the systemd daemon:

   ```bash
   sudo systemctl daemon-reload
   ```

4. Enable and start the service:

   ```bash
   sudo systemctl enable higher-relay
   sudo systemctl start higher-relay
   ```

5. Check the status of the service:

   ```bash
   sudo systemctl status higher-relay
   ```

## Conclusion

Your relay will be running at localhost:3334. Feel free to serve it with nginx or any other reverse proxy.
