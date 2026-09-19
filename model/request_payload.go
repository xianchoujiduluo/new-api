package model

import (
	"context"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// DefaultRequestPayloadMaxBytes is the per-column cap applied to captured
// headers and bodies. Bodies are user/upstream controlled and may contain large
// multimodal base64 payloads, so an explicit bound is required to keep the
// ClickHouse table (and network writes) predictable.
const DefaultRequestPayloadMaxBytes = 64 * 1024

// RequestPayloadMaxBytes returns the configured per-column capture limit.
func RequestPayloadMaxBytes() int {
	limit := common.GetEnvOrDefault("LOG_PAYLOAD_MAX_BYTES", DefaultRequestPayloadMaxBytes)
	if limit <= 0 {
		return DefaultRequestPayloadMaxBytes
	}
	return limit
}

// RequestPayload stores the raw request/response material for a relay call.
// It is only persisted when the RecordPayloadEnabled option is on. Rows are
// keyed by request_id, which is the stable identifier shared with the logs
// table (ClickHouse logs.id is not a reliable primary key).
//
// The table is created on whichever database LOG_DB points at: a custom
// MergeTree DDL for ClickHouse, or AutoMigrate for SQLite/MySQL/PostgreSQL.
type RequestPayload struct {
	RequestId string `json:"request_id" gorm:"column:request_id;index:idx_request_payloads_request_id"`
	LogId     int64  `json:"log_id" gorm:"column:log_id"`

	CreatedAt int64  `json:"created_at" gorm:"column:created_at;index:idx_request_payloads_created_at"`
	UserId    int    `json:"user_id" gorm:"column:user_id"`
	Username  string `json:"username" gorm:"column:username"`
	TokenId   int    `json:"token_id" gorm:"column:token_id"`
	TokenName string `json:"token_name" gorm:"column:token_name"`
	ChannelId int    `json:"channel_id" gorm:"column:channel_id"`
	ModelName string `json:"model_name" gorm:"column:model_name"`

	ClientIp string `json:"client_ip" gorm:"column:client_ip"`

	RequestHeaders  string `json:"request_headers" gorm:"column:request_headers"`
	RequestBody     string `json:"request_body" gorm:"column:request_body"`
	RequestBodySize int64  `json:"request_body_size" gorm:"column:request_body_size"`

	StatusCode       int    `json:"status_code" gorm:"column:status_code"`
	IsStream         bool   `json:"is_stream" gorm:"column:is_stream"`
	ResponseHeaders  string `json:"response_headers" gorm:"column:response_headers"`
	ResponseBody     string `json:"response_body" gorm:"column:response_body"`
	ResponseBodySize int64  `json:"response_body_size" gorm:"column:response_body_size"`

	IsTruncated  bool   `json:"is_truncated" gorm:"column:is_truncated"`
	ErrorMessage string `json:"error_message" gorm:"column:error_message"`
}

// RequestPayloadSupported reports whether the request/response payload feature
// can persist to the active log database. It is supported on SQLite, MySQL,
// PostgreSQL, and ClickHouse.
func RequestPayloadSupported() bool {
	return LOG_DB != nil
}

// requestPayloadCreateTableSQL mirrors the logs table conventions: MergeTree,
// monthly partitions, and no TTL (cleanup follows the manual log-cleanup task).
// Text columns use ZSTD to compensate for the large bodies stored here.
func requestPayloadCreateTableSQL() string {
	return `
CREATE TABLE IF NOT EXISTS request_payloads (
	request_id String DEFAULT '',
	log_id Int64 DEFAULT 0,
	created_at Int64 DEFAULT 0,
	user_id Int32 DEFAULT 0,
	username String DEFAULT '',
	token_id Int32 DEFAULT 0,
	token_name String DEFAULT '',
	channel_id Int32 DEFAULT 0,
	model_name String DEFAULT '',
	client_ip String DEFAULT '',
	request_headers String CODEC(ZSTD(3)) DEFAULT '',
	request_body String CODEC(ZSTD(3)) DEFAULT '',
	request_body_size Int64 DEFAULT 0,
	status_code Int32 DEFAULT 0,
	is_stream UInt8 DEFAULT 0,
	response_headers String CODEC(ZSTD(3)) DEFAULT '',
	response_body String CODEC(ZSTD(3)) DEFAULT '',
	response_body_size Int64 DEFAULT 0,
	is_truncated UInt8 DEFAULT 0,
	error_message String CODEC(ZSTD(3)) DEFAULT ''
)
ENGINE = MergeTree()
PARTITION BY toYYYYMM(toDateTime(created_at))
ORDER BY (created_at, request_id)`
}

func migrateRequestPayloadTable() error {
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return LOG_DB.Exec(requestPayloadCreateTableSQL()).Error
	}
	return LOG_DB.AutoMigrate(&RequestPayload{})
}

