package model

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// QuotaData 柱状图数据
type QuotaData struct {
	Id              int    `json:"id"`
	UserID          int    `json:"user_id" gorm:"index"`
	Username        string `json:"username" gorm:"index:idx_qdt_model_user_name,priority:2;size:64;default:''"`
	ModelName       string `json:"model_name" gorm:"index:idx_qdt_model_user_name,priority:1;size:64;default:''"`
	CreatedAt       int64  `json:"created_at" gorm:"bigint;index:idx_qdt_created_at,priority:2"`
	UseGroup        string `json:"use_group" gorm:"index;size:64;default:''"`
	TokenID         int    `json:"token_id" gorm:"index;default:0"`
	ChannelID       int    `json:"channel_id" gorm:"index;default:0"`
	ChannelName     string `json:"channel_name,omitempty" gorm:"-"`
	NodeName        string `json:"node_name" gorm:"index;size:64;default:''"`
	TokenUsed       int    `json:"token_used" gorm:"default:0"`
	InputTokens     int    `json:"input_tokens" gorm:"default:0"`
	CachedTokens    int    `json:"cached_tokens" gorm:"default:0"`
	ReasoningTokens int    `json:"reasoning_tokens" gorm:"default:0"`
	Count           int    `json:"count" gorm:"default:0"`
	Quota           int    `json:"quota" gorm:"default:0"`
}

type QuotaDataLogParams struct {
	UserID          int
	Username        string
	ModelName       string
	Quota           int
	CreatedAt       int64
	TokenUsed       int
	InputTokens     int
	CachedTokens    int
	ReasoningTokens int
	UseGroup        string
	TokenID         int
	ChannelID       int
	NodeName        string
}

func UpdateQuotaData() {
	for {
		if common.DataExportEnabled {
			common.SysLog("正在更新数据看板数据...")
			SaveQuotaDataCache()
		}
		time.Sleep(time.Duration(common.DataExportInterval) * time.Minute)
	}
}

var CacheQuotaData = make(map[string]*QuotaData)
var CacheQuotaDataLock = sync.Mutex{}

func logQuotaDataCache(quotaData *QuotaData) {
	key := fmt.Sprintf("%d\x00%s\x00%s\x00%d\x00%s\x00%d\x00%d\x00%s",
		quotaData.UserID,
		quotaData.Username,
		quotaData.ModelName,
		quotaData.CreatedAt,
		quotaData.UseGroup,
		quotaData.TokenID,
		quotaData.ChannelID,
		quotaData.NodeName,
	)
	count := quotaData.Count
	quota := quotaData.Quota
	tokenUsed := quotaData.TokenUsed
	inputTokens := quotaData.InputTokens
	cachedTokens := quotaData.CachedTokens
	reasoningTokens := quotaData.ReasoningTokens
	cachedQuotaData, ok := CacheQuotaData[key]
	if ok {
		cachedQuotaData.Count += count
		cachedQuotaData.Quota += quota
		cachedQuotaData.TokenUsed += tokenUsed
		cachedQuotaData.InputTokens += inputTokens
		cachedQuotaData.CachedTokens += cachedTokens
		cachedQuotaData.ReasoningTokens += reasoningTokens
		quotaData = cachedQuotaData
	}
	CacheQuotaData[key] = quotaData
}

func LogQuotaData(params QuotaDataLogParams) {
	// 只精确到小时
	createdAt := params.CreatedAt - (params.CreatedAt % 3600)
	quotaData := &QuotaData{
		UserID:          params.UserID,
		Username:        params.Username,
		ModelName:       params.ModelName,
		CreatedAt:       createdAt,
		UseGroup:        params.UseGroup,
		TokenID:         params.TokenID,
		ChannelID:       params.ChannelID,
		NodeName:        params.NodeName,
		Count:           1,
		Quota:           params.Quota,
		TokenUsed:       params.TokenUsed,
		InputTokens:     params.InputTokens,
		CachedTokens:    params.CachedTokens,
		ReasoningTokens: params.ReasoningTokens,
	}

	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	logQuotaDataCache(quotaData)
}

func SaveQuotaDataCache() {
	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	size := len(CacheQuotaData)
	// 如果缓存中有数据，就保存到数据库中
	// 1. 先查询数据库中是否有数据
	// 2. 如果有数据，就更新数据
	// 3. 如果没有数据，就插入数据
	for _, quotaData := range CacheQuotaData {
		quotaDataDB := &QuotaData{}
		DB.Table("quota_data").
			Where("user_id = ? and username = ? and model_name = ? and created_at = ? and use_group = ? and token_id = ? and channel_id = ? and node_name = ?",
				quotaData.UserID, quotaData.Username, quotaData.ModelName, quotaData.CreatedAt, quotaData.UseGroup, quotaData.TokenID, quotaData.ChannelID, quotaData.NodeName).
			First(quotaDataDB)
		if quotaDataDB.Id > 0 {
			//quotaDataDB.Count += quotaData.Count
			//quotaDataDB.Quota += quotaData.Quota
			//DB.Table("quota_data").Save(quotaDataDB)
			increaseQuotaData(quotaData)
		} else {
			DB.Table("quota_data").Create(quotaData)
		}
	}
	CacheQuotaData = make(map[string]*QuotaData)
	common.SysLog(fmt.Sprintf("保存数据看板数据成功，共保存%d条数据", size))
}

