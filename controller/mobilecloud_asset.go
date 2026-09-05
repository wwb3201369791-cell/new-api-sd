package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/assetstore"
	mobilecloudasset "github.com/QuantumNous/new-api/service/mobilecloudasset"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var defaultAssetGroupMu sync.Mutex

type assetOwnershipError struct {
	resource string
	provider string
}

func (e *assetOwnershipError) Error() string {
	return fmt.Sprintf("%s not found or does not belong to the current customer", e.resource)
}

type assetConflictError struct {
	code    string
	message string
}

func (e *assetConflictError) Error() string {
	if e == nil {
		return "asset conflict"
	}
	return e.message
}

// Upstream asset APIs are exposed as a provider-neutral management
// API. The browser never receives AK/SK; credentials are read from the
// selected admin-configured task-plugin channel.
func ListMobileCloudAssetGroups(c *gin.Context) {
	client, channel, err := mobileCloudAssetClient(c)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	if _, err := ensureMobileCloudDefaultGroup(c.Request.Context(), c.GetInt("id"), channel.Id, client); err != nil {
		assetAPIError(c, err)
		return
	}
	ownedGroups, _, err := model.ListMobileCloudAssetGroups(c.Request.Context(), c.GetInt("id"), channel.Id, 0, 10000)
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
	ownedIDs := make(map[string]struct{}, len(ownedGroups))
	for _, group := range ownedGroups {
		if group.ProviderGroupID != "" {
			ownedIDs[group.ProviderGroupID] = struct{}{}
		}
	}
	if value := strings.TrimSpace(c.Query("group_ids")); value != "" {
		requestedIDs := splitAssetIDs(value)
		for _, providerID := range requestedIDs {
			if _, ok := ownedIDs[providerID]; !ok {
				assetAPIError(c, &assetOwnershipError{resource: "asset group", provider: providerID})
				return
			}
		}
		body["groupIds"] = requestedIDs
	} else if len(ownedIDs) > 0 {
		body["groupIds"] = assetIDKeys(ownedIDs)
	} else {
		writeEmptyAssetList(c)
		return
	}
	response, err := client.ListAssetGroups(c.Request.Context(), body)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	restrictProviderResponse(response, ownedIDs, "groupId")
	// Keep a local ownership/index copy while returning the provider envelope
	// unchanged so callers can use the same fields as the official API.
	logAssetIndexError(c, "group", channel.Id, persistAssetGroups(c.Request.Context(), c.GetInt("id"), channel.Id, providerBody(response)))
	assetAPIResponse(c, response)
}

