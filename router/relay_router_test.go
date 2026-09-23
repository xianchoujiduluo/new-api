package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListModelsSupportsOpenAIAndGeminiAuthentication(t *testing.T) {
	setupRelayRouterTestDB(t)

	user := model.User{
		Username: "models-user",
		Status:   common.UserStatusEnabled,
		Group:    "default",
		Quota:    100,
	}
	require.NoError(t, model.DB.Create(&user).Error)
	require.NoError(t, model.DB.Create(&model.Token{
		UserId:         user.Id,
		Key:            "modelstestkey",
		Status:         common.TokenStatusEnabled,
		ExpiredTime:    -1,
		UnlimitedQuota: true,
	}).Error)

	engine := gin.New()
	SetRelayRouter(engine)

	tests := []struct {
		name           string
		path           string
		headerName     string
		expectedObject string
		expectedField  string
	}{
		{
			name:           "OpenAI bearer token",
			path:           "/v1/models",
			headerName:     "Authorization",
			expectedObject: "list",
			expectedField:  "data",
		},
		{
			name:          "Gemini API key header",
			path:          "/v1/models",
			headerName:    "x-goog-api-key",
			expectedField: "models",
		},
		{
			name:          "Gemini API key query",
			path:          "/v1/models?key=modelstestkey",
			expectedField: "models",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			if test.headerName != "" {
				value := "modelstestkey"
				if test.headerName == "Authorization" {
					value = "Bearer " + value
				}
				request.Header.Set(test.headerName, value)
			}

			engine.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusOK, recorder.Code)
			var payload map[string]any
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
			assert.Contains(t, payload, test.expectedField)
			assert.NotContains(t, payload, "error")
			if test.expectedObject != "" {
				assert.Equal(t, test.expectedObject, payload["object"])
			}
		})
	}
}

func setupRelayRouterTestDB(t *testing.T) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	originalIsMasterNode := common.IsMasterNode
	originalRedisEnabled := common.RedisEnabled
	originalSQLitePath := common.SQLitePath
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	originalSQLDSN, hadSQLDSN := os.LookupEnv("SQL_DSN")

	common.IsMasterNode = false
	common.RedisEnabled = false
	common.SQLitePath = fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	require.NoError(t, os.Setenv("SQL_DSN", "local"))
	require.NoError(t, model.InitDB())
	model.LOG_DB = model.DB
	require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.Token{}, &model.Ability{}))

	t.Cleanup(func() {
		if sqlDB, err := model.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
		common.IsMasterNode = originalIsMasterNode
		common.RedisEnabled = originalRedisEnabled
		common.SQLitePath = originalSQLitePath
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		if hadSQLDSN {
			require.NoError(t, os.Setenv("SQL_DSN", originalSQLDSN))
		} else {
			require.NoError(t, os.Unsetenv("SQL_DSN"))
		}
	})
}

// TestSetRelayRouterRegistersResponsesCompactEndpoint guards the compaction
// entry point. POST /v1/responses is deliberately NOT registered here: it comes
// from the plugin protocol registry (jsplugin.HostProtocols) via
// SetTaskPluginProtocolRouter, and registering it twice panics at startup.
func TestSetRelayRouterRegistersResponsesCompactEndpoint(t *testing.T) {
	setupRelayRouterTestDB(t)

	engine := gin.New()
	require.NotPanics(t, func() { SetRelayRouter(engine) })

	actual := make(map[string]struct{}, len(engine.Routes()))
	for _, route := range engine.Routes() {
		actual[route.Method+" "+route.Path] = struct{}{}
	}
	assert.Contains(t, actual, http.MethodPost+" /v1/responses/compact")
	assert.NotContains(t, actual, http.MethodPost+" /v1/responses")
}

// TestSetRouterRegistersWithoutDuplicatePanic guards against a path being
// registered twice. Gin panics during registration, before the server starts,
// which crash-loops the process; per-router tests cannot see it because only the
// full SetRouter composition registers every route in the real order.
//
// This is why /v1/responses is asserted here: it comes from the plugin protocol
// registry (jsplugin.HostProtocols), not from the relay router, so adding it to
// the relay router too is a duplicate.
func TestSetRouterRegistersWithoutDuplicatePanic(t *testing.T) {
	setupRelayRouterTestDB(t)
	gin.SetMode(gin.TestMode)

	t.Setenv("FRONTEND_BASE_URL", "/frontend-under-test")
	originalIsMasterNode := common.IsMasterNode
	common.IsMasterNode = false
	t.Cleanup(func() { common.IsMasterNode = originalIsMasterNode })

	engine := gin.New()
	require.NotPanics(t, func() { SetRouter(engine, WebAssets{}) })

	registered := 0
	for _, route := range engine.Routes() {
		if route.Method == http.MethodPost && route.Path == "/v1/responses" {
			registered++
		}
	}
	assert.Equal(t, 1, registered, "/v1/responses must be registered exactly once")
}
