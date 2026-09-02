package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupMobileCloudAssetConnectionTestDB(t *testing.T) {
	t.Helper()
	originalDB := model.DB
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.Channel{}))
	model.DB = database
	t.Cleanup(func() { model.DB = originalDB })
}

func TestTestMobileCloudAssetConnectionUsesSelectedChannelAndHidesCredentials(t *testing.T) {
	setupMobileCloudAssetConnectionTestDB(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/openapi-maas/exp/aicc/v2/asset-group/query", r.URL.Path)
		assert.NotEmpty(t, r.URL.Query().Get("AccessKey"))
		assert.NotEmpty(t, r.URL.Query().Get("Signature"))
		body, readErr := io.ReadAll(r.Body)
		assert.NoError(t, readErr)
		assert.Contains(t, string(body), `"pageNo":1`)
		assert.Contains(t, string(body), `"pageSize":1`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"requestId":"upstream-asset-test-1","state":"OK","body":{"data":[],"total":0}}`))
	}))
	defer upstream.Close()

	enabled := true
	channel := &model.Channel{
		Type:    constant.ChannelTypeTaskPlugin,
		Name:    "mobilecloud-asset-test",
		Key:     "VIDEO_BEARER_KEY",
		BaseURL: &upstream.URL,
	}
	channel.SetSetting(dto.ChannelSettings{
		TaskPluginKey:     "mobilecloud",
		AssetEnabled:      &enabled,
		AssetBaseURL:      upstream.URL,
		AssetAccessKey:    "ASSET_AK",
		AssetSecretKey:    "ASSET_SK",
		AssetResourcePool: "CIDC-CORE-00",
	})
	require.NoError(t, model.DB.Create(channel).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "1"}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/asset-test/1", nil)
	TestMobileCloudAssetConnection(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	assert.Contains(t, body, `"success":true`)
	assert.Contains(t, body, `"upstream_request_id":"upstream-asset-test-1"`)
	assert.Contains(t, body, `"provider":"mobilecloud"`)
	assert.NotContains(t, body, "ASSET_AK")
	assert.NotContains(t, body, "ASSET_SK")
}

func TestTestMobileCloudAssetConnectionRejectsDisabledLibrary(t *testing.T) {
	setupMobileCloudAssetConnectionTestDB(t)
	enabled := false
	channel := &model.Channel{
		Type: constant.ChannelTypeTaskPlugin,
		Name: "mobilecloud-disabled",
		Key:  "VIDEO_BEARER_KEY",
	}
	channel.SetSetting(dto.ChannelSettings{
		TaskPluginKey:  "mobilecloud",
		AssetEnabled:   &enabled,
		AssetAccessKey: "ASSET_AK",
		AssetSecretKey: "ASSET_SK",
	})
	require.NoError(t, model.DB.Create(channel).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "1"}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/asset-test/1", nil)
	TestMobileCloudAssetConnection(ctx)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "asset library is disabled")
}
