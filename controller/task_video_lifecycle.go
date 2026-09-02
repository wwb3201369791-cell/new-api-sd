package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	relaychannel "github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// ListTaskPluginVideos implements the list half of the OpenAI/Ark video task
// lifecycle. The database is the source of truth; upstream is queried only by
// the normal polling worker, which keeps list calls cheap and deterministic.
func ListTaskPluginVideos(c *gin.Context) {
	page := parseBoundedQueryInt(c.Query("page"), 1, 1, 10000)
	if value := c.Query("page_num"); value != "" {
		page = parseBoundedQueryInt(value, page, 1, 10000)
	}
	pageSize := parseBoundedQueryInt(c.Query("page_size"), 20, 1, 100)
	if value := c.Query("limit"); value != "" {
		pageSize = parseBoundedQueryInt(value, pageSize, 1, 100)
	}

	params := model.SyncTaskQueryParams{
		Status: c.Query("status"),
		Action: c.Query("action"),
	}
	// Ark's provider-native endpoint is bound to one plugin. Filtering by the
	// plugin platform prevents a mobile-cloud list from leaking tasks produced
	// by unrelated legacy channels. OpenAI /v1/videos accepts an optional
	// platform filter and otherwise lists all video task platforms.
	if relay.IsArkSeedanceTaskPath(c.Request.URL.Path) {
		params.Platform = constant.TaskPlatform(strings.TrimSpace(c.Query("platform")))
		if params.Platform == "" {
			params.Platform = constant.TaskPlatform("mobilecloud")
		}
	} else if value := strings.TrimSpace(c.Query("platform")); value != "" {
		params.Platform = constant.TaskPlatform(value)
	}

	start := (page - 1) * pageSize
	tasks := model.TaskGetAllUserTask(c.GetInt("id"), start, pageSize, params)
	total := model.TaskCountAllTasks(model.SyncTaskQueryParams{
		UserID:   strconv.Itoa(c.GetInt("id")),
		Status:   params.Status,
		Action:   params.Action,
		Platform: params.Platform,
	})
	items := make([]any, 0, len(tasks))
	for _, task := range tasks {
		if task == nil {
			continue
		}
		if relay.IsArkSeedanceTaskPath(c.Request.URL.Path) {
			body, taskErr := arkVideoListItem(task)
			if taskErr != nil {
				writeVideoLifecycleError(c, taskErr)
				return
			}
			var item any
			if err := common.Unmarshal(body, &item); err != nil {
				writeVideoLifecycleError(c, service.TaskErrorWrapper(err, "task_data_invalid", http.StatusInternalServerError))
				return
			}
			items = append(items, item)
			continue
		}
		items = append(items, task.ToOpenAIVideo())
	}
	hasMore := int64(start+len(tasks)) < total
	if relay.IsArkSeedanceTaskPath(c.Request.URL.Path) {
		c.JSON(http.StatusOK, gin.H{"data": items, "has_more": hasMore})
		return
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": items, "has_more": hasMore})
}

func arkVideoListItem(task *model.Task) ([]byte, *taskdto.TaskError) {
	return relay.BuildArkSeedanceTaskResponse(task)
}

// DeleteTaskPluginVideo forwards DELETE to a provider hook when available,
// then transitions the durable local task with a CAS guard. Mobile Cloud uses
// the same provider DELETE endpoint for queued cancellation and terminal
// deletion; the semantic action is passed to the plugin for other providers.
func DeleteTaskPluginVideo(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("task_id"))
	if taskID == "" {
		writeVideoLifecycleError(c, service.TaskErrorWrapperLocal(errors.New("task_id is required"), "invalid_request_error", http.StatusBadRequest))
		return
	}
	task, exists, err := model.GetByTaskId(c.GetInt("id"), taskID)
	if err != nil {
		writeVideoLifecycleError(c, service.TaskErrorWrapper(err, "get_task_failed", http.StatusInternalServerError))
		return
	}
	if !exists || task == nil {
		writeVideoLifecycleError(c, service.TaskErrorWrapperLocal(errors.New("task not found"), "not_found", http.StatusNotFound))
		return
	}

	active := task.Status != model.TaskStatusSuccess && task.Status != model.TaskStatusFailure
	action := strings.ToLower(strings.TrimSpace(c.Query("action")))
	if action == "" {
		if active {
			action = "cancel"
		} else {
			action = "delete"
		}
	}
	if action != "cancel" && action != "delete" {
		writeVideoLifecycleError(c, service.TaskErrorWrapperLocal(errors.New("action must be cancel or delete"), "invalid_request_error", http.StatusBadRequest))
		return
	}

	if err := forwardTaskControl(c, task, action); err != nil {
		writeVideoLifecycleError(c, err)
		return
	}
	if active && action == "cancel" {
		if !markTaskCancelled(task) {
			// A poller won the race. Reloading gives the caller an authoritative
			// terminal response without charging/refunding twice.
			task, _, _ = model.GetByTaskId(c.GetInt("id"), taskID)
		}
	}
	status := "deleted"
	if action == "cancel" {
		status = "cancelled"
	}
	setTaskTraceHeaders(c)
	c.JSON(http.StatusOK, gin.H{"id": taskID, "status": status})
}

