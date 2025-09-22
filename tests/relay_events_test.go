package tests

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/bitkarrot/higher/keyderivation"
	"github.com/nbd-wtf/go-nostr"
)

// waitForRelay tries to connect to the relay until it becomes available or timeout elapses.
func waitForRelay(t *testing.T, url string, timeout time.Duration) {
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
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("relay at %s did not become ready within %s", url, timeout)
}

func TestAccessControl_MasterAndTeam(t *testing.T) {
	// Generate a mnemonic we'll use both for starting the relay and deriving keys locally
	der, err := keyderivation.NewNostrKeyDeriver("")
	if err != nil {
		t.Fatalf("failed to create deriver: %v", err)
	}
	mnemonic := der.GetMnemonic()

	// Prepare environment for relay subprocess
	env := os.Environ()
	env = append(env,
		"DB_ENGINE=badger",
		"BLOSSOM_ENABLED=false",
		// Set a non-empty TEAM_DOMAIN that won't load any team members so non-derived keys are rejected
		"TEAM_DOMAIN=test.invalid",
		"RELAY_MNEMONIC="+mnemonic,
		"MAX_DERIVATION_INDEX=10",
		// Minimal required relays settings
		"RELAY_NAME=TestRelay",
		"RELAY_PUBKEY=00",
		"RELAY_DESCRIPTION=Test Relay",
	)

	// Launch relay: go run . from project root
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = filepath.Clean("..") // project root relative to tests/
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start relay subprocess: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	// Wait for relay to be ready
	relayURL := "ws://localhost:3334"
	waitForRelay(t, relayURL, 10*time.Second)

	// Connect a client once to reuse for publishes
	ctx := context.Background()
	rel, err := nostr.RelayConnect(ctx, relayURL)
	if err != nil {
		t.Fatalf("failed to connect to relay: %v", err)
	}
	defer rel.Close()

	// Helper to create and sign a kind 1 event
	createEvent := func(privHex, content string) (*nostr.Event, error) {
		e := nostr.Event{
			Kind:      nostr.KindTextNote,
			Content:   content,
			CreatedAt: nostr.Now(),
		}
		if err := e.Sign(privHex); err != nil {
			return nil, err
		}
		return &e, nil
	}

	// 1) Master-derived keys should be accepted
	for i := uint32(0); i < 3; i++ {
		kp, err := der.DeriveKeyBIP32(i)
		if err != nil {
			t.Fatalf("failed to derive key %d: %v", i, err)
		}
		evt, err := createEvent(kp.PrivateKey, fmt.Sprintf("hello from derived index %d at %d", i, time.Now().UnixNano()))
		if err != nil {
			t.Fatalf("failed to create event: %v", err)
		}
		if err := rel.Publish(ctx, *evt); err != nil {
			t.Fatalf("derived index %d publish error: %v", i, err)
		} else {
			t.Logf("derived index %d (npub=%s) \n - publish OK, event id=%s", i, kp.PublicKeyNIP, evt.ID)
		}
	}

	// 2) Random keys should be rejected when TEAM_DOMAIN is set (and empty team list)
	for i := 0; i < 3; i++ {
		priv := nostr.GeneratePrivateKey()
		evt, err := createEvent(priv, "random key attempt")
		if err != nil {
			t.Fatalf("failed to create event with random key: %v", err)
		}
		if err := rel.Publish(ctx, *evt); err == nil {
			t.Fatalf("expected random key publish to be rejected, but got no error")
		} else {
			t.Logf("random key publish rejected as expected: %v", err)
		}
	}
}
