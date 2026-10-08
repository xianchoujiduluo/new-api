package proxy_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateEntriesRejectsDuplicateIdentityAndBadURL(t *testing.T) {
	valid := []Entry{{Id: "p1", Name: "office", Url: "socks5://127.0.0.1:1080", Enabled: true}}

	require.NoError(t, ValidateEntries(valid))

	duplicateID := []Entry{
		{Id: "p1", Name: "a", Url: "http://127.0.0.1:8080"},
		{Id: "p1", Name: "b", Url: "http://127.0.0.1:8081"},
	}
	require.ErrorContains(t, ValidateEntries(duplicateID), "id 重复")

	duplicateName := []Entry{
		{Id: "p1", Name: "office", Url: "http://127.0.0.1:8080"},
		{Id: "p2", Name: "office", Url: "http://127.0.0.1:8081"},
	}
	require.ErrorContains(t, ValidateEntries(duplicateName), "名称重复")

	badURL := []Entry{{Id: "p1", Name: "office", Url: "ftp://127.0.0.1:1080"}}
	require.Error(t, ValidateEntries(badURL))
}

// TestMissingReferencesBlocksDeletingAReferencedEntry pins the rule that keeps
// channels from silently switching to a direct connection when their proxy is
// removed from the registry.
func TestMissingReferencesBlocksDeletingAReferencedEntry(t *testing.T) {
	proposed := []Entry{{Id: "keep", Name: "keep", Url: "http://127.0.0.1:8080"}}
	references := map[string]int{"keep": 2, "gone": 3, "unused": 0}

	missing := MissingReferences(proposed, references)

	assert.Equal(t, []string{"gone"}, missing)
}
