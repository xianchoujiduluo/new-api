package frontend_setting

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

// DownloadURLOptionKey is the persisted option containing the frontend
// archive URL. An empty value keeps the embedded frontend active.
const DownloadURLOptionKey = "frontend_setting.download_url"

// DownloadProxyOptionKey is the persisted optional proxy used only for
// downloading the external frontend archive.
const DownloadProxyOptionKey = "frontend_setting.download_proxy"

// FrontendSetting contains settings for an externally published frontend.
type FrontendSetting struct {
	DownloadURL   string `json:"download_url"`
	DownloadProxy string `json:"download_proxy"`
}

var frontendSetting = FrontendSetting{}

func init() {
	config.GlobalConfig.Register("frontend_setting", &frontendSetting)
}

// GetFrontendSetting returns the registered frontend settings.
func GetFrontendSetting() *FrontendSetting {
	return &frontendSetting
}

// ValidateDownloadURL validates the URL format without applying network
// policy. SSRF and redirect policy are enforced again immediately before the
// download, because those policies can change at runtime.
func ValidateDownloadURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil {
		return fmt.Errorf("前端下载地址无效: %w", err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("前端下载地址必须使用 http 或 https 协议")
	}
	if parsed.User != nil {
		return fmt.Errorf("前端下载地址不允许包含用户名或密码")
	}
	return nil
}

// ValidateDownloadProxy validates the optional frontend download proxy using
// the same strict rules as channel proxy settings. Empty values explicitly
// disable the dedicated proxy and retain the normal HTTP client behavior.
func ValidateDownloadProxy(raw string) error {
	if _, err := common.ParseProxyURLStrict(strings.TrimSpace(raw)); err != nil {
		return fmt.Errorf("前端下载代理无效: %w", err)
	}
	return nil
}
