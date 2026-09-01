package plugins_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadMobileCloudPlugin(t *testing.T) *jsplugin.LoadedPlugin {
	t.Helper()
	source, err := builtinplugins.Source("mobilecloud")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "mobilecloud"})
	require.NoError(t, err)
	return plugin
}

func asJSONMap(t *testing.T, value any) map[string]any {
	t.Helper()
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, common.Unmarshal(encoded, &decoded))
	return decoded
}

func TestMobileCloudPluginBuildsNativeRequest(t *testing.T) {
	plugin := loadMobileCloudPlugin(t)
	value, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
		"baseUrl":       "https://mobilecloud.example",
		"apiKey":        "MAAS_KEY",
		"upstreamModel": "doubao-seedance-2.0",
		"requestBody": map[string]any{
			"model":          "doubao-seedance-2.0",
			"prompt":         "a lighthouse at sunset",
			"images":         []any{"https://cdn.example/reference.png"},
			"duration":       5,
			"ratio":          "16:9",
			"generate_audio": true,
		},
	})
	require.NoError(t, err)
	request := asJSONMap(t, value)
	assert.Equal(t, "https://mobilecloud.example/api/v3/contents/generations/tasks", request["url"])
	assert.Equal(t, "POST", request["method"])
	headerMap, ok := request["headers"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Bearer MAAS_KEY", headerMap["Authorization"])
	body, ok := request["body"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "doubao-seedance-2.0", body["model"])
	assert.Equal(t, float64(5), body["duration"])
	assert.Equal(t, "16:9", body["ratio"])
	assert.Equal(t, true, body["generate_audio"])
	content, ok := body["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 2)
	assert.Equal(t, "image_url", content[0].(map[string]any)["type"])
	assert.Equal(t, "text", content[1].(map[string]any)["type"])
}

func TestMobileCloudPluginForwardsSeedanceProviderFields(t *testing.T) {
	plugin := loadMobileCloudPlugin(t)
	value, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
		"baseUrl":       "https://mobilecloud.example",
		"apiKey":        "MAAS_KEY",
		"upstreamModel": "doubao-seedance-2.0",
		"requestBody": map[string]any{
			"model":      "doubao-seedance-2.0",
			"content":    []any{map[string]any{"type": "text", "text": "a city at dawn"}},
			"duration":   -1,
			"resolution": "1080p",
			"seed":       42,
			"draft":      true,
		},
	})
	require.NoError(t, err)
	request := asJSONMap(t, value)
	body, ok := request["body"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(-1), body["duration"])
	assert.Equal(t, float64(42), body["seed"])
	assert.Equal(t, true, body["draft"])
	_, hasSeconds := body["seconds"]
	assert.False(t, hasSeconds)
}

func TestMobileCloudPluginRejectsMalformedSeedanceContent(t *testing.T) {
	plugin := loadMobileCloudPlugin(t)
	_, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"path":  "/api/v3/contents/generations/tasks",
		"model": "doubao-seedance-2.0",
		"body": map[string]any{"kind": "json", "value": map[string]any{
			"model":   "doubao-seedance-2.0",
			"content": []any{map[string]any{"type": "image_url", "image_url": map[string]any{}}},
		}},
	})
	require.Error(t, err)
}

func TestMobileCloudPluginRejectsOutOfRangeDuration(t *testing.T) {
	plugin := loadMobileCloudPlugin(t)
	_, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"path":  "/api/v3/contents/generations/tasks",
		"model": "doubao-seedance-2.0",
		"body": map[string]any{"kind": "json", "value": map[string]any{
			"model": "doubao-seedance-2.0", "duration": 3601,
			"content": []any{map[string]any{"type": "text", "text": "hello"}},
		}},
	})
	require.Error(t, err)
}

func TestMobileCloudPluginPreservesArkContentRequest(t *testing.T) {
	plugin := loadMobileCloudPlugin(t)
	value, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
		"baseUrl":       "https://mobilecloud.example",
		"apiKey":        "MAAS_KEY",
		"upstreamModel": "doubao-seedance-2.0",
		"requestBody": map[string]any{
			"model": "doubao-seedance-2.0",
			"content": []any{
				map[string]any{"type": "text", "text": "a red kite over the sea"},
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://cdn.example/reference.png"}},
			},
			"duration":   5,
			"resolution": "720p",
		},
	})
	require.NoError(t, err)
	request := asJSONMap(t, value)
	body, ok := request["body"].(map[string]any)
	require.True(t, ok)
	content, ok := body["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 2)
	assert.Equal(t, "text", content[0].(map[string]any)["type"])
	assert.Equal(t, "a red kite over the sea", content[0].(map[string]any)["text"])
	assert.Equal(t, "image_url", content[1].(map[string]any)["type"])
	assert.Equal(t, "720p", body["resolution"])
}

func TestMobileCloudPluginNormalizesVolcanoConcreteModelName(t *testing.T) {
	plugin := loadMobileCloudPlugin(t)
	value, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
		"baseUrl":       "https://mobilecloud.example",
		"apiKey":        "MAAS_KEY",
		"upstreamModel": "doubao-seedance-2-0-260128",
		"requestBody": map[string]any{
			"model":   "doubao-seedance-2-0-260128",
			"content": []any{map[string]any{"type": "text", "text": "a mountain"}},
		},
	})
	require.NoError(t, err)
	request := asJSONMap(t, value)
	body, ok := request["body"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "doubao-seedance-2.0", body["model"])
}