func increaseQuotaData(quotaData *QuotaData) {
	err := DB.Table("quota_data").
		Where("user_id = ? and username = ? and model_name = ? and created_at = ? and use_group = ? and token_id = ? and channel_id = ? and node_name = ?",
			quotaData.UserID, quotaData.Username, quotaData.ModelName, quotaData.CreatedAt, quotaData.UseGroup, quotaData.TokenID, quotaData.ChannelID, quotaData.NodeName).
		Updates(map[string]interface{}{
			"count":            gorm.Expr("count + ?", quotaData.Count),
			"quota":            gorm.Expr("quota + ?", quotaData.Quota),
			"token_used":       gorm.Expr("token_used + ?", quotaData.TokenUsed),
			"input_tokens":     gorm.Expr("input_tokens + ?", quotaData.InputTokens),
			"cached_tokens":    gorm.Expr("cached_tokens + ?", quotaData.CachedTokens),
			"reasoning_tokens": gorm.Expr("reasoning_tokens + ?", quotaData.ReasoningTokens),
		}).Error
	if err != nil {
		common.SysLog(fmt.Sprintf("increaseQuotaData error: %s", err))
	}
}

type QuotaDataFilters struct {
	ChannelIDs []int
	ModelNames []string
}

func (filters QuotaDataFilters) apply(query *gorm.DB) *gorm.DB {
	if len(filters.ChannelIDs) > 0 {
		query = query.Where("channel_id IN ?", filters.ChannelIDs)
	}
	if len(filters.ModelNames) > 0 {
		query = query.Where("model_name IN ?", filters.ModelNames)
	}
	return query
}

func GetQuotaDataByUsername(username string, startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	return GetQuotaDataByUsernameWithFilters(username, startTime, endTime, QuotaDataFilters{})
}

func GetQuotaDataByUsernameWithFilters(username string, startTime int64, endTime int64, filters QuotaDataFilters) (quotaData []*QuotaData, err error) {
	var quotaDatas []*QuotaData
	query := DB.Table("quota_data").Select("user_id, username, model_name, channel_id, created_at, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used, sum(input_tokens) as input_tokens, sum(cached_tokens) as cached_tokens, sum(reasoning_tokens) as reasoning_tokens").
		Where("username = ? and created_at >= ? and created_at <= ?", username, startTime, endTime)
	err = filters.apply(query).Group("user_id, username, model_name, channel_id, created_at").Find(&quotaDatas).Error
	if err != nil {
		return quotaDatas, err
	}
	err = fillQuotaChannelNames(quotaDatas)
	return quotaDatas, err
}

func GetQuotaDataByUserId(userId int, startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	return GetQuotaDataByUserIdWithFilters(userId, startTime, endTime, QuotaDataFilters{})
}

func GetQuotaDataByUserIdWithFilters(userId int, startTime int64, endTime int64, filters QuotaDataFilters) (quotaData []*QuotaData, err error) {
	var quotaDatas []*QuotaData
	query := DB.Table("quota_data").Select("user_id, username, model_name, channel_id, created_at, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used, sum(input_tokens) as input_tokens, sum(cached_tokens) as cached_tokens, sum(reasoning_tokens) as reasoning_tokens").
		Where("user_id = ? and created_at >= ? and created_at <= ?", userId, startTime, endTime)
	err = filters.apply(query).Group("user_id, username, model_name, channel_id, created_at").Find(&quotaDatas).Error
	if err != nil {
		return quotaDatas, err
	}
	err = fillQuotaChannelNames(quotaDatas)
	return quotaDatas, err
}

func GetQuotaDataGroupByUser(startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	var quotaDatas []*QuotaData
	err = DB.Table("quota_data").
		Select("username, created_at, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used").
		Where("created_at >= ? and created_at <= ?", startTime, endTime).
		Group("username, created_at").
		Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetAllQuotaDates(startTime int64, endTime int64, username string) (quotaData []*QuotaData, err error) {
	return GetAllQuotaDatesWithFilters(startTime, endTime, username, QuotaDataFilters{})
}

func GetAllQuotaDatesWithFilters(startTime int64, endTime int64, username string, filters QuotaDataFilters) (quotaData []*QuotaData, err error) {
	if username != "" {
		return GetQuotaDataByUsernameWithFilters(username, startTime, endTime, filters)
	}
	var quotaDatas []*QuotaData
	query := DB.Table("quota_data").Select("model_name, channel_id, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used, sum(input_tokens) as input_tokens, sum(cached_tokens) as cached_tokens, sum(reasoning_tokens) as reasoning_tokens, created_at").
		Where("created_at >= ? and created_at <= ?", startTime, endTime)
	err = filters.apply(query).Group("model_name, channel_id, created_at").Find(&quotaDatas).Error
	if err != nil {
		return quotaDatas, err
	}
	err = fillQuotaChannelNames(quotaDatas)
	return quotaDatas, err
}

func fillQuotaChannelNames(rows []*QuotaData) error {
	ids := make([]int, 0)
	seen := make(map[int]struct{})
	for _, row := range rows {
		if row.ChannelID > 0 {
			if _, ok := seen[row.ChannelID]; !ok {
				seen[row.ChannelID] = struct{}{}
				ids = append(ids, row.ChannelID)
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var channels []struct {
		ID   int    `gorm:"column:id"`
		Name string `gorm:"column:name"`
	}
	if err := DB.Table("channels").Select("id, name").Where("id IN ?", ids).Find(&channels).Error; err != nil {
		return err
	}
	names := make(map[int]string, len(channels))
	for _, channel := range channels {
		names[channel.ID] = channel.Name
	}
	for _, row := range rows {
		row.ChannelName = names[row.ChannelID]
		if row.ChannelName == "" && row.ChannelID > 0 {
			row.ChannelName = "channel-" + strconv.Itoa(row.ChannelID)
		}
	}
	return nil
}
