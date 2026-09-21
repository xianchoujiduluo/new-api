package model

import (
	"context"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// LogQuotaRecalcRequest describes a bounded, manual re-pricing of consume logs.
//
// The contract deliberately mirrors QuotaDataRepairRequest: a dry run (Apply
// false) computes everything and reports the outcome without writing, so an
// operator can inspect the deltas before committing.
type LogQuotaRecalcRequest struct {
	ModelName      string
	StartTimestamp int64
	EndTimestamp   int64
	Apply          bool
}

// LogQuotaRecalcReport summarizes what a dry run would do, or what an applied
// run did. OldQuotaSum/NewQuotaSum only cover logs that were recomputable.
type LogQuotaRecalcReport struct {
	ModelName      string `json:"model_name"`
	StartTimestamp int64  `json:"start_timestamp"`
	EndTimestamp   int64  `json:"end_timestamp"`
	Applied        bool   `json:"applied"`

	ScannedLogs      int64 `json:"scanned_logs"`
	RecomputableLogs int64 `json:"recomputable_logs"`
	SkippedLogs      int64 `json:"skipped_logs"`
	ChangedLogs      int64 `json:"changed_logs"`
	UpdatedLogs      int64 `json:"updated_logs"`

	OldQuotaSum int64 `json:"old_quota_sum"`
	NewQuotaSum int64 `json:"new_quota_sum"`

	// SkipReasons counts why individual logs were left untouched, keyed by a
	// stable machine-readable reason.
	SkipReasons map[string]int64 `json:"skip_reasons"`

	// AffectedBuckets are the quota_data rows whose quota column was recomputed.
	AffectedBuckets int64 `json:"affected_buckets"`
	UpdatedBuckets  int64 `json:"updated_buckets"`
}

// LogQuotaRecomputer re-prices a single consume log. Implementations live
// outside model to keep billing logic out of the persistence layer.
// Returning ok=false leaves the row untouched and records reason.
type LogQuotaRecomputer func(log *Log) (newQuota int64, ok bool, reason string)

// ErrRecalcRequiresRelationalLogs is returned when the log database cannot
// support in-place updates (ClickHouse DELETE/UPDATE are asynchronous mutations).
var ErrRecalcRequiresRelationalLogs = errors.New("quota recalculation requires a relational log database (SQLite, MySQL, or PostgreSQL)")

const (
	SkipReasonUnparsableOther = "unparsable_log_other"
	SkipReasonUnsupportedBill = "unsupported_billing_mode"
	SkipReasonRequestProbe    = "expression_needs_request_probe"
	SkipReasonNoUsage         = "no_usage_recorded"
	SkipReasonNotConsume      = "not_a_consume_log"

	// recalcBatchSize bounds how many rows are updated per statement so a large
	// range does not hold one long transaction.
	recalcBatchSize = 200
)

// RecalculateLogQuota re-prices consume logs for one model and time range.
func RecalculateLogQuota(ctx context.Context, request LogQuotaRecalcRequest, recompute LogQuotaRecomputer) (LogQuotaRecalcReport, error) {
	report := LogQuotaRecalcReport{
		ModelName:      request.ModelName,
		StartTimestamp: request.StartTimestamp,
		EndTimestamp:   request.EndTimestamp,
		Applied:        request.Apply,
		SkipReasons:    map[string]int64{},
	}
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return report, ErrRecalcRequiresRelationalLogs
	}
	if request.ModelName == "" {
		return report, errors.New("model name is required")
	}
	if request.EndTimestamp <= 0 {
		request.EndTimestamp = common.GetTimestamp()
	}
	if request.StartTimestamp <= 0 || request.StartTimestamp >= request.EndTimestamp {
		return report, errors.New("a valid start timestamp before the end timestamp is required")
	}
	if LOG_DB == nil {
		return report, errors.New("database is not initialized")
	}
	if recompute == nil {
		return report, errors.New("a quota recomputer is required")
	}

	logs, err := listConsumeLogsForRecalc(ctx, request)
	if err != nil {
		return report, err
	}
	report.ScannedLogs = int64(len(logs))

	updates := make([]logQuotaRecalcUpdate, 0, len(logs))
	// touched keys the quota_data buckets that must be re-aggregated.
	touched := make(map[quotaDataRecalcKey]struct{})

	for _, logRow := range logs {
		newQuota, ok, reason := recompute(logRow)
		if !ok {
			if reason == "" {
				reason = SkipReasonUnsupportedBill
			}
			report.SkipReasons[reason]++
			report.SkippedLogs++
			continue
		}
		report.RecomputableLogs++
		report.OldQuotaSum += int64(logRow.Quota)
		report.NewQuotaSum += newQuota
		if newQuota == int64(logRow.Quota) {
			continue
		}
		report.ChangedLogs++
		updates = append(updates, logQuotaRecalcUpdate{id: logRow.Id, quota: int(newQuota)})
		touched[quotaDataRecalcKeyOf(logRow)] = struct{}{}
	}
	report.AffectedBuckets = int64(len(touched))

	if !request.Apply || len(updates) == 0 {
		return report, nil
	}

	if err := applyLogQuotaUpdates(ctx, updates); err != nil {
		return report, err
	}
	report.UpdatedLogs = int64(len(updates))

	updatedBuckets, err := reAggregateQuotaDataBuckets(ctx, touched)
	if err != nil {
		return report, err
	}
	report.UpdatedBuckets = updatedBuckets

	recordQuotaRecalcAudit(&report)
	return report, nil
}

