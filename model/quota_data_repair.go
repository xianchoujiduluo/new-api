package model

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const quotaDataRepairMaxRangeSeconds int64 = 366 * 24 * 60 * 60

// QuotaDataRepairRequest describes a bounded, manual quota_data repair.
// StartTimestamp and EndTimestamp are expanded to complete hour buckets.
type QuotaDataRepairRequest struct {
	ChannelID int
	// NodeName identifies the node that owns rows created with CreateMissing.
	// Existing rows are matched without this field because historical logs do
	// not carry a reliable node identity.
	NodeName        string
	StartTimestamp  int64
	EndTimestamp    int64
	Apply           bool
	CreateMissing   bool
	ReplaceExisting bool
}

// QuotaDataRepairReport contains both the source data summary and the actions
// that were applied (or would be applied in dry-run mode).
type QuotaDataRepairReport struct {
	ChannelID       int   `json:"channel_id"`
	StartTimestamp  int64 `json:"start_timestamp"`
	EndTimestamp    int64 `json:"end_timestamp"`
	ScannedLogs     int64 `json:"scanned_logs"`
	AggregateGroups int64 `json:"aggregate_groups"`
	MatchedGroups   int64 `json:"matched_groups"`
	PlannedUpdates  int64 `json:"planned_updates"`
	PlannedCreates  int64 `json:"planned_creates"`
	UpdatedGroups   int64 `json:"updated_groups"`
	CreatedGroups   int64 `json:"created_groups"`
	UnmatchedGroups int64 `json:"unmatched_groups"`
	AmbiguousGroups int64 `json:"ambiguous_groups"`
	InvalidLogOther int64 `json:"invalid_log_other"`
	InputTokens     int64 `json:"input_tokens"`
	CachedTokens    int64 `json:"cached_tokens"`
	ReasoningTokens int64 `json:"reasoning_tokens"`
}

type quotaDataRepairKey struct {
	UserID    int
	Username  string
	ModelName string
	CreatedAt int64
	UseGroup  string
	TokenID   int
	ChannelID int
}

type quotaDataRepairAggregate struct {
	Count           int64
	Quota           int64
	TokenUsed       int64
	InputTokens     int64
	CachedTokens    int64
	ReasoningTokens int64
}

type quotaDataRepairLogRow struct {
	UserID           int    `gorm:"column:user_id"`
	Username         string `gorm:"column:username"`
	ModelName        string `gorm:"column:model_name"`
	CreatedAt        int64  `gorm:"column:created_at"`
	PromptTokens     int    `gorm:"column:prompt_tokens"`
	CompletionTokens int    `gorm:"column:completion_tokens"`
	ReasoningTokens  int    `gorm:"column:reasoning_tokens"`
	Quota            int    `gorm:"column:quota"`
	UseGroup         string `gorm:"column:group"`
	TokenID          int    `gorm:"column:token_id"`
	ChannelID        int    `gorm:"column:channel_id"`
	Other            string `gorm:"column:other"`
}

type quotaDataRepairLogOther struct {
	CacheTokens           float64 `json:"cache_tokens"`
	InputTokensTotal      float64 `json:"input_tokens_total"`
	CacheCreationTokens   float64 `json:"cache_creation_tokens"`
	CacheCreationTokens5m float64 `json:"cache_creation_tokens_5m"`
	CacheCreationTokens1h float64 `json:"cache_creation_tokens_1h"`
	ReasoningTokens       float64 `json:"reasoning_tokens"`
	UsageSemantic         string  `json:"usage_semantic"`
	Claude                bool    `json:"claude"`
}

// quotaDataRepairLogGroupColumn returns the dialect-specific quoted name for
// the logs.group column. group is reserved by PostgreSQL (and can be treated
// as a keyword by other supported databases), so it must not be interpolated
// as a bare identifier in a SELECT list.
func quotaDataRepairLogGroupColumn() string {
	if logGroupCol != "" {
		return logGroupCol
	}
	if common.UsingLogDatabase(common.DatabaseTypePostgreSQL) {
		return `"group"`
	}
	return "`group`"
}

func quotaDataRepairLogSelectColumns() []string {
	return quotaDataRepairLogSelectColumnsForDB(nil)
}

// quotaDataRepairLogSelectColumnsForDB keeps the repair command usable while a
// deployment is being upgraded. reasoning_tokens was added after the original
// logs schema; omitting it from the SELECT is safe because the row parser
// leaves the field at zero and can still recover all older detail fields from
// Other.
func quotaDataRepairLogSelectColumnsForDB(db *gorm.DB) []string {
	columns := []string{
		"user_id", "username", "model_name", "created_at", "prompt_tokens",
		"completion_tokens", "quota", quotaDataRepairLogGroupColumn(),
		"token_id", "channel_id", "other",
	}
	if db == nil || db.Migrator().HasColumn(&Log{}, "reasoning_tokens") {
		columns = append(columns[:6], append([]string{"reasoning_tokens"}, columns[6:]...)...)
	}
	return columns
}

