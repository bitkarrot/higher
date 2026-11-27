package main

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestIsValidSHA256Hex(t *testing.T) {
	valid := strings.Repeat("a", 64)
	if !isValidSHA256Hex(valid) {
		t.Errorf("expected 64 lowercase hex chars to be valid")
	}
	if !isValidSHA256Hex(strings.Repeat("AB09", 16)) {
		t.Errorf("expected uppercase hex chars to be valid")
	}
	invalid := []string{
		"",
		strings.Repeat("a", 63),
		strings.Repeat("a", 65),
		strings.Repeat("../", 21) + "/", // 64-char traversal attempt
		strings.Repeat("g", 64),         // not hex
		"../../../../etc/passwd",
	}
	for _, s := range invalid {
		if isValidSHA256Hex(s) {
			t.Errorf("expected %q to be invalid", s)
		}
	}
}

func TestIsPublicIP(t *testing.T) {
	cases := []struct {
		ip     string
		public bool
	}{
		{"8.8.8.8", true},
		{"1.1.1.1", true},
		{"2606:4700:4700::1111", true},
		{"127.0.0.1", false},
		{"10.0.0.1", false},
		{"172.16.0.1", false},
		{"192.168.1.1", false},
		{"169.254.169.254", false}, // cloud metadata, link-local
		{"100.64.0.1", false},      // CGNAT
		{"100.127.255.254", false}, // CGNAT end of range
		{"0.0.0.0", false},
		{"224.0.0.1", false}, // multicast
		{"::1", false},       // v6 loopback
		{"fd00::1", false},   // v6 unique local
		{"fe80::1", false},   // v6 link-local
	}
	for _, c := range cases {
		ip := parseIP(t, c.ip)
		if got := isPublicIP(ip); got != c.public {
			t.Errorf("isPublicIP(%s) = %v, want %v", c.ip, got, c.public)
		}
	}
}

func parseIP(t *testing.T, s string) net.IP {
	t.Helper()
	ip := net.ParseIP(s)
	if ip == nil {
		t.Fatalf("test bug: cannot parse IP %q", s)
	}
	return ip
}

func TestPublicIPDialContextRejectsNonPublic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, addr := range []string{"127.0.0.1:80", "10.0.0.1:80", "169.254.169.254:80", "[::1]:80"} {
		_, err := publicIPDialContext(ctx, "tcp", addr)
		if err == nil {
			t.Errorf("expected dialing %s to be rejected", addr)
			continue
		}
		if !strings.Contains(err.Error(), "non-public") {
			t.Errorf("dialing %s: expected non-public rejection, got: %v", addr, err)
		}
	}
}
