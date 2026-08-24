package updater

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestVerifyManifestRejectsRollbackAndWrongChannel(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{
		Schema: 1, Channel: "edge", Version: "edge-test", Sequence: 42,
		CreatedAt: time.Now().UTC(), MinUpdaterVersion: "1.0.0", KeyID: "test",
		Artifacts: map[string]Artifact{"linux-arm64": {URL: "https://example.invalid/a", SHA256: strings.Repeat("0", 64), Size: 1}},
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	signature := []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, raw)))
	keys := map[string]ed25519.PublicKey{"test": publicKey}
	if _, err := VerifyManifest(raw, signature, "edge", 41, keys); err != nil {
		t.Fatalf("expected valid manifest: %v", err)
	}
	if _, err := VerifyManifest(raw, signature, "edge", 42, keys); err == nil {
		t.Fatal("expected rollback sequence to be rejected")
	}
	if _, err := VerifyManifest(raw, signature, "stable", 0, keys); err == nil {
		t.Fatal("expected channel mismatch to be rejected")
	}
}

func TestVerifyManifestRejectsTampering(t *testing.T) {
	publicKey, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	raw := []byte(`{"schema":1,"channel":"edge","version":"one","sequence":1,"created_at":"2026-08-17T00:00:00Z","min_updater_version":"1.0.0","key_id":"test","artifacts":{}}`)
	signature := []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, raw)))
	tampered := append([]byte(nil), raw...)
	tampered[len(tampered)-2] = ' '
	if _, err := VerifyManifest(tampered, signature, "edge", 0, map[string]ed25519.PublicKey{"test": publicKey}); err == nil {
		t.Fatal("expected tampered manifest to be rejected")
	}
}
