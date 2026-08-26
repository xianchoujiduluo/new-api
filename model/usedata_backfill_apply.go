package model

import (
	"context"

	"gorm.io/gorm"
)

type quotaTokenBackfillGroupOutcome uint8

const (
	quotaTokenBackfillGroupUnmatched quotaTokenBackfillGroupOutcome = iota
	quotaTokenBackfillGroupAmbiguous
	quotaTokenBackfillGroupMismatched
	quotaTokenBackfillGroupUnchanged
	quotaTokenBackfillGroupUpdated
)

type quotaTokenBackfillApplyResult struct {
	UpdatedGroups    int64
	UnmatchedGroups  int64
	AmbiguousGroups  int64
	MismatchedGroups int64
}

func applyQuotaTokenBackfillAggregates(ctx context.Context, aggregates map[quotaTokenBackfillKey]quotaTokenBackfillValue, startTimestamp int64, endTimestamp int64) (quotaTokenBackfillApplyResult, error) {
	applyResult := quotaTokenBackfillApplyResult{}
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		targets, err := loadQuotaTokenBackfillTargets(tx, aggregates, startTimestamp, endTimestamp)
		if err != nil {
			return err
		}
		for key, value := range aggregates {
			outcome, err := updateQuotaTokenBackfillGroup(tx, key, value, targets)
			if err != nil {
				return err
			}
			switch outcome {
			case quotaTokenBackfillGroupUpdated:
				applyResult.UpdatedGroups++
			case quotaTokenBackfillGroupUnmatched:
				applyResult.UnmatchedGroups++
			case quotaTokenBackfillGroupAmbiguous:
				applyResult.AmbiguousGroups++
			case quotaTokenBackfillGroupMismatched:
				applyResult.MismatchedGroups++
			}
		}
		return nil
	})
	return applyResult, err
}

func loadQuotaTokenBackfillTargets(tx *gorm.DB, aggregates map[quotaTokenBackfillKey]quotaTokenBackfillValue, startTimestamp int64, endTimestamp int64) (map[quotaTokenBackfillKey][]int, error) {
	var quotaRows []QuotaData
	query := tx.Select([]string{
		"id", "user_id", "username", "model_name", "created_at", "use_group", "token_id", "channel_id",
	}).Where("created_at >= ? AND created_at < ?", startTimestamp, endTimestamp)
	channelIDs := make([]int, 0)
	seenChannels := make(map[int]struct{})
	for key := range aggregates {
		if _, ok := seenChannels[key.ChannelID]; ok {
			continue
		}
		seenChannels[key.ChannelID] = struct{}{}
		channelIDs = append(channelIDs, key.ChannelID)
	}
	if len(channelIDs) > 0 {
		query = query.Where("channel_id IN ?", channelIDs)
	}
	if err := query.Order("id ASC").Find(&quotaRows).Error; err != nil {
		return nil, err
	}
	targets := make(map[quotaTokenBackfillKey][]int, len(quotaRows))
	for _, quotaRow := range quotaRows {
		key := quotaTokenBackfillKey{
			UserID: quotaRow.UserID, Username: quotaRow.Username, ModelName: quotaRow.ModelName,
			CreatedAt: quotaRow.CreatedAt, UseGroup: quotaRow.UseGroup, TokenID: quotaRow.TokenID, ChannelID: quotaRow.ChannelID,
		}
		targets[key] = append(targets[key], quotaRow.Id)
	}
	return targets, nil
}

func updateQuotaTokenBackfillGroup(tx *gorm.DB, key quotaTokenBackfillKey, value quotaTokenBackfillValue, targets map[quotaTokenBackfillKey][]int) (quotaTokenBackfillGroupOutcome, error) {
	targetIDs := targets[key]
	switch len(targetIDs) {
	case 0:
		return quotaTokenBackfillGroupUnmatched, nil
	case 1:
		// A single quota_data row is safe to repair. Rows with the same
		// dimensions on multiple nodes cannot be attributed from logs, so leave
		// them untouched for a manual, node-aware repair.
	default:
		return quotaTokenBackfillGroupAmbiguous, nil
	}
	var current QuotaData
	if err := lockForUpdate(tx).Where("id = ?", targetIDs[0]).First(&current).Error; err != nil {
		return quotaTokenBackfillGroupUnchanged, err
	}
	// Logs do not contain a node name. Only repair a row automatically when
	// its existing accounting counters exactly match the complete log
	// aggregate for this dimension/hour. A mismatch means that logs may be
	// incomplete or another node may own part of the usage; filling details
	// in that case would attribute multiple nodes to one quota_data row.
	if int64(current.Count) != value.Count || int64(current.Quota) != value.Quota || int64(current.TokenUsed) != value.TokenUsed {
		return quotaTokenBackfillGroupMismatched, nil
	}
	// Preserve non-zero values so a retry or a partially flushed node cannot be
	// reset, and avoid counting the same historical usage twice.
	updates := make(map[string]any, 3)
	if current.InputTokens <= 0 && value.InputTokens > 0 {
		updates["input_tokens"] = quotaDataRepairInt(value.InputTokens)
	}
	if current.CachedTokens <= 0 && value.CachedTokens > 0 {
		updates["cached_tokens"] = quotaDataRepairInt(value.CachedTokens)
	}
	if current.ReasoningTokens <= 0 && value.ReasoningTokens > 0 {
		updates["reasoning_tokens"] = quotaDataRepairInt(value.ReasoningTokens)
	}
	if len(updates) == 0 {
		return quotaTokenBackfillGroupUnchanged, nil
	}
	if err := tx.Model(&QuotaData{}).Where("id = ?", targetIDs[0]).Updates(updates).Error; err != nil {
		return quotaTokenBackfillGroupUnchanged, err
	}
	return quotaTokenBackfillGroupUpdated, nil
}