func CreateMobileCloudAssetGroup(c *gin.Context) {
	client, channel, err := mobileCloudAssetClient(c)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	// Every customer gets a stable default group, even when their first asset
	// operation is creating a custom group rather than listing assets.
	if _, err := ensureMobileCloudDefaultGroup(c.Request.Context(), c.GetInt("id"), channel.Id, client); err != nil {
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
	if isRunyuanChannel(channel) {
		// Runyuan documents AIGC groups as account-scoped and idempotent: one
		// provider group is shared by all calls made with the same AK/SK. Do not
		// pretend that a second tenant-local group can be created upstream.
		assetAPIError(c, &assetConflictError{
			code:    "ASSET_GROUP_LIMIT",
			message: "当前素材服务仅提供一个默认 AIGC 素材组，请直接使用默认素材组",
		})
		return
	}
	groupName := strings.TrimSpace(stringValue(body["groupName"]))
	if groupName == "" || len([]rune(groupName)) > 64 {
		assetAPIError(c, errors.New("groupName is required and must be at most 64 characters"))
		return
	}
	body["groupName"] = groupName
	if description := stringValue(body["description"]); len([]rune(description)) > 300 {
		assetAPIError(c, errors.New("description must be at most 300 characters"))
		return
	}
	if existing, lookupErr := model.FindMobileCloudAssetGroupByName(c.Request.Context(), c.GetInt("id"), channel.Id, groupName); lookupErr != nil {
		assetAPIError(c, lookupErr)
		return
	} else if existing != nil {
		assetAPIError(c, &assetConflictError{
			code:    "ASSET_GROUP_NAME_EXISTS",
			message: "asset group name already exists for this customer and channel",
		})
		return
	}
	response, err := client.CreateAssetGroup(c.Request.Context(), body)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	logAssetIndexError(c, "group", channel.Id, persistAssetGroups(c.Request.Context(), c.GetInt("id"), channel.Id, providerBody(response)))
	assetAPIResponse(c, response)
}

func GetMobileCloudAssetGroup(c *gin.Context) {
	client, channel, err := mobileCloudAssetClient(c)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	groupID := strings.TrimSpace(c.Param("group_id"))
	group, err := requireOwnedAssetGroup(c, channel.Id, groupID)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	response, err := client.GetAssetGroup(c.Request.Context(), group.ProviderGroupID)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	assetAPIResponse(c, response)
}

func UpdateMobileCloudAssetGroup(c *gin.Context) {
	client, channel, err := mobileCloudAssetClient(c)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	groupID := strings.TrimSpace(c.Param("group_id"))
	group, err := requireOwnedAssetGroup(c, channel.Id, groupID)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	if isRunyuanChannel(channel) {
		shared, sharedErr := runyuanGroupShared(c.Request.Context(), channel.Id, group.ProviderGroupID)
		if sharedErr != nil {
			assetAPIError(c, sharedErr)
			return
		}
		if shared {
			assetAPIError(c, &assetConflictError{
				code:    "ASSET_GROUP_SHARED",
				message: "该素材组正在被多个客户使用，当前无法更新",
			})
			return
		}
	}
	body, err := readAssetJSON(c)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	response, err := client.UpdateAssetGroup(c.Request.Context(), group.ProviderGroupID, body)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	logAssetIndexError(c, "group", channel.Id, persistAssetGroups(c.Request.Context(), c.GetInt("id"), channel.Id, providerBody(response)))
	assetAPIResponse(c, response)
}

func DeleteMobileCloudAssetGroup(c *gin.Context) {
	client, channel, err := mobileCloudAssetClient(c)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	groupID := strings.TrimSpace(c.Param("group_id"))
	group, err := requireOwnedAssetGroup(c, channel.Id, groupID)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	if group.IsDefault {
		assetAPIError(c, errors.New("default asset group cannot be deleted"))
		return
	}
	if isRunyuanChannel(channel) {
		shared, sharedErr := runyuanGroupShared(c.Request.Context(), channel.Id, group.ProviderGroupID)
		if sharedErr != nil {
			assetAPIError(c, sharedErr)
			return
		}
		if shared {
			assetAPIError(c, &assetConflictError{
				code:    "ASSET_GROUP_SHARED",
				message: "该素材组正在被多个客户使用，当前无法删除",
			})
			return
		}
	}
	response, err := client.DeleteAssetGroup(c.Request.Context(), group.ProviderGroupID)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	if err := model.DeleteMobileCloudAssetGroupIndex(c.Request.Context(), c.GetInt("id"), channel.Id, group.ProviderGroupID); err != nil {
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
	if _, err := ensureMobileCloudDefaultGroup(c.Request.Context(), c.GetInt("id"), channel.Id, client); err != nil {
		assetAPIError(c, err)
		return
	}
	ownedGroups, _, err := model.ListMobileCloudAssetGroups(c.Request.Context(), c.GetInt("id"), channel.Id, 0, 10000)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	ownedIDs := make(map[string]struct{}, len(ownedGroups))
	for _, group := range ownedGroups {
		if group.ProviderGroupID != "" {
			ownedIDs[group.ProviderGroupID] = struct{}{}
		}
	}
	body := map[string]any{
		"pageNo":    parseAssetPage(c.Query("page")),
		"pageSize":  parseAssetPageSize(c.Query("page_size")),
		"groupType": "AIGC",
	}
	if value := strings.TrimSpace(c.Query("group_id")); value != "" {
		if _, ok := ownedIDs[value]; !ok {
			assetAPIError(c, &assetOwnershipError{resource: "asset group", provider: value})
			return
		}
		body["groupIds"] = []string{value}
	} else if len(ownedIDs) > 0 {
		body["groupIds"] = assetIDKeys(ownedIDs)
	} else {
		writeEmptyAssetList(c)
		return
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
	restrictProviderResponse(response, ownedIDs, "groupId")
	// Runyuan may expose one account-level AIGC group for every customer that
	// shares the configured AK/SK. Group filtering alone would then return
	// another customer's assets. Keep only provider asset IDs already indexed
	// for this customer; assets created outside the gateway remain hidden until
	// the customer registers them through this API.
	if response != nil && response.Provider == "runyuan" {
		ownedAssets, _, indexErr := model.ListMobileCloudAssets(c.Request.Context(), c.GetInt("id"), channel.Id, "", "", "", 0, 10000)
		if indexErr != nil {
			assetAPIError(c, indexErr)
			return
		}
		ownedAssetIDs := make(map[string]struct{}, len(ownedAssets))
		for _, asset := range ownedAssets {
			if asset.ProviderAssetID != "" {
				ownedAssetIDs[asset.ProviderAssetID] = struct{}{}
			}
		}
		restrictProviderAssetResponse(response, ownedAssetIDs)
	}
	logAssetIndexError(c, "asset", channel.Id, persistAssets(c.Request.Context(), c.GetInt("id"), channel.Id, providerBody(response)))
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
	groupID := strings.TrimSpace(stringValue(body["groupId"]))
	if groupID == "" {
		groupID, err = ensureMobileCloudDefaultGroup(c.Request.Context(), c.GetInt("id"), channel.Id, client)
		if err != nil {
			assetAPIError(c, err)
			return
		}
	} else if _, err := requireOwnedAssetGroup(c, channel.Id, groupID); err != nil {
		assetAPIError(c, err)
		return
	}
	body["groupId"] = groupID
	response, err := client.CreateAsset(c.Request.Context(), body)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	logAssetIndexError(c, "asset", channel.Id, persistAssets(c.Request.Context(), c.GetInt("id"), channel.Id, providerBody(response)))
	logAssetIndexError(c, "asset", channel.Id, persistCreatedAsset(c.Request.Context(), c.GetInt("id"), channel.Id, body, response))
	assetAPIResponse(c, response)
}

// GetMobileCloudAssetStorage exposes only non-sensitive storage capabilities
// so the web UI can validate files before uploading. Provider credentials are
// intentionally never returned.
func GetMobileCloudAssetStorage(c *gin.Context) {
	store := assetstore.Get()
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "asset storage is not configured"})
		return
	}
	config := store.Config()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"enabled":        true,
			"upload_enabled": store.UploadsEnabled(),
			"mode":           config.Mode,
			"max_bytes":      config.MaxBytes,
			"allowed_types":  []string{"image/jpeg", "image/png", "image/webp", "image/gif", "video/mp4", "video/webm", "video/quicktime", "audio/mpeg", "audio/wav", "audio/x-wav", "audio/ogg"},
		},
	})
}

// TestMobileCloudAssetConnection performs a read-only request against the
// selected task-plugin channel's asset provider.  Video generation uses the
// channel Bearer key, while the asset library uses its separate AK/SK pair;
// keeping this check independent makes credential mistakes visible before a
// user starts an upload or creates an asset group.
func TestMobileCloudAssetConnection(c *gin.Context) {
	channelID, err := strconv.Atoi(strings.TrimSpace(c.Param("id")))
	if err != nil || channelID <= 0 {
		assetAPIError(c, errors.New("channel id must be a positive integer"))
		return
	}
	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	if channel == nil {
		assetAPIError(c, errors.New("channel not found"))
		return
	}
	client, err := mobileCloudAssetClientForChannel(channel)
	if err != nil {
		assetAPIError(c, err)
		return
	}

	requestCtx := context.Background()
	if c.Request != nil && c.Request.Context() != nil {
		requestCtx = c.Request.Context()
	}
	requestCtx, cancel := context.WithTimeout(requestCtx, 15*time.Second)
	defer cancel()
	response, err := client.ListAssetGroups(requestCtx, map[string]any{
		"pageNo":    1,
		"pageSize":  1,
		"groupType": "AIGC",
	})
	if err != nil {
		assetAPIError(c, err)
		return
	}
	if response == nil {
		assetAPIError(c, errors.New("asset provider returned an empty response"))
		return
	}

	data := gin.H{
		"channel_id":   channel.Id,
		"channel_name": channel.Name,
		"provider":     response.Provider,
		"status_code":  response.StatusCode,
	}
	if requestID := providerRequestID(response); requestID != "" {
		data["upstream_request_id"] = requestID
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "asset library connection succeeded",
		"data":    data,
	})
}

