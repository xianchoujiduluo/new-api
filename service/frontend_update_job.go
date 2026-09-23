package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// FrontendDownloadState describes where an asynchronous frontend download is.
//
// Unlike the backend, a frontend release needs no process restart: staging
// extracts the archive into a stable directory next to the live one, and
// activation is a directory swap. Splitting the two keeps the slow step (the
// transfer) observable and the disruptive step (swapping live assets)
// instantaneous and explicit.
type FrontendDownloadState string

const (
	FrontendStateIdle        FrontendDownloadState = "idle"
	FrontendStateDownloading FrontendDownloadState = "downloading"
	FrontendStateExtracting  FrontendDownloadState = "extracting"
	FrontendStateReady       FrontendDownloadState = "ready"
	FrontendStateFailed      FrontendDownloadState = "failed"
)

// ErrFrontendDownloadInProgress is returned when a transfer is already running.
var ErrFrontendDownloadInProgress = errors.New("已有前端更新下载任务正在进行")

// FrontendDownloadJob is the observable progress of a frontend download.
type FrontendDownloadJob struct {
	State      FrontendDownloadState `json:"state"`
	Source     string                `json:"source,omitempty"`
	BytesDone  int64                 `json:"bytes_done"`
	BytesTotal int64                 `json:"bytes_total"`
	Percent    float64               `json:"percent"`
	Message    string                `json:"message,omitempty"`
	Error      string                `json:"error,omitempty"`
	StagedAt   int64                 `json:"staged_at"`
	FinishedAt int64                 `json:"finished_at"`
}

// FrontendDownloadReporter observes a staged frontend archive. Implementations
// must not block: Progress runs on the archive copy path.
type FrontendDownloadReporter interface {
	Total(totalBytes int64, source string)
	Progress(doneBytes, totalBytes int64)
	Stage(stage FrontendDownloadState, message string)
}

type frontendDownloadTracker struct {
	mu         sync.Mutex
	running    bool
	job        FrontendDownloadJob
	lastReport time.Time
}

var defaultFrontendDownloadTracker = &frontendDownloadTracker{
	job: FrontendDownloadJob{State: FrontendStateIdle},
}

func (t *frontendDownloadTracker) claim() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.running {
		return false
	}
	t.running = true
	t.lastReport = time.Time{}
	t.job = FrontendDownloadJob{
		State:   FrontendStateDownloading,
		Message: "正在下载前端压缩包",
	}
	return true
}

func (t *frontendDownloadTracker) Total(totalBytes int64, source string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.job.State = FrontendStateDownloading
	t.job.Message = "正在下载前端压缩包"
	t.job.Source = source
	t.job.BytesDone = 0
	t.job.BytesTotal = totalBytes
	t.job.Percent = 0
}

func (t *frontendDownloadTracker) Progress(doneBytes, totalBytes int64) {
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

func (t *frontendDownloadTracker) Stage(stage FrontendDownloadState, message string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.job.State = stage
	t.job.Message = message
	if stage != FrontendStateDownloading && t.job.BytesTotal > 0 {
		t.job.BytesDone = t.job.BytesTotal
		t.job.Percent = 100
	}
}

func (t *frontendDownloadTracker) finish(source string, stagedAt int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.running = false
	t.job.State = FrontendStateReady
	t.job.Message = "前端已就绪，激活后生效"
	t.job.Error = ""
	t.job.Source = source
	t.job.StagedAt = stagedAt
	if t.job.BytesTotal > 0 {
		t.job.BytesDone = t.job.BytesTotal
		t.job.Percent = 100
	}
	t.job.FinishedAt = common.GetTimestamp()
}

func (t *frontendDownloadTracker) fail(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.running = false
	message := ""
	if err != nil {
		message = err.Error()
	}
	t.job.State = FrontendStateFailed
	t.job.Error = message
	t.job.Message = ""
	t.job.FinishedAt = common.GetTimestamp()
}

func (t *frontendDownloadTracker) snapshot() FrontendDownloadJob {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.job
}

// reset returns the tracker to idle after the staged release is discarded.
func (t *frontendDownloadTracker) reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.running {
		return
	}
	t.job = FrontendDownloadJob{State: FrontendStateIdle}
}

// FrontendDownloadJobSnapshot returns the in-process progress, falling back to
// the on-disk staged release when no transfer is tracked (for example after a
// restart, which clears the in-process job).
func FrontendDownloadJobSnapshot() FrontendDownloadJob {
	job := defaultFrontendDownloadTracker.snapshot()
	if job.State != FrontendStateIdle {
		return job
	}
	staged := StagedFrontendRelease(FrontendAssetsDir())
	if !staged.Staged {
		return job
	}
	return FrontendDownloadJob{
		State:    FrontendStateReady,
		Message:  "前端已就绪，激活后生效",
		Source:   staged.Source,
		StagedAt: staged.StagedAt,
	}
}

// StartConfiguredFrontendDownload stages the configured frontend archive in the
// background and returns the initial state immediately.
func StartConfiguredFrontendDownload() (FrontendDownloadJob, error) {
	tracker := defaultFrontendDownloadTracker
	if !tracker.claim() {
		return FrontendDownloadJobSnapshot(), ErrFrontendDownloadInProgress
	}

	// The request context dies with the handler, so the transfer owns its own.
	ctx, cancel := context.WithTimeout(context.Background(), frontendUpdateTimeout)

	go func() {
		defer cancel()
		runConfiguredFrontendDownload(ctx, tracker)
	}()

	return tracker.snapshot(), nil
}

func runConfiguredFrontendDownload(ctx context.Context, tracker *frontendDownloadTracker) {
	updater := getDefaultFrontendUpdater()
	previousReporter := updater.Reporter
	updater.Reporter = tracker
	defer func() {
		updater.Reporter = previousReporter
	}()

	err := updater.Stage(ctx, ConfiguredFrontendDownloadURL())
	if err != nil {
		common.SysError("frontend update staging failed: " + err.Error())
		tracker.fail(err)
		return
	}
	staged := StagedFrontendRelease(updater.Root)
	tracker.finish(staged.Source, staged.StagedAt)
}

// ActivateStagedFrontend swaps the live assets for the staged release. It is a
// directory rename, so the disruptive step stays instantaneous.
func ActivateStagedFrontend() error {
	if err := getDefaultFrontendUpdater().Activate(); err != nil {
		return err
	}
	defaultFrontendDownloadTracker.reset()
	return nil
}

// DiscardStagedFrontend removes a staged release without touching the live one.
func DiscardStagedFrontend() (bool, error) {
	updater := getDefaultFrontendUpdater()
	staging := FrontendStagingDir(updater.Root)
	if _, err := os.Lstat(staging); err != nil {
		if os.IsNotExist(err) {
			defaultFrontendDownloadTracker.reset()
			return false, nil
		}
		return false, fmt.Errorf("读取暂存前端目录失败: %w", err)
	}
	if err := os.RemoveAll(staging); err != nil {
		return false, fmt.Errorf("丢弃暂存前端目录失败: %w", err)
	}
	removeFrontendStagingMeta(updater.Root)
	defaultFrontendDownloadTracker.reset()
	return true, nil
}
