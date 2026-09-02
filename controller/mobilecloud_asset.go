package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	mobilecloudasset "github.com/QuantumNous/new-api/service/mobilecloudasset"
	"github.com/gin-gonic/gin"
)

// Mobile Cloud's asset OpenAPI is exposed as a provider-neutral management
// API. The browser never receives AK/SK; credentials are read from the
// selected admin-configured task-plugin channel.
func ListMobileCloudAssetGroups(c *gin.Context) {
	client, channel, err := mobileCloudAssetClient(c)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	body := map[string]any{
		"pageNo":   parseAssetPage(c.Query("page")),
		"pageSize": parseAssetPageSize(c.Query("page_size")),
	}
	if value := strings.TrimSpace(c.Query("group_type")); value != "" {
		body["groupType"] = value
	}
	if value := strings.TrimSpace(c.Query("name")); value != "" {
		body["groupName"] = value
	}
	if value := strings.TrimSpace(c.Query("group_ids")); value != "" {
		body["groupIds"] = splitAssetIDs(value)
	}
	response, err := client.ListAssetGroups(c.Request.Context(), body)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	// Keep a local ownership/index copy while returning the provider envelope
	// unchanged so callers can use the same fields as the official API.
	persistAssetGroups(c.Request.Context(), c.GetInt("id"), channel.Id, providerBody(response))
	assetAPIResponse(c, response)
}

func CreateMobileCloudAssetGroup(c *gin.Context) {
	client, channel, err := mobileCloudAssetClient(c)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	body, err := readAssetJSON(c)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	groupType := strings.TrimSpace(stringValue(body["groupType"]))
	if groupType == "" {
		groupType = "AIGC"
	}
	if !strings.EqualFold(groupType, "AIGC") {
		assetAPIError(c, errors.New("only AIGC groups can be created through this endpoint; real-person groups require verification"))
		return
	}
	body["groupType"] = "AIGC"
	if name := strings.TrimSpace(stringValue(body["groupName"])); name == "" || len([]rune(name)) > 64 {
		assetAPIError(c, errors.New("groupName is required and must be at most 64 characters"))
		return
	}
	if description := stringValue(body["description"]); len([]rune(description)) > 300 {
		assetAPIError(c, errors.New("description must be at most 300 characters"))
		return
	}
	response, err := client.CreateAssetGroup(c.Request.Context(), body)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	persistAssetGroups(c.Request.Context(), c.GetInt("id"), channel.Id, providerBody(response))
	assetAPIResponse(c, response)
}

func GetMobileCloudAssetGroup(c *gin.Context) {
	client, _, err := mobileCloudAssetClient(c)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	response, err := client.GetAssetGroup(c.Request.Context(), c.Param("group_id"))
	if err != nil {
		assetAPIError(c, err)
		return
	}
	assetAPIResponse(c, response)
}

func UpdateMobileCloudAssetGroup(c *gin.Context) {
	client, _, err := mobileCloudAssetClient(c)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	body, err := readAssetJSON(c)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	response, err := client.UpdateAssetGroup(c.Request.Context(), c.Param("group_id"), body)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	assetAPIResponse(c, response)
}

func DeleteMobileCloudAssetGroup(c *gin.Context) {
	client, _, err := mobileCloudAssetClient(c)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	response, err := client.DeleteAssetGroup(c.Request.Context(), c.Param("group_id"))
	if err != nil {
		assetAPIError(c, err)
		return
	}
	assetAPIResponse(c, response)
}

func ListMobileCloudAssets(c *gin.Context) {
	client, channel, err := mobileCloudAssetClient(c)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	body := map[string]any{
		"pageNo":    parseAssetPage(c.Query("page")),
		"pageSize":  parseAssetPageSize(c.Query("page_size")),
		"groupType": "AIGC",
	}
	if value := strings.TrimSpace(c.Query("group_id")); value != "" {
		body["groupIds"] = []string{value}
	}
	if value := strings.TrimSpace(c.Query("group_type")); value != "" {
		body["groupType"] = value
	}
	if value := strings.TrimSpace(c.Query("name")); value != "" {
		body["assetName"] = value
	}
	if value := strings.TrimSpace(c.Query("status")); value != "" {
		body["statuses"] = splitAssetIDs(value)
	}
	response, err := client.ListAssets(c.Request.Context(), body)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	persistAssets(c.Request.Context(), c.GetInt("id"), channel.Id, providerBody(response))
	assetAPIResponse(c, response)
}

