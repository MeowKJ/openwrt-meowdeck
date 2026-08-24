package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const maxArtifactBytes = 16 << 20

type State struct {
	Channel              string    `json:"channel"`
	LastAcceptedSequence uint64    `json:"last_accepted_sequence"`
	Version              string    `json:"version"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type Options struct {
	Channel       string
	ManifestURL   string
	StatePath     string
	InstallPath   string
	ConfigPath    string
	ServiceScript string
	HealthURL     string
	VersionURL    string
	TrustedKeys   map[string]ed25519.PublicKey
	HTTPClient    *http.Client
	ArtifactKey   string
}

func DefaultOptions(channel string) Options {
	releasePath := channel
	if channel == "stable" {
		releasePath = "latest"
	}
	return Options{
		Channel:     channel,
		ManifestURL: "https://github.com/MeowKJ/openwrt-meowdeck/releases/" + releasePath + "/download/manifest.json",
		StatePath:   "/etc/meowdeck/update-state.json", InstallPath: "/usr/bin/meowdeck",
		ConfigPath: "/etc/meowdeck/config.json", ServiceScript: "/etc/init.d/meowdeck",
		HealthURL: "http://127.0.0.1:9080/healthz", VersionURL: "http://127.0.0.1:9080/api/v1/version",
		ArtifactKey: RuntimeArtifactKey(),
	}
}

func Run(ctx context.Context, options Options) (string, error) {
	if options.Channel != "edge" && options.Channel != "stable" {
		return "", fmt.Errorf("unsupported update channel %q", options.Channel)
	}
	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{Timeout: 5 * time.Minute}
	}
	if len(options.TrustedKeys) == 0 {
		keys, err := EmbeddedTrustedKeys()
		if err != nil {
			return "", err
		}
		options.TrustedKeys = keys
	}
	if len(options.TrustedKeys) == 0 {
		return "", fmt.Errorf("no trusted update keys are embedded")
	}
	state, err := loadState(options.StatePath)
	if err != nil {
		return "", err
	}
	if state.Channel != "" && state.Channel != options.Channel {
		state = State{Channel: options.Channel}
	}
	manifestRaw, err := fetch(ctx, options.HTTPClient, options.ManifestURL, 1<<20)
	if err != nil {
		return "", fmt.Errorf("download manifest: %w", err)
	}
	signatureRaw, err := fetch(ctx, options.HTTPClient, options.ManifestURL+".sig", 4096)
	if err != nil {
		return "", fmt.Errorf("download manifest signature: %w", err)
	}
	manifest, err := VerifyManifest(manifestRaw, signatureRaw, options.Channel, state.LastAcceptedSequence, options.TrustedKeys)
	if err != nil {
		return "", err
	}
	artifact, ok := manifest.Artifacts[options.ArtifactKey]
	if !ok {
		return "", fmt.Errorf("manifest does not contain %s", options.ArtifactKey)
	}
	if artifact.Size <= 0 || artifact.Size > maxArtifactBytes {
		return "", fmt.Errorf("artifact size %d is outside the accepted range", artifact.Size)
	}
	archive, err := fetch(ctx, options.HTTPClient, artifact.URL, maxArtifactBytes)
	if err != nil {
		return "", fmt.Errorf("download artifact: %w", err)
	}
	if int64(len(archive)) != artifact.Size {
		return "", fmt.Errorf("artifact size mismatch: got %d want %d", len(archive), artifact.Size)
	}
	digest := sha256.Sum256(archive)
	if hex.EncodeToString(digest[:]) != strings.ToLower(artifact.SHA256) {
		return "", fmt.Errorf("artifact SHA-256 mismatch")
	}
	workDirectory, err := os.MkdirTemp("/tmp", "meowdeck-update-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(workDirectory)
	candidate := filepath.Join(workDirectory, "meowdeck.download")
	if err := extractBinary(archive, candidate); err != nil {
		return "", err
	}
	if err := exec.CommandContext(ctx, candidate, "self-test", "-config", options.ConfigPath).Run(); err != nil {
		return "", fmt.Errorf("candidate self-test failed: %w", err)
	}
	if err := installAndVerify(ctx, options, candidate, manifest.Version); err != nil {
		return "", err
	}
	nextState := State{Channel: options.Channel, LastAcceptedSequence: manifest.Sequence, Version: manifest.Version, UpdatedAt: time.Now().UTC()}
	if err := writeJSONAtomic(options.StatePath, nextState, 0o600); err != nil {
		return "", fmt.Errorf("save accepted sequence: %w", err)
	}
	return manifest.Version, nil
}

func fetch(ctx context.Context, client *http.Client, address string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status %s", response.Status)
	}
	reader := io.LimitReader(response.Body, limit+1)
	raw, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, fmt.Errorf("response exceeds %d bytes", limit)
	}
	return raw, nil
}

func extractBinary(archive []byte, target string) error {
	gzipReader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return fmt.Errorf("open release archive: %w", err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read release archive: %w", err)
		}
		if header.Typeflag != tar.TypeReg || filepath.Base(header.Name) != "meowdeck" {
			continue
		}
		if header.Size <= 0 || header.Size > maxArtifactBytes {
			return fmt.Errorf("binary size %d is outside the accepted range", header.Size)
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
		if err != nil {
			return err
		}
		_, copyErr := io.CopyN(output, tarReader, header.Size)
		syncErr := output.Sync()
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if syncErr != nil {
			return syncErr
		}
		return closeErr
	}
	return fmt.Errorf("release archive does not contain meowdeck")
}

func installAndVerify(ctx context.Context, options Options, candidate, expectedVersion string) error {
	directory := filepath.Dir(options.InstallPath)
	newPath := filepath.Join(directory, ".meowdeck.new")
	rollbackPath := filepath.Join(directory, ".meowdeck.rollback")
	previousPath := filepath.Join(directory, "meowdeck.previous")
	_ = os.Remove(newPath)
	_ = os.Remove(rollbackPath)
	if err := copySynced(candidate, newPath, 0o755); err != nil {
		return fmt.Errorf("stage update: %w", err)
	}
	if err := copySynced(options.InstallPath, rollbackPath, 0o755); err != nil {
		return fmt.Errorf("stage rollback binary: %w", err)
	}
	if err := os.Rename(newPath, options.InstallPath); err != nil {
		return fmt.Errorf("activate update: %w", err)
	}
	if err := syncDirectory(directory); err != nil {
		return fmt.Errorf("sync installation directory: %w", err)
	}
	rollback := func(cause error) error {
		_ = os.Rename(rollbackPath, options.InstallPath)
		_ = syncDirectory(directory)
		_ = exec.CommandContext(context.Background(), options.ServiceScript, "restart").Run()
		return cause
	}
	if err := exec.CommandContext(ctx, options.ServiceScript, "restart").Run(); err != nil {
		return rollback(fmt.Errorf("restart updated service: %w", err))
	}
	if err := waitForVersion(ctx, options, expectedVersion); err != nil {
		return rollback(err)
	}
	if err := os.Rename(rollbackPath, previousPath); err != nil {
		return fmt.Errorf("retain last-known-good binary: %w", err)
	}
	return syncDirectory(directory)
}

func waitForVersion(ctx context.Context, options Options, expected string) error {
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		health, healthErr := fetch(ctx, options.HTTPClient, options.HealthURL, 1024)
		versionRaw, versionErr := fetch(ctx, options.HTTPClient, options.VersionURL, 4096)
		if healthErr == nil && strings.TrimSpace(string(health)) == "ok" && versionErr == nil {
			var response struct {
				Version string `json:"version"`
			}
			if json.Unmarshal(versionRaw, &response) == nil && response.Version == expected {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return fmt.Errorf("updated service did not report version %s", expected)
}

func loadState(path string) (State, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return State{}, nil
	}
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(raw, &state); err != nil {
		return State{}, fmt.Errorf("decode update state: %w", err)
	}
	return state, nil
}

func writeJSONAtomic(path string, value any, mode os.FileMode) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".update-state-*.new")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(raw); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	return syncDirectory(directory)
}

func copySynced(source, target string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return err
	}
	if err := output.Sync(); err != nil {
		_ = output.Close()
		return err
	}
	return output.Close()
}

func syncDirectory(directory string) error {
	handle, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer handle.Close()
	return handle.Sync()
}
