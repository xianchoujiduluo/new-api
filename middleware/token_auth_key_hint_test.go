package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupTokenAuthTestDB captures the gin log writers, which is where both
// common.SysLog and logger.LogDebug actually write.
func setupTokenAuthTestDB(t *testing.T) *bytes.Buffer {
	t.Helper()
	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousType := common.MainDatabaseType()
	previousRedis := common.RedisEnabled
	previousDebug := common.DebugEnabled

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Token{}, &model.User{}))

	var logs bytes.Buffer
	previousWriter := gin.DefaultWriter
	previousErrorWriter := gin.DefaultErrorWriter
	gin.DefaultWriter = io.MultiWriter(previousWriter, &logs)
	gin.DefaultErrorWriter = io.MultiWriter(previousErrorWriter, &logs)

	model.DB = db
	model.LOG_DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.DebugEnabled = true

	t.Cleanup(func() {
		gin.DefaultWriter = previousWriter
		gin.DefaultErrorWriter = previousErrorWriter
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.SetMainDatabaseType(previousType)
		common.RedisEnabled = previousRedis
		common.DebugEnabled = previousDebug
	})
	return &logs
}

func runTokenAuth(t *testing.T, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	for k, v := range headers {
		c.Request.Header.Set(k, v)
	}
	TokenAuth()(c)
	return recorder
}

// TestTokenAuthLogsClientCredentialVerbatim pins that a rejected request reports
// the credential exactly as the client sent it, alongside the value actually
// looked up. Without the raw value a mismatch caused by parsing (the "sk-" strip
// and the channel-pin suffix) cannot be distinguished from a genuinely unknown
// token.
func TestTokenAuthLogsClientCredentialVerbatim(t *testing.T) {
	logs := setupTokenAuthTestDB(t)

	runTokenAuth(t, map[string]string{"Authorization": "Bearer sk-abcdefghijklmnop-42"})

	output := logs.String()
	require.Contains(t, output, "TokenAuth credential rejected:")

	raw := strings.Index(output, `raw_authorization="Bearer sk-abcdefghijklmnop-42"`)
	assert.GreaterOrEqual(t, raw, 0, "the header must be logged verbatim, including the scheme")
	// looked_up is the stripped, suffix-truncated value that is actually queried.
	assert.Contains(t, output, `looked_up="abcdefghijklmnop"`)
	assert.Contains(t, output, "len=16")
}

// TestTokenAuthLogsMjSecretFallbackVerbatim covers the midjourney-proxy path,
// where the credential comes from a different header.
func TestTokenAuthLogsMjSecretFallbackVerbatim(t *testing.T) {
	logs := setupTokenAuthTestDB(t)

	runTokenAuth(t, map[string]string{
		"Authorization": "midjourney-proxy",
		"mj-api-secret": "sk-mjsecretvalue",
	})

	output := logs.String()
	require.Contains(t, output, "TokenAuth credential rejected:")
	assert.Contains(t, output, `raw_mj_secret="sk-mjsecretvalue"`)
	assert.Contains(t, output, `looked_up="mjsecretvalue"`)
}
