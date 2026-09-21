package service

import (
	"math"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// RecomputeConsumeLogQuota re-prices one consume log against the *current*
// pricing settings for its model. It returns ok=false together with a stable
// skip reason whenever the historical record cannot be reproduced faithfully;
// such rows must be left untouched rather than guessed.
//
// Pricing is resolved from live settings (the point of the operation is to
// correct stale pricing), while usage and the group ratio are taken from the
// log itself so historical context is preserved.
func RecomputeConsumeLogQuota(logRow *model.Log) (int64, bool, string) {
	if logRow == nil || logRow.Type != model.LogTypeConsume {
		return 0, false, model.SkipReasonNotConsume
	}

	details := parseRecalcLogOther(logRow.Other)
	if details == nil {
		return 0, false, model.SkipReasonUnparsableOther
	}
	if logRow.PromptTokens == 0 && logRow.CompletionTokens == 0 {
		return 0, false, model.SkipReasonNoUsage
	}

	group := logRow.Group
	if group == "" {
		group = details.stringValue("group")
	}
	groupRatio := ratio_setting.GetGroupRatio(group)

	modelName := logRow.ModelName

	// Current expression pricing takes precedence, then per-call price, then the
	// classic ratio table.
	if billing_setting.GetBillingMode(modelName) == billing_setting.BillingModeTieredExpr {
		if expr, ok := billing_setting.GetBillingExpr(modelName); ok {
			return recomputeWithExpression(logRow, details, expr, groupRatio)
		}
	}

	if price, ok := ratio_setting.GetModelPrice(modelName, false); ok {
		return quotaFromPrice(price, groupRatio), true, ""
	}

	return recomputeWithRatio(logRow, details, modelName, groupRatio)
}

// recomputeWithExpression re-runs the model's current billing expression at the
// log's original timestamp, so time-dependent tiers (peak/off-peak) resolve the
// way they did when the request was made.
func recomputeWithExpression(logRow *model.Log, details recalcLogOther, expr string, groupRatio float64) (int64, bool, string) {
	usedVars := billingexpr.UsedVars(expr)
	// Request probes cannot be replayed: the historical request headers/body are
	// not stored, so any expression depending on them would be evaluated with
	// empty inputs and produce a wrong price.
	if usedVars["param"] || usedVars["header"] {
		return 0, false, model.SkipReasonRequestProbe
	}

	usage := &dto.Usage{
		PromptTokens:     logRow.PromptTokens,
		CompletionTokens: logRow.CompletionTokens,
		TotalTokens:      logRow.PromptTokens + logRow.CompletionTokens,
	}
	usage.PromptTokensDetails.CachedTokens = details.intValue("cache_tokens")

	params := BuildTieredTokenParams(usage, false, usedVars)

	snapshot := &billingexpr.BillingSnapshot{
		BillingMode:  billing_setting.BillingModeTieredExpr,
		ModelName:    logRow.ModelName,
		ExprString:   expr,
		ExprHash:     billingexpr.ExprHashString(expr),
		GroupRatio:   groupRatio,
		QuotaPerUnit: common.QuotaPerUnit,
		ExprVersion:  billingexpr.ExprVersion(expr),
	}

	reference := time.Unix(logRow.CreatedAt, 0)
	result, err := billingexpr.ComputeTieredQuotaWithRequest(
		snapshot,
		params,
		billingexpr.RequestInput{ReferenceTime: &reference},
	)
	if err != nil {
		return 0, false, model.SkipReasonUnsupportedBill
	}
	return int64(result.ActualQuotaAfterGroup), true, ""
}

// recomputeWithRatio reproduces the classic token-ratio price for the common
// case: no per-call price, no separately-priced sub-categories.
func recomputeWithRatio(logRow *model.Log, details recalcLogOther, modelName string, groupRatio float64) (int64, bool, string) {
	// Multi-modal sub-categories are priced separately and the log does not carry
	// enough structure to rebuild them exactly.
	if details.intValue("audio_input") > 0 || details.intValue("audio_output") > 0 ||
		details.boolValue("audio") || details.boolValue("image") {
		return 0, false, model.SkipReasonUnsupportedBill
	}

	modelRatio, _, _ := ratio_setting.GetModelRatio(modelName)
	completionRatio := ratio_setting.GetCompletionRatio(modelName)
	cacheRatio, _ := ratio_setting.GetCacheRatio(modelName)
	cacheCreationRatio, _ := ratio_setting.GetCreateCacheRatio(modelName)

	promptTokens := float64(logRow.PromptTokens)
	completionTokens := float64(logRow.CompletionTokens)
	cacheTokens := float64(details.intValue("cache_tokens"))
	cacheCreationTokens := float64(details.intValue("cache_creation_tokens"))

	// Cache tokens are billed at their own ratio, so subtract them from the base.
	baseTokens := promptTokens - cacheTokens - cacheCreationTokens
	if baseTokens < 0 {
		baseTokens = 0
	}

	billable := baseTokens +
		cacheTokens*cacheRatio +
		cacheCreationTokens*cacheCreationRatio +
		completionTokens*completionRatio

	// ratio 1 == $2 / 1M tokens == 1 quota per token.
	quota := billable * modelRatio * groupRatio
	return int64(quotaRounded(quota)), true, ""
}

func quotaFromPrice(price, groupRatio float64) int64 {
	return int64(quotaRounded(price * groupRatio * common.QuotaPerUnit))
}

func quotaRounded(value float64) int {
	if math.IsNaN(value) || value <= 0 {
		return 0
	}
	return common.QuotaRound(value)
}

// recalcLogOther is a read-only view over a log's `other` JSON.
type recalcLogOther map[string]any

func parseRecalcLogOther(raw string) recalcLogOther {
	if raw == "" {
		return recalcLogOther{}
	}
	parsed := recalcLogOther{}
	if err := common.UnmarshalJsonStr(raw, &parsed); err != nil {
		return nil
	}
	return parsed
}

func (o recalcLogOther) intValue(key string) int {
	switch value := o[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case int64:
		return int(value)
	default:
		return 0
	}
}

func (o recalcLogOther) boolValue(key string) bool {
	value, ok := o[key].(bool)
	return ok && value
}

func (o recalcLogOther) stringValue(key string) string {
	value, _ := o[key].(string)
	return value
}
