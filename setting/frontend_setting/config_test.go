package frontend_setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateDownloadProxy(t *testing.T) {
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "empty", value: "", valid: true},
		{name: "http", value: "http://proxy.example:8080", valid: true},
		{name: "https", value: "https://proxy.example:8443", valid: true},
		{name: "socks5", value: "socks5://proxy.example:1080", valid: true},
		{name: "socks5h", value: "socks5h://proxy.example:1080", valid: true},
		{name: "credentials", value: "http://user:pass@proxy.example:8080", valid: true},
		{name: "unsupported scheme", value: "ftp://proxy.example:21", valid: false},
		{name: "path is rejected", value: "http://proxy.example:8080/path", valid: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateDownloadProxy(test.value)
			if test.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
