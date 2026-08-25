package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type backendReleaseFixture struct {
	Manifest      BackendManifest
	ManifestBytes []byte
	Signature     string
	Binary        []byte
	PublicKey     ed25519.PublicKey
}

func newBackendReleaseFixture(t *testing.T, version, commit string) backendReleaseFixture {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	binary := []byte("verified backend candidate")
	digest := sha256.Sum256(binary)
	manifest := BackendManifest{
		SchemaVersion: 1,
		Version:       version,
		Commit:        commit,
		Artifacts: map[string]BackendArtifact{
			"linux-amd64": {
				OS:     "linux",
				Arch:   "amd64",
				URL:    "new-api-linux-amd64",
				SHA256: hex.EncodeToString(digest[:]),
				Size:   int64(len(binary)),
			},
		},
	}
	manifestBytes, err := common.Marshal(manifest)
	require.NoError(t, err)
	signature := ed25519.Sign(privateKey, manifestBytes)
	return backendReleaseFixture{
		Manifest:      manifest,
		ManifestBytes: manifestBytes,
		Signature:     base64.StdEncoding.EncodeToString(signature),
		Binary:        binary,
		PublicKey:     publicKey,
	}
}

func (fixture backendReleaseFixture) server(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/manifest.json":
			_, _ = response.Write(fixture.ManifestBytes)
		case "/manifest.json.sig":
			_, _ = response.Write([]byte(fixture.Signature))
		case "/new-api-linux-amd64":
			_, _ = response.Write(fixture.Binary)
		default:
			http.NotFound(response, request)
		}
	}))
}

func newTestBackendUpdater(root string, server *httptest.Server, fixture backendReleaseFixture) *BackendUpdater {
	updater := NewBackendUpdater(root, server.Client())
	updater.URLValidator = nil
	updater.PublicKey = fixture.PublicKey
	updater.GOOS = "linux"
	updater.GOARCH = "amd64"
	updater.Supervised = true
	updater.CandidateValidator = func(_ context.Context, path string, manifest BackendManifest) error {
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if string(content) != string(fixture.Binary) || manifest.Version != fixture.Manifest.Version {
			return errors.New("candidate mismatch")
		}
		return nil
	}
	return updater
}

func TestBackendUpdaterRequiresSupervisedLinuxLauncher(t *testing.T) {
	updater := &BackendUpdater{GOOS: "linux"}
	_, err := updater.Update(context.Background(), "https://example.com/backend-manifest.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "未由后端启动器托管")

	updater.GOOS = "windows"
	updater.Supervised = true
	_, err = updater.Update(context.Background(), "https://example.com/backend-manifest.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "仅支持 Linux")
}

func TestBackendUpdaterInstallsVerifiedReleaseAndIsIdempotent(t *testing.T) {
	fixture := newBackendReleaseFixture(t, "v1.2.3", strings.Repeat("a", 40))
	server := fixture.server(t)
	defer server.Close()
	root := t.TempDir()
	updater := newTestBackendUpdater(root, server, fixture)

	result, err := updater.Update(context.Background(), server.URL+"/manifest.json")
	require.NoError(t, err)
	assert.True(t, result.Changed)
	assert.Equal(t, fixture.Manifest.Version, result.Version)
	target, err := os.Readlink(filepath.Join(root, "pending"))
	require.NoError(t, err)
	assert.Equal(t, "versions/v1.2.3-"+strings.Repeat("a", 40)+"/new-api", filepath.ToSlash(target))
	installed, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(target)))
	require.NoError(t, err)
	assert.Equal(t, fixture.Binary, installed)

	require.NoError(t, os.Rename(filepath.Join(root, "pending"), filepath.Join(root, "current")))
	result, err = updater.Update(context.Background(), server.URL+"/manifest.json")
	require.NoError(t, err)
	assert.False(t, result.Changed)
}