func CreateMobileCloudAsset(c *gin.Context) {
	client, channel, err := mobileCloudAssetClient(c)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	body, err := readAssetJSON(c)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	name := strings.TrimSpace(stringValue(body["assetName"]))
	assetURL := strings.TrimSpace(stringValue(body["assetUrl"]))
	assetType := strings.TrimSpace(stringValue(body["assetType"]))
	parsedURL, parseErr := url.Parse(assetURL)
	if name == "" || len([]rune(name)) > 64 || parseErr != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		assetAPIError(c, errors.New("assetName and a public HTTP(S) assetUrl are required; assetName must be at most 64 characters"))
		return
	}
	switch assetType {
	case "Image", "Video", "Audio":
	default:
		assetAPIError(c, errors.New("assetType must be Image, Video, or Audio"))
		return
	}
	if strings.TrimSpace(stringValue(body["groupId"])) == "" {
		assetAPIError(c, errors.New("groupId is required"))
		return
	}
	response, err := client.CreateAsset(c.Request.Context(), body)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	persistAssets(c.Request.Context(), c.GetInt("id"), channel.Id, providerBody(response))
	assetAPIResponse(c, response)
}

func GetMobileCloudAsset(c *gin.Context) {
	client, _, err := mobileCloudAssetClient(c)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	response, err := client.GetAsset(c.Request.Context(), c.Param("asset_id"))
	if err != nil {
		assetAPIError(c, err)
		return
	}
	assetAPIResponse(c, response)
}

func UpdateMobileCloudAsset(c *gin.Context) {
	client, _, err := mobileCloudAssetClient(c)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	body, err := readAssetJSON(c)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	response, err := client.UpdateAsset(c.Request.Context(), c.Param("asset_id"), body)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	assetAPIResponse(c, response)
}

func DeleteMobileCloudAsset(c *gin.Context) {
	client, _, err := mobileCloudAssetClient(c)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	response, err := client.DeleteAsset(c.Request.Context(), c.Param("asset_id"))
	if err != nil {
		assetAPIError(c, err)
		return
	}
	assetAPIResponse(c, response)
}

func CreateMobileCloudRealPersonSession(c *gin.Context) {
	client, _, err := mobileCloudAssetClient(c)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	body, err := readAssetJSON(c)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	response, err := client.CreateRealPersonSession(c.Request.Context(), body)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	assetAPIResponse(c, response)
}

func GetMobileCloudAssetGroupByBytedToken(c *gin.Context) {
	client, _, err := mobileCloudAssetClient(c)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	body, err := readAssetJSON(c)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	if strings.TrimSpace(stringValue(body["bytedToken"])) == "" {
		assetAPIError(c, errors.New("bytedToken is required"))
		return
	}
	response, err := client.GetAssetGroupByBytedToken(c.Request.Context(), body)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	assetAPIResponse(c, response)
}

func mobileCloudAssetClient(c *gin.Context) (*mobilecloudasset.Client, *model.Channel, error) {
	channelID, _ := strconv.Atoi(strings.TrimSpace(c.Query("channel_id")))
	var channel *model.Channel
	var err error
	if channelID > 0 {
		channel, err = model.GetChannelById(channelID, true)
	} else {
		channels, listErr := model.GetAllChannels(0, 0, true, true)
		err = listErr
		for _, candidate := range channels {
			if candidate == nil || candidate.Type != constant.ChannelTypeTaskPlugin {
				continue
			}
			setting := candidate.GetSetting()
			if strings.EqualFold(strings.TrimSpace(setting.TaskPluginKey), "mobilecloud") && setting.MobileCloudAssetLibraryEnabled() {
				channel = candidate
				break
			}
		}
	}
	if err != nil {
		return nil, nil, err
	}
	if channel == nil {
		return nil, nil, errors.New("no Mobile Cloud channel with asset AccessKey/SecretKey is configured")
	}
	if channel.Type != constant.ChannelTypeTaskPlugin {
		return nil, nil, errors.New("selected channel must be a task plugin channel")
	}
	setting := channel.GetSetting()
	if !strings.EqualFold(strings.TrimSpace(setting.TaskPluginKey), "mobilecloud") {
		return nil, nil, errors.New("selected channel is not a Mobile Cloud task plugin channel")
	}
	if !setting.MobileCloudAssetLibraryEnabled() {
		return nil, nil, errors.New("Mobile Cloud asset library is disabled or its credentials are incomplete")
	}
	client, err := mobilecloudasset.NewClient(mobilecloudasset.Config{
		BaseURL: setting.AssetBaseURL, AccessKey: setting.AssetAccessKey, SecretKey: setting.AssetSecretKey,
		ResourcePool: setting.AssetResourcePool,
	})
	if err != nil {
		return nil, nil, err
	}
	return client, channel, nil
}