// logQuotaRecalcUpdate is one pending in-place quota correction.
type logQuotaRecalcUpdate struct {
	id    int
	quota int
}

// quotaDataRecalcKey mirrors the quota_data aggregation identity. node_name is
// intentionally excluded: historical consume logs carry no reliable node
// identity, so matching ignores it (same approach as QuotaDataRepair).
type quotaDataRecalcKey struct {
	UserID    int
	Username  string
	ModelName string
	CreatedAt int64
	UseGroup  string
	TokenID   int
	ChannelID int
}

func quotaDataRecalcKeyOf(logRow *Log) quotaDataRecalcKey {
	return quotaDataRecalcKey{
		UserID:    logRow.UserId,
		Username:  logRow.Username,
		ModelName: logRow.ModelName,
		CreatedAt: logRow.CreatedAt - (logRow.CreatedAt % 3600),
		UseGroup:  logRow.Group,
		TokenID:   logRow.TokenId,
		ChannelID: logRow.ChannelId,
	}
}

func listConsumeLogsForRecalc(ctx context.Context, request LogQuotaRecalcRequest) ([]*Log, error) {
	var logs []*Log
	err := LOG_DB.WithContext(ctx).
		Model(&Log{}).
		Where("type = ?", LogTypeConsume).
		Where("model_name = ?", request.ModelName).
		Where("created_at >= ? AND created_at < ?", request.StartTimestamp, request.EndTimestamp).
		Order("id asc").
		Find(&logs).Error
	if err != nil {
		return nil, err
	}
	return logs, nil
}