func forwardTaskControl(c *gin.Context, task *model.Task, action string) *taskdto.TaskError {
	if task == nil || !taskHasPluginExecution(task) {
		return nil
	}
	adaptor, err := initTaskArtifactAdaptor(task)
	if err != nil {
		return service.TaskErrorWrapper(err, "task_plugin_unavailable", http.StatusServiceUnavailable)
	}
	provider, ok := adaptor.(relaychannel.TaskControlRequestProvider)
	if !ok {
		return nil
	}
	descriptor, err := provider.BuildControlRequest(task, action)
	if err != nil {
		return service.TaskErrorWrapper(err, "task_control_request_failed", http.StatusBadGateway)
	}
	if descriptor == nil || descriptor.URL == "" {
		return nil
	}
	var body io.Reader
	if len(descriptor.Body) > 0 {
		body = bytes.NewReader(descriptor.Body)
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), descriptor.Method, descriptor.URL, body)
	if err != nil {
		return service.TaskErrorWrapper(err, "task_control_request_failed", http.StatusBadGateway)
	}
	for name, value := range descriptor.Headers {
		req.Header.Set(name, value)
	}
	if len(descriptor.Body) > 0 && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	channelModel, err := model.CacheGetChannel(task.ChannelId)
	if err != nil || channelModel == nil {
		return service.TaskErrorWrapper(fmt.Errorf("channel %d is unavailable", task.ChannelId), "channel_unavailable", http.StatusServiceUnavailable)
	}
	baseURL := channelModel.GetBaseURL()
	if baseURL == "" {
		baseURL = constant.GetChannelBaseURL(channelModel.Type)
	}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelId: task.ChannelId, ChannelType: channelModel.Type, ChannelBaseUrl: baseURL,
		ApiKey: channelModel.Key, ChannelSetting: channelModel.GetSetting(),
	}}
	resp, err := relaychannel.DoRequest(c, req, info)
	if err != nil {
		return service.TaskErrorWrapper(err, "task_control_request_failed", http.StatusBadGateway)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	providerBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	message := strings.TrimSpace(string(providerBody))
	if message == "" {
		message = http.StatusText(resp.StatusCode)
	}
	return service.TaskErrorWrapper(fmt.Errorf("upstream task control failed: %s", message), "upstream_task_control_failed", resp.StatusCode)
}

func markTaskCancelled(task *model.Task) bool {
	if task == nil || task.Status == model.TaskStatusSuccess || task.Status == model.TaskStatusFailure {
		return false
	}
	from := task.Status
	task.Status = model.TaskStatusFailure
	task.Progress = "100%"
	task.FinishTime = time.Now().Unix()
	task.FailReason = "cancelled by user"
	data := make(map[string]any)
	if len(task.Data) > 0 {
		// Preserve provider-specific fields while ensuring an empty or malformed
		// snapshot still exposes a stable cancellation status to API clients.
		_ = common.Unmarshal(task.Data, &data)
	}
	data["status"] = "cancelled"
	if _, ok := data["error"]; !ok {
		data["error"] = map[string]any{"code": "task_cancelled", "message": task.FailReason}
	}
	task.SetData(data)
	won, err := task.UpdateWithStatus(from)
	if err != nil || !won {
		return false
	}
	if task.Quota > 0 {
		service.RefundTaskQuota(context.Background(), task, task.FailReason)
	}
	return true
}

func writeVideoLifecycleError(c *gin.Context, taskErr *taskdto.TaskError) {
	if taskErr == nil {
		return
	}
	setTaskTraceHeaders(c)
	taskErr = service.PublicTaskError(taskErr, c.GetString(common.RequestIdKey))
	status := taskErr.StatusCode
	if status < 100 || status > 599 {
		status = http.StatusBadGateway
	}
	if relay.IsArkSeedanceTaskPath(c.Request.URL.Path) {
		c.JSON(status, gin.H{"error": gin.H{"code": taskErr.Code, "message": taskErr.Message}})
		return
	}
	c.JSON(status, gin.H{"error": types.OpenAIError{Message: taskErr.Message, Type: "invalid_request_error", Code: taskErr.Code}})
}

func parseBoundedQueryInt(value string, fallback, min, max int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < min || parsed > max {
		return fallback
	}
	return parsed
}
