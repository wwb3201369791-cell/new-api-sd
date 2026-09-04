package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	mobilecloudasset "github.com/QuantumNous/new-api/service/mobilecloudasset"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestProviderItemsUnwrapsMobileCloudPagination(t *testing.T) {
	items := providerItems(map[string]any{
		"list": []any{map[string]any{"assetId": "asset-1"}},
	})
	require.Len(t, items, 1)
	require.Equal(t, "asset-1", items[0].(map[string]any)["assetId"])
}

func TestProviderItemsKeepsSingleResource(t *testing.T) {
	resource := map[string]any{"assetId": "asset-1", "assetName": "demo"}
	items := providerItems(resource)
	require.Equal(t, []any{resource}, items)
}

func TestLocalAssetObjectKeyRejectsForeignURLs(t *testing.T) {
	require.Equal(t, "abc.mp4", localAssetObjectKey("https://gateway.example/api/mobilecloud/uploads/abc.mp4"))
	require.Empty(t, localAssetObjectKey("https://gateway.example/api/mobilecloud/uploads/../secret"))
	require.Empty(t, localAssetObjectKey("https://gateway.example/files/abc.mp4"))
}

func TestNormalizeRunyuanAssetBodyForAssetAndGroupResults(t *testing.T) {
	asset := normalizeRunyuanAssetBody(map[string]any{
		"ResponseMetadata": map[string]any{"Action": "CreateAsset"},
		"Result":           map[string]any{"Id": "asset-1"},
	})
	require.Equal(t, "asset-1", asset["assetId"])
	require.Empty(t, asset["groupId"])

	group := normalizeRunyuanAssetBody(map[string]any{
		"ResponseMetadata": map[string]any{"Action": "CreateAssetGroup"},
		"Result":           map[string]any{"Id": "group-1", "Name": "AIGC"},
	})
	require.Equal(t, "group-1", group["groupId"])
	require.Equal(t, "AIGC", group["groupName"])
}

func TestNormalizeRunyuanAssetListPreservesPagination(t *testing.T) {
	value := normalizeRunyuanAssetBody(map[string]any{
		"ResponseMetadata": map[string]any{"Action": "ListAssets"},
		"Result": map[string]any{
			"TotalCount": 1,
			"PageNumber": 1,
			"PageSize":   20,
			"Items": []any{map[string]any{
				"Id": "asset-1", "Name": "hero", "URL": "https://example.com/hero.png", "AssetType": "Image",
			}},
		},
	})
	require.Equal(t, 1, value["totalCount"])
	items, ok := value["items"].([]any)
	require.True(t, ok)
	require.Len(t, items, 1)
	require.Equal(t, "asset-1", items[0].(map[string]any)["assetId"])
	require.Equal(t, "https://example.com/hero.png", items[0].(map[string]any)["assetUrl"])
}

func TestRestrictProviderResponseFiltersForeignGroups(t *testing.T) {
	response := &mobilecloudasset.Response{Body: []byte(`{"requestId":"req-1","state":"OK","body":{"data":[{"groupId":"owned","assetId":"asset-1"},{"groupId":"foreign","assetId":"asset-2"}],"total":2}}`)}
	restrictProviderResponse(response, map[string]struct{}{"owned": {}}, "groupId")
	require.Contains(t, string(response.Body), `"asset-1"`)
	require.NotContains(t, string(response.Body), `"asset-2"`)
	require.NotContains(t, string(response.Body), `"foreign"`)
}

func TestRestrictProviderResponseFiltersRunyuanPascalCaseGroups(t *testing.T) {
	response := &mobilecloudasset.Response{Provider: "runyuan", Body: []byte(`{"ResponseMetadata":{"Action":"ListAssets"},"Result":{"Items":[{"Id":"asset-1","GroupId":"owned"},{"Id":"asset-2","GroupId":"foreign"}],"TotalCount":2}}`)}
	restrictProviderResponse(response, map[string]struct{}{"owned": {}}, "groupId")
	require.Contains(t, string(response.Body), `"asset-1"`)
	require.NotContains(t, string(response.Body), `"asset-2"`)
	require.NotContains(t, string(response.Body), `"foreign"`)
	require.Contains(t, string(response.Body), `"TotalCount":1`)
}

