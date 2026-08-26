package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackfillQuotaTokenDetailsWindowRequiresAccountingMatch(t *testing.T) {
	truncateTables(t)
	const hour = int64(1787587200)

	require.NoError(t, DB.Create(&Log{
		UserId: 1, Username: "alice", ModelName: "gpt-test", CreatedAt: hour + 1,
		Type: LogTypeConsume, PromptTokens: 10, CompletionTokens: 2, Quota: 3,
		ChannelId: 40, TokenId: 7, Group: "default", Other: `{"input_tokens_total":10}`,
	}).Error)
	quota := &QuotaData{
		UserID: 1, Username: "alice", ModelName: "gpt-test", CreatedAt: hour,
		UseGroup: "default", TokenID: 7, ChannelID: 40, NodeName: "node-a",
		Count: 2, Quota: 3, TokenUsed: 12,
	}
	require.NoError(t, DB.Create(quota).Error)

	result, err := BackfillQuotaTokenDetailsWindow(context.Background(), hour, hour+3600)
	require.NoError(t, err)
	assert.Equal(t, int64(1), result.MismatchedGroups)
	assert.Zero(t, result.UpdatedGroups)

	var got QuotaData
	require.NoError(t, DB.First(&got, quota.Id).Error)
	assert.Zero(t, got.InputTokens)
}

func TestBackfillQuotaTokenDetailsWindowFillsEmptyDetailsAfterMatch(t *testing.T) {
	truncateTables(t)
	const hour = int64(1787587200)

	require.NoError(t, DB.Create(&Log{
		UserId: 1, Username: "alice", ModelName: "gpt-test", CreatedAt: hour + 1,
		Type: LogTypeConsume, PromptTokens: 10, CompletionTokens: 2, Quota: 3,
		ChannelId: 40, TokenId: 7, Group: "default", Other: `{"input_tokens_total":10,"cache_tokens":2,"reasoning_tokens":1}`,
	}).Error)
	quota := &QuotaData{
		UserID: 1, Username: "alice", ModelName: "gpt-test", CreatedAt: hour,
		UseGroup: "default", TokenID: 7, ChannelID: 40, NodeName: "node-a",
		Count: 1, Quota: 3, TokenUsed: 12,
	}
	require.NoError(t, DB.Create(quota).Error)

	result, err := BackfillQuotaTokenDetailsWindow(context.Background(), hour, hour+3600)
	require.NoError(t, err)
	assert.Equal(t, int64(1), result.UpdatedGroups)
	assert.Zero(t, result.MismatchedGroups)

	var got QuotaData
	require.NoError(t, DB.First(&got, quota.Id).Error)
	assert.Equal(t, 10, got.InputTokens)
	assert.Equal(t, 2, got.CachedTokens)
	assert.Equal(t, 1, got.ReasoningTokens)
}

func TestBackfillQuotaTokenDetailsWindowSkipsMultipleNodeRows(t *testing.T) {
	truncateTables(t)
	const hour = int64(1787587200)

	require.NoError(t, DB.Create(&Log{
		UserId: 1, Username: "alice", ModelName: "gpt-test", CreatedAt: hour + 1,
		Type: LogTypeConsume, PromptTokens: 10, CompletionTokens: 2, Quota: 3,
		ChannelId: 40, TokenId: 7, Group: "default", Other: `{"input_tokens_total":10}`,
	}).Error)
	for _, nodeName := range []string{"node-a", "node-b"} {
		require.NoError(t, DB.Create(&QuotaData{
			UserID: 1, Username: "alice", ModelName: "gpt-test", CreatedAt: hour,
			UseGroup: "default", TokenID: 7, ChannelID: 40, NodeName: nodeName,
			Count: 1, Quota: 3, TokenUsed: 12,
		}).Error)
	}

	result, err := BackfillQuotaTokenDetailsWindow(context.Background(), hour, hour+3600)
	require.NoError(t, err)
	assert.Equal(t, int64(1), result.AmbiguousGroups)
	assert.Zero(t, result.UpdatedGroups)
}
