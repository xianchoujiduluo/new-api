package model

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestValidateUserTokenLogsMaskedKey pins that a failed lookup reports which key
// was queried. Before this, the only signal was "record not found", so an
// unexpected lookup value could not be told apart from a genuinely unknown
// token. The key is reported masked, never in full.
func TestValidateUserTokenLogsMaskedKey(t *testing.T) {
	previousDB := DB
	previousLogDB := LOG_DB
	previousType := common.MainDatabaseType()
	previousRedis := common.RedisEnabled

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Token{}))
	DB = db
	LOG_DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	// initCol is unexported and only runs from InitDB/InitLogDB; without it
	// commonKeyCol is empty and the "key = ?" predicate is malformed.
	initCol()

	var logs bytes.Buffer
	previousWriter := gin.DefaultWriter
	// common.SysLog writes through gin.DefaultWriter, not the standard logger.
	gin.DefaultWriter = io.MultiWriter(previousWriter, &logs)

	t.Cleanup(func() {
		gin.DefaultWriter = previousWriter
		DB = previousDB
		LOG_DB = previousLogDB
		common.SetMainDatabaseType(previousType)
		common.RedisEnabled = previousRedis
	})

	_, err = ValidateUserToken("unknownkey1234567890")
	require.ErrorIs(t, err, ErrTokenInvalid)

	output := logs.String()
	assert.Contains(t, output, "ValidateUserToken: failed to get token (key=")
	assert.Contains(t, output, "key=unkn**********7890")
	assert.Contains(t, output, "len=20")
	// The raw key must never reach the log.
	assert.NotContains(t, output, "unknownkey1234567890")
	assert.Contains(t, output, fmt.Sprintf("len=%d", len("unknownkey1234567890")))
}