func TestBackendUpdaterRejectsTamperedManifestWithoutSwitching(t *testing.T) {
	fixture := newBackendReleaseFixture(t, "v1.2.4", strings.Repeat("b", 40))
	fixture.Signature = base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
	server := fixture.server(t)
	defer server.Close()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "versions", "stable"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "versions", "stable", "new-api"), []byte("stable"), 0755))
	require.NoError(t, os.Symlink("versions/stable/new-api", filepath.Join(root, "current")))
	updater := newTestBackendUpdater(root, server, fixture)

	_, err := updater.Update(context.Background(), server.URL+"/manifest.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "签名校验失败")
	target, readErr := os.Readlink(filepath.Join(root, "current"))
	require.NoError(t, readErr)
	assert.Equal(t, "versions/stable/new-api", filepath.ToSlash(target))
}

func TestBackendUpdaterRejectsChecksumMismatchBeforeActivation(t *testing.T) {
	fixture := newBackendReleaseFixture(t, "v1.2.5", strings.Repeat("c", 40))
	fixture.Binary[0] ^= 0xff
	server := fixture.server(t)
	defer server.Close()
	root := t.TempDir()
	updater := newTestBackendUpdater(root, server, fixture)

	_, err := updater.Update(context.Background(), server.URL+"/manifest.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SHA-256 校验失败")
	assert.NoFileExists(t, filepath.Join(root, "pending"))
}

func TestBackendUpdaterRejectsCandidateValidationFailure(t *testing.T) {
	fixture := newBackendReleaseFixture(t, "v1.2.6", strings.Repeat("d", 40))
	server := fixture.server(t)
	defer server.Close()
	root := t.TempDir()
	updater := newTestBackendUpdater(root, server, fixture)
	updater.CandidateValidator = func(context.Context, string, BackendManifest) error {
		return errors.New("candidate version mismatch")
	}

	_, err := updater.Update(context.Background(), server.URL+"/manifest.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "candidate version mismatch")
	assert.NoFileExists(t, filepath.Join(root, "pending"))
}

func TestBackendUpdaterCheckReportsAvailableReleaseWithoutInstalling(t *testing.T) {
	fixture := newBackendReleaseFixture(t, "v1.2.7", strings.Repeat("f", 40))
	server := fixture.server(t)
	defer server.Close()
	root := t.TempDir()
	updater := newTestBackendUpdater(root, server, fixture)

	result, err := updater.Check(context.Background(), server.URL+"/manifest.json")
	require.NoError(t, err)
	assert.Equal(t, fixture.Manifest.Version, result.LatestVersion)
	assert.Equal(t, fixture.Manifest.Commit, result.LatestCommit)
	assert.True(t, result.UpdateAvailable)
	assert.NoFileExists(t, filepath.Join(root, "pending"))
	assert.NoDirExists(t, filepath.Join(root, "versions"))
}

func TestConfiguredBackendPublicKeyUsesEmbeddedKeyAndEnvironmentOverride(t *testing.T) {
	t.Setenv("BACKEND_UPDATE_PUBLIC_KEY", "")
	publicKey, err := configuredBackendPublicKey()
	require.NoError(t, err)
	assert.Equal(
		t,
		"8DxuvAeXP2JBMlFhIk1LKwItiAVygCCDe8sCMMeBTDU=",
		base64.StdEncoding.EncodeToString(publicKey),
	)

	overridePublicKey, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	t.Setenv("BACKEND_UPDATE_PUBLIC_KEY", base64.StdEncoding.EncodeToString(overridePublicKey))
	publicKey, err = configuredBackendPublicKey()
	require.NoError(t, err)
	assert.Equal(t, overridePublicKey, publicKey)
}

func TestStageBackendReleaseLeavesCurrentVersionUntouched(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "versions"), 0755))
	require.NoError(t, os.Symlink("versions/old/new-api", filepath.Join(root, "current")))

	changed, err := stageBackendRelease(root, "versions/new/new-api")
	require.NoError(t, err)
	assert.True(t, changed)
	current, err := os.Readlink(filepath.Join(root, "current"))
	require.NoError(t, err)
	pending, err := os.Readlink(filepath.Join(root, "pending"))
	require.NoError(t, err)
	assert.Equal(t, "versions/old/new-api", filepath.ToSlash(current))
	assert.Equal(t, "versions/new/new-api", filepath.ToSlash(pending))
}

func TestReadBackendReleaseLinkRejectsEscapingTarget(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "versions"), 0755))
	require.NoError(t, os.Symlink("../outside/new-api", filepath.Join(root, "pending")))

	_, err := readBackendReleaseLink(root, filepath.Join(root, "pending"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "超出版本目录")
}

