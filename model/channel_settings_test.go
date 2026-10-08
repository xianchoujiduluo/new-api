package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelValidateSettingsRejectsInvalidHTTPTransport(t *testing.T) {
	tests := []struct {
		name    string
		setting dto.ChannelSettings
		wantErr string
	}{
		{
			name:    "auto with shards is valid",
			setting: dto.ChannelSettings{HTTPProtocol: "auto", HTTP2ConnectionShards: 4},
		},
		{
			name:    "http1 with shards greater than one rejected",
			setting: dto.ChannelSettings{HTTPProtocol: "http1", HTTP2ConnectionShards: 2},
			wantErr: "http2_connection_shards",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := &Channel{}
			channel.SetSetting(tt.setting)
			err := channel.ValidateSettings()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestAdvancedCustomChannelRequiresModelListRouteOnlyWhenUpdateChecksEnabled(t *testing.T) {
	inferenceRoute := dto.AdvancedCustomRoute{
		IncomingPath: "/v1/chat/completions",
		UpstreamPath: "/v1/chat/completions",
		Converter:    "none",
	}

	tests := []struct {
		name          string
		checksEnabled bool
		routes        []dto.AdvancedCustomRoute
		wantErr       string
	}{
		{
			name:   "legacy channel without discovery route remains valid",
			routes: []dto.AdvancedCustomRoute{inferenceRoute},
		},
		{
			name:          "enabled checks require discovery route",
			checksEnabled: true,
			routes:        []dto.AdvancedCustomRoute{inferenceRoute},
			wantErr:       dto.AdvancedCustomModelListPath,
		},
		{
			name:          "enabled checks accept discovery route",
			checksEnabled: true,
			routes: []dto.AdvancedCustomRoute{
				inferenceRoute,
				{
					IncomingPath: dto.AdvancedCustomModelListPath,
					UpstreamPath: dto.AdvancedCustomModelListPath,
					Converter:    "none",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := &Channel{Type: constant.ChannelTypeAdvancedCustom}
			channel.SetOtherSettings(dto.ChannelOtherSettings{
				UpstreamModelUpdateCheckEnabled: tt.checksEnabled,
				AdvancedCustom: &dto.AdvancedCustomConfig{
					Routes: tt.routes,
				},
			})

			err := channel.ValidateSettings()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// TestChannelSettingResolvesProxyReference pins the two accessors apart:
// GetSetting resolves a registry reference into Proxy for consumers, while
// GetRawSetting leaves it unresolved so read-modify-write paths cannot copy the
// resolved URL back into storage and end up with both fields populated.
func TestChannelSettingResolvesProxyReference(t *testing.T) {
	cfg := config.GlobalConfig.Get("proxy_setting")
	before := config.GlobalConfig.ExportAllConfigs()
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(cfg, map[string]string{
			"proxies": before["proxy_setting.proxies"],
		}))
	})
	require.NoError(t, config.UpdateConfigFromMap(cfg, map[string]string{
		"proxies": `[{"id":"p1","name":"office","url":"socks5://127.0.0.1:1080","enabled":true}]`,
	}))

	channel := &Channel{}
	channel.SetSetting(dto.ChannelSettings{ProxyRef: "p1"})

	resolved := channel.GetSetting()
	assert.Equal(t, "socks5://127.0.0.1:1080", resolved.Proxy)
	assert.Equal(t, "p1", resolved.ProxyRef)

	raw := channel.GetRawSetting()
	assert.Empty(t, raw.Proxy, "a raw read must not materialize the referenced URL")
	assert.Equal(t, "p1", raw.ProxyRef)

	// Simulate the update path: editing the manual field must not push the
	// resolved value into storage.
	raw.Proxy = "http://127.0.0.1:8080"
	channel.SetSetting(raw)
	stored := channel.GetRawSetting()
	assert.Equal(t, "p1", stored.ProxyRef)
	assert.Equal(t, "http://127.0.0.1:8080", stored.Proxy)
}

// TestChannelSettingDisabledProxyReferenceFallsBackToDirect pins that a disabled
// entry stops applying without breaking the channel: the consumer sees no proxy,
// which is the same as an unconfigured channel, and the reference is retained so
// re-enabling it restores the behavior.
func TestChannelSettingDisabledProxyReferenceFallsBackToDirect(t *testing.T) {
	cfg := config.GlobalConfig.Get("proxy_setting")
	before := config.GlobalConfig.ExportAllConfigs()
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(cfg, map[string]string{
			"proxies": before["proxy_setting.proxies"],
		}))
	})
	require.NoError(t, config.UpdateConfigFromMap(cfg, map[string]string{
		"proxies": `[{"id":"p1","name":"office","url":"socks5://127.0.0.1:1080","enabled":false}]`,
	}))

	channel := &Channel{}
	channel.SetSetting(dto.ChannelSettings{ProxyRef: "p1"})

	assert.Empty(t, channel.GetSetting().Proxy)
	assert.Equal(t, "p1", channel.GetRawSetting().ProxyRef)
}

// TestChannelProxyRefsCountsOnlyReferences pins that the reference count used to
// guard registry deletion reads the stored reference, not the resolved URL, and
// skips channels whose settings cannot be parsed instead of failing the save.
func TestChannelProxyRefsCountsOnlyReferences(t *testing.T) {
	previousDB := DB
	previousType := common.MainDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}))
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	initCol()
	t.Cleanup(func() {
		DB = previousDB
		common.SetMainDatabaseType(previousType)
	})

	for _, setting := range []string{
		`{"proxy_ref":"p1"}`,
		`{"proxy_ref":"p1"}`,
		`{"proxy":"http://127.0.0.1:8080"}`,
		`{"proxy_ref":"p2"}`,
		`not-json`,
	} {
		raw := setting
		require.NoError(t, DB.Create(&Channel{Name: "c", Key: "k", Setting: &raw}).Error)
	}

	counts, err := ChannelProxyRefs()
	require.NoError(t, err)

	assert.Equal(t, 2, counts["p1"])
	assert.Equal(t, 1, counts["p2"])
	assert.NotContains(t, counts, "")
}
