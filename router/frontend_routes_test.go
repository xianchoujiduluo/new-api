package router

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestFrontendUpdateRoutesAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)

	// Download, poll, activate and discard are separate routes so an operator
	// can inspect a staged release before it replaces the live frontend.
	want := map[string]bool{
		http.MethodPost + " /api/frontend/download":  false,
		http.MethodGet + " /api/frontend/download":   false,
		http.MethodPost + " /api/frontend/activate":  false,
		http.MethodDelete + " /api/frontend/staging": false,
	}
	for _, route := range engine.Routes() {
		key := route.Method + " " + route.Path
		if _, ok := want[key]; ok {
			assert.NotEmpty(t, route.Handler)
			want[key] = true
		}
	}
	for key, found := range want {
		assert.True(t, found, "route is not registered: %s", key)
	}
}

func TestBackendUpdateRoutesAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)

	want := map[string]bool{
		http.MethodGet + " /api/backend-update/status":     false,
		http.MethodPost + " /api/backend-update/check":     false,
		http.MethodPost + " /api/backend-update/download":  false,
		http.MethodGet + " /api/backend-update/download":   false,
		http.MethodDelete + " /api/backend-update/pending": false,
		http.MethodPost + " /api/backend-update/restart":   false,
		http.MethodPost + " /api/backend-update/rollback":  false,
	}
	for _, route := range engine.Routes() {
		key := route.Method + " " + route.Path
		if _, ok := want[key]; ok {
			assert.NotEmpty(t, route.Handler)
			want[key] = true
		}
	}
	for route, registered := range want {
		assert.True(t, registered, "%s is not registered", route)
	}
}
