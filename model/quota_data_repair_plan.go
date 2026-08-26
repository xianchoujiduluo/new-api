package model

import (
	"math"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type quotaDataRepairUpdate struct {
	ID                 int
	InputTokens        int
	CachedTokens       int
	ReasoningTokens    int
	InputTokensSet     bool
	CachedTokensSet    bool
	ReasoningTokensSet bool
	ReplaceExisting    bool
}

type quotaDataRepairPlan struct {
	Updates         []quotaDataRepairUpdate
	Creates         []QuotaData
	MatchedGroups   int64
	UnmatchedGroups int64
	AmbiguousGroups int64
}

func buildQuotaDataRepairPlan(tx *gorm.DB, aggregates map[quotaDataRepairKey]quotaDataRepairAggregate, createMissing, replaceExisting bool, nodeName string, channelID int, start, end int64) (quotaDataRepairPlan, error) {
	plan := quotaDataRepairPlan{}
	var rows []QuotaData
	if err := tx.Select([]string{"id", "user_id", "username", "model_name", "created_at", "use_group", "token_id", "channel_id", "node_name", "input_tokens", "cached_tokens", "reasoning_tokens"}).
		Where("channel_id = ? AND created_at >= ? AND created_at < ?", channelID, start, end).
		Order("id ASC").Find(&rows).Error; err != nil {
		return plan, err
	}
	rowsByKey := make(map[quotaDataRepairKey][]QuotaData)
	for _, row := range rows {
		key := quotaDataRepairKey{
			UserID: row.UserID, Username: row.Username, ModelName: row.ModelName,
			CreatedAt: row.CreatedAt, UseGroup: row.UseGroup, TokenID: row.TokenID, ChannelID: row.ChannelID,
		}
		rowsByKey[key] = append(rowsByKey[key], row)
	}
	for key, aggregate := range aggregates {
		matches := rowsByKey[key]
		switch len(matches) {
		case 0:
			plan.UnmatchedGroups++
			if createMissing {
				plan.Creates = append(plan.Creates, quotaDataRepairQuotaData(key, aggregate, nodeName))
			}
		case 1:
			plan.MatchedGroups++
			update := quotaDataRepairUpdate{ID: matches[0].Id, ReplaceExisting: replaceExisting}
			if replaceExisting || (matches[0].InputTokens <= 0 && aggregate.InputTokens > 0) {
				update.InputTokens = quotaDataRepairInt(aggregate.InputTokens)
				update.InputTokensSet = true
			}
			if replaceExisting || (matches[0].CachedTokens <= 0 && aggregate.CachedTokens > 0) {
				update.CachedTokens = quotaDataRepairInt(aggregate.CachedTokens)
				update.CachedTokensSet = true
			}
			if replaceExisting || (matches[0].ReasoningTokens <= 0 && aggregate.ReasoningTokens > 0) {
				update.ReasoningTokens = quotaDataRepairInt(aggregate.ReasoningTokens)
				update.ReasoningTokensSet = true
			}
			if update.InputTokensSet || update.CachedTokensSet || update.ReasoningTokensSet {
				plan.Updates = append(plan.Updates, update)
			}
		default:
			plan.AmbiguousGroups++
		}
	}
	return plan, nil
}

func applyQuotaDataRepairPlan(tx *gorm.DB, plan quotaDataRepairPlan) error {
	for _, update := range plan.Updates {
		// Lock the row before assigning repaired details. The regular quota-data
		// flusher updates the same row with atomic increments; holding this lock
		// prevents a concurrent update from being interleaved with the repair.
		var current QuotaData
		if err := lockForUpdate(tx).Where("id = ?", update.ID).First(&current).Error; err != nil {
			return err
		}
		values := make(map[string]interface{}, 3)
		if update.InputTokensSet && (update.ReplaceExisting || current.InputTokens <= 0) {
			values["input_tokens"] = update.InputTokens
		}
		if update.CachedTokensSet && (update.ReplaceExisting || current.CachedTokens <= 0) {
			values["cached_tokens"] = update.CachedTokens
		}
		if update.ReasoningTokensSet && (update.ReplaceExisting || current.ReasoningTokens <= 0) {
			values["reasoning_tokens"] = update.ReasoningTokens
		}
		if len(values) == 0 {
			continue
		}
		result := tx.Model(&QuotaData{}).Where("id = ?", update.ID).Updates(values)
		if result.Error != nil {
			return result.Error
		}
		// The row was locked and loaded above, so a zero RowsAffected result can
		// legitimately mean that another compatible writer already applied the
		// same values (not that the row disappeared). In particular, MySQL may
		// report zero for an UPDATE whose values are unchanged.
	}
	for i := range plan.Creates {
		if err := tx.Create(&plan.Creates[i]).Error; err != nil {
			return err
		}
	}
	return nil
}

func quotaDataRepairQuotaData(key quotaDataRepairKey, aggregate quotaDataRepairAggregate, nodeName string) QuotaData {
	return QuotaData{
		UserID: key.UserID, Username: key.Username, ModelName: key.ModelName, CreatedAt: key.CreatedAt,
		UseGroup: key.UseGroup, TokenID: key.TokenID, ChannelID: key.ChannelID, NodeName: nodeName,
		Count: quotaDataRepairInt(aggregate.Count), Quota: quotaDataRepairInt(aggregate.Quota),
		TokenUsed: quotaDataRepairInt(aggregate.TokenUsed), InputTokens: quotaDataRepairInt(aggregate.InputTokens),
		CachedTokens: quotaDataRepairInt(aggregate.CachedTokens), ReasoningTokens: quotaDataRepairInt(aggregate.ReasoningTokens),
	}
}

func quotaDataRepairInt(value int64) int {
	return common.QuotaFromFloat(float64(value))
}

func addSignedInt64(total, value int64) int64 {
	if value > 0 && total > math.MaxInt64-value {
		return math.MaxInt64
	}
	if value < 0 && total < math.MinInt64-value {
		return math.MinInt64
	}
	return total + value
}
