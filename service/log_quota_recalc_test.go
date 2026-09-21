package service

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recalcTestExpr is the production time-of-day pricing expression that prompted
// the recalculation feature: peak hours cost twice the off-peak input rate.
const recalcTestExpr = `weekday("Asia/Shanghai") >= 1 && weekday("Asia/Shanghai") <= 5 && ((hour("Asia/Shanghai") >= 9 && hour("Asia/Shanghai") < 12) || (hour("Asia/Shanghai") >= 14 && hour("Asia/Shanghai") < 18)) ? tier("peak", p * 2 + cr * 0.04 + c * 8) : tier("off_peak", p * 1 + cr * 0.02 + c * 4)`

const (
	// Friday 2026-08-21 10:30 Asia/Shanghai - inside the weekday peak window.
	recalcPeakTimestamp = int64(1787279400)
	// Saturday 2026-08-22 10:30 Asia/Shanghai - same clock hour, weekend.
	recalcWeekendTimestamp = int64(1787365800)
	// Friday 2026-08-21 08:59 Asia/Shanghai - weekday, before the peak window.
	recalcEarlyTimestamp = int64(1787273940)
)

// configureRecalcExpr installs recalcTestExpr as the tiered expression for the
// test model and restores the previous billing configuration afterwards.
func configureRecalcExpr(t *testing.T, modelName, expr string) {
	t.Helper()
	before := config.GlobalConfig.ExportAllConfigs()
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(
			config.GlobalConfig.Get("billing_setting"),
			map[string]string{
				"billing_mode": before["billing_setting.billing_mode"],
				"billing_expr": before["billing_setting.billing_expr"],
			},
		))
	})
	require.NoError(t, config.UpdateConfigFromMap(
		config.GlobalConfig.Get("billing_setting"),
		map[string]string{
			"billing_mode": `{"` + modelName + `":"tiered_expr"}`,
			"billing_expr": `{"` + modelName + `":` + strconv.Quote(expr) + `}`,
		},
	))
}

func recalcTestLog(createdAt int64) *model.Log {
	return &model.Log{
		Type:             model.LogTypeConsume,
		ModelName:        "recalc-mapped-model",
		Group:            "default",
		PromptTokens:     1000,
		CompletionTokens: 100,
		CreatedAt:        createdAt,
		Other:            `{"cache_tokens":200}`,
	}
}

// TestRecomputeConsumeLogQuotaResolvesTiersAtOriginalTimestamp is the regression
// guard for the whole feature: a log written under the wrong default ratio must
// be re-priced with the current expression evaluated at *its own* timestamp.
//
// With p=1000, cr=200, c=100 and the expression subtracting cr from p
// (BuildTieredTokenParams does p -= cr when "cr" is used), the effective input
// is 800 tokens:
//
//	peak     = 800*2 + 200*0.04 + 100*8 = 2408 -> 2408/1e6*500000 = 1204
//	off_peak = 800*1 + 200*0.02 + 100*4 = 1204 -> 1204/1e6*500000 = 602
func TestRecomputeConsumeLogQuotaResolvesTiersAtOriginalTimestamp(t *testing.T) {
	configureRecalcExpr(t, "recalc-mapped-model", recalcTestExpr)

	cases := []struct {
		name      string
		createdAt int64
		wantQuota int64
	}{
		{"weekday peak window", recalcPeakTimestamp, 1204},
		{"weekend same clock hour", recalcWeekendTimestamp, 602},
		{"weekday before peak window", recalcEarlyTimestamp, 602},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			quota, ok, reason := RecomputeConsumeLogQuota(recalcTestLog(tc.createdAt))
			require.True(t, ok, "unexpected skip reason: %s", reason)
			assert.Equal(t, tc.wantQuota, quota)
		})
	}
}

// TestRecomputeConsumeLogQuotaRejectsUnreproducibleLogs verifies the skip
// contract: anything that cannot be re-derived faithfully must be reported
// instead of being guessed.
func TestRecomputeConsumeLogQuotaRejectsUnreproducibleLogs(t *testing.T) {
	t.Run("expression needs request probe", func(t *testing.T) {
		configureRecalcExpr(t, "recalc-mapped-model", `header("x-tier") == "pro" ? tier("pro", p * 4) : tier("std", p * 1)`)

		quota, ok, reason := RecomputeConsumeLogQuota(recalcTestLog(recalcPeakTimestamp))
		assert.False(t, ok)
		assert.Equal(t, model.SkipReasonRequestProbe, reason)
		assert.Zero(t, quota)
	})

	t.Run("not a consume log", func(t *testing.T) {
		logRow := recalcTestLog(recalcPeakTimestamp)
		logRow.Type = model.LogTypeSystem

		_, ok, reason := RecomputeConsumeLogQuota(logRow)
		assert.False(t, ok)
		assert.Equal(t, model.SkipReasonNotConsume, reason)
	})

	t.Run("no usage recorded", func(t *testing.T) {
		logRow := recalcTestLog(recalcPeakTimestamp)
		logRow.PromptTokens = 0
		logRow.CompletionTokens = 0

		_, ok, reason := RecomputeConsumeLogQuota(logRow)
		assert.False(t, ok)
		assert.Equal(t, model.SkipReasonNoUsage, reason)
	})
}
