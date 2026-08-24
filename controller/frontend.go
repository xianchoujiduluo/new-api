package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// UpdateFrontend downloads and atomically activates the configured frontend
// archive. The route is protected by RootAuth and deliberately accepts no URL
// in the request body.
func UpdateFrontend(c *gin.Context) {
	if err := service.UpdateConfiguredFrontend(c.Request.Context()); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}
