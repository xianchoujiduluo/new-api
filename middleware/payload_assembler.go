package middleware

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// consumeSSE parses SSE frames and appends the assistant-visible text to the
// assembled buffer. It intentionally supports the dominant response shapes
// (OpenAI chat/responses, Claude messages, Gemini) so the stored "assembled
// content" is human-readable rather than raw protocol frames.
func (p *payloadCapture) consumeSSE(frame []byte) {
	limit := model.RequestPayloadMaxBytes()
	for _, line := range strings.Split(string(frame), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		if p.assembled.Len() >= limit {
			return
		}
		p.appendSSEData(payload, limit)
	}
}

func (p *payloadCapture) appendSSEData(payload string, limit int) {
	var frame struct {
		Choices []struct {
			Delta struct {
				Content   string `json:"content"`
				Reasoning string `json:"reasoning_content"`
			} `json:"delta"`
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Delta struct {
			Text string `json:"text"`
		} `json:"delta"`
		Type         string `json:"type"`
		DeltaContent struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta_content"`
	}
	if err := common.UnmarshalJsonStr(payload, &frame); err != nil {
		return
	}
	for _, choice := range frame.Choices {
		p.writeBounded(choice.Delta.Content, limit)
		if choice.Delta.Content == "" {
			p.writeBounded(choice.Message.Content, limit)
		}
	}
	// Responses API text delta.
	if strings.Contains(frame.Type, "output_text") || frame.Delta.Text != "" {
		p.writeBounded(frame.Delta.Text, limit)
	}
	if frame.DeltaContent.Text != "" {
		p.writeBounded(frame.DeltaContent.Text, limit)
	}
}

func (p *payloadCapture) writeBounded(s string, limit int) {
	if s == "" || p.assembled.Len() >= limit {
		return
	}
	remain := limit - p.assembled.Len()
	if len(s) > remain {
		s = s[:remain]
	}
	p.assembled.WriteString(s)
}
