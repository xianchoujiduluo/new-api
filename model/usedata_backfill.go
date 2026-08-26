package model

import (
	"context"
	"errors"
	"math"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

type QuotaTokenBackfillWindowResult struct {
	ScannedLogs      int64 `json:"scanned_logs"`
	UpdatedGroups    int64 `json:"updated_groups"`
	UnmatchedGroups  int64 `json:"unmatched_groups"`
	AmbiguousGroups  int64 `json:"ambiguous_groups"`
	MismatchedGroups int64 `json:"mismatched_groups"`
	InvalidLogOther  int64 `json:"invalid_log_other"`
}

type quotaTokenBackfillKey struct {
	UserID    int
	Username  string
	ModelName string
	CreatedAt int64
	UseGroup  string
	TokenID   int
	ChannelID int
}

type quotaTokenBackfillValue struct {
	Count           int64
	Quota           int64
	TokenUsed       int64
	InputTokens     int64
	CachedTokens    int64
	ReasoningTokens int64
}

type quotaTokenBackfillScanResult struct {
	Aggregates      map[quotaTokenBackfillKey]quotaTokenBackfillValue
	ScannedLogs     int64
	InvalidLogOther int64
}

func FindNextConsumeLogTimestamp(ctx context.Context, startTimestamp int64, endTimestamp int64) (int64, bool, error) {
	if LOG_DB == nil {
		return 0, false, errors.New("log database is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var log Log
	err := LOG_DB.WithContext(ctx).
		Model(&Log{}).
		Select([]string{"created_at"}).
		Where("type = ? AND created_at >= ? AND created_at <= ?", LogTypeConsume, startTimestamp, endTimestamp).
		Order("created_at ASC").
		Limit(1).
		First(&log).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return log.CreatedAt, true, nil
}

func BackfillQuotaTokenDetailsWindow(ctx context.Context, startTimestamp int64, endTimestamp int64) (QuotaTokenBackfillWindowResult, error) {
	result := QuotaTokenBackfillWindowResult{}
	if LOG_DB == nil || DB == nil {
		return result, errors.New("database is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	scanResult, err := scanQuotaTokenBackfillLogs(ctx, startTimestamp, endTimestamp)
	if err != nil {
		return result, err
	}
	result.ScannedLogs = scanResult.ScannedLogs
	result.InvalidLogOther = scanResult.InvalidLogOther
	if len(scanResult.Aggregates) == 0 {
		return result, nil
	}
	applyResult, err := applyQuotaTokenBackfillAggregates(ctx, scanResult.Aggregates, startTimestamp, endTimestamp)
	if err != nil {
		return result, err
	}
	result.UpdatedGroups = applyResult.UpdatedGroups
	result.UnmatchedGroups = applyResult.UnmatchedGroups
	result.AmbiguousGroups = applyResult.AmbiguousGroups
	result.MismatchedGroups = applyResult.MismatchedGroups
	return result, nil
}

func scanQuotaTokenBackfillLogs(ctx context.Context, startTimestamp int64, endTimestamp int64) (quotaTokenBackfillScanResult, error) {
	scanResult := quotaTokenBackfillScanResult{
		Aggregates: make(map[quotaTokenBackfillKey]quotaTokenBackfillValue),
	}
	rows, err := LOG_DB.WithContext(ctx).
		Model(&Log{}).
		Select(quotaDataRepairLogSelectColumnsForDB(LOG_DB)).
		Where("type = ? AND created_at >= ? AND created_at < ?", LogTypeConsume, startTimestamp, endTimestamp).
		Order("created_at ASC").
		Rows()
	if err != nil {
		return scanResult, err
	}
	defer rows.Close()

	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return scanResult, err
		}
		var log Log
		if err := LOG_DB.ScanRows(rows, &log); err != nil {
			return scanResult, err
		}
		key := quotaTokenBackfillKey{
			UserID:    log.UserId,
			Username:  log.Username,
			ModelName: log.ModelName,
			CreatedAt: log.CreatedAt - log.CreatedAt%3600,
			UseGroup:  log.Group,
			TokenID:   log.TokenId,
			ChannelID: log.ChannelId,
		}
		value := scanResult.Aggregates[key]
		if !accumulateQuotaTokenBackfillLog(&value, log) {
			scanResult.InvalidLogOther++
		}
		scanResult.Aggregates[key] = value
		scanResult.ScannedLogs++
	}
	if err := rows.Err(); err != nil {
		return scanResult, err
	}
	return scanResult, nil
}

func accumulateQuotaTokenBackfillLog(value *quotaTokenBackfillValue, log Log) bool {
	if value == nil {
		return true
	}
	value.Count = addSignedInt64(value.Count, 1)
	value.Quota = addSignedInt64(value.Quota, int64(log.Quota))
	value.TokenUsed = addPositiveTokenCount(value.TokenUsed, log.PromptTokens)
	value.TokenUsed = addPositiveTokenCount(value.TokenUsed, log.CompletionTokens)
	logDetails, valid := quotaDataRepairLogDetails(log.Other)
	value.InputTokens = addPositiveTokenCount64(value.InputTokens, quotaDataRepairInputTokens(log.PromptTokens, logDetails))
	if logDetails.CacheTokens > 0 {
		value.CachedTokens = addPositiveTokenCount64(value.CachedTokens, int64(common.QuotaFromFloat(logDetails.CacheTokens)))
	}
	reasoningTokens := int64(log.ReasoningTokens)
	if reasoningTokens <= 0 && logDetails.ReasoningTokens > 0 {
		reasoningTokens = int64(common.QuotaFromFloat(logDetails.ReasoningTokens))
	}
	value.ReasoningTokens = addPositiveTokenCount64(value.ReasoningTokens, reasoningTokens)
	return valid
}

func addPositiveTokenCount(total int64, value int) int64 {
	if value <= 0 {
		return total
	}
	return addPositiveTokenCount64(total, int64(value))
}

func addPositiveTokenCount64(total int64, value int64) int64 {
	if value <= 0 {
		return total
	}
	if total > math.MaxInt64-value {
		return math.MaxInt64
	}
	return total + value
}