// UploadMobileCloudAsset stores a browser-selected file in the configured
// local/S3-compatible object store and registers its public URL with the
// selected upstream asset group. Omitting group_id uses the customer's default
// group.
func UploadMobileCloudAsset(c *gin.Context) {
	store := assetstore.Get()
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "asset storage is not configured"})
		return
	}
	if !store.UploadsEnabled() {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "multipart uploads are disabled; submit a public assetUrl instead"})
		return
	}
	client, channel, clientErr := mobileCloudAssetClient(c)
	if clientErr != nil {
		assetAPIError(c, clientErr)
		return
	}
	groupID := strings.TrimSpace(c.PostForm("group_id"))
	if groupID == "" {
		groupID, clientErr = ensureMobileCloudDefaultGroup(c.Request.Context(), c.GetInt("id"), channel.Id, client)
		if clientErr != nil {
			assetAPIError(c, clientErr)
			return
		}
	} else if _, clientErr = requireOwnedAssetGroup(c, channel.Id, groupID); clientErr != nil {
		assetAPIError(c, clientErr)
		return
	}
	limit := store.Config().MaxBytes
	// Multipart framing adds a small amount of overhead. The storage layer
	// still enforces the exact byte limit while this protects request parsing.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit+1<<20)
	if err := c.Request.ParseMultipartForm(limit + 1<<20); err != nil {
		assetAPIError(c, fmt.Errorf("invalid multipart upload: %w", err))
		return
	}
	header, err := c.FormFile("file")
	if err != nil {
		assetAPIError(c, errors.New("multipart field 'file' is required"))
		return
	}
	if header.Size < 0 || header.Size > limit {
		assetAPIError(c, fmt.Errorf("asset exceeds configured size limit of %d bytes", limit))
		return
	}
	file, err := header.Open()
	if err != nil {
		assetAPIError(c, fmt.Errorf("open uploaded asset: %w", err))
		return
	}
	defer file.Close()
	contentType := assetstore.NormalizeContentType(header.Header.Get("Content-Type"))
	var sniffed [512]byte
	read, readErr := io.ReadFull(file, sniffed[:])
	if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
		assetAPIError(c, fmt.Errorf("inspect uploaded asset: %w", readErr))
		return
	}
	if detected := assetstore.NormalizeContentType(http.DetectContentType(sniffed[:read])); !assetstore.IsAllowedContentType(contentType) {
		contentType = detected
	}
	if !assetstore.IsAllowedContentType(contentType) {
		assetAPIError(c, errors.New("only image, video, and audio assets are supported"))
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		assetAPIError(c, fmt.Errorf("rewind uploaded asset: %w", err))
		return
	}
	name := strings.TrimSpace(c.PostForm("asset_name"))
	if name == "" {
		name = strings.TrimSpace(filepath.Base(header.Filename))
	}
	if name == "" || len([]rune(name)) > 64 {
		assetAPIError(c, errors.New("asset_name is required and must be at most 64 characters"))
		return
	}
	object, err := store.Upload(c.Request.Context(), file, name, contentType, header.Size)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	assetURL, err := store.URL(object.Key, requestAssetBaseURL(c))
	if err != nil {
		// Avoid orphaning a successfully written object when the public URL is
		// misconfigured (for example, no ServerAddress behind a reverse proxy).
		_ = store.Delete(c.Request.Context(), object.Key)
		assetAPIError(c, err)
		return
	}
	object.URL = assetURL
	result := gin.H{"upload": object}

	assetType := strings.TrimSpace(c.PostForm("asset_type"))
	if assetType == "" {
		assetType = assetTypeFromContentType(contentType)
	}
	if assetType != "Image" && assetType != "Video" && assetType != "Audio" {
		_ = store.Delete(c.Request.Context(), object.Key)
		assetAPIError(c, errors.New("asset_type must be Image, Video, or Audio"))
		return
	}
	providerRequest := map[string]any{"assetName": name, "assetUrl": assetURL, "assetType": assetType, "groupId": groupID}
	response, providerErr := client.CreateAsset(c.Request.Context(), providerRequest)
	if providerErr != nil {
		// Keep the uploaded object in the response so the user can retry
		// provider registration without uploading the bytes again.
		var providerResponseErr *mobilecloudasset.ProviderError
		if !errors.As(providerErr, &providerResponseErr) || providerResponseErr == nil {
			assetAPIError(c, providerErr)
			return
		}
		logger.LogWarn(c.Request.Context(), "asset provider response status=%d code=%q detail=%q", providerResponseErr.StatusCode, providerResponseErr.Code, common.LocalLogPreview(common.MaskSensitiveInfo(providerResponseErr.Message)))
		status, code, message := publicAssetProviderError(providerResponseErr)
		result["error"] = gin.H{"code": code, "message": message}
		c.JSON(status, gin.H{"success": false, "code": code, "message": message, "data": result})
		return
	}
	logAssetIndexError(c, "asset", channel.Id, persistAssets(c.Request.Context(), c.GetInt("id"), channel.Id, providerBody(response)))
	logAssetIndexError(c, "asset", channel.Id, persistCreatedAsset(c.Request.Context(), c.GetInt("id"), channel.Id, providerRequest, response))
	result["asset"] = providerResponseValue(response)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": result})
}

