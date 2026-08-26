package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepairQuotaDataUpdatesExistingRowsIdempotently(t *testing.T) {
	truncateTables(t)
	const (
		channelID = 40
		hour      = int64(1787587200)
	)
	require.NoError(t, DB.Create(&Log{
		UserId: 1, Username: "root", ModelName: "gpt-test", CreatedAt: hour + 10,
		Type: LogTypeConsume, PromptTokens: 100, CompletionTokens: 8, Quota: 20,
		ChannelId: channelID, TokenId: 1, Group: "default", Other: `{"cache_tokens":30}`,
	}).Error)
	require.NoError(t, DB.Create(&Log{
		UserId: 1, Username: "root", ModelName: "gpt-test", CreatedAt: hour + 20,
		Type: LogTypeConsume, PromptTokens: 50, CompletionTokens: 4, ReasoningTokens: 6, Quota: 10,
		ChannelId: channelID, TokenId: 1, Group: "default", Other: `{"cache_tokens":20,"input_tokens_total":70}`,
	}).Error)
	quota := &QuotaData{
		UserID: 1, Username: "root", ModelName: "gpt-test", CreatedAt: hour,
		UseGroup: "default", TokenID: 1, ChannelID: channelID, NodeName: "node-a",
		Count: 2, Quota: 30, TokenUsed: 162,
	}
	require.NoError(t, DB.Create(quota).Error)

	report, err := RepairQuotaData(context.Background(), QuotaDataRepairRequest{
		ChannelID: channelID, StartTimestamp: hour, EndTimestamp: hour + 3599, Apply: true,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(2), report.ScannedLogs)
	assert.Equal(t, int64(1), report.AggregateGroups)
	assert.Equal(t, int64(1), report.UpdatedGroups)
	assert.Equal(t, int64(0), report.CreatedGroups)

	var got QuotaData
	require.NoError(t, DB.First(&got, quota.Id).Error)
	assert.Equal(t, 170, got.InputTokens)
	assert.Equal(t, 50, got.CachedTokens)
	assert.Equal(t, 6, got.ReasoningTokens)
	assert.Equal(t, 2, got.Count)
	assert.Equal(t, 30, got.Quota)
	assert.Equal(t, 162, got.TokenUsed)

	second, err := RepairQuotaData(context.Background(), QuotaDataRepairRequest{
		ChannelID: channelID, StartTimestamp: hour, EndTimestamp: hour + 3599, Apply: true,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(0), second.UpdatedGroups)
	require.NoError(t, DB.First(&got, quota.Id).Error)
	assert.Equal(t, 170, got.InputTokens)
	assert.Equal(t, 50, got.CachedTokens)
}

func TestRepairQuotaDataDryRunDoesNotWrite(t *testing.T) {
	truncateTables(t)
	const hour = int64(1787587200)
	require.NoError(t, DB.Create(&Log{
		UserId: 2, Username: "alice", ModelName: "gpt-test", CreatedAt: hour + 1,
		Type: LogTypeConsume, PromptTokens: 12, ChannelId: 7,
	}).Error)
	quota := &QuotaData{
		UserID: 2, Username: "alice", ModelName: "gpt-test", CreatedAt: hour,
		ChannelID: 7, Count: 1, TokenUsed: 12,
	}
	require.NoError(t, DB.Create(quota).Error)

	report, err := RepairQuotaData(context.Background(), QuotaDataRepairRequest{
		ChannelID: 7, StartTimestamp: hour, EndTimestamp: hour + 3599,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), report.PlannedUpdates)
	assert.Equal(t, int64(0), report.UpdatedGroups)

	var got QuotaData
	require.NoError(t, DB.First(&got, quota.Id).Error)
	assert.Zero(t, got.InputTokens)
}

func TestRepairQuotaDataCanCreateMissingRowsWhenRequested(t *testing.T) {
	truncateTables(t)
	const hour = int64(1787587200)
	require.NoError(t, DB.Create(&Log{
		UserId: 3, Username: "bob", ModelName: "gpt-test", CreatedAt: hour + 1,
		Type: LogTypeConsume, PromptTokens: 9, CompletionTokens: 3, Quota: 4,
		ChannelId: 8, TokenId: 2, Group: "default", Other: `{"cache_tokens":2}`,
	}).Error)

	report, err := RepairQuotaData(context.Background(), QuotaDataRepairRequest{
		ChannelID: 8, StartTimestamp: hour, EndTimestamp: hour + 3599,
		Apply: true, CreateMissing: true,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), report.CreatedGroups)

	var got QuotaData
	require.NoError(t, DB.Where("channel_id = ?", 8).First(&got).Error)
	assert.Equal(t, 1, got.Count)
	assert.Equal(t, 4, got.Quota)
	assert.Equal(t, 12, got.TokenUsed)
	assert.Equal(t, 9, got.InputTokens)
	assert.Equal(t, 2, got.CachedTokens)
}

func TestNormalizeQuotaDataRepairRangeUsesHourBuckets(t *testing.T) {
	start, end, err := normalizeQuotaDataRepairRange(1, 1787589010, 1787592600)
	require.NoError(t, err)
	assert.Equal(t, int64(1787587200), start)
	assert.Equal(t, int64(1787594400), end)
}
