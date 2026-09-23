package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStartFrontendUpdateDownloadFailsWithoutConfiguredURL pins the contract
// that makes the UI usable: the endpoint returns immediately and the failure is
// observable through the job snapshot instead of hanging the HTTP request.
func TestStartFrontendUpdateDownloadFailsWithoutConfiguredURL(t *testing.T) {
	previousOptions := common.OptionMap
	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})

	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(
		http.MethodPost, "/api/frontend/download", nil)

	StartFrontendUpdateDownload(context)

	require.Equal(t, http.StatusAccepted, response.Code)

	var job service.FrontendDownloadJob
	require.Eventually(t, func() bool {
		job = service.FrontendDownloadJobSnapshot()
		return job.State == service.FrontendStateFailed
	}, 5*time.Second, 20*time.Millisecond,
		"a missing download URL must surface as a failed job")

	assert.Equal(t, "前端下载地址未配置", job.Error)
}
