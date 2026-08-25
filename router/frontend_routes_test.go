package router

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestFrontendUpdateRouteIsRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)

	for _, route := range engine.Routes() {
		if route.Method == http.MethodPost && route.Path == "/api/frontend/update" {
			assert.NotEmpty(t, route.Handler)
			return
		}
	}
	t.Fatalf("frontend update route is not registered")
}

func TestBackendUpdateRoutesAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)

	want := map[string]bool{
		http.MethodGet + " /api/backend-update/status":      false,
		http.MethodPost + " /api/backend-update/check":    false,
		http.MethodPost + " /api/backend-update/apply":    false,
		http.MethodPost + " /api/backend-update/rollback": false,
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