// ServeMobileCloudAsset serves local objects without requiring a user token;
// The upstream must be able to fetch the URL asynchronously after registration.
// Object keys are random, single path segments, and traversal is rejected by
// the storage boundary.
func ServeMobileCloudAsset(c *gin.Context) {
	store := assetstore.Get()
	if store == nil {
		c.Status(http.StatusNotFound)
		return
	}
	path, err := store.LocalPath(c.Param("object_key"))
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Content-Disposition", "inline")
	http.ServeContent(c.Writer, c.Request, filepath.Base(path), info.ModTime(), file)
}

func GetMobileCloudAsset(c *gin.Context) {
	client, channel, err := mobileCloudAssetClient(c)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	assetID := strings.TrimSpace(c.Param("asset_id"))
	asset, err := requireOwnedAsset(c, channel.Id, assetID)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	response, err := client.GetAsset(c.Request.Context(), asset.ProviderAssetID)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	assetAPIResponse(c, response)
}

func UpdateMobileCloudAsset(c *gin.Context) {
	client, channel, err := mobileCloudAssetClient(c)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	assetID := strings.TrimSpace(c.Param("asset_id"))
	asset, err := requireOwnedAsset(c, channel.Id, assetID)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	body, err := readAssetJSON(c)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	response, err := client.UpdateAsset(c.Request.Context(), asset.ProviderAssetID, body)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	logAssetIndexError(c, "asset", channel.Id, persistAssets(c.Request.Context(), c.GetInt("id"), channel.Id, providerBody(response)))
	assetAPIResponse(c, response)
}

func DeleteMobileCloudAsset(c *gin.Context) {
	client, channel, err := mobileCloudAssetClient(c)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	assetID := strings.TrimSpace(c.Param("asset_id"))
	asset, err := requireOwnedAsset(c, channel.Id, assetID)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	assetURL := asset.AssetURL
	response, err := client.DeleteAsset(c.Request.Context(), asset.ProviderAssetID)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	if key := localAssetObjectKey(assetURL); key != "" {
		if store := assetstore.Get(); store != nil {
			_ = store.Delete(c.Request.Context(), key)
		}
	}
	if err := model.DeleteMobileCloudAssetIndex(c.Request.Context(), c.GetInt("id"), channel.Id, asset.ProviderAssetID); err != nil {
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
	body, err := readAssetJSONAllowEmpty(c)
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

func QueryMobileCloudModelTokensConsumed(c *gin.Context) {
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
	response, err := client.QueryModelTokensConsumed(c.Request.Context(), body)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	assetAPIResponse(c, response)
}

// CancelMobileCloudOrder exposes the upstream按量计费预置模型退订 API.  It
// deliberately lives beside the operator billing endpoints and is not used by
// video task cancellation.
func CancelMobileCloudOrder(c *gin.Context) {
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
	instanceIDs, ok := body["instanceIds"]
	if !ok || !hasStringItems(instanceIDs) {
		assetAPIError(c, errors.New("instanceIds must be a non-empty array"))
		return
	}
	response, err := client.CancelOrder(c.Request.Context(), body)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	assetAPIResponse(c, response)
}

func hasStringItems(value any) bool {
	switch items := value.(type) {
	case []any:
		if len(items) == 0 {
			return false
		}
		for _, item := range items {
			if strings.TrimSpace(stringValue(item)) == "" {
				return false
			}
		}
		return true
	case []string:
		if len(items) == 0 {
			return false
		}
		for _, item := range items {
			if strings.TrimSpace(item) == "" {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func QueryMobileCloudAiccCreditDeduction(c *gin.Context) {
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
	response, err := client.QueryAiccCreditDeduction(c.Request.Context(), body)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	assetAPIResponse(c, response)
}

func CreateMobileCloudAiccDeductionExportTask(c *gin.Context) {
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
	response, err := client.CreateAiccDeductionExportTask(c.Request.Context(), body)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	assetAPIResponse(c, response)
}

func GetMobileCloudAiccDeductionExportTask(c *gin.Context) {
	client, _, err := mobileCloudAssetClient(c)
	if err != nil {
		assetAPIError(c, err)
		return
	}
	taskID := strings.TrimSpace(c.Param("task_id"))
	if taskID == "" || len(taskID) > 128 {
		assetAPIError(c, errors.New("task_id is required"))
		return
	}
	response, err := client.GetAiccDeductionExportTask(c.Request.Context(), taskID)
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
			pluginKey := strings.ToLower(strings.TrimSpace(setting.TaskPluginKey))
			if (pluginKey == "mobilecloud" || pluginKey == "runyuan") && setting.MobileCloudAssetLibraryEnabled() {
				channel = candidate
				break
			}
		}
	}
	if err != nil {
		return nil, nil, err
	}
	if channel == nil {
		return nil, nil, errors.New("no Mobile Cloud or Runyuan channel with asset AccessKey/SecretKey is configured")
	}
	client, err := mobileCloudAssetClientForChannel(channel)
	if err != nil {
		return nil, nil, err
	}
	return client, channel, nil
}

func mobileCloudAssetClientForChannel(channel *model.Channel) (*mobilecloudasset.Client, error) {
	if channel == nil {
		return nil, errors.New("channel not found")
	}
	if channel.Type != constant.ChannelTypeTaskPlugin {
		return nil, errors.New("selected channel must be a task plugin channel")
	}
	setting := channel.GetSetting()
	pluginKey := strings.ToLower(strings.TrimSpace(setting.TaskPluginKey))
	if pluginKey != "mobilecloud" && pluginKey != "runyuan" {
		return nil, errors.New("selected channel is not a Mobile Cloud or Runyuan task plugin channel")
	}
	if err := setting.ValidateMobileCloudAssets(); err != nil {
		return nil, err
	}
	if !setting.MobileCloudAssetLibraryEnabled() {
		return nil, errors.New("asset library is disabled or its credentials are incomplete")
	}
	return mobilecloudasset.NewClient(mobilecloudasset.Config{
		Provider: pluginKey,
		BaseURL:  setting.AssetBaseURL, AccessKey: setting.AssetAccessKey, SecretKey: setting.AssetSecretKey,
		ResourcePool: setting.AssetResourcePool,
	})
}

func isRunyuanChannel(channel *model.Channel) bool {
	if channel == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(channel.GetSetting().TaskPluginKey), "runyuan")
}

func providerRequestID(response *mobilecloudasset.Response) string {
	if response == nil {
		return ""
	}
	if response.Header != nil {
		for _, key := range []string{"X-Request-Id", "X-Request-ID", "Request-Id", "Request-ID", "X-Tt-Logid"} {
			if value := strings.TrimSpace(response.Header.Get(key)); value != "" {
				return value
			}
		}
	}
	var envelope map[string]any
	if common.Unmarshal(response.Body, &envelope) == nil {
		for _, key := range []string{"requestId", "requestID", "request_id", "RequestId", "RequestID"} {
			if value := stringValue(envelope[key]); value != "" {
				return value
			}
		}
		if metadata, ok := envelope["ResponseMetadata"].(map[string]any); ok {
			for _, key := range []string{"RequestId", "RequestID", "requestId", "requestID", "request_id"} {
				if value := stringValue(metadata[key]); value != "" {
					return value
				}
			}
		}
	}
	return ""
}

func readAssetJSON(c *gin.Context) (map[string]any, error) {
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		return nil, errors.New("request body must be valid JSON")
	}
	if body == nil {
		body = map[string]any{}
	}
	normalizeAssetRequestAliases(body)
	return body, nil
}

// readAssetJSONAllowEmpty is used only by endpoints whose upstream contract
// permits an omitted request body. A missing body is equivalent to an empty
// JSON object; malformed non-empty JSON remains a client error.
func readAssetJSONAllowEmpty(c *gin.Context) (map[string]any, error) {
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return map[string]any{}, nil
	}
	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, errors.New("request body must be valid JSON")
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, nil
	}
	var body map[string]any
	if err := common.Unmarshal(data, &body); err != nil {
		return nil, errors.New("request body must be valid JSON")
	}
	if body == nil {
		body = map[string]any{}
	}
	normalizeAssetRequestAliases(body)
	return body, nil
}

