package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"testing"
)

func TestSigningKeyAcceptsSeedAndRejectsInvalidLength(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	key, err := signingKey(base64.StdEncoding.EncodeToString(seed))
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != ed25519.PrivateKeySize {
		t.Fatalf("unexpected private key size %d", len(key))
	}
	if _, err := signingKey(base64.StdEncoding.EncodeToString([]byte("short"))); err == nil {
		t.Fatal("expected invalid key length to be rejected")
	}
}
