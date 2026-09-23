package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// BackendDownloadState describes where an asynchronous backend download is.
//
// The update is deliberately split so the operator sees the download finish
// before anything restarts: the download stages a release under `versions/`
// and points the `pending` link at it, and the restart endpoint is what hands
// control to the launcher.
type BackendDownloadState string

const (
	BackendDownloadIdle        BackendDownloadState = "idle"
	BackendDownloadFetching    BackendDownloadState = "fetching"
	BackendDownloadDownloading BackendDownloadState = "downloading"
	BackendDownloadVerifying   BackendDownloadState = "verifying"
	BackendDownloadInstalling  BackendDownloadState = "installing"
	BackendDownloadReady       BackendDownloadState = "ready"
	BackendDownloadFailed      BackendDownloadState = "failed"
)

// progressReportInterval throttles byte-level progress writes: io.Copy calls the
// reader with a 32KB buffer, which would otherwise take the job lock thousands
// of times per artifact.
const progressReportInterval = 250 * time.Millisecond

// ErrBackendDownloadInProgress is returned when a download is already running.
// It is a conflict rather than an error to wait on: the previous implementation
// serialized callers behind a mutex, so a second click appeared to hang.
var ErrBackendDownloadInProgress = errors.New("已有后端更新下载任务正在进行")

// BackendDownloadJob is the observable progress of a backend download. It is
// process-local; after a restart the staged release is recovered from the
// `pending` link via BackendUpdateStatusSnapshot.
type BackendDownloadJob struct {
	State      BackendDownloadState `json:"state"`
	Version    string               `json:"version,omitempty"`
	Commit     string               `json:"commit,omitempty"`
	BytesDone  int64                `json:"bytes_done"`
	BytesTotal int64                `json:"bytes_total"`
	Percent    float64              `json:"percent"`
	Message    string               `json:"message,omitempty"`
	Error      string               `json:"error,omitempty"`
	StartedAt  int64                `json:"started_at"`
	FinishedAt int64                `json:"finished_at"`
}

// BackendDownloadReporter receives coarse download progress. Implementations
// must not block: this runs on the artifact copy path.
type BackendDownloadReporter interface {
	Target(version, commit string, totalBytes int64)
	Progress(doneBytes, totalBytes int64)
	Stage(stage BackendDownloadState, message string)
}

type backendDownloadTracker struct {
	mu         sync.Mutex
	running    bool
	job        BackendDownloadJob
	lastReport time.Time
}

var defaultBackendDownloadTracker = &backendDownloadTracker{
	job: BackendDownloadJob{State: BackendDownloadIdle},
}

// claim atomically takes ownership of the download slot.
func (t *backendDownloadTracker) claim() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.running {
		return false
	}
	t.running = true
	t.lastReport = time.Time{}
	t.job = BackendDownloadJob{
		State:     BackendDownloadFetching,
		Message:   "正在获取后端更新清单",
		StartedAt: common.GetTimestamp(),
	}
	return true
}

func (t *backendDownloadTracker) fail(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.running = false
	message := ""
	if err != nil {
		message = err.Error()
	}
	t.job.State = BackendDownloadFailed
	t.job.Error = message
	t.job.Message = ""
	t.job.FinishedAt = common.GetTimestamp()
}

func (t *backendDownloadTracker) Target(version, commit string, totalBytes int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.job.State = BackendDownloadDownloading
	t.job.Message = "正在下载后端更新制品"
	t.job.Version = version
	t.job.Commit = commit
	t.job.BytesDone = 0
	t.job.BytesTotal = totalBytes
	t.job.Percent = 0
}

func (t *backendDownloadTracker) Progress(doneBytes, totalBytes int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.lastReport.IsZero() && time.Since(t.lastReport) < progressReportInterval {
		return
	}
	t.lastReport = time.Now()
	t.job.BytesDone = doneBytes
	if totalBytes > 0 {
		t.job.BytesTotal = totalBytes
		t.job.Percent = float64(doneBytes) / float64(totalBytes) * 100
		if t.job.Percent > 100 {
			t.job.Percent = 100
		}
	}
}

func (t *backendDownloadTracker) Stage(stage BackendDownloadState, message string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.job.State = stage
	t.job.Message = message
	if stage != BackendDownloadDownloading && t.job.BytesTotal > 0 {
		t.job.BytesDone = t.job.BytesTotal
		t.job.Percent = 100
	}
}

func (t *backendDownloadTracker) finish(version, commit string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.running = false
	t.job.State = BackendDownloadReady
	t.job.Message = "新版本已就绪，重启后生效"
	t.job.Error = ""
	t.job.Version = version
	t.job.Commit = commit
	if t.job.BytesTotal > 0 {
		t.job.BytesDone = t.job.BytesTotal
		t.job.Percent = 100
	}
	t.job.FinishedAt = common.GetTimestamp()
}

func (t *backendDownloadTracker) snapshot() BackendDownloadJob {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.job
}

// reset returns the tracker to idle after the staged release is discarded.
func (t *backendDownloadTracker) reset() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.running {
		return false
	}
	t.job = BackendDownloadJob{State: BackendDownloadIdle}
	return true
}

// BackendDownloadJobSnapshot returns the current progress without blocking.
func BackendDownloadJobSnapshot() BackendDownloadJob {
	return defaultBackendDownloadTracker.snapshot()
}

// StartConfiguredBackendDownload begins downloading the configured backend
// release in the background and returns the initial job state immediately, so
// the caller is never blocked for the duration of the transfer.
func StartConfiguredBackendDownload() (BackendDownloadJob, error) {
	tracker := defaultBackendDownloadTracker
	if !tracker.claim() {
		return tracker.snapshot(), ErrBackendDownloadInProgress
	}

	// The request context is cancelled as soon as the handler returns, so the
	// download must own its own deadline.
	ctx, cancel := context.WithTimeout(context.Background(), backendUpdateTimeout)

	go func() {
		defer cancel()
		runConfiguredBackendDownload(ctx, tracker)
	}()

	return tracker.snapshot(), nil
}

func runConfiguredBackendDownload(ctx context.Context, tracker *backendDownloadTracker) {
	updater := getDefaultBackendUpdater()
	previousReporter := updater.Reporter
	updater.Reporter = tracker
	defer func() {
		updater.Reporter = previousReporter
	}()

	result, err := UpdateConfiguredBackend(ctx)
	if err != nil {
		common.SysError("backend update download failed: " + err.Error())
		tracker.fail(err)
		return
	}
	tracker.finish(result.Version, result.Commit)
}

// ResetBackendDownloadJob clears a finished job after its staged release was
// discarded, so the UI returns to "update available".
func ResetBackendDownloadJob() {
	defaultBackendDownloadTracker.reset()
}

// DiscardBackendDownload drops the staged-but-not-activated release by removing
// the `pending` link, leaving the running version untouched.
func DiscardBackendDownload() (bool, error) {
	removed, err := DiscardPendingBackend(BackendUpdateDir())
	if err != nil {
		return false, err
	}
	defaultBackendDownloadTracker.reset()
	return removed, nil
}

// backendDownloadProgressMessage keeps the stage copy in one place.
func backendDownloadProgressMessage(stage BackendDownloadState) string {
	switch stage {
	case BackendDownloadVerifying:
		return "正在校验后端更新制品"
	case BackendDownloadInstalling:
		return "正在安装后端更新制品"
	default:
		return strings.TrimSpace(string(stage))
	}
}