// normalizeAssetRequestAliases keeps the public asset API compatible with
// clients that use the snake_case names documented by the OpenAI-style
// gateway. Provider clients receive the existing camelCase fields. When both
// forms are supplied, the canonical camelCase value wins deterministically.
func normalizeAssetRequestAliases(body map[string]any) {
	aliases := map[string]string{
		"group_id":   "groupId",
		"group_type": "groupType",
		"group_name": "groupName",
		"asset_name": "assetName",
		"asset_url":  "assetUrl",
		"asset_type": "assetType",
		"page_no":    "pageNo",
		"page_size":  "pageSize",
	}
	for alias, canonical := range aliases {
		if _, exists := body[canonical]; exists {
			continue
		}
		if value, exists := body[alias]; exists {
			body[canonical] = value
		}
	}
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
	if response.Provider == "runyuan" {
		return normalizeRunyuanAssetBody(envelope)
	}
	return envelope
}

// restrictProviderResponse enforces tenant isolation even if an upstream
// revision ignores the requested groupIds filter. The filtered envelope is
// returned to the caller and used for local indexing.
func restrictProviderResponse(response *mobilecloudasset.Response, allowed map[string]struct{}, idKey string) {
	if response == nil || len(response.Body) == 0 || len(allowed) == 0 {
		return
	}
	var envelope map[string]any
	if common.Unmarshal(response.Body, &envelope) != nil {
		return
	}
	if body, ok := envelope["body"]; ok {
		filtered, keep := restrictProviderValue(body, allowed, idKey)
		if keep {
			envelope["body"] = filtered
		} else {
			envelope["body"] = map[string]any{"data": []any{}, "total": 0}
		}
	} else {
		filtered, keep := restrictProviderValue(envelope, allowed, idKey)
		if !keep {
			return
		}
		envelope, _ = filtered.(map[string]any)
	}
	if data, err := common.Marshal(envelope); err == nil {
		response.Body = data
	}
}

// restrictProviderAssetResponse applies a second ownership boundary for
// providers whose group identifier is shared across tenants (Runyuan's
// account-level AIGC group is one example). An empty allow-list intentionally
// produces an empty result instead of bypassing the filter.
func restrictProviderAssetResponse(response *mobilecloudasset.Response, allowed map[string]struct{}) {
	if response == nil || len(response.Body) == 0 {
		return
	}
	var envelope map[string]any
	if common.Unmarshal(response.Body, &envelope) != nil {
		return
	}
	if body, ok := envelope["body"]; ok {
		filtered, _ := restrictProviderValue(body, allowed, "assetId")
		envelope["body"] = filtered
	} else {
		filtered, keep := restrictProviderValue(envelope, allowed, "assetId")
		if !keep {
			envelope = map[string]any{"Result": map[string]any{"Items": []any{}, "TotalCount": 0}}
		} else if value, ok := filtered.(map[string]any); ok {
			envelope = value
		}
	}
	if data, err := common.Marshal(envelope); err == nil {
		response.Body = data
	}
}

func restrictProviderValue(value any, allowed map[string]struct{}, idKey string) (any, bool) {
	switch current := value.(type) {
	case map[string]any:
		if providerID := providerFieldValue(current, idKey); providerID != "" {
			if _, ok := allowed[providerID]; !ok {
				return nil, false
			}
		}
		out := make(map[string]any, len(current))
		filteredList := false
		filteredCount := 0
		for key, nested := range current {
			filtered, keep := restrictProviderValue(nested, allowed, idKey)
			if !keep && (key == "data" || key == "list" || key == "items" || key == "records" || key == "results" || key == "body" || key == "Result" || key == "result") {
				if key == "data" || key == "list" || key == "items" || key == "records" || key == "results" {
					out[key] = []any{}
				}
				continue
			}
			if keep {
				if originalItems, ok := nested.([]any); ok {
					if filteredItems, ok := filtered.([]any); ok && len(filteredItems) < len(originalItems) {
						filteredList = true
						filteredCount = len(filteredItems)
					}
				}
				out[key] = filtered
			}
		}
		if filteredList {
			for _, key := range []string{"total", "totalCount", "TotalCount"} {
				if _, exists := out[key]; exists {
					out[key] = filteredCount
				}
			}
		}
		return out, true
	case []any:
		out := make([]any, 0, len(current))
		for _, item := range current {
			if filtered, keep := restrictProviderValue(item, allowed, idKey); keep {
				out = append(out, filtered)
			}
		}
		return out, true
	default:
		return value, true
	}
}

