package model

import (
	"context"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// MobileCloudAssetGroup mirrors the provider group identity locally so the web
// console can list assets without making a signed upstream request on every
// page load. RawData preserves provider fields for forward compatibility.
type MobileCloudAssetGroup struct {
	ID              int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID          int    `json:"user_id" gorm:"index"`
	ChannelID       int    `json:"channel_id" gorm:"index"`
	ProviderGroupID string `json:"provider_group_id" gorm:"column:provider_group_id;type:varchar(191);index"`
	IsDefault       bool   `json:"is_default" gorm:"index"`
	GroupType       string `json:"group_type" gorm:"type:varchar(32)"`
	Name            string `json:"name" gorm:"type:varchar(191)"`
	Description     string `json:"description" gorm:"type:text"`
	RawData         string `json:"-" gorm:"type:text"`
	CreatedAt       int64  `json:"created_at" gorm:"index"`
	UpdatedAt       int64  `json:"updated_at"`
}

type MobileCloudAsset struct {
	ID              int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID          int    `json:"user_id" gorm:"index"`
	ChannelID       int    `json:"channel_id" gorm:"index"`
	ProviderAssetID string `json:"provider_asset_id" gorm:"column:provider_asset_id;type:varchar(191);index"`
	ProviderGroupID string `json:"provider_group_id,omitempty" gorm:"column:provider_group_id;type:varchar(191);index"`
	Name            string `json:"name" gorm:"type:varchar(191)"`
	Type            string `json:"type" gorm:"type:varchar(32);index"`
	Status          string `json:"status" gorm:"type:varchar(32);index"`
	AssetURL        string `json:"asset_url,omitempty" gorm:"type:text"`
	ErrorMessage    string `json:"error_message,omitempty" gorm:"type:text"`
	RawData         string `json:"-" gorm:"type:text"`
	CreatedAt       int64  `json:"created_at" gorm:"index"`
	UpdatedAt       int64  `json:"updated_at"`
}

func (g *MobileCloudAssetGroup) SetRawData(value any) {
	if data, err := common.Marshal(value); err == nil {
		g.RawData = string(data)
	}
}

func (a *MobileCloudAsset) SetRawData(value any) {
	if data, err := common.Marshal(value); err == nil {
		a.RawData = string(data)
	}
}

func ListMobileCloudAssetGroups(ctx context.Context, userID, channelID, offset, limit int) ([]MobileCloudAssetGroup, int64, error) {
	query := DB.WithContext(ctx).Model(&MobileCloudAssetGroup{}).Where("user_id = ?", userID)
	if channelID > 0 {
		query = query.Where("channel_id = ?", channelID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []MobileCloudAssetGroup
	if err := query.Order("id desc").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func ListMobileCloudAssets(ctx context.Context, userID, channelID int, groupID, assetType, status string, offset, limit int) ([]MobileCloudAsset, int64, error) {
	query := DB.WithContext(ctx).Model(&MobileCloudAsset{}).Where("user_id = ?", userID)
	if channelID > 0 {
		query = query.Where("channel_id = ?", channelID)
	}
	if groupID != "" {
		query = query.Where("provider_group_id = ?", groupID)
	}
	if assetType != "" {
		query = query.Where("type = ?", assetType)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []MobileCloudAsset
	if err := query.Order("id desc").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func GetMobileCloudAssetGroup(ctx context.Context, userID int, id int64) (*MobileCloudAssetGroup, error) {
	var item MobileCloudAssetGroup
	err := DB.WithContext(ctx).Where("user_id = ? AND id = ?", userID, id).First(&item).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &item, err
}

func GetMobileCloudAsset(ctx context.Context, userID int, id int64) (*MobileCloudAsset, error) {
	var item MobileCloudAsset
	err := DB.WithContext(ctx).Where("user_id = ? AND id = ?", userID, id).First(&item).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &item, err
}

// FindMobileCloudAssetGroupByProviderID resolves a provider group only within
// the current customer and channel. Provider IDs are not globally trusted:
// this lookup is the ownership boundary used by the proxy before forwarding
// get/update/delete requests upstream.
func FindMobileCloudAssetGroupByProviderID(ctx context.Context, userID, channelID int, providerID string) (*MobileCloudAssetGroup, error) {
	var item MobileCloudAssetGroup
	query := DB.WithContext(ctx).Where("user_id = ? AND provider_group_id = ?", userID, providerID)
	if channelID > 0 {
		query = query.Where("channel_id = ?", channelID)
	}
	err := query.First(&item).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &item, err
}

// FindMobileCloudAssetGroupByName resolves a group name within one customer
// and channel.  Names are compared case-insensitively after trimming so the
// uniqueness rule is consistent across SQLite, MySQL, and PostgreSQL
// collations.
func FindMobileCloudAssetGroupByName(ctx context.Context, userID, channelID int, name string) (*MobileCloudAssetGroup, error) {
	normalized := strings.TrimSpace(name)
	if normalized == "" {
		return nil, nil
	}
	var groups []MobileCloudAssetGroup
	if err := DB.WithContext(ctx).
		Where("user_id = ? AND channel_id = ?", userID, channelID).
		Find(&groups).Error; err != nil {
		return nil, err
	}
	for index := range groups {
		if strings.EqualFold(strings.TrimSpace(groups[index].Name), normalized) {
			return &groups[index], nil
		}
	}
	return nil, nil
}

func FindMobileCloudAssetByProviderID(ctx context.Context, userID, channelID int, providerID string) (*MobileCloudAsset, error) {
	var item MobileCloudAsset
	query := DB.WithContext(ctx).Where("user_id = ? AND provider_asset_id = ?", userID, providerID)
	if channelID > 0 {
		query = query.Where("channel_id = ?", channelID)
	}
	err := query.First(&item).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &item, err
}

func FindDefaultMobileCloudAssetGroup(ctx context.Context, userID, channelID int) (*MobileCloudAssetGroup, error) {
	var item MobileCloudAssetGroup
	err := DB.WithContext(ctx).
		Where("user_id = ? AND channel_id = ? AND is_default = ?", userID, channelID, true).
		Order("id asc").First(&item).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &item, err
}

func MarkMobileCloudAssetGroupDefault(ctx context.Context, userID, channelID int, providerID string) error {
	tx := DB.WithContext(ctx).Model(&MobileCloudAssetGroup{}).
		Where("user_id = ? AND channel_id = ?", userID, channelID).
		Update("is_default", false)
	if tx.Error != nil {
		return tx.Error
	}
	return DB.WithContext(ctx).Model(&MobileCloudAssetGroup{}).
		Where("user_id = ? AND channel_id = ? AND provider_group_id = ?", userID, channelID, providerID).
		Update("is_default", true).Error
}

func DeleteMobileCloudAssetGroupIndex(ctx context.Context, userID, channelID int, providerID string) error {
	if err := DB.WithContext(ctx).Where("user_id = ? AND channel_id = ? AND provider_group_id = ?", userID, channelID, providerID).Delete(&MobileCloudAssetGroup{}).Error; err != nil {
		return err
	}
	return DB.WithContext(ctx).Where("user_id = ? AND channel_id = ? AND provider_group_id = ?", userID, channelID, providerID).Delete(&MobileCloudAsset{}).Error
}

func DeleteMobileCloudAssetIndex(ctx context.Context, userID, channelID int, providerID string) error {
	return DB.WithContext(ctx).Where("user_id = ? AND channel_id = ? AND provider_asset_id = ?", userID, channelID, providerID).Delete(&MobileCloudAsset{}).Error
}

func TouchMobileCloudAssetGroup(group *MobileCloudAssetGroup) {
	if group.CreatedAt == 0 {
		group.CreatedAt = time.Now().Unix()
	}
	group.UpdatedAt = time.Now().Unix()
}

func TouchMobileCloudAsset(asset *MobileCloudAsset) {
	if asset.CreatedAt == 0 {
		asset.CreatedAt = time.Now().Unix()
	}
	asset.UpdatedAt = time.Now().Unix()
}
