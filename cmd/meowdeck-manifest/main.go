package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MeowKJ/openwrt-meowdeck/internal/updater"
)

func main() {
	directory := flag.String("dist", "dist", "release asset directory")
	channel := flag.String("channel", "", "edge or stable")
	version := flag.String("version", "", "release version")
	sequence := flag.Uint64("sequence", 0, "monotonic release sequence")
	keyID := flag.String("key-id", "", "signing key identifier")
	baseURL := flag.String("base-url", "", "release asset base URL")
	flag.Parse()
	if *channel == "" || *version == "" || *sequence == 0 || *keyID == "" || *baseURL == "" {
		fatalf("channel, version, sequence, key-id, and base-url are required")
	}
	privateKey, err := signingKey(os.Getenv("MEOWDECK_SIGNING_PRIVATE_KEY"))
	if err != nil {
		fatalf("%v", err)
	}
	artifacts := make(map[string]updater.Artifact)
	entries, err := filepath.Glob(filepath.Join(*directory, "meowdeck-linux-*.tar.gz"))
	if err != nil {
		fatalf("list artifacts: %v", err)
	}
	for _, entry := range entries {
		raw, err := os.ReadFile(entry)
		if err != nil {
			fatalf("read %s: %v", entry, err)
		}
		name := filepath.Base(entry)
		arch := strings.TrimSuffix(strings.TrimPrefix(name, "meowdeck-"), ".tar.gz")
		digest := sha256.Sum256(raw)
		artifacts[arch] = updater.Artifact{
			URL:    strings.TrimRight(*baseURL, "/") + "/" + name,
			SHA256: hex.EncodeToString(digest[:]), Size: int64(len(raw)),
		}
	}
	if len(artifacts) == 0 {
		fatalf("no release archives found in %s", *directory)
	}
	manifest := updater.Manifest{
		Schema: updater.ManifestSchema, Channel: *channel, Version: *version, Sequence: *sequence,
		CreatedAt: time.Now().UTC(), MinUpdaterVersion: updater.UpdaterVersion, KeyID: *keyID, Artifacts: artifacts,
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		fatalf("encode manifest: %v", err)
	}
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, raw))
	if err := os.WriteFile(filepath.Join(*directory, "manifest.json"), raw, 0o644); err != nil {
		fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(*directory, "manifest.json.sig"), []byte(signature+"\n"), 0o644); err != nil {
		fatalf("write manifest signature: %v", err)
	}
}

func signingKey(encoded string) (ed25519.PrivateKey, error) {
	if encoded == "" {
		return nil, fmt.Errorf("MEOWDECK_SIGNING_PRIVATE_KEY is required")
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode signing key: %w", err)
	}
	switch len(raw) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(raw), nil
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(raw), nil
	default:
		return nil, fmt.Errorf("signing key must contain a %d-byte seed or %d-byte private key", ed25519.SeedSize, ed25519.PrivateKeySize)
	}
}

func fatalf(format string, arguments ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "meowdeck-manifest: "+format+"\n", arguments...)
	os.Exit(1)
}
