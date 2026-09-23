package controller

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// StartFrontendUpdateDownload stages the configured frontend archive in the
// background and returns immediately. The caller polls GetFrontendUpdateDownload
// for progress; the live assets are untouched until ActivateFrontend is called.
// The route is protected by RootAuth and deliberately accepts no URL in the
// request body.
func StartFrontendUpdateDownload(c *gin.Context) {
	job, err := service.StartConfiguredFrontendDownload()
	if err != nil {
		if errors.Is(err, service.ErrFrontendDownloadInProgress) {
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

func GetFrontendUpdateDownload(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    service.FrontendDownloadJobSnapshot(),
	})
}

// ActivateFrontend swaps the staged release into place. It is a directory
// rename and returns immediately.
func ActivateFrontend(c *gin.Context) {
	if err := service.ActivateStagedFrontend(); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": nil})
}

// DiscardFrontendUpdate drops a staged release without touching live assets.
func DiscardFrontendUpdate(c *gin.Context) {
	removed, err := service.DiscardStagedFrontend()
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
