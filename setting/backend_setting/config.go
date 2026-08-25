package backend_setting

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

const (
	ManifestURLOptionKey   = "backend_setting.manifest_url"
	DownloadProxyOptionKey = "backend_setting.download_proxy"
	DefaultManifestURL     = "https://github.com/xianchoujiduluo/new-api/releases/download/latestBackend/backend-manifest.json"
)

// BackendSetting contains download settings for backend updates. The trust key
// is deliberately process configuration, not a database-backed option.
type BackendSetting struct {
	ManifestURL   string `json:"manifest_url"`
	DownloadProxy string `json:"download_proxy"`
}

var backendSetting = BackendSetting{ManifestURL: DefaultManifestURL}

func init() {
	config.GlobalConfig.Register("backend_setting", &backendSetting)
}

func GetBackendSetting() *BackendSetting {
	return &backendSetting
}

func ValidateManifestURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil {
		return fmt.Errorf("后端更新清单地址无效: %w", err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("后端更新清单地址必须使用 http 或 https 协议")
	}
	if parsed.User != nil {
		return fmt.Errorf("后端更新清单地址不允许包含用户名或密码")
	}
	return nil
}

func ValidateDownloadProxy(raw string) error {
	if _, err := common.ParseProxyURLStrict(strings.TrimSpace(raw)); err != nil {
		return fmt.Errorf("后端下载代理无效: %w", err)
	}
	return nil
}
