package middleware

import (
	"bytes"
	"net/http"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// payloadCapture accumulates the client-visible request/response material for a
// single relay call. It is only allocated when payload recording is enabled.
type payloadCapture struct {
	mu sync.Mutex

	requestBody string
	truncated   bool

	// assembled holds the assistant-visible text reconstructed from an SSE
	// stream. For non-stream responses the raw body is used instead.
	assembled strings.Builder
	sse       bool

	// raw keeps a bounded copy of every response write. It is the response body
	// for non-streaming responses and the fallback when assembly yields nothing.
	raw strings.Builder

	statusCode int
	complete   bool
}

func (p *payloadCapture) observe(b []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.complete || len(b) == 0 {
		return
	}
	limit := model.RequestPayloadMaxBytes()

	if p.raw.Len() < limit {
		remain := limit - p.raw.Len()
		if remain >= len(b) {
			p.raw.Write(b)
		} else {
			p.raw.Write(b[:remain])
			p.truncated = true
		}
	}

	if isSSEFrame(b) {
		p.sse = true
		if p.assembled.Len() < limit {
			p.consumeSSE(b)
		}
	}
}

func isSSEFrame(b []byte) bool {
	trimmed := bytes.TrimLeft(b, " \r\n")
	return bytes.HasPrefix(trimmed, []byte("data:")) || bytes.HasPrefix(trimmed, []byte("event:"))
}

// payloadResponseWriter wraps gin.ResponseWriter and tees the response body
// into the capture without altering write/flush semantics.
type payloadResponseWriter struct {
	gin.ResponseWriter
	capture *payloadCapture
}

func (w *payloadResponseWriter) Write(b []byte) (int, error) {
	if w.capture != nil {
		w.capture.observe(b)
	}
	return w.ResponseWriter.Write(b)
}

func (w *payloadResponseWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func (w *payloadResponseWriter) Flush() {
	if w.capture != nil {
		w.capture.statusCode = w.Status()
	}
	w.ResponseWriter.Flush()
}

func (w *payloadResponseWriter) WriteHeader(code int) {
	if w.capture != nil {
		w.capture.statusCode = code
	}
	w.ResponseWriter.WriteHeader(code)
}

// RequestPayloadCapture mounts payload capture for relay endpoints. It is a
// no-op unless the option is enabled and the ClickHouse log database is active.
func RequestPayloadCapture() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !common.RecordPayloadEnabled || !common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
			c.Next()
			return
		}
		if c.Request == nil || c.Request.Method != http.MethodPost {
			c.Next()
			return
		}

		capture := &payloadCapture{}

		original := c.Writer
		c.Writer = &payloadResponseWriter{ResponseWriter: original, capture: capture}

		c.Next()

		c.Writer = original
		capture.statusCode = original.Status()
		capture.complete = true
		// Only relay traffic is captured. The tag is set by the relay route
		// groups and is available once c.Next() returns.
		if tag, _ := c.Get(RouteTagKey); tag == "relay" {
			recordPayload(c, capture)
		}
	}
}

func recordPayload(c *gin.Context, capture *payloadCapture) {
	if capture == nil {
		return
	}

	// Read the request body now (after the handler ran). Requests rejected before
	// the body is consumed have no storage, so they are skipped cheaply.
	if bodyStorage, err := common.GetBodyStorage(c); err == nil {
		if raw, err := bodyStorage.Bytes(); err == nil {
			captured, truncated := model.TruncatePayloadBody(string(raw), model.RequestPayloadMaxBytes())
			capture.requestBody = captured
			capture.truncated = capture.truncated || truncated
		}
	}

	responseBody := capture.raw.String()
	if capture.sse {
		if assembled := capture.assembled.String(); assembled != "" {
			responseBody = assembled
		}
	}
	responseBody, bodyTruncated := model.TruncatePayloadBody(responseBody, model.RequestPayloadMaxBytes())

	requestHeaders := map[string][]string{}
	if c.Request != nil {
		requestHeaders = c.Request.Header
	}
	responseHeaders := map[string][]string{}
	if c.Writer != nil {
		responseHeaders = c.Writer.Header()
	}

	requestId := c.GetString(common.RequestIdKey)
	if requestId == "" {
		return
	}

	// Respect the per-user IP recording preference, matching consume logs.
	clientIp := ""
	userId := c.GetInt("id")
	if userSetting, err := model.GetUserSetting(userId, false); err == nil && userSetting.RecordIpLog {
		clientIp = c.ClientIP()
	}

	payload := &model.RequestPayload{
		RequestId:        requestId,
		CreatedAt:        common.GetTimestamp(),
		UserId:           userId,
		Username:         c.GetString("username"),
		TokenId:          c.GetInt("token_id"),
		TokenName:        c.GetString("token_name"),
		ChannelId:        c.GetInt("channel_id"),
		ModelName:        c.GetString("original_model"),
		ClientIp:         clientIp,
		RequestHeaders:   model.SanitizeHeaders(requestHeaders),
		RequestBody:      capture.requestBody,
		RequestBodySize:  int64(len(capture.requestBody)),
		StatusCode:       capture.statusCode,
		IsStream:         capture.sse,
		ResponseHeaders:  model.SanitizeHeaders(responseHeaders),
		ResponseBody:     responseBody,
		ResponseBodySize: int64(len(responseBody)),
		IsTruncated:      capture.truncated || bodyTruncated,
	}
	if err := model.InsertRequestPayload(payload); err != nil {
		common.SysError("failed to record request payload: " + err.Error())
	}
}
