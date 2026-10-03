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

// setupTokenAuthTestDB wires an in-memory token store and captures the gin log
// writers so the diagnostic lines can be asserted.
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

	// Both common.SysLog and logger.LogDebug write through the gin writers, not
	// the standard logger, so capture there.
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

func runTokenAuth(t *testing.T, authorization string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	if authorization != "" {
		c.Request.Header.Set("Authorization", authorization)
	}
	TokenAuth()(c)
	return recorder
}

// TestTokenAuthLogsChannelPinSuffixTruncation pins that the credential sent by
// the client and the value actually looked up are both reported when a
// channel-pin suffix forces the key to be truncated.
//
// The diagnostic is emitted before any database access, so it is asserted on its
// own; the lookup that follows cannot be set up here because commonKeyCol is
// initialised by the unexported initCol in package model. The real lookup path
// is covered by TestValidateUserTokenLogsMaskedKey in package model.
func TestTokenAuthLogsChannelPinSuffixTruncation(t *testing.T) {
	logs := setupTokenAuthTestDB(t)

	runTokenAuth(t, "Bearer sk-abcdefghijklmnop-42")

	output := logs.String()
	require.Contains(t, output, "token key carried a channel-pin suffix")

	sent := strings.Index(output, "sent=")
	lookedUp := strings.Index(output, "looked_up=")
	require.GreaterOrEqual(t, sent, 0, "diagnostic must report the client-supplied key")
	require.GreaterOrEqual(t, lookedUp, 0, "diagnostic must report the queried key")

	// Both values are masked, and they must differ: the suffix was dropped.
	// sent keeps the original "sk-" prefix; looked_up is the stripped, truncated
	// value that is actually queried.
	assert.Contains(t, output, "sent=sk-a**********p-42")
	assert.Contains(t, output, "looked_up=abcd**********mnop")
	// Neither segment may leak in full.
	assert.NotContains(t, output, "abcdefghijklmnop")
}
