package middleware

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPayloadCaptureAssemblesOpenAIChatSSE(t *testing.T) {
	capture := &payloadCapture{}

	capture.observe([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n"))
	capture.observe([]byte("data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n"))
	capture.observe([]byte("data: [DONE]\n\n"))

	require.True(t, capture.sse)
	assert.Equal(t, "Hello world", capture.assembled.String())
}

func TestPayloadCaptureAssemblesResponsesAPITextDelta(t *testing.T) {
	capture := &payloadCapture{}

	capture.observe([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":{\"text\":\"abc\"}}\n\n"))
	capture.observe([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":{\"text\":\"def\"}}\n\n"))

	assert.Equal(t, "abcdef", capture.assembled.String())
}

func TestPayloadCaptureKeepsRawNonStreamBody(t *testing.T) {
	capture := &payloadCapture{}

	capture.observe([]byte(`{"id":"x","choices":[{"message":{"content":"hi"}}]}`))

	assert.False(t, capture.sse)
	assert.Contains(t, capture.raw.String(), `"hi"`)
	assert.Empty(t, capture.assembled.String())
}

func TestPayloadCaptureIgnoresAfterComplete(t *testing.T) {
	capture := &payloadCapture{}
	capture.observe([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\n\n"))
	capture.complete = true
	capture.observe([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"b\"}}]}\n\n"))

	assert.Equal(t, "a", capture.assembled.String())
}