func readAssetJSON(c *gin.Context) (map[string]any, error) {
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		return nil, errors.New("request body must be valid JSON")
	}
	if body == nil {
		body = map[string]any{}
	}
	return body, nil
}

func providerBody(response *mobilecloudasset.Response) any {
	if response == nil || len(response.Body) == 0 {
		return nil
	}
	var envelope map[string]any
	if common.Unmarshal(response.Body, &envelope) != nil {
		return nil
	}
	if body, ok := envelope["body"]; ok {
		return body
	}
	return envelope
}

func persistAssetGroups(ctx context.Context, userID, channelID int, value any) {
	envelope, ok := value.(map[string]any)
	if !ok {
		return
	}
	items := []any{envelope}
	if data, exists := envelope["data"].([]any); exists {
		items = data
	}
	for _, item := range items {
		group, ok := item.(map[string]any)
		if !ok {
			continue
		}
		providerID := stringValue(group["groupId"])
		if providerID == "" {
			continue
		}
		var row model.MobileCloudAssetGroup
		if err := model.DB.WithContext(ctx).Where("user_id = ? AND provider_group_id = ?", userID, providerID).First(&row).Error; err != nil {
			row = model.MobileCloudAssetGroup{UserID: userID, ChannelID: channelID, ProviderGroupID: providerID}
		}
		row.ChannelID = channelID
		row.GroupType = stringValue(group["groupType"])
		row.Name = stringValue(group["groupName"])
		row.Description = stringValue(group["description"])
		row.SetRawData(group)
		model.TouchMobileCloudAssetGroup(&row)
		_ = model.DB.WithContext(ctx).Save(&row).Error
	}
}

func persistAssets(ctx context.Context, userID, channelID int, value any) {
	envelope, ok := value.(map[string]any)
	if !ok {
		if providerID := stringValue(value); providerID != "" {
			return
		}
		return
	}
	items := []any{envelope}
	if data, exists := envelope["data"].([]any); exists {
		items = data
	}
	for _, item := range items {
		asset, ok := item.(map[string]any)
		if !ok {
			continue
		}
		providerID := stringValue(asset["assetId"])
		if providerID == "" {
			continue
		}
		var row model.MobileCloudAsset
		if err := model.DB.WithContext(ctx).Where("user_id = ? AND provider_asset_id = ?", userID, providerID).First(&row).Error; err != nil {
			row = model.MobileCloudAsset{UserID: userID, ChannelID: channelID, ProviderAssetID: providerID}
		}
		row.ChannelID = channelID
		row.ProviderGroupID = stringValue(asset["groupId"])
		row.Name = stringValue(asset["assetName"])
		row.Type = stringValue(asset["assetType"])
		row.Status = stringValue(asset["status"])
		row.AssetURL = stringValue(asset["assetUrl"])
		row.ErrorMessage = stringValue(asset["errorMessage"])
		row.SetRawData(asset)
		model.TouchMobileCloudAsset(&row)
		_ = model.DB.WithContext(ctx).Save(&row).Error
	}
}

func assetAPIResponse(c *gin.Context, response *mobilecloudasset.Response) {
	if response == nil || len(response.Body) == 0 {
		common.ApiSuccess(c, nil)
		return
	}
	var value any
	if err := common.Unmarshal(response.Body, &value); err != nil {
		common.ApiSuccess(c, gin.H{"raw": string(response.Body)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": value})
}

func assetAPIError(c *gin.Context, err error) {
	message := "Mobile Cloud asset request failed"
	status := http.StatusBadRequest
	var providerErr *mobilecloudasset.ProviderError
	if errors.As(err, &providerErr) && providerErr != nil {
		message = providerErr.Error()
		status = providerErr.StatusCode
		if status < 400 || status > 599 {
			status = http.StatusBadGateway
		} else if status >= 500 {
			status = http.StatusBadGateway
		}
	}
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		message = err.Error()
	}
	c.JSON(status, gin.H{"success": false, "message": message})
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func parseAssetPage(value string) int {
	page, err := strconv.Atoi(value)
	if err != nil || page < 1 {
		return 1
	}
	if page > 10000 {
		return 10000
	}
	return page
}

func parseAssetPageSize(value string) int {
	size, err := strconv.Atoi(value)
	if err != nil || size < 1 {
		return 20
	}
	if size > 100 {
		return 100
	}
	return size
}

func splitAssetIDs(value string) []string {
	parts := strings.Split(value, ",")
	ids := make([]string, 0, len(parts))
	for _, part := range parts {
		if id := strings.TrimSpace(part); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}
