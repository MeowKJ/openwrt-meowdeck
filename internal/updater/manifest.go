package updater

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	ManifestSchema = 1
	UpdaterVersion = "1.0.0"
)

// trustedKeysBase64 is replaced at release build time using -ldflags -X. Its
// decoded value is a JSON object mapping key IDs to base64 Ed25519 public keys.
var trustedKeysBase64 = "e30=" // {}

type Artifact struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type Manifest struct {
	Schema            int                 `json:"schema"`
	Channel           string              `json:"channel"`
	Version           string              `json:"version"`
	Sequence          uint64              `json:"sequence"`
	CreatedAt         time.Time           `json:"created_at"`
	MinUpdaterVersion string              `json:"min_updater_version"`
	KeyID             string              `json:"key_id"`
	Artifacts         map[string]Artifact `json:"artifacts"`
}

func EmbeddedTrustedKeys() (map[string]ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(trustedKeysBase64)
	if err != nil {
		return nil, fmt.Errorf("decode embedded trusted keys: %w", err)
	}
	var encoded map[string]string
	if err := json.Unmarshal(raw, &encoded); err != nil {
		return nil, fmt.Errorf("parse embedded trusted keys: %w", err)
	}
	keys := make(map[string]ed25519.PublicKey, len(encoded))
	for id, value := range encoded {
		key, err := base64.StdEncoding.DecodeString(value)
		if err != nil || len(key) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("invalid trusted key %q", id)
		}
		keys[id] = ed25519.PublicKey(key)
	}
	return keys, nil
}

func VerifyManifest(raw, encodedSignature []byte, channel string, lastSequence uint64, keys map[string]ed25519.PublicKey) (Manifest, error) {
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	if manifest.Schema != ManifestSchema {
		return Manifest{}, fmt.Errorf("unsupported manifest schema %d", manifest.Schema)
	}
	if manifest.Channel != channel {
		return Manifest{}, fmt.Errorf("manifest channel %q does not match %q", manifest.Channel, channel)
	}
	if manifest.Sequence <= lastSequence {
		return Manifest{}, fmt.Errorf("manifest sequence %d is not newer than %d", manifest.Sequence, lastSequence)
	}
	if manifest.Version == "" || manifest.Sequence == 0 || manifest.CreatedAt.IsZero() {
		return Manifest{}, fmt.Errorf("manifest identity fields are incomplete")
	}
	if len(manifest.Artifacts) == 0 {
		return Manifest{}, fmt.Errorf("manifest contains no artifacts")
	}
	for name, artifact := range manifest.Artifacts {
		parsed, err := url.Parse(artifact.URL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return Manifest{}, fmt.Errorf("artifact %q has an invalid HTTPS URL", name)
		}
		digest, err := hex.DecodeString(artifact.SHA256)
		if err != nil || len(digest) != sha256.Size {
			return Manifest{}, fmt.Errorf("artifact %q has an invalid SHA-256", name)
		}
		if artifact.Size <= 0 || artifact.Size > maxArtifactBytes {
			return Manifest{}, fmt.Errorf("artifact %q has an invalid size", name)
		}
	}
	if compareSemver(UpdaterVersion, manifest.MinUpdaterVersion) < 0 {
		return Manifest{}, fmt.Errorf("updater %s is older than required %s", UpdaterVersion, manifest.MinUpdaterVersion)
	}
	key, ok := keys[manifest.KeyID]
	if !ok {
		return Manifest{}, fmt.Errorf("manifest key %q is not trusted", manifest.KeyID)
	}
	signature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encodedSignature)))
	if err != nil {
		return Manifest{}, fmt.Errorf("decode manifest signature: %w", err)
	}
	if !ed25519.Verify(key, raw, signature) {
		return Manifest{}, fmt.Errorf("manifest signature is invalid")
	}
	return manifest, nil
}

func RuntimeArtifactKey() string {
	arch := runtime.GOARCH
	if arch == "arm" {
		arch = "armv7"
	}
	return "linux-" + arch
}

func compareSemver(left, right string) int {
	parse := func(value string) [3]int {
		var result [3]int
		parts := strings.SplitN(strings.TrimPrefix(value, "v"), ".", 4)
		for index := 0; index < len(parts) && index < 3; index++ {
			digits := parts[index]
			if cut := strings.IndexFunc(digits, func(r rune) bool { return r < '0' || r > '9' }); cut >= 0 {
				digits = digits[:cut]
			}
			result[index], _ = strconv.Atoi(digits)
		}
		return result
	}
	a, b := parse(left), parse(right)
	for index := range a {
		if a[index] < b[index] {
			return -1
		}
		if a[index] > b[index] {
			return 1
		}
	}
	return 0
}
