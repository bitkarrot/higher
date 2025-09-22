package tests

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/bitkarrot/higher/keyderivation"
	"github.com/nbd-wtf/go-nostr"
)

// createNIP98AuthHeader builds a NIP-98 Authorization header using the given private key.
// The 'u' tag should include the absolute URL per NIP-98 recommendations.
func createNIP98AuthHeader(method, absoluteURL, privHex string) (string, error) {
	e := nostr.Event{
		Kind:      27235, // NIP-98 HTTP Auth
		CreatedAt: nostr.Now(),
		Tags: nostr.Tags{
			{"method", method},
			{"u", absoluteURL},
			{"nonce", fmt.Sprintf("%d", time.Now().UnixNano())},
		},
	}
	if err := e.Sign(privHex); err != nil {
		return "", err
	}
	b, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	return "Nostr " + base64.StdEncoding.EncodeToString(b), nil
}

// waitPortClosedWS waits until no relay is accepting WS connections at the given URL.
func waitPortClosedWS(t *testing.T, url string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		rel, err := nostr.RelayConnect(ctx, url)
		cancel()
		if err != nil {
			return // cannot connect -> port is closed
		}
		rel.Close()
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("relay still responding at %s after %s", url, timeout)
}

// startRelay starts `go run .` in the project root with provided environment variables.
func startRelay(t *testing.T, extraEnv []string) *exec.Cmd {
	t.Helper()
	env := os.Environ()
	env = append(env, extraEnv...)
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = filepath.Clean("..")
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start relay subprocess: %v", err)
	}
	return cmd
}

func waitReadyWS(t *testing.T, url string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		rel, err := nostr.RelayConnect(ctx, url)
		cancel()
		if err == nil {
			rel.Close()
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("relay not ready at %s after %s", url, timeout)
}

func TestBlossomUpload_MasterDerivedVsRandom(t *testing.T) {
	// Create temp blossom dir
	tmpDir := t.TempDir() + string(os.PathSeparator)

	// Create deriver and use its mnemonic for relay
	der, err := keyderivation.NewNostrKeyDeriver("")
	if err != nil {
		t.Fatalf("deriver: %v", err)
	}
	mn := der.GetMnemonic()

	env := []string{
		"DB_ENGINE=badger",
		"BLOSSOM_ENABLED=true",
		"BLOSSOM_PATH=" + tmpDir,
		"BLOSSOM_URL=http://localhost:3334",
		"TEAM_DOMAIN=test.invalid", // enforce membership checks for non-derived keys
		"RELAY_MNEMONIC=" + mn,
		"MAX_DERIVATION_INDEX=10",
		"RELAY_NAME=TestRelay",
		"RELAY_PUBKEY=00",
		"RELAY_DESCRIPTION=Test Relay",
	}
	// Ensure previous relay on fixed port is fully closed
	waitPortClosedWS(t, "ws://localhost:3334", 5*time.Second)

	cmd := startRelay(t, env)
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	waitReadyWS(t, "ws://localhost:3334", 10*time.Second)

	client := &http.Client{Timeout: 10 * time.Second}
	blob := make([]byte, 64)
	_, _ = rand.Read(blob)

	// Derived key attempt should succeed
	kp, err := der.DeriveKeyBIP32(0)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	auth, err := createNIP98AuthHeader("PUT", "http://localhost:3334/upload", kp.PrivateKey)
	if err != nil {
		t.Fatalf("auth: %v", err)
	}
	req, _ := http.NewRequest("PUT", "http://localhost:3334/upload", bytes.NewReader(blob))
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("derived upload http error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected success for derived upload, got %d: %s", resp.StatusCode, string(b))
	}

	// Random key attempt should fail (team domain set and not derived)
	randomPriv := nostr.GeneratePrivateKey()
	auth2, err := createNIP98AuthHeader("PUT", "http://localhost:3334/upload", randomPriv)
	if err != nil {
		t.Fatalf("auth2: %v", err)
	}
	req2, _ := http.NewRequest("PUT", "http://localhost:3334/upload", bytes.NewReader(blob))
	req2.Header.Set("Authorization", auth2)
	req2.Header.Set("Content-Type", "application/octet-stream")
	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatalf("random upload http error: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode == 200 || resp2.StatusCode == 201 {
		b, _ := io.ReadAll(resp2.Body)
		t.Fatalf("expected failure for random upload, got %d: %s", resp2.StatusCode, string(b))
	}
}

func TestReadsRestricted_MasterOnly(t *testing.T) {
	// Start relay with read restriction
	der, err := keyderivation.NewNostrKeyDeriver("")
	if err != nil {
		t.Fatalf("deriver: %v", err)
	}
	mn := der.GetMnemonic()

	env := []string{
		"DB_ENGINE=badger",
		"BLOSSOM_ENABLED=false",
		"READS_RESTRICTED=true",
		"TEAM_DOMAIN=", // irrelevant for reads restriction
		"RELAY_MNEMONIC=" + mn,
		"MAX_DERIVATION_INDEX=10",
		"RELAY_NAME=TestRelay",
		"RELAY_PUBKEY=00",
		"RELAY_DESCRIPTION=Test Relay",
	}
	cmd := startRelay(t, env)
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	waitReadyWS(t, "ws://localhost:3334", 10*time.Second)

	ctx := context.Background()
	rel, err := nostr.RelayConnect(ctx, "ws://localhost:3334")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer rel.Close()

	// Publish one event from a derived key
	kp, err := der.DeriveKeyBIP32(0)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	evt := nostr.Event{Kind: nostr.KindTextNote, Content: "hello", CreatedAt: nostr.Now()}
	if err := evt.Sign(kp.PrivateKey); err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := rel.Publish(ctx, evt); err != nil {
		t.Fatalf("publish derived: %v", err)
	}

	// Query for derived author -> should succeed
	evts, err := rel.QuerySync(ctx, nostr.Filter{Authors: []string{kp.PublicKey}})
	if err != nil {
		t.Fatalf("query derived author error: %v", err)
	}
	if len(evts) == 0 {
		t.Fatalf("expected at least 1 event for derived author, got 0")
	}

	// Query for random author -> should be rejected (reads restricted)
	rpriv := nostr.GeneratePrivateKey()
	rpub, _ := nostr.GetPublicKey(rpriv)
	_, err = rel.QuerySync(ctx, nostr.Filter{Authors: []string{rpub}})
	if err == nil {
		t.Fatalf("expected query with non-derived author to be rejected, but got no error")
	}
}
