package model

import (
	"context"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type quotaDataRepairScanReport struct {
	ScannedLogs     int64
	InvalidLogOther int64
	InputTokens     int64
	CachedTokens    int64
	ReasoningTokens int64
}

func collectQuotaDataRepairLogs(ctx context.Context, channelID int, start, end int64) (map[quotaDataRepairKey]quotaDataRepairAggregate, quotaDataRepairScanReport, error) {
	aggregates := make(map[quotaDataRepairKey]quotaDataRepairAggregate)
	scanReport := quotaDataRepairScanReport{}
	if err := ctx.Err(); err != nil {
		return nil, scanReport, err
	}
	rows, err := LOG_DB.WithContext(ctx).Model(&Log{}).
		Select(quotaDataRepairLogSelectColumnsForDB(LOG_DB)).
		Where("type = ? AND channel_id = ? AND created_at >= ? AND created_at < ?", LogTypeConsume, channelID, start, end).
		Order("created_at ASC").Rows()
	if err != nil {
		return nil, scanReport, err
	}
	defer rows.Close()

	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, scanReport, err
		}
		var logRow quotaDataRepairLogRow
		if err := LOG_DB.ScanRows(rows, &logRow); err != nil {
			return nil, scanReport, err
		}
		key := quotaDataRepairKey{
			UserID:    logRow.UserID,
			Username:  logRow.Username,
			ModelName: logRow.ModelName,
			CreatedAt: logRow.CreatedAt - logRow.CreatedAt%3600,
			UseGroup:  logRow.UseGroup,
			TokenID:   logRow.TokenID,
			ChannelID: logRow.ChannelID,
		}
		aggregate := aggregates[key]
		aggregate.Count = addSignedInt64(aggregate.Count, 1)
		aggregate.Quota = addSignedInt64(aggregate.Quota, int64(logRow.Quota))
		aggregate.TokenUsed = addPositiveTokenCount(aggregate.TokenUsed, logRow.PromptTokens)
		aggregate.TokenUsed = addPositiveTokenCount(aggregate.TokenUsed, logRow.CompletionTokens)

		other, valid := quotaDataRepairLogDetails(logRow.Other)
		if !valid {
			scanReport.InvalidLogOther++
		}
		aggregate.InputTokens = addPositiveTokenCount64(
			aggregate.InputTokens,
			quotaDataRepairInputTokens(logRow.PromptTokens, other),
		)
		if other.CacheTokens > 0 {
			cached := int64(common.QuotaFromFloat(other.CacheTokens))
			aggregate.CachedTokens = addPositiveTokenCount64(aggregate.CachedTokens, cached)
		}
		reasoningTokens := int64(logRow.ReasoningTokens)
		if reasoningTokens <= 0 && other.ReasoningTokens > 0 {
			reasoningTokens = int64(common.QuotaFromFloat(other.ReasoningTokens))
		}
		aggregate.ReasoningTokens = addPositiveTokenCount64(aggregate.ReasoningTokens, reasoningTokens)
		aggregates[key] = aggregate
		scanReport.ScannedLogs++
	}
	if err := rows.Err(); err != nil {
		return nil, scanReport, err
	}
	for _, aggregate := range aggregates {
		scanReport.InputTokens = addPositiveTokenCount64(scanReport.InputTokens, aggregate.InputTokens)
		scanReport.CachedTokens = addPositiveTokenCount64(scanReport.CachedTokens, aggregate.CachedTokens)
		scanReport.ReasoningTokens = addPositiveTokenCount64(scanReport.ReasoningTokens, aggregate.ReasoningTokens)
	}
	return aggregates, scanReport, nil
}

func quotaDataRepairLogDetails(other string) (quotaDataRepairLogOther, bool) {
	if other == "" {
		return quotaDataRepairLogOther{}, true
	}
	var details quotaDataRepairLogOther
	if err := common.UnmarshalJsonStr(other, &details); err != nil {
		return quotaDataRepairLogOther{}, false
	}
	for _, value := range []float64{
		details.CacheTokens,
		details.InputTokensTotal,
		details.CacheCreationTokens,
		details.CacheCreationTokens5m,
		details.CacheCreationTokens1h,
		details.ReasoningTokens,
	} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return quotaDataRepairLogOther{}, false
		}
	}
	return details, true
}

func quotaDataRepairInputTokens(promptTokens int, details quotaDataRepairLogOther) int64 {
	if details.InputTokensTotal > 0 {
		return int64(common.QuotaFromFloat(details.InputTokensTotal))
	}
	total := addPositiveTokenCount(0, promptTokens)
	if !details.isClaude() {
		return total
	}
	// Native Claude usage reports prompt_tokens excluding cache read/write
	// tokens. The split 5m/1h values and aggregate value describe the same
	// cache writes, so use the larger representation rather than adding twice.
	total = addPositiveTokenCount64(total, int64(common.QuotaFromFloat(details.CacheTokens)))
	total = addPositiveTokenCount64(total, quotaDataRepairCacheCreationTokens(details))
	return total
}

func (details quotaDataRepairLogOther) isClaude() bool {
	return details.Claude || strings.EqualFold(strings.TrimSpace(details.UsageSemantic), "anthropic")
}

func quotaDataRepairCacheCreationTokens(details quotaDataRepairLogOther) int64 {
	aggregate := int64(common.QuotaFromFloat(details.CacheCreationTokens))
	split := addPositiveTokenCount64(
		int64(common.QuotaFromFloat(details.CacheCreationTokens5m)),
		int64(common.QuotaFromFloat(details.CacheCreationTokens1h)),
	)
	if split > aggregate {
		return split
	}
	return aggregate
}
