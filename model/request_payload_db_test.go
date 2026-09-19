package model

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupRequestPayloadDB wires a temporary SQLite database as the log DB so the
// relational persistence path (AutoMigrate + insert/query/delete) is exercised
// with a real database engine.
func setupRequestPayloadDB(t *testing.T) {
	t.Helper()
	previous := LOG_DB
	previousLogType := common.LogDatabaseType()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Log{}, &RequestPayload{}))

	LOG_DB = db
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		LOG_DB = previous
		common.SetLogDatabaseType(previousLogType)
	})
}

func TestRequestPayloadSupportedWithoutClickHouse(t *testing.T) {
	setupRequestPayloadDB(t)

	assert.True(t, RequestPayloadSupported())
}

func TestRequestPayloadRoundTripOnRelationalDB(t *testing.T) {
	setupRequestPayloadDB(t)
	ctx := context.Background()

	require.NoError(t, InsertRequestPayload(&RequestPayload{
		RequestId:       "req-roundtrip",
		CreatedAt:       1000,
		UserId:          7,
		Username:        "alice",
		ModelName:       "gpt-4o",
		ClientIp:        "203.0.113.5",
		RequestHeaders:  `{"Authorization":"[REDACTED]"}`,
		RequestBody:     `{"messages":[]}`,
		ResponseHeaders: `{"Content-Type":"application/json"}`,
		ResponseBody:    "hello",
	}))

	stored, err := GetRequestPayloadByRequestId(ctx, "req-roundtrip")
	require.NoError(t, err)
	assert.Equal(t, 7, stored.UserId)
	assert.Equal(t, "alice", stored.Username)
	assert.Equal(t, "203.0.113.5", stored.ClientIp)
	assert.Equal(t, "hello", stored.ResponseBody)
	assert.Contains(t, stored.RequestHeaders, "[REDACTED]")
}

func TestGetRequestPayloadReportsNotFound(t *testing.T) {
	setupRequestPayloadDB(t)

	_, err := GetRequestPayloadByRequestId(context.Background(), "missing")

	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestGetLogUserIdByRequestId(t *testing.T) {
	setupRequestPayloadDB(t)
	ctx := context.Background()

	require.NoError(t, LOG_DB.Create(&Log{UserId: 42, RequestId: "req-owner"}).Error)

	ownerId, found, err := GetLogUserIdByRequestId(ctx, "req-owner")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, 42, ownerId)

	_, found, err = GetLogUserIdByRequestId(ctx, "req-unknown")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestDeleteOldRequestPayloadsOnRelationalDB(t *testing.T) {
	setupRequestPayloadDB(t)
	ctx := context.Background()

	require.NoError(t, InsertRequestPayload(&RequestPayload{RequestId: "old", CreatedAt: 100}))
	require.NoError(t, InsertRequestPayload(&RequestPayload{RequestId: "new", CreatedAt: 900}))

	require.NoError(t, DeleteOldRequestPayloads(ctx, 500))

	_, err := GetRequestPayloadByRequestId(ctx, "old")
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	remaining, err := GetRequestPayloadByRequestId(ctx, "new")
	require.NoError(t, err)
	assert.Equal(t, "new", remaining.RequestId)
}

func TestDeleteOldRequestPayloadsSkipsWhenLogDBNil(t *testing.T) {
	previous := LOG_DB
	LOG_DB = nil
	t.Cleanup(func() { LOG_DB = previous })

	require.NoError(t, DeleteOldRequestPayloads(context.Background(), 500))
}