func TestMobileCloudPluginValidatesArkProtocolContent(t *testing.T) {
	plugin := loadMobileCloudPlugin(t)
	_, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"path":  "/api/v3/contents/generations/tasks",
		"model": "doubao-seedance-2.0",
		"body":  map[string]any{"kind": "json", "value": map[string]any{"model": "doubao-seedance-2.0"}},
	})
	require.Error(t, err)

	value, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"path":  "/api/v3/contents/generations/tasks",
		"model": "doubao-seedance-2.0",
		"body": map[string]any{"kind": "json", "value": map[string]any{
			"model":   "doubao-seedance-2.0",
			"content": []any{map[string]any{"type": "text", "text": "a red kite over the sea"}},
		}},
	})
	require.NoError(t, err)
	decoded := asJSONMap(t, value)
	assert.Equal(t, "doubao-seedance-2.0", decoded["model"])
	assert.Equal(t, "text_to_video", decoded["action"])
}

func TestMobileCloudPluginBuildsNonBillingChannelTestRequest(t *testing.T) {
	plugin := loadMobileCloudPlugin(t)
	value, err := plugin.Engine.Call(t.Context(), "buildChannelTestRequest", map[string]any{
		"baseUrl":       "https://mobilecloud.example/",
		"apiKey":        "MAAS_KEY",
		"model":         "doubao-seedance-2.0",
		"upstreamModel": "doubao-seedance-2.0",
	})
	require.NoError(t, err)
	request := asJSONMap(t, value)
	assert.Equal(t, "https://mobilecloud.example/api/v3/mapping/query", request["url"])
	assert.Equal(t, "POST", request["method"])
	headerMap, ok := request["headers"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Bearer MAAS_KEY", headerMap["Authorization"])
	body, ok := request["body"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "doubao-seedance-2.0", body["model"])
}

func TestMobileCloudPluginNormalizesBaseURLAndEscapesTaskID(t *testing.T) {
	plugin := loadMobileCloudPlugin(t)
	value, err := plugin.Engine.Call(t.Context(), "buildQueryRequest", map[string]any{
		"baseUrl": "https://mobilecloud.example///",
		"apiKey":  "MAAS_KEY",
		"taskId":  "task/with-space",
	})
	require.NoError(t, err)
	request := asJSONMap(t, value)
	assert.Equal(t, "https://mobilecloud.example/api/v3/contents/generations/tasks/task%2Fwith-space", request["url"])
}

func TestMobileCloudPluginParsesTaskLifecycle(t *testing.T) {
	plugin := loadMobileCloudPlugin(t)
	created, err := plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{}, map[string]any{
		"body": map[string]any{
			"id":    "cgt-test-1",
			"model": "doubao-seedance-2-0-260128",
		},
	})
	require.NoError(t, err)
	createdMap := asJSONMap(t, created)
	assert.Equal(t, "cgt-test-1", createdMap["taskId"])

	for _, tc := range []struct {
		status   string
		expected string
	}{
		{"queued", "QUEUED"},
		{"running", "IN_PROGRESS"},
		{"succeeded", "SUCCESS"},
		{"failed", "FAILURE"},
		{"cancelled", "FAILURE"},
		{"expired", "FAILURE"},
	} {
		t.Run(tc.status, func(t *testing.T) {
			body := map[string]any{"status": tc.status}
			if tc.status == "succeeded" {
				body["content"] = map[string]any{"video_url": "https://cdn.example/result.mp4"}
				body["usage"] = map[string]any{"completion_tokens": 1234, "total_tokens": 1300}
			}
			value, callErr := plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{}, body)
			require.NoError(t, callErr)
			result := asJSONMap(t, value)
			assert.Equal(t, tc.expected, result["status"])
			if tc.status == "succeeded" {
				assert.Equal(t, "https://cdn.example/result.mp4", result["url"])
				assert.Equal(t, float64(1234), result["completionTokens"])
				assert.Equal(t, float64(1300), result["totalTokens"])
			}
		})
	}
}

func TestMobileCloudPluginUsesCompletionUsageOnSuccess(t *testing.T) {
	plugin := loadMobileCloudPlugin(t)
	value, err := plugin.Engine.Call(t.Context(), "extractUsageOnComplete", nil, nil, map[string]any{
		"status":     "succeeded",
		"usage":      map[string]any{"completion_tokens": 9876, "total_tokens": 9999},
		"content":    map[string]any{"video_url": "https://cdn.example/result.mp4"},
		"resolution": "720p",
	})
	require.NoError(t, err)
	facts := asJSONMap(t, value)
	assert.Equal(t, float64(9876), facts["tokens"])
	assert.Equal(t, "720p", facts["resolution"])
}

func TestMobileCloudPluginFetchesArtifactsWithGet(t *testing.T) {
	plugin := loadMobileCloudPlugin(t)
	value, err := plugin.Engine.Call(t.Context(), "buildContentRequest", map[string]any{
		"artifactKey":   "video",
		"clientRequest": map[string]any{"method": "HEAD"},
		"data": map[string]any{
			"content": map[string]any{"video_url": "https://cdn.example/result.mp4"},
		},
	})
	require.NoError(t, err)
	request := asJSONMap(t, value)
	assert.Equal(t, "https://cdn.example/result.mp4", request["url"])
	assert.Equal(t, "GET", request["method"])
	assert.Equal(t, true, request["credentialless"])
}
