package model

import (
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeHeadersRedactsCredentials(t *testing.T) {
	headers := map[string][]string{
		"Authorization":  {"Bearer sk-secret"},
		"X-Api-Key":      {"sk-abc"},
		"Cookie":         {"session=1"},
		"Set-Cookie":     {"a=b"},
		"X-Request-Id":   {"req-1"},
		"Content-Type":   {"application/json"},
		"X-Custom-Token": {"tok"},
	}

	rendered := SanitizeHeaders(headers)

	assert.Contains(t, rendered, `"Authorization":"`+redactedHeaderValue+`"`)
	assert.Contains(t, rendered, `"X-Api-Key":"`+redactedHeaderValue+`"`)
	assert.Contains(t, rendered, `"Cookie":"`+redactedHeaderValue+`"`)
	assert.Contains(t, rendered, `"Set-Cookie":"`+redactedHeaderValue+`"`)
	assert.Contains(t, rendered, `"X-Custom-Token":"`+redactedHeaderValue+`"`)
	assert.Contains(t, rendered, `"X-Request-Id":"req-1"`)
	assert.Contains(t, rendered, `"Content-Type":"application/json"`)
	assert.NotContains(t, rendered, "sk-secret")
	assert.NotContains(t, rendered, "sk-abc")
}

func TestSanitizeHeadersEmpty(t *testing.T) {
	assert.Equal(t, "{}", SanitizeHeaders(nil))
}

func TestIsSensitiveHeader(t *testing.T) {
	cases := map[string]bool{
		"Authorization":       true,
		"authorization":       true,
		"Proxy-Authorization": true,
		"x-goog-api-key":      true,
		"apikey":              true,
		"X-Secret":            true,
		"X-Access-Token":      true,
		"X-Request-Id":        false,
		"Content-Type":        false,
		"Accept":              false,
	}
	for name, want := range cases {
		assert.Equal(t, want, isSensitiveHeader(name), name)
	}
}

func TestTruncatePayloadBody(t *testing.T) {
	small, truncated := TruncatePayloadBody("hello", 64)
	assert.Equal(t, "hello", small)
	assert.False(t, truncated)

	large, truncated := TruncatePayloadBody("abcdef", 3)
	assert.Equal(t, "abc", large)
	assert.True(t, truncated)

	// Invalid UTF-8 must not remain, or JSON marshaling would fail later.
	invalid, _ := TruncatePayloadBody(string([]byte{0xff, 0xfe, 'a'}), 64)
	assert.True(t, utf8.ValidString(invalid))
}
