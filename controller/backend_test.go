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

func clearBackendUpdateOptions(t *testing.T) {
	t.Helper()
	previousOptions := common.OptionMap
	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
		service.ResetBackendDownloadJob()
	})
	for {
		select {
		case <-service.ProcessRestartRequests():
			continue
		default:
		}
		break
	}
}

// TestStartBackendUpdateDownloadFailsWithoutConfiguredManifest pins the contract
// that makes the UI usable: the endpoint returns immediately and the failure is
// observable through the job snapshot instead of hanging the HTTP request.
func TestStartBackendUpdateDownloadFailsWithoutConfiguredManifest(t *testing.T) {
	clearBackendUpdateOptions(t)

	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/backend-update/download", nil)
	StartBackendUpdateDownload(context)

	require.Equal(t, http.StatusAccepted, response.Code)

	var job service.BackendDownloadJob
	require.Eventually(t, func() bool {
		job = service.BackendDownloadJobSnapshot()
		return job.State == service.BackendDownloadFailed
	}, 5*time.Second, 20*time.Millisecond, "a missing manifest must surface as a failed job")

	assert.Equal(t, "后端更新清单地址未配置", job.Error)

	select {
	case <-service.ProcessRestartRequests():
		t.Fatal("a failed download must not request a restart")
	default:
	}
}

// TestRestartBackendRequestsProcessRestart keeps the restart decoupled from the
// download: it is the only endpoint allowed to hand control to the launcher.
func TestRestartBackendRequestsProcessRestart(t *testing.T) {
	clearBackendUpdateOptions(t)

	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/backend-update/restart", nil)
	RestartBackend(context)

	require.Equal(t, http.StatusOK, response.Code)
	select {
	case <-service.ProcessRestartRequests():
	case <-time.After(2 * time.Second):
		t.Fatal("the restart endpoint must request a process restart")
	}
}