func TestValidateBackendCandidateRejectsInvalidExecutableAndVersion(t *testing.T) {
	invalid := filepath.Join(t.TempDir(), "new-api")
	require.NoError(t, os.WriteFile(invalid, []byte("not an ELF executable"), 0755))
	err := validateBackendCandidate(context.Background(), invalid, BackendManifest{Version: "v1.0.0"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ELF")

	if runtime.GOOS != "linux" {
		t.Skip("candidate execution is Linux-only")
	}
	trueExecutable, lookupErr := exec.LookPath("true")
	require.NoError(t, lookupErr)
	err = validateBackendCandidate(context.Background(), trueExecutable, BackendManifest{Version: "v1.0.0"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "候选版本")
}

func TestBackendSignatureURLPreservesQueryParameters(t *testing.T) {
	got, err := backendSignatureURL("https://example.com/backend-manifest.json?token=redacted")
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/backend-manifest.json.sig?token=redacted", got)
}

func TestProcessRestartRequestIsCoalesced(t *testing.T) {
	select {
	case <-ProcessRestartRequests():
	default:
	}
	RequestProcessRestart()
	RequestProcessRestart()
	select {
	case <-ProcessRestartRequests():
	default:
		t.Fatal("expected restart request")
	}
	select {
	case <-ProcessRestartRequests():
		t.Fatal("duplicate restart request was not coalesced")
	default:
	}
}

func TestBackendUpdateStatusReportsPersistedReleaseState(t *testing.T) {
	root := t.TempDir()
	writeBackendReleaseFixture(t, root, "active", "v1.2.3", strings.Repeat("a", 40))
	writeBackendReleaseFixture(t, root, "next", "v1.2.4", strings.Repeat("b", 40))
	writeBackendReleaseFixture(t, root, "old", "v1.2.2", strings.Repeat("c", 40))
	require.NoError(t, os.Symlink("versions/active/new-api", filepath.Join(root, "current")))
	require.NoError(t, os.Symlink("versions/next/new-api", filepath.Join(root, "pending")))
	require.NoError(t, os.Symlink("versions/old/new-api", filepath.Join(root, "previous")))

	status, err := BackendUpdateStatusSnapshot(root)
	require.NoError(t, err)
	assert.Equal(t, "v1.2.3", status.CurrentVersion)
	assert.Equal(t, "v1.2.4", status.PendingVersion)
	assert.Equal(t, "v1.2.2", status.PreviousVersion)
	assert.True(t, status.RestartRequired)
	assert.True(t, status.CanRollback)
}

func TestStageBackendRollbackUsesPreviousReleaseWithoutChangingCurrent(t *testing.T) {
	t.Setenv("NEW_API_BACKEND_SUPERVISED", "1")
	root := t.TempDir()
	writeBackendReleaseFixture(t, root, "active", "v2.0.0", strings.Repeat("d", 40))
	writeBackendReleaseFixture(t, root, "old", "v1.9.0", strings.Repeat("e", 40))
	require.NoError(t, os.Symlink("versions/active/new-api", filepath.Join(root, "current")))
	require.NoError(t, os.Symlink("versions/old/new-api", filepath.Join(root, "previous")))

	result, err := StageBackendRollback(root)
	require.NoError(t, err)
	assert.Equal(t, "v1.9.0", result.Version)
	current, err := os.Readlink(filepath.Join(root, "current"))
	require.NoError(t, err)
	pending, err := os.Readlink(filepath.Join(root, "pending"))
	require.NoError(t, err)
	assert.Equal(t, "versions/active/new-api", filepath.ToSlash(current))
	assert.Equal(t, "versions/old/new-api", filepath.ToSlash(pending))
}

func writeBackendReleaseFixture(t *testing.T, root, id, version, commit string) {
	t.Helper()
	directory := filepath.Join(root, "versions", id)
	require.NoError(t, os.MkdirAll(directory, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "new-api"), []byte("fixture"), 0755))
	manifest, err := common.Marshal(BackendManifest{SchemaVersion: 1, Version: version, Commit: commit})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "manifest.json"), manifest, 0644))
}
