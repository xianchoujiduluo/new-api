package controller

import (
	"errors"
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

// StartBackendUpdateDownload stages the configured release in the background and
// returns immediately. The caller polls GetBackendUpdateDownload for progress;
// nothing restarts until RestartBackend is called explicitly.
func StartBackendUpdateDownload(c *gin.Context) {
	job, err := service.StartConfiguredBackendDownload()
	if err != nil {
		if errors.Is(err, service.ErrBackendDownloadInProgress) {
			c.JSON(http.StatusConflict, gin.H{
				"success": false,
				"message": err.Error(),
				"data":    job,
			})
			return
		}
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"success": true, "message": "", "data": job})
}

func GetBackendUpdateDownload(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    service.BackendDownloadJobSnapshot(),
	})
}

// DiscardBackendUpdate drops a staged release without restarting, so a mistaken
// download does not require a restart-and-rollback cycle.
func DiscardBackendUpdate(c *gin.Context) {
	removed, err := service.DiscardBackendDownload()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    gin.H{"discarded": removed},
	})
}

// RestartBackend hands control to the launcher, which promotes the staged
// release and rolls back automatically if the candidate fails health checks.
func RestartBackend(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": nil})
	service.RequestProcessRestart()
}

// StageBackendRollbackOnly stages the previous release without restarting.
func StageBackendRollbackOnly(c *gin.Context) {
	result, err := service.StageBackendRollback(service.BackendUpdateDir())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": result})
}