func TestRestrictRunyuanAssetsToLocallyOwnedIDs(t *testing.T) {
	response := &mobilecloudasset.Response{Provider: "runyuan", Body: []byte(`{"ResponseMetadata":{"Action":"ListAssets"},"Result":{"Items":[{"Id":"asset-owned","GroupId":"shared"},{"Id":"asset-foreign","GroupId":"shared"}],"TotalCount":2}}`)}
	restrictProviderAssetResponse(response, map[string]struct{}{"asset-owned": {}})
	require.Contains(t, string(response.Body), `"asset-owned"`)
	require.NotContains(t, string(response.Body), `"asset-foreign"`)
	require.Contains(t, string(response.Body), `"TotalCount":1`)
}

func TestRestrictRunyuanAssetsWithEmptyIndexReturnsEmptyList(t *testing.T) {
	response := &mobilecloudasset.Response{Provider: "runyuan", Body: []byte(`{"ResponseMetadata":{"Action":"ListAssets"},"Result":{"Items":[{"Id":"asset-foreign","GroupId":"shared"}],"TotalCount":1}}`)}
	restrictProviderAssetResponse(response, map[string]struct{}{})
	require.NotContains(t, string(response.Body), `"asset-foreign"`)
	require.Contains(t, string(response.Body), `"Items":[]`)
	require.Contains(t, string(response.Body), `"TotalCount":0`)
}

func TestEnsureMobileCloudDefaultGroupIsCreatedOnce(t *testing.T) {
	originalDB := model.DB
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.MobileCloudAssetGroup{}, &model.MobileCloudAsset{}))
	model.DB = database
	t.Cleanup(func() { model.DB = originalDB })

	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		require.Equal(t, "/api/openapi-maas/exp/aicc/v2/asset-group", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"requestId":"req-1","state":"OK","body":{"groupId":"group-default","groupType":"AIGC","groupName":"customer-7-default"}}`))
	}))
	defer upstream.Close()
	client, err := mobilecloudasset.NewClient(mobilecloudasset.Config{BaseURL: upstream.URL, AccessKey: "ak", SecretKey: "sk"})
	require.NoError(t, err)

	first, err := ensureMobileCloudDefaultGroup(context.Background(), 7, 11, client)
	require.NoError(t, err)
	second, err := ensureMobileCloudDefaultGroup(context.Background(), 7, 11, client)
	require.NoError(t, err)
	require.Equal(t, "group-default", first)
	require.Equal(t, first, second)
	require.Equal(t, 1, requests)
	var groups []model.MobileCloudAssetGroup
	require.NoError(t, model.DB.Find(&groups).Error)
	require.Len(t, groups, 1)
	require.True(t, groups[0].IsDefault)
}

func TestFindMobileCloudAssetGroupByNameIsScopedAndNormalized(t *testing.T) {
	originalDB := model.DB
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.MobileCloudAssetGroup{}))
	model.DB = database
	t.Cleanup(func() { model.DB = originalDB })

	require.NoError(t, model.DB.Create(&model.MobileCloudAssetGroup{
		UserID: 7, ChannelID: 11, ProviderGroupID: "group-owned", Name: "Customer Assets",
	}).Error)
	require.NoError(t, model.DB.Create(&model.MobileCloudAssetGroup{
		UserID: 8, ChannelID: 11, ProviderGroupID: "group-other-user", Name: "Customer Assets",
	}).Error)
	require.NoError(t, model.DB.Create(&model.MobileCloudAssetGroup{
		UserID: 7, ChannelID: 12, ProviderGroupID: "group-other-channel", Name: "Customer Assets",
	}).Error)

	owned, err := model.FindMobileCloudAssetGroupByName(context.Background(), 7, 11, "  customer assets ")
	require.NoError(t, err)
	require.NotNil(t, owned)
	require.Equal(t, "group-owned", owned.ProviderGroupID)

	notOwned, err := model.FindMobileCloudAssetGroupByName(context.Background(), 7, 11, "other name")
	require.NoError(t, err)
	require.Nil(t, notOwned)
}

func TestRunyuanGroupSharedDetectsCrossCustomerProviderIdentity(t *testing.T) {
	originalDB := model.DB
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.MobileCloudAssetGroup{}))
	model.DB = database
	t.Cleanup(func() { model.DB = originalDB })

	require.NoError(t, model.DB.Create(&model.MobileCloudAssetGroup{UserID: 7, ChannelID: 11, ProviderGroupID: "shared"}).Error)
	shared, err := runyuanGroupShared(context.Background(), 11, "shared")
	require.NoError(t, err)
	require.False(t, shared)
	require.NoError(t, model.DB.Create(&model.MobileCloudAssetGroup{UserID: 8, ChannelID: 11, ProviderGroupID: "shared"}).Error)
	shared, err = runyuanGroupShared(context.Background(), 11, "shared")
	require.NoError(t, err)
	require.True(t, shared)
}

func TestAssetAPIErrorMapsDuplicateGroupNameToConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/asset-groups", nil)

	assetAPIError(context, &assetConflictError{
		code:    "ASSET_GROUP_NAME_EXISTS",
		message: "asset group name already exists for this customer and channel",
	})

	require.Equal(t, http.StatusConflict, response.Code)
	require.Contains(t, response.Body.String(), `"code":"ASSET_GROUP_NAME_EXISTS"`)
	require.Contains(t, response.Body.String(), "already exists")
}

func TestAssetAPIErrorHidesProviderModerationDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/assets", nil)

	assetAPIError(context, &mobilecloudasset.ProviderError{
		StatusCode: http.StatusBadRequest,
		Code:       "InputImageSensitiveContentDetected.PrivacyInformation",
		Message:    "private provider detail",
	})

	require.Equal(t, http.StatusUnprocessableEntity, response.Code)
	require.Contains(t, response.Body.String(), `"code":"ASSET_CONTENT_REJECTED"`)
	require.Contains(t, response.Body.String(), "素材未通过审核")
	require.NotContains(t, response.Body.String(), "PrivacyInformation")
	require.NotContains(t, response.Body.String(), "private provider detail")
}

func TestAssetAPIErrorHidesProviderConfigurationDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodGet, "/v1/asset-groups", nil)

	assetAPIError(context, errors.New("no Mobile Cloud or Runyuan channel with asset AccessKey/SecretKey is configured"))

	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.Contains(t, response.Body.String(), `"code":"ASSET_SERVICE_UNAVAILABLE"`)
	require.Contains(t, response.Body.String(), "素材服务暂时不可用")
	require.NotContains(t, response.Body.String(), "Mobile Cloud")
	require.NotContains(t, response.Body.String(), "Runyuan")
	require.NotContains(t, response.Body.String(), "AccessKey")
}

func TestAssetAPIResponseNormalizesRunyuanEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodGet, "/v1/asset-groups", nil)

	assetAPIResponse(context, &mobilecloudasset.Response{
		Provider:   "runyuan",
		StatusCode: http.StatusOK,
		Body:       []byte(`{"ResponseMetadata":{"RequestId":"runyuan-private-id","Action":"ListAssetGroups"},"Result":{"Id":"group-1","Name":"demo","GroupType":"AIGC"}}`),
	})

	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"requestId":"runyuan-private-id"`)
	require.Contains(t, response.Body.String(), `"groupId":"group-1"`)
	require.NotContains(t, response.Body.String(), "ResponseMetadata")
	require.NotContains(t, response.Body.String(), "ListAssetGroups")
}

