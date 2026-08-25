package backend_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateManifestURL(t *testing.T) {
	assert.NoError(t, ValidateManifestURL("https://example.com/backend-manifest.json"))
	assert.Error(t, ValidateManifestURL("file:///tmp/backend-manifest.json"))
	assert.Error(t, ValidateManifestURL("https://user:password@example.com/manifest.json"))
}

func TestValidateDownloadProxy(t *testing.T) {
	assert.NoError(t, ValidateDownloadProxy(""))
	assert.NoError(t, ValidateDownloadProxy("http://proxy.example.com:8080"))
	assert.Error(t, ValidateDownloadProxy("ftp://proxy.example.com"))
}