func providerFieldValue(value map[string]any, field string) string {
	keys := []string{field}
	switch field {
	case "groupId":
		keys = append(keys, "groupID", "GroupId", "GroupID", "group_id")
	case "assetId":
		keys = append(keys, "assetID", "AssetId", "AssetID", "asset_id", "Id")
	}
	for _, key := range keys {
		if id := stringValue(value[key]); id != "" {
			return id
		}
	}
	return ""
}

// normalizeRunyuanAssetBody converts Runyuan's Ark Action envelope into the
// stable camelCase shape consumed by the existing local asset index and public
// asset contract. Provider-specific Action/ResponseMetadata fields are kept
// out of client responses; the raw response remains available to the
// administrator diagnostics path before this projection.
func normalizeRunyuanAssetBody(envelope map[string]any) map[string]any {
	result, ok := envelope["Result"].(map[string]any)
	if !ok || result == nil {
		return envelope
	}
	resultAction := ""
	if metadata, ok := envelope["ResponseMetadata"].(map[string]any); ok {
		resultAction = stringValue(metadata["Action"])
	}
	isAssetResult := strings.Contains(resultAction, "Asset") && !strings.Contains(resultAction, "Group")
	out := make(map[string]any, len(result)+8)
	for key, value := range result {
		out[key] = value
	}
	if id := stringValue(result["Id"]); id != "" {
		if isAssetResult || result["URL"] != nil || result["AssetType"] != nil || result["GroupId"] != nil {
			out["assetId"] = id
		} else {
			out["groupId"] = id
		}
	}
	if value, exists := result["Name"]; exists {
		if isAssetResult || result["URL"] != nil || result["AssetType"] != nil || result["GroupId"] != nil {
			out["assetName"] = value
		} else {
			out["groupName"] = value
		}
	}
	if value, exists := result["Description"]; exists {
		out["description"] = value
	}
	if value, exists := result["GroupType"]; exists {
		out["groupType"] = value
	}
	if value, exists := result["ProjectName"]; exists {
		out["projectName"] = value
	}
	if value, exists := result["URL"]; exists {
		out["assetUrl"] = value
	}
	if value, exists := result["AssetType"]; exists {
		out["assetType"] = value
	}
	if value, exists := result["GroupId"]; exists {
		out["groupId"] = value
	}
	if value, exists := result["Status"]; exists {
		out["status"] = value
	}
	if value, exists := result["ErrorMessage"]; exists {
		out["errorMessage"] = value
	}
	if items, exists := result["Items"].([]any); exists {
		normalizedItems := make([]any, 0, len(items))
		for _, item := range items {
			if itemMap, ok := item.(map[string]any); ok {
				normalizedItems = append(normalizedItems, normalizeRunyuanAssetItem(itemMap))
			} else {
				normalizedItems = append(normalizedItems, item)
			}
		}
		out["items"] = normalizedItems
	}
	if value, exists := result["TotalCount"]; exists {
		out["totalCount"] = value
		out["total"] = value
	}
	if value, exists := result["PageNumber"]; exists {
		out["pageNo"] = value
		out["page"] = value
	}
	if value, exists := result["PageSize"]; exists {
		out["pageSize"] = value
	}
	return out
}

func normalizeRunyuanAssetItem(item map[string]any) map[string]any {
	copy := make(map[string]any, len(item)+8)
	for key, value := range item {
		copy[key] = value
	}
	return normalizeRunyuanAssetBody(map[string]any{"Result": copy})
}

func persistAssetGroups(ctx context.Context, userID, channelID int, value any) error {
	envelope, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	items := providerItems(envelope)
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
		if err := model.DB.WithContext(ctx).Where("user_id = ? AND channel_id = ? AND provider_group_id = ?", userID, channelID, providerID).First(&row).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			row = model.MobileCloudAssetGroup{UserID: userID, ChannelID: channelID, ProviderGroupID: providerID}
		}
		row.ChannelID = channelID
		row.GroupType = stringValue(group["groupType"])
		row.Name = stringValue(group["groupName"])
		row.Description = stringValue(group["description"])
		row.SetRawData(group)
		model.TouchMobileCloudAssetGroup(&row)
		if err := model.DB.WithContext(ctx).Save(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func persistAssets(ctx context.Context, userID, channelID int, value any) error {
	envelope, ok := value.(map[string]any)
	if !ok {
		if providerID := stringValue(value); providerID != "" {
			return nil
		}
		return nil
	}
	items := providerItems(envelope)
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
		if err := model.DB.WithContext(ctx).Where("user_id = ? AND channel_id = ? AND provider_asset_id = ?", userID, channelID, providerID).First(&row).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
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
		if err := model.DB.WithContext(ctx).Save(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func persistCreatedAsset(ctx context.Context, userID, channelID int, request map[string]any, response *mobilecloudasset.Response) error {
	providerValue := providerBody(response)
	providerID := providerAssetID(providerValue)
	if providerID == "" {
		return nil
	}
	status := "PROCESSING"
	if providerMap, ok := providerValue.(map[string]any); ok {
		if providerStatus := stringValue(providerMap["status"]); providerStatus != "" {
			status = providerStatus
		}
	}
	value := map[string]any{
		"assetId":   providerID,
		"groupId":   request["groupId"],
		"assetName": request["assetName"],
		"assetUrl":  request["assetUrl"],
		"assetType": request["assetType"],
		"status":    status,
	}
	return persistAssets(ctx, userID, channelID, value)
}

func logAssetIndexError(c *gin.Context, resource string, channelID int, err error) {
	if err == nil {
		return
	}
	logger.LogWarn(c.Request.Context(), "asset %s index persistence failed user_id=%d channel_id=%d error=%v", resource, c.GetInt("id"), channelID, err)
}

func requireOwnedAssetGroup(c *gin.Context, channelID int, providerID string) (*model.MobileCloudAssetGroup, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return nil, &assetOwnershipError{resource: "asset group"}
	}
	group, err := model.FindMobileCloudAssetGroupByProviderID(c.Request.Context(), c.GetInt("id"), channelID, providerID)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, &assetOwnershipError{resource: "asset group", provider: providerID}
	}
	return group, nil
}

func requireOwnedAsset(c *gin.Context, channelID int, providerID string) (*model.MobileCloudAsset, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return nil, &assetOwnershipError{resource: "asset"}
	}
	asset, err := model.FindMobileCloudAssetByProviderID(c.Request.Context(), c.GetInt("id"), channelID, providerID)
	if err != nil {
		return nil, err
	}
	if asset == nil {
		return nil, &assetOwnershipError{resource: "asset", provider: providerID}
	}
	return asset, nil
}