// sensitiveHeaderNames are compared case-insensitively for exact matches.
var sensitiveHeaderNames = map[string]struct{}{
	"authorization":       {},
	"proxy-authorization": {},
	"api-key":             {},
	"apikey":              {},
	"x-api-key":           {},
	"x-goog-api-key":      {},
	"x-auth-token":        {},
	"x-access-token":      {},
	"anthropic-api-key":   {},
	"openai-api-key":      {},
	"cookie":              {},
	"set-cookie":          {},
	"x-csrf-token":        {},
	"x-xsrf-token":        {},
}

const redactedHeaderValue = "[REDACTED]"

// isSensitiveHeader reports whether a header must be redacted before storage.
// It uses an explicit name list plus conservative substring/suffix matching so
// newly introduced credential headers are redacted even without an allowlist
// update, while ordinary headers (e.g. x-request-id) are preserved.
func isSensitiveHeader(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return false
	}
	if _, ok := sensitiveHeaderNames[lower]; ok {
		return true
	}
	switch {
	case strings.Contains(lower, "authorization"),
		strings.Contains(lower, "api-key"),
		strings.Contains(lower, "apikey"),
		strings.Contains(lower, "cookie"),
		strings.Contains(lower, "secret"):
		return true
	case strings.HasSuffix(lower, "-token"), strings.HasSuffix(lower, "_token"):
		return true
	}
	return false
}

// SanitizeHeaders renders request/response headers as a JSON object with
// credential-bearing values redacted. Multiple values are joined with ", ".
func SanitizeHeaders(headers map[string][]string) string {
	if len(headers) == 0 {
		return "{}"
	}
	out := make(map[string]string, len(headers))
	for name, values := range headers {
		if isSensitiveHeader(name) {
			out[name] = redactedHeaderValue
			continue
		}
		out[name] = strings.Join(values, ", ")
	}
	data, err := common.Marshal(out)
	if err != nil {
		common.SysError("failed to marshal sanitized headers: " + err.Error())
		return "{}"
	}
	return string(data)
}

// TruncatePayloadBody bounds a captured string to maxBytes (byte length) and
// reports whether truncation occurred. The result is forced to valid UTF-8 so
// the payload can be JSON-serialized even when the source is binary.
func TruncatePayloadBody(body string, maxBytes int) (string, bool) {
	if maxBytes <= 0 {
		maxBytes = DefaultRequestPayloadMaxBytes
	}
	truncated := false
	if len(body) > maxBytes {
		body = body[:maxBytes]
		truncated = true
	}
	return strings.ToValidUTF8(body, "�"), truncated
}

// GetLogUserIdByRequestId resolves the owning user of a usage log by request id.
// It is used to authorize payload reads: only the owner (or an admin) may view
// the raw request/response material.
func GetLogUserIdByRequestId(ctx context.Context, requestId string) (int, bool, error) {
	var row struct {
		UserId int `gorm:"column:user_id"`
	}
	err := LOG_DB.WithContext(ctx).
		Model(&Log{}).
		Select("user_id").
		Where("request_id = ?", requestId).
		Limit(1).
		Scan(&row).Error
	if err != nil {
		return 0, false, err
	}
	if row.UserId == 0 {
		return 0, false, nil
	}
	return row.UserId, true, nil
}

func InsertRequestPayload(payload *RequestPayload) error {
	if payload == nil {
		return nil
	}
	if err := LOG_DB.Create(payload).Error; err != nil {
		return err
	}
	return nil
}

func GetRequestPayloadByRequestId(ctx context.Context, requestId string) (*RequestPayload, error) {
	var payload RequestPayload
	err := LOG_DB.WithContext(ctx).
		Model(&RequestPayload{}).
		Where("request_id = ?", requestId).
		Limit(1).
		Find(&payload).Error
	if err != nil {
		return nil, err
	}
	if payload.RequestId == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return &payload, nil
}

// DeleteOldRequestPayloads removes payload rows older than the cutoff. It is
// invoked from the manual log-cleanup task so payload data shares the same
// retention policy as the logs table.
func DeleteOldRequestPayloads(ctx context.Context, targetTimestamp int64) error {
	if LOG_DB == nil {
		return nil
	}
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return LOG_DB.WithContext(ctx).Exec(
			"ALTER TABLE request_payloads DELETE WHERE created_at < ? SETTINGS mutations_sync = 1",
			targetTimestamp,
		).Error
	}
	// Relational databases delete in bounded batches so a large backlog does not
	// hold one long transaction.
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		result := LOG_DB.WithContext(ctx).
			Where("created_at < ?", targetTimestamp).
			Limit(1000).
			Delete(&RequestPayload{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
	}
}