func TestProviderResponseValueNormalizesRunyuanCreateEnvelope(t *testing.T) {
	value := providerResponseValue(&mobilecloudasset.Response{
		Provider: "runyuan",
		Body:     []byte(`{"ResponseMetadata":{"RequestId":"runyuan-private-id","Action":"CreateAsset"},"Result":{"Id":"asset-1","Name":"demo","URL":"https://upstream.example/demo.png","AssetType":"Image"}}`),
	})
	envelope, ok := value.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "runyuan-private-id", envelope["requestId"])
	require.NotContains(t, envelope, "ResponseMetadata")
	body, ok := envelope["body"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "asset-1", body["assetId"])
	require.Equal(t, "demo", body["assetName"])
	data, err := common.Marshal(value)
	require.NoError(t, err)
	require.NotContains(t, string(data), "CreateAsset")
}

func TestPersistCreatedAssetKeepsTerminalProviderStatus(t *testing.T) {
	originalDB := model.DB
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.MobileCloudAsset{}))
	model.DB = database
	t.Cleanup(func() { model.DB = originalDB })

	response := &mobilecloudasset.Response{Body: []byte(`{"body":{"assetId":"asset-terminal","groupId":"group-1","status":"ACTIVE"}}`)}
	require.NoError(t, persistCreatedAsset(context.Background(), 7, 11, map[string]any{
		"assetName": "demo",
		"assetUrl":  "https://example.com/demo.png",
		"assetType": "Image",
		"groupId":   "group-1",
	}, response))

	var asset model.MobileCloudAsset
	require.NoError(t, model.DB.Where("provider_asset_id = ?", "asset-terminal").First(&asset).Error)
	require.Equal(t, "ACTIVE", asset.Status)
}
