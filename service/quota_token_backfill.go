package service

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const quotaTokenBackfillWindowSeconds int64 = 60 * 60

type quotaTokenBackfillPayload struct {
	StartTimestamp int64 `json:"start_timestamp"`
	EndTimestamp   int64 `json:"end_timestamp"`
}

type quotaTokenBackfillState struct {
	NextTimestamp    int64 `json:"next_timestamp"`
	ScannedLogs      int64 `json:"scanned_logs"`
	UpdatedGroups    int64 `json:"updated_groups"`
	UnmatchedGroups  int64 `json:"unmatched_groups"`
	AmbiguousGroups  int64 `json:"ambiguous_groups"`
	MismatchedGroups int64 `json:"mismatched_groups"`
	InvalidLogOther  int64 `json:"invalid_log_other"`
}

type quotaTokenBackfillHandler struct{}

func (quotaTokenBackfillHandler) Type() string {
	return model.SystemTaskTypeQuotaTokenBackfillV1
}

func (quotaTokenBackfillHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := quotaTokenBackfillPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		failSystemTask(task, runnerID, err)
		return
	}
	if payload.EndTimestamp <= 0 {
		failSystemTask(task, runnerID, errors.New("end timestamp is required"))
		return
	}

	state := quotaTokenBackfillState{}
	if err := task.DecodeState(&state); err != nil {
		failSystemTask(task, runnerID, err)
		return
	}
	if err := model.ValidateQuotaDataRepairSchema(); err != nil {
		failSystemTask(task, runnerID, err)
		return
	}
	if state.NextTimestamp <= 0 && payload.StartTimestamp > 0 {
		state.NextTimestamp = payload.StartTimestamp
	}
	// Drain this process' pending cache before reading logs. This removes the
	// most common source of double counting when the startup repair runs just
	// after the backend has been upgraded. Other nodes should still be paused
	// for a manual repair, as noted by the standalone command.
	if common.DataExportEnabled {
		model.SaveQuotaDataCache()
	}

	for {
		nextTimestamp, found, err := model.FindNextConsumeLogTimestamp(ctx, state.NextTimestamp, payload.EndTimestamp)
		if err != nil {
			failSystemTask(task, runnerID, err)
			return
		}
		if !found {
			break
		}

		windowStart := nextTimestamp - nextTimestamp%quotaTokenBackfillWindowSeconds
		windowEnd := windowStart + quotaTokenBackfillWindowSeconds
		if windowEnd > payload.EndTimestamp+1 {
			windowEnd = payload.EndTimestamp + 1
		}
		windowResult, err := model.BackfillQuotaTokenDetailsWindow(ctx, windowStart, windowEnd)
		if err != nil {
			failSystemTask(task, runnerID, err)
			return
		}

		state.NextTimestamp = windowEnd
		state.ScannedLogs += windowResult.ScannedLogs
		state.UpdatedGroups += windowResult.UpdatedGroups
		state.UnmatchedGroups += windowResult.UnmatchedGroups
		state.AmbiguousGroups += windowResult.AmbiguousGroups
		state.MismatchedGroups += windowResult.MismatchedGroups
		state.InvalidLogOther += windowResult.InvalidLogOther
		if err := model.UpdateSystemTaskState(task.TaskID, runnerID, state); err != nil {
			logSystemTaskLockError(ctx, task, err)
			return
		}
	}

	if err := model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, state, ""); err != nil {
		common.SysLog("quota token details backfill failed to persist result: " + err.Error())
	}
}

func init() {
	RegisterSystemTaskHandler(quotaTokenBackfillHandler{})
}

func StartQuotaTokenDetailsBackfill() error {
	if !common.IsMasterNode || !common.DataExportEnabled {
		return nil
	}
	// A process crash can leave the task row in running state until the
	// regular runner performs its stale-lock pass. Run that pass before
	// checking for an existing task so a stale backfill is immediately eligible
	// for a retry instead of blocking all future startup repairs.
	if err := model.ExpireStaleSystemTaskLocks(common.GetTimestamp()); err != nil {
		return err
	}
	latestTask, err := model.GetLatestSystemTask(model.SystemTaskTypeQuotaTokenBackfillV1)
	if err != nil {
		return err
	}
	if latestTask != nil && latestTask.Status == model.SystemTaskStatusSucceeded {
		return nil
	}

	now := common.GetTimestamp()
	endTimestamp := quotaTokenBackfillEndTimestamp(now)
	if endTimestamp <= 0 {
		return nil
	}
	startTimestamp, found, err := model.FindNextConsumeLogTimestamp(context.Background(), 0, endTimestamp)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	startTimestamp -= startTimestamp % quotaTokenBackfillWindowSeconds
	_, _, err = EnqueueSystemTask(model.SystemTaskTypeQuotaTokenBackfillV1, quotaTokenBackfillPayload{
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
	})
	return err
}

// quotaTokenBackfillEndTimestamp leaves a settling period before the newest
// repairable hour. A quota-data flush normally runs every few minutes; the
// extra minute accounts for scheduler and network jitter while avoiding an
// absolute overwrite of another node's in-memory increments.
func quotaTokenBackfillEndTimestamp(now int64) int64 {
	if now <= 0 {
		return 0
	}
	intervalMinutes := common.DataExportInterval
	if intervalMinutes < 0 {
		intervalMinutes = 0
	}
	safetySeconds := (int64(intervalMinutes) + 1) * 60
	if safetySeconds < 60 {
		safetySeconds = 60
	}
	boundary := now - now%quotaTokenBackfillWindowSeconds
	for boundary > 0 && now-(boundary-1) < safetySeconds {
		boundary -= quotaTokenBackfillWindowSeconds
	}
	if boundary <= 0 {
		return 0
	}
	return boundary - 1
}
