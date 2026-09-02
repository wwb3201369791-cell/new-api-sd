package plugins_test

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadRunyuanPlugin(t *testing.T) *jsplugin.LoadedPlugin {
	t.Helper()
	source, err := builtinplugins.Source("runyuan")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "runyuan"})
	require.NoError(t, err)
	return plugin
}

func TestRunyuanPluginBuildsBearerVideoRequest(t *testing.T) {
	plugin := loadRunyuanPlugin(t)
	value, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", map[string]any{
		"baseUrl":       "https://runyuan.example///",
		"apiKey":        "RY_KEY",
		"upstreamModel": "doubao-seedance-2-0-260128",
		"requestBody": map[string]any{
			"model":      "doubao-seedance-2-0-260128",
			"prompt":     "a lighthouse at sunset",
			"duration":   5,
			"resolution": "720p",
		},
	})
	require.NoError(t, err)
	request := asJSONMap(t, value)
	assert.Equal(t, "https://runyuan.example/v1/video/tasks", request["url"])
	assert.Equal(t, "POST", request["method"])
	headerMap, ok := request["headers"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Bearer RY_KEY", headerMap["Authorization"])
	body, ok := request["body"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "doubao-seedance-2.0", body["model"])
	assert.Equal(t, float64(5), body["duration"])
	assert.Equal(t, "720p", body["resolution"])
	content, ok := body["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 1)
	assert.Equal(t, "text", content[0].(map[string]any)["type"])
}

func TestRunyuanPluginUsesNonBillingTaskProbe(t *testing.T) {
	plugin := loadRunyuanPlugin(t)
	value, err := plugin.Engine.Call(t.Context(), "buildChannelTestRequest", map[string]any{
		"baseUrl":       "https://runyuan.example/",
		"apiKey":        "RY_KEY",
		"model":         "doubao-seedance-2.0",
		"upstreamModel": "doubao-seedance-2.0",
	})
	require.NoError(t, err)
	request := asJSONMap(t, value)
	assert.Equal(t, "https://runyuan.example/v1/video/tasks/channel-test-nonexistent", request["url"])
	assert.Equal(t, "GET", request["method"])
	assert.Equal(t, []any{float64(404)}, request["acceptedStatusCodes"])
	assert.Equal(t, true, request["acceptErrorResponse"])
	_, hasBody := request["body"]
	assert.False(t, hasBody)
	headerMap, ok := request["headers"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Bearer RY_KEY", headerMap["Authorization"])
}

func TestRunyuanPluginParsesTaskLifecycle(t *testing.T) {
	plugin := loadRunyuanPlugin(t)
	created, err := plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{}, map[string]any{
		"body": map[string]any{
			"status":  "submitted",
			"task_id": "sd-test-1",
		},
	})
	require.NoError(t, err)
	createdMap := asJSONMap(t, created)
	assert.Equal(t, "sd-test-1", createdMap["taskId"])

	value, err := plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{}, map[string]any{
		"status": "succeeded",
		"content": map[string]any{
			"video_url": "https://cdn.example/result.mp4",
		},
		"usage": map[string]any{
			"completion_tokens": 1234,
			"total_tokens":      1300,
		},
	})
	require.NoError(t, err)
	result := asJSONMap(t, value)
	assert.Equal(t, "SUCCESS", result["status"])
	assert.Equal(t, "https://cdn.example/result.mp4", result["url"])
	assert.Equal(t, float64(1234), result["completionTokens"])
	assert.Equal(t, float64(1300), result["totalTokens"])

	failed, err := plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{}, map[string]any{
		"status": "failed",
		"error": map[string]any{
			"code":    "OutputVideoSensitiveContentDetected",
			"message": "sensitive content",
		},
	})
	require.NoError(t, err)
	failedMap := asJSONMap(t, failed)
	assert.Equal(t, "FAILURE", failedMap["status"])
	assert.Equal(t, "sensitive content", failedMap["reason"])
}

func TestRunyuanPluginAcceptsArkStyleContentPath(t *testing.T) {
	plugin := loadRunyuanPlugin(t)
	value, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"path":  "/runyuan/v1/video/tasks",
		"model": "doubao-seedance-2.0",
		"body": map[string]any{"kind": "json", "value": map[string]any{
			"model":   "doubao-seedance-2.0",
			"content": []any{map[string]any{"type": "text", "text": "hello"}},
		}},
	})
	require.NoError(t, err)
	decoded := asJSONMap(t, value)
	assert.Equal(t, "doubao-seedance-2.0", decoded["model"])
	assert.Equal(t, "text_to_video", decoded["action"])
}

func TestRunyuanPluginRejectsMissingTaskID(t *testing.T) {
	plugin := loadRunyuanPlugin(t)
	_, err := plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{}, map[string]any{
		"body": map[string]any{"status": "submitted"},
	})
	require.Error(t, err)
}
