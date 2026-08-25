package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func GetBackendUpdateStatus(c *gin.Context) {
	status, err := service.BackendUpdateStatusSnapshot(service.BackendUpdateDir())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": status})
}

func CheckBackendUpdate(c *gin.Context) {
	result, err := service.CheckConfiguredBackend(c.Request.Context())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": result})
}

func ApplyBackendUpdate(c *gin.Context) {
	result, err := service.UpdateConfiguredBackend(c.Request.Context())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    result,
	})
	if result.Changed {
		service.RequestProcessRestart()
	}
}

func RollbackBackendUpdate(c *gin.Context) {
	result, err := service.StageBackendRollback(service.BackendUpdateDir())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": result})
	service.RequestProcessRestart()
}
