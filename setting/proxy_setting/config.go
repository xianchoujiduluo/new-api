package proxy_setting

import (
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

// ProxySettingOptionKeyPrefix is the persisted namespace for the shared proxy
// registry. Values are stored through the generic option system, so removing the
// registry entirely is a matter of clearing these keys.
const (
	ProxySettingKey = "proxy_setting.proxies"
)

// Entry is one named proxy in the shared registry. Channels reference it by Id,
// so renaming an entry does not break existing channels and editing the URL
// takes effect everywhere the entry is referenced.
type Entry struct {
	// Id is the stable identity a channel stores. It never changes after
	// creation, even when Name changes.
	Id string `json:"id"`
	// Name is the operator-facing label shown in pickers.
	Name string `json:"name"`
	// Url is the proxy address, optionally carrying credentials.
	Url string `json:"url"`
	// Enabled is false when the entry is being kept for reference but should no
	// longer be applied. A disabled entry resolves to the direct connection.
	Enabled bool `json:"enabled"`
}

// ProxySetting holds the shared proxy registry. It is registered with the
// generic config system so it persists alongside other settings.
type ProxySetting struct {
	Proxies []Entry `json:"proxies"`
}

var proxySetting = ProxySetting{}

func init() {
	config.GlobalConfig.Register("proxy_setting", &proxySetting)
}

// GetProxies returns a copy of the registry so callers cannot mutate it in place.
func GetProxies() []Entry {
	entries := make([]Entry, len(proxySetting.Proxies))
	copy(entries, proxySetting.Proxies)
	return entries
}

// Lookup finds an entry by its stable Id.
func Lookup(id string) (Entry, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Entry{}, false
	}
	for _, entry := range proxySetting.Proxies {
		if entry.Id == id {
			return entry, true
		}
	}
	return Entry{}, false
}

// ResolveURL returns the proxy URL a channel reference should use.
//
// The second result reports whether the reference was resolvable at all, which
// lets callers distinguish "no proxy configured" from "configured entry is gone
// or disabled". A missing or disabled entry yields an empty URL: the request
// then goes out directly, matching the behavior of a channel with no proxy.
func ResolveURL(ref string) (string, bool) {
	entry, ok := Lookup(ref)
	if !ok {
		return "", false
	}
	if !entry.Enabled {
		return "", true
	}
	return strings.TrimSpace(entry.Url), true
}

// ValidateEntries validates a registry before it is persisted.
//
// Errors are reported with the offending name or position so the operator can
// find the row without guessing. Proxy URLs accept http, https, socks5, and
// socks5h, and may carry credentials.
func ValidateEntries(entries []Entry) error {
	ids := make(map[string]int, len(entries))
	names := make(map[string]int, len(entries))
	for index, entry := range entries {
		entry.Id = strings.TrimSpace(entry.Id)
		entry.Name = strings.TrimSpace(entry.Name)
		if entry.Id == "" {
			return fmt.Errorf("代理条目缺少 id（第 %d 项）", index+1)
		}
		if entry.Name == "" {
			return fmt.Errorf("代理条目缺少名称（第 %d 项）", index+1)
		}
		if previous, duplicated := ids[entry.Id]; duplicated {
			return fmt.Errorf("代理条目 id 重复：%s（第 %d 项与第 %d 项）", entry.Id, previous+1, index+1)
		}
		if previous, duplicated := names[entry.Name]; duplicated {
			return fmt.Errorf("代理名称重复：%s（第 %d 项与第 %d 项）", entry.Name, previous+1, index+1)
		}
		ids[entry.Id] = index
		names[entry.Name] = index

		if _, err := common.ParseProxyURLStrict(strings.TrimSpace(entry.Url)); err != nil {
			return fmt.Errorf("代理 %s 的地址无效: %w", entry.Name, err)
		}
	}
	return nil
}

// MissingReferences returns ids that channels still reference but the proposed
// registry no longer contains, so a save can refuse to strand them. Callers
// gather the reference counts from channels, keeping this package free of
// database access.
func MissingReferences(entries []Entry, references map[string]int) []string {
	known := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		known[strings.TrimSpace(entry.Id)] = struct{}{}
	}
	missing := make([]string, 0, len(references))
	for id, count := range references {
		if count <= 0 {
			continue
		}
		if _, ok := known[id]; !ok {
			missing = append(missing, id)
		}
	}
	sort.Strings(missing)
	return missing
}
