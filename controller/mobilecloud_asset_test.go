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