func applyLogQuotaUpdates(ctx context.Context, updates []logQuotaRecalcUpdate) error {
	// Group ids by the new quota so rows that converge on the same value are
	// updated together, while each statement stays bounded by recalcBatchSize.
	byQuota := make(map[int][]int)
	for _, update := range updates {
		byQuota[update.quota] = append(byQuota[update.quota], update.id)
	}

	return LOG_DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for quota, ids := range byQuota {
			for start := 0; start < len(ids); start += recalcBatchSize {
				end := min(start+recalcBatchSize, len(ids))
				if err := tx.Model(&Log{}).
					Where("id IN ?", ids[start:end]).
					Update("quota", quota).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// reAggregateQuotaDataBuckets overwrites the dashboard buckets that the log
// corrections affect. Tokens are aggregated straight from the logs so the
// dashboard stays consistent with the corrected rows.
func reAggregateQuotaDataBuckets(ctx context.Context, touched map[quotaDataRecalcKey]struct{}) (int64, error) {
	if len(touched) == 0 {
		return 0, nil
	}
	// Flush the in-memory delta cache first: it accumulates increments and would
	// otherwise be added on top of the corrected values, double counting.
	SaveQuotaDataCache()

	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()

	var updated int64
	for key := range touched {
		if err := ctx.Err(); err != nil {
			return updated, err
		}

		var agg struct {
			Quota      int64 `gorm:"column:quota"`
			TokenUsed  int64 `gorm:"column:token_used"`
			Count      int64 `gorm:"column:count"`
			InputToken int64 `gorm:"column:input_tokens"`
			Reasoning  int64 `gorm:"column:reasoning_tokens"`
		}
		// `group` is a reserved word: commonGroupCol quotes it per dialect.
		err := LOG_DB.WithContext(ctx).Model(&Log{}).
			Select("COALESCE(SUM(quota),0) AS quota, COALESCE(SUM(prompt_tokens + completion_tokens),0) AS token_used, COUNT(*) AS count, COALESCE(SUM(prompt_tokens),0) AS input_tokens, COALESCE(SUM(reasoning_tokens),0) AS reasoning_tokens").
			Where("type = ?", LogTypeConsume).
			Where("user_id = ? AND username = ? AND model_name = ?", key.UserID, key.Username, key.ModelName).
			Where("created_at >= ? AND created_at < ?", key.CreatedAt, key.CreatedAt+3600).
			Where(commonGroupCol+" = ? AND token_id = ? AND channel_id = ?", key.UseGroup, key.TokenID, key.ChannelID).
			Scan(&agg).Error
		if err != nil {
			return updated, err
		}

		// cached_tokens is intentionally left alone: it lives in the log's JSON
		// `other` payload, not in an aggregatable column.
		result := DB.WithContext(ctx).Table("quota_data").
			Where("user_id = ? AND username = ? AND model_name = ?", key.UserID, key.Username, key.ModelName).
			Where("created_at = ? AND use_group = ? AND token_id = ? AND channel_id = ?", key.CreatedAt, key.UseGroup, key.TokenID, key.ChannelID).
			Updates(map[string]any{
				"quota":            int(agg.Quota),
				"token_used":       int(agg.TokenUsed),
				"count":            int(agg.Count),
				"input_tokens":     int(agg.InputToken),
				"reasoning_tokens": int(agg.Reasoning),
			})
		if result.Error != nil {
			return updated, result.Error
		}
		updated += result.RowsAffected
	}
	return updated, nil
}

// recordQuotaRecalcAudit writes a system log so an in-place re-pricing stays
// traceable after the original quota values are overwritten.
func recordQuotaRecalcAudit(report *LogQuotaRecalcReport) {
	if report == nil {
		return
	}
	other := NewLogOther()
	other.SetPublic("quota_recalc", map[string]any{
		"model_name":     report.ModelName,
		"start":          report.StartTimestamp,
		"end":            report.EndTimestamp,
		"scanned_logs":   report.ScannedLogs,
		"updated_logs":   report.UpdatedLogs,
		"skipped_logs":   report.SkippedLogs,
		"old_quota_sum":  report.OldQuotaSum,
		"new_quota_sum":  report.NewQuotaSum,
		"updated_bucket": report.UpdatedBuckets,
	})
	entry := &Log{
		UserId:    0,
		Username:  "system",
		CreatedAt: common.GetTimestamp(),
		Type:      LogTypeSystem,
		Content: fmt.Sprintf(
			"请求配额重算：模型 %s，更新 %d 条日志（旧合计 %d，新合计 %d）",
			report.ModelName, report.UpdatedLogs, report.OldQuotaSum, report.NewQuotaSum,
		),
		Other: other.JSONString(),
	}
	if err := createLog(entry); err != nil {
		common.SysError("failed to record quota recalculation audit: " + err.Error())
	}
}

// logQuotaRecalcUpdate is one pending in-place quota correction.
