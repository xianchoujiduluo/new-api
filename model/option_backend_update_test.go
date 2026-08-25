package model

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/backend_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateBackendUpdateOptions(t *testing.T) {
	require.NoError(t, validateOptionValue(
		backend_setting.ManifestURLOptionKey,
		"https://example.com/backend-manifest.json",
	))
	assert.Error(t, validateOptionValue(backend_setting.ManifestURLOptionKey, "file:///tmp/manifest.json"))
	assert.Error(t, validateOptionValue(backend_setting.DownloadProxyOptionKey, "ftp://proxy.example.com"))
}
