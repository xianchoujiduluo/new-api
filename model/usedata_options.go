package model

import (
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

func ParseQuotaDataFilters(channelValues, modelValues []string) QuotaDataFilters {
	filters := QuotaDataFilters{}
	for _, raw := range channelValues {
		for _, value := range strings.Split(raw, ",") {
			if id, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && id > 0 {
				filters.ChannelIDs = appendUniqueInt(filters.ChannelIDs, id)
			}
		}
	}
	for _, raw := range modelValues {
		for _, value := range strings.Split(raw, ",") {
			value = strings.TrimSpace(value)
			if value != "" && !containsString(filters.ModelNames, value) {
				filters.ModelNames = append(filters.ModelNames, value)
			}
		}
	}
	return filters
}

func appendUniqueInt(values []int, value int) []int {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func containsString(values []string, value string) bool {
	for _, existing := range values {
		if existing == value {
			return true
		}
	}
	return false
}

type QuotaDataFilterOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type QuotaDataFilterOptions struct {
	Channels []QuotaDataFilterOption `json:"channels"`
	Models   []QuotaDataFilterOption `json:"models"`
}

func GetQuotaDataFilterOptions(userID int, role int, startTimestamp int64, endTimestamp int64) (QuotaDataFilterOptions, error) {
	options := QuotaDataFilterOptions{
		Channels: make([]QuotaDataFilterOption, 0),
		Models:   make([]QuotaDataFilterOption, 0),
	}
	query := DB.Table("quota_data").Where("created_at >= ? AND created_at <= ?", startTimestamp, endTimestamp)
	if role < common.RoleAdminUser {
		query = query.Where("user_id = ?", userID)
	}

	channelNames := make(map[int]string)
	modelNames := make(map[string]struct{})
	var channelIDs []int
	if err := query.Distinct("channel_id").Where("channel_id > 0").Pluck("channel_id", &channelIDs).Error; err != nil {
		return options, err
	}

	if len(channelIDs) > 0 {
		var channels []struct {
			ID   int    `gorm:"column:id"`
			Name string `gorm:"column:name"`
		}
		if err := DB.Table("channels").Select("id, name").Where("id IN ?", channelIDs).Find(&channels).Error; err != nil {
			return options, err
		}
		for _, channel := range channels {
			channelNames[channel.ID] = channel.Name
		}
		for _, channelID := range channelIDs {
			label := channelNames[channelID]
			if label == "" {
				label = "channel-" + strconv.Itoa(channelID)
			}
			options.Channels = append(options.Channels, QuotaDataFilterOption{
				Value: strconv.Itoa(channelID),
				Label: label,
			})
		}
	}

	var observedModelNames []string
	if err := query.Distinct("model_name").Where("model_name <> ''").Pluck("model_name", &observedModelNames).Error; err != nil {
		return options, err
	}
	for _, modelName := range observedModelNames {
		modelName = strings.TrimSpace(modelName)
		if modelName != "" {
			modelNames[modelName] = struct{}{}
		}
	}
	modelList := make([]string, 0, len(modelNames))
	for modelName := range modelNames {
		modelList = append(modelList, modelName)
	}
	sort.Strings(modelList)
	for _, modelName := range modelList {
		options.Models = append(options.Models, QuotaDataFilterOption{
			Value: modelName,
			Label: modelName,
		})
	}
	return options, nil
}