func ensureMobileCloudDefaultGroup(ctx context.Context, userID, channelID int, client *mobilecloudasset.Client) (string, error) {
	defaultAssetGroupMu.Lock()
	defer defaultAssetGroupMu.Unlock()
	group, err := model.FindDefaultMobileCloudAssetGroup(ctx, userID, channelID)
	if err != nil {
		return "", err
	}
	if group != nil && strings.TrimSpace(group.ProviderGroupID) != "" {
		return group.ProviderGroupID, nil
	}
	if client == nil {
		return "", errors.New("asset provider client is not configured")
	}
	response, err := client.CreateAssetGroup(ctx, map[string]any{
		"groupType":   "AIGC",
		"groupName":   fmt.Sprintf("customer-%d-default", userID),
		"description": "Default asset group managed by New API",
	})
	if err != nil {
		return "", err
	}
	providerID := providerGroupID(providerBody(response))
	if providerID == "" {
		return "", errors.New("asset provider did not return a groupId")
	}
	if err := persistAssetGroups(ctx, userID, channelID, providerBody(response)); err != nil {
		return "", err
	}
	if err := model.MarkMobileCloudAssetGroupDefault(ctx, userID, channelID, providerID); err != nil {
		return "", err
	}
	return providerID, nil
}

func runyuanGroupShared(ctx context.Context, channelID int, providerID string) (bool, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return false, nil
	}
	var userIDs []int
	if err := model.DB.WithContext(ctx).
		Model(&model.MobileCloudAssetGroup{}).
		Where("channel_id = ? AND provider_group_id = ?", channelID, providerID).
		Distinct("user_id").Pluck("user_id", &userIDs).Error; err != nil {
		return false, err
	}
	return len(userIDs) > 1, nil
}

func providerGroupID(value any) string {
	return providerIDFromValue(value, "groupId", "groupID", "GroupId", "Id")
}

func providerAssetID(value any) string {
	return providerIDFromValue(value, "assetId", "assetID", "AssetId", "Id")
}

func providerIDFromValue(value any, keys ...string) string {
	switch current := value.(type) {
	case string:
		return strings.TrimSpace(current)
	case map[string]any:
		for _, key := range keys {
			if id := stringValue(current[key]); id != "" {
				return id
			}
		}
		for _, key := range []string{"body", "data", "result", "Result", "resource"} {
			if nested, ok := current[key]; ok {
				if id := providerIDFromValue(nested, keys...); id != "" {
					return id
				}
			}
		}
	case []any:
		for _, item := range current {
			if id := providerIDFromValue(item, keys...); id != "" {
				return id
			}
		}
	}
	return ""
}

func assetIDKeys(ids map[string]struct{}) []string {
	keys := make([]string, 0, len(ids))
	for id := range ids {
		keys = append(keys, id)
	}
	return keys
}

func writeEmptyAssetList(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"requestId":    "",
			"state":        "OK",
			"errorCode":    nil,
			"errorMessage": nil,
			"body":         gin.H{"data": []any{}, "total": 0},
		},
	})
}

// providerItems unwraps the pagination envelopes used by the supported asset
// providers and keeps single-resource responses intact. Providers have used
// both `list` and `data` in different API revisions, so persistence accepts
// either shape.
func providerItems(value map[string]any) []any {
	for _, key := range []string{"list", "items", "data", "results", "records"} {
		nested, exists := value[key]
		if !exists {
			continue
		}
		if items, ok := nested.([]any); ok {
			return items
		}
		if record, ok := nested.(map[string]any); ok {
			items := providerItems(record)
			if len(items) > 0 {
				return items
			}
		}
	}
	return []any{value}
}

func findPersistedAssetURL(ctx context.Context, userID int, providerID string) string {
	var row model.MobileCloudAsset
	if err := model.DB.WithContext(ctx).Where("user_id = ? AND provider_asset_id = ?", userID, providerID).First(&row).Error; err != nil {
		return ""
	}
	return row.AssetURL
}

func localAssetObjectKey(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	const prefix = "/api/mobilecloud/uploads/"
	if !strings.HasPrefix(parsed.Path, prefix) {
		return ""
	}
	key, err := url.PathUnescape(strings.TrimPrefix(parsed.Path, prefix))
	if err != nil || strings.Contains(key, "/") || strings.Contains(key, "\\") || key == "" {
		return ""
	}
	return key
}