// ValidateQuotaDataRepairSchema verifies that the destination has the columns
// required to persist token details. The normal application startup migration
// creates them; the standalone repair command intentionally does not migrate
// unrelated tables, so it returns an actionable error when run too early.
func ValidateQuotaDataRepairSchema() error {
	if DB == nil {
		return errors.New("database is not initialized")
	}
	if !DB.Migrator().HasTable("quota_data") {
		return errors.New("quota_data table does not exist; start the backend once to initialize the database")
	}
	missing := make([]string, 0, 3)
	for _, column := range []string{"input_tokens", "cached_tokens", "reasoning_tokens"} {
		if !DB.Migrator().HasColumn(&QuotaData{}, column) {
			missing = append(missing, column)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("quota_data 缺少 token 明细列: %s；请先重启当前版本后端完成数据库迁移，再重新运行回填命令", strings.Join(missing, ", "))
	}
	return nil
}

// RepairQuotaData rebuilds token-detail columns for one channel and time
// range. Existing empty detail fields are filled from the logs by default;
// ReplaceExisting enables an absolute replacement. Billing/count columns are
// left unchanged for existing rows; missing rows are created only when
// CreateMissing is true.
func RepairQuotaData(ctx context.Context, request QuotaDataRepairRequest) (QuotaDataRepairReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	start, end, err := normalizeQuotaDataRepairRange(request.ChannelID, request.StartTimestamp, request.EndTimestamp)
	if err != nil {
		return QuotaDataRepairReport{}, err
	}
	if LOG_DB == nil || DB == nil {
		return QuotaDataRepairReport{}, errors.New("database is not initialized")
	}
	if err := ValidateQuotaDataRepairSchema(); err != nil {
		return QuotaDataRepairReport{}, err
	}

	report := QuotaDataRepairReport{
		ChannelID:      request.ChannelID,
		StartTimestamp: start,
		EndTimestamp:   end,
	}
	aggregates, scanReport, err := collectQuotaDataRepairLogs(ctx, request.ChannelID, start, end)
	if err != nil {
		return report, err
	}
	report.ScannedLogs = scanReport.ScannedLogs
	report.InvalidLogOther = scanReport.InvalidLogOther
	report.InputTokens = scanReport.InputTokens
	report.CachedTokens = scanReport.CachedTokens
	report.ReasoningTokens = scanReport.ReasoningTokens
	report.AggregateGroups = int64(len(aggregates))

	var plan quotaDataRepairPlan
	if request.Apply {
		err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var planErr error
			plan, planErr = buildQuotaDataRepairPlan(tx, aggregates, request.CreateMissing, request.ReplaceExisting, request.NodeName, request.ChannelID, start, end)
			if planErr != nil {
				return planErr
			}
			return applyQuotaDataRepairPlan(tx, plan)
		})
	} else {
		plan, err = buildQuotaDataRepairPlan(DB.WithContext(ctx), aggregates, request.CreateMissing, request.ReplaceExisting, request.NodeName, request.ChannelID, start, end)
	}
	if err != nil {
		return report, err
	}

	report.MatchedGroups = plan.MatchedGroups
	report.PlannedUpdates = int64(len(plan.Updates))
	report.PlannedCreates = int64(len(plan.Creates))
	report.UnmatchedGroups = plan.UnmatchedGroups
	report.AmbiguousGroups = plan.AmbiguousGroups
	if request.Apply {
		report.UpdatedGroups = report.PlannedUpdates
		report.CreatedGroups = report.PlannedCreates
	}
	return report, nil
}

func normalizeQuotaDataRepairRange(channelID int, start, end int64) (int64, int64, error) {
	if channelID <= 0 {
		return 0, 0, errors.New("channel id must be positive")
	}
	if start <= 0 || end <= 0 || end < start {
		return 0, 0, errors.New("invalid time range")
	}
	if end-start > quotaDataRepairMaxRangeSeconds {
		return 0, 0, errors.New("time range cannot exceed 366 days")
	}
	normalizedStart := start - start%3600
	normalizedEnd := end - end%3600
	if normalizedEnd > math.MaxInt64-3600 {
		return 0, 0, errors.New("time range is too large")
	}
	normalizedEndExclusive := normalizedEnd + 3600
	if normalizedEndExclusive-normalizedStart > quotaDataRepairMaxRangeSeconds {
		return 0, 0, errors.New("time range cannot exceed 366 days after hour alignment")
	}
	return normalizedStart, normalizedEndExclusive, nil
}
