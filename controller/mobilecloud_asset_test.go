package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
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