func assetAPIResponse(c *gin.Context, response *mobilecloudasset.Response) {
	if response == nil || len(response.Body) == 0 {
		common.ApiSuccess(c, nil)
		return
	}
	var value any
	if err := common.Unmarshal(response.Body, &value); err != nil {
		common.ApiSuccess(c, gin.H{"raw": "素材服务返回了无法解析的响应"})
		return
	}
	if response.Provider == "runyuan" {
		// Present the same envelope for every configured channel. The provider's
		// Action/ResponseMetadata fields are implementation details and are not
		// part of the public asset contract.
		if envelope, ok := value.(map[string]any); ok {
			body := normalizeRunyuanAssetBody(envelope)
			if _, leaked := body["ResponseMetadata"]; leaked {
				body = map[string]any{}
			}
			value = map[string]any{
				"requestId":    providerRequestID(response),
				"state":        "OK",
				"errorCode":    nil,
				"errorMessage": nil,
				"body":         body,
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": value})
}

func assetAPIError(c *gin.Context, err error) {
	message := "asset provider request failed"
	status := http.StatusBadRequest
	code := ""
	var ownershipErr *assetOwnershipError
	if errors.As(err, &ownershipErr) {
		status = http.StatusNotFound
	} else {
		var conflictErr *assetConflictError
		if errors.As(err, &conflictErr) && conflictErr != nil {
			status = http.StatusConflict
			code = conflictErr.code
			message = conflictErr.Error()
		} else {
			var transportErr *mobilecloudasset.TransportError
			if errors.As(err, &transportErr) && transportErr != nil {
				if transportErr.IsTimeout() {
					status = http.StatusGatewayTimeout
					code = "ASSET_PROVIDER_TIMEOUT"
					message = "素材请求超时，请稍后重试。"
				} else {
					status = http.StatusBadGateway
					code = "ASSET_PROVIDER_UNREACHABLE"
					message = "素材服务暂时不可用，请稍后重试。"
				}
				logger.LogWarn(c.Request.Context(), "asset provider transport error provider=%s method=%s path=%s timeout=%t cause=%s", transportErr.Provider, transportErr.Method, transportErr.Path, transportErr.IsTimeout(), transportErr.CauseType())
			} else {
				var providerErr *mobilecloudasset.ProviderError
				if errors.As(err, &providerErr) && providerErr != nil {
					// Keep the provider response in operator logs, but return a
					// provider-neutral message to API users.  In particular, do not
					// expose moderation codes such as PrivacyInformation.
					logger.LogWarn(c.Request.Context(), "asset provider response status=%d code=%q detail=%q", providerErr.StatusCode, providerErr.Code, common.LocalLogPreview(common.MaskSensitiveInfo(providerErr.Message)))
					status, code, message = publicAssetProviderError(providerErr)
				} else if err != nil && strings.TrimSpace(err.Error()) != "" {
					message = err.Error()
					if isAssetInternalDetail(message) {
						status = http.StatusServiceUnavailable
						code = "ASSET_SERVICE_UNAVAILABLE"
						message = "素材服务暂时不可用，请稍后重试。"
					}
				}
			}
		}
	}
	response := gin.H{"success": false, "message": message}
	if code != "" {
		response["code"] = code
	}
	c.JSON(status, response)
}

// publicAssetProviderError converts an upstream asset response into the
// stable, provider-neutral contract exposed to API users.  The original
// ProviderError is logged above for administrator diagnostics only.
func publicAssetProviderError(providerErr *mobilecloudasset.ProviderError) (int, string, string) {
	if providerErr == nil {
		return http.StatusBadGateway, "ASSET_PROVIDER_ERROR", "素材服务暂时不可用，请稍后重试。"
	}
	status := providerErr.StatusCode
	if isAssetContentRejected(providerErr.Code, providerErr.Message) {
		return http.StatusUnprocessableEntity, "ASSET_CONTENT_REJECTED", "素材未通过审核，请更换素材后重试。"
	}
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return http.StatusBadGateway, "ASSET_SERVICE_CONFIG", "素材服务配置暂不可用，请联系管理员。"
	case http.StatusTooManyRequests:
		return http.StatusTooManyRequests, "ASSET_RATE_LIMITED", "素材服务当前繁忙，请稍后重试。"
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return http.StatusGatewayTimeout, "ASSET_PROVIDER_TIMEOUT", "素材请求超时，请稍后重试。"
	default:
		if status < 400 || status > 599 {
			status = http.StatusBadGateway
		}
		if status >= 500 {
			status = http.StatusBadGateway
		}
		return status, "ASSET_PROVIDER_ERROR", "素材请求未通过校验，请检查参数和公网素材地址。"
	}
}

func isAssetContentRejected(code, message string) bool {
	value := strings.ToLower(strings.TrimSpace(code + " " + message))
	for _, marker := range []string{
		"privacyinformation",
		"privacy information",
		"sensitivecontent",
		"sensitive content",
		"contentmoderation",
		"content moderation",
		"content_safety",
		"content safety",
		"内容审核",
		"隐私信息",
		"真人肖像",
	} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func isAssetInternalDetail(message string) bool {
	value := strings.ToLower(strings.TrimSpace(message))
	for _, marker := range []string{
		"mobile cloud",
		"runyuan",
		"asset provider",
		"accesskey",
		"access key",
		"secretkey",
		"secret key",
		"ecloud.",
		"asset_base_url",
		"provider base url",
	} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func providerResponseValue(response *mobilecloudasset.Response) any {
	if response == nil || len(response.Body) == 0 {
		return nil
	}
	var value any
	if err := common.Unmarshal(response.Body, &value); err != nil {
		return gin.H{"raw": "素材服务返回了无法解析的响应"}
	}
	if response.Provider == "runyuan" {
		if envelope, ok := value.(map[string]any); ok {
			return map[string]any{
				"requestId":    providerRequestID(response),
				"state":        "OK",
				"errorCode":    nil,
				"errorMessage": nil,
				"body":         normalizeRunyuanAssetBody(envelope),
			}
		}
	}
	return value
}

func assetTypeFromContentType(contentType string) string {
	contentType = assetstore.NormalizeContentType(contentType)
	if strings.HasPrefix(contentType, "image/") {
		return "Image"
	}
	if strings.HasPrefix(contentType, "video/") {
		return "Video"
	}
	return "Audio"
}

func requestAssetBaseURL(c *gin.Context) string {
	configured := strings.TrimRight(strings.TrimSpace(system_setting.ServerAddress), "/")
	if parsed, err := url.Parse(configured); err == nil && parsed.Hostname() != "" {
		host := strings.ToLower(parsed.Hostname())
		if host != "localhost" && host != "127.0.0.1" && host != "::1" {
			return configured
		}
	}
	proto := strings.TrimSpace(strings.Split(c.GetHeader("X-Forwarded-Proto"), ",")[0])
	if proto != "http" && proto != "https" {
		proto = "http"
	}
	if host := strings.TrimSpace(c.Request.Host); host != "" {
		return proto + "://" + host
	}
	return configured
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
