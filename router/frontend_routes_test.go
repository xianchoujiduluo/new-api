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
