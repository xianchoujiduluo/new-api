package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// logQuotaRecalcRequest is the payload for re-pricing historical consume logs of
// one model over one time range.
type logQuotaRecalcRequest struct {
	ModelName      string `json:"model_name"`
	StartTimestamp int64  `json:"start_timestamp"`
	EndTimestamp   int64  `json:"end_timestamp"`
	// Apply is false by default so an accidental call only reports the deltas.
	Apply bool `json:"apply"`
}

// RecalculateLogQuota re-prices consume logs against the current billing
// settings. It is a root-only maintenance endpoint: the operation rewrites
// historical quota values in place, which cannot be undone from the UI.
func RecalculateLogQuota(c *gin.Context) {
	var request logQuotaRecalcRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return
	}

	report, err := model.RecalculateLogQuota(c.Request.Context(), model.LogQuotaRecalcRequest{
		ModelName:      request.ModelName,
		StartTimestamp: request.StartTimestamp,
		EndTimestamp:   request.EndTimestamp,
		Apply:          request.Apply,
	}, service.RecomputeConsumeLogQuota)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, report)
}
