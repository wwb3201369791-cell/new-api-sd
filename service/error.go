package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
)

var taskSensitiveCredentialPattern = regexp.MustCompile(`(?i)(authorization\s*[:=]\s*bearer\s+|(?:api[_-]?key|access[_-]?key|secret[_-]?key|token)\s*[:=]\s*)([^\s,;"']+)`)

func MidjourneyErrorWrapper(code int, desc string) *taskdto.MidjourneyResponse {
	return &taskdto.MidjourneyResponse{
		Code:        code,
		Description: desc,
	}
}

func MidjourneyErrorWithStatusCodeWrapper(code int, desc string, statusCode int) *taskdto.MidjourneyResponseWithStatusCode {
	return &taskdto.MidjourneyResponseWithStatusCode{
		StatusCode: statusCode,
		Response:   *MidjourneyErrorWrapper(code, desc),
	}
}

//// OpenAIErrorWrapper wraps an error into an OpenAIErrorWithStatusCode
//func OpenAIErrorWrapper(err error, code string, statusCode int) *dto.OpenAIErrorWithStatusCode {
//	text := err.Error()
//	lowerText := strings.ToLower(text)
//	if !strings.HasPrefix(lowerText, "get file base64 from url") && !strings.HasPrefix(lowerText, "mime type is not supported") {
//		if strings.Contains(lowerText, "post") || strings.Contains(lowerText, "dial") || strings.Contains(lowerText, "http") {
//			common.SysLog(fmt.Sprintf("error: %s", text))
//			text = "请求上游地址失败"
//		}
//	}
//	openAIError := dto.OpenAIError{
//		Message: text,
//		Type:    "new_api_error",
//		Code:    code,
//	}
//	return &dto.OpenAIErrorWithStatusCode{
//		Error:      openAIError,
//		StatusCode: statusCode,
//	}
//}
//
//func OpenAIErrorWrapperLocal(err error, code string, statusCode int) *dto.OpenAIErrorWithStatusCode {
//	openaiErr := OpenAIErrorWrapper(err, code, statusCode)
//	openaiErr.LocalError = true
//	return openaiErr
//}

func ClaudeErrorWrapper(err error, code string, statusCode int) *dto.ClaudeErrorWithStatusCode {
	text := err.Error()
	lowerText := strings.ToLower(text)
	if !strings.HasPrefix(lowerText, "get file base64 from url") {
		if strings.Contains(lowerText, "post") || strings.Contains(lowerText, "dial") || strings.Contains(lowerText, "http") {
			common.SysLog(fmt.Sprintf("error: %s", text))
			text = "请求上游地址失败"
		}
	}
	claudeError := types.ClaudeError{
		Message: text,
		Type:    "new_api_error",
	}
	return &dto.ClaudeErrorWithStatusCode{
		Error:      claudeError,
		StatusCode: statusCode,
	}
}

func ClaudeErrorWrapperLocal(err error, code string, statusCode int) *dto.ClaudeErrorWithStatusCode {
	claudeErr := ClaudeErrorWrapper(err, code, statusCode)
	claudeErr.LocalError = true
	return claudeErr
}

func RelayErrorHandler(ctx context.Context, resp *http.Response, showBodyWhenFail bool) (newApiErr *types.NewAPIError) {
	newApiErr = types.InitOpenAIError(types.ErrorCodeBadResponseStatusCode, resp.StatusCode)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	CloseResponseBodyGracefully(resp)
	var errResponse dto.GeneralErrorResponse
	responseBodyText := string(responseBody)
	responseBodyPreview := common.LocalLogPreview(responseBodyText)
	buildErrWithBody := func(message string) error {
		if message == "" {
			return fmt.Errorf("bad response status code %d, body: %s", resp.StatusCode, responseBodyText)
		}
		return fmt.Errorf("bad response status code %d, message: %s, body: %s", resp.StatusCode, message, responseBodyText)
	}

	err = common.Unmarshal(responseBody, &errResponse)
	if err != nil {
		if showBodyWhenFail {
			newApiErr.Err = buildErrWithBody("")
		} else {
			logger.LogError(ctx, fmt.Sprintf("bad response status code %d, body: %s", resp.StatusCode, responseBodyPreview))
			newApiErr.Err = fmt.Errorf("bad response status code %d", resp.StatusCode)
		}
		return
	}

	if common.GetJsonType(errResponse.Error) == "object" {
		// General format error (OpenAI, Anthropic, Gemini, etc.)
		oaiError := errResponse.TryToOpenAIError()
		if oaiError != nil {
			newApiErr = types.WithOpenAIError(*oaiError, resp.StatusCode)
			if showBodyWhenFail {
				newApiErr.Err = buildErrWithBody(newApiErr.Error())
			}
			return
		}
	}
	message := errResponse.ToMessage()
	if message == "" {
		// The body parsed as JSON but carried no usable error message; log the
		// raw body so the upstream failure remains diagnosable.
		logger.LogError(ctx, fmt.Sprintf("bad response status code %d with empty error message, body: %s", resp.StatusCode, responseBodyPreview))
	}
	newApiErr = types.NewOpenAIError(errors.New(message), types.ErrorCodeBadResponseStatusCode, resp.StatusCode)
	if showBodyWhenFail {
		newApiErr.Err = buildErrWithBody(newApiErr.Error())
	}
	return
}

func ResetStatusCode(newApiErr *types.NewAPIError, statusCodeMappingStr string) {
	if newApiErr == nil {
		return
	}
	if statusCodeMappingStr == "" || statusCodeMappingStr == "{}" {
		return
	}
	statusCodeMapping := make(map[string]any)
	err := common.Unmarshal([]byte(statusCodeMappingStr), &statusCodeMapping)
	if err != nil {
		return
	}
	if newApiErr.StatusCode == http.StatusOK {
		return
	}
	codeStr := strconv.Itoa(newApiErr.StatusCode)
	if value, ok := statusCodeMapping[codeStr]; ok {
		intCode, ok := parseStatusCodeMappingValue(value)
		if !ok {
			return
		}
		newApiErr.StatusCode = intCode
	}
}

func parseStatusCodeMappingValue(value any) (int, bool) {
	switch v := value.(type) {
	case string:
		if v == "" {
			return 0, false
		}
		statusCode, err := strconv.Atoi(v)
		if err != nil {
			return 0, false
		}
		return statusCode, true
	case float64:
		if v != math.Trunc(v) {
			return 0, false
		}
		return int(v), true
	case int:
		return v, true
	case json.Number:
		statusCode, err := strconv.Atoi(v.String())
		if err != nil {
			return 0, false
		}
		return statusCode, true
	default:
		return 0, false
	}
}

func TaskErrorWrapperLocal(err error, code string, statusCode int) *taskdto.TaskError {
	openaiErr := TaskErrorWrapper(err, code, statusCode)
	openaiErr.LocalError = true
	return openaiErr
}

func TaskErrorWrapper(err error, code string, statusCode int) *taskdto.TaskError {
	if err == nil {
		err = errors.New("task request failed")
	}
	text := err.Error()
	// Keep diagnostics useful for operators while ensuring provider URLs,
	// credentials and query strings are never copied verbatim into API errors.
	text = redactTaskErrorText(text)
	common.SysLog(fmt.Sprintf("task error code=%s status=%d detail=%s", code, statusCode, common.LocalLogPreview(text)))
	//避免暴露内部错误
	taskError := &taskdto.TaskError{
		Code:       code,
		Message:    text,
		StatusCode: statusCode,
		Error:      &taskDiagnosticError{cause: err, message: text},
	}

	return taskError
}

func redactTaskErrorText(text string) string {
	text = common.MaskSensitiveInfo(text)
	text = taskSensitiveCredentialPattern.ReplaceAllString(text, "$1***")
	return common.LocalLogPreview(text)
}

type taskDiagnosticError struct {
	cause   error
	message string
}

func (e *taskDiagnosticError) Error() string { return e.message }

func (e *taskDiagnosticError) Unwrap() error { return e.cause }

// PublicTaskError projects an internal task failure into a stable, user-facing
// error contract. Provider status codes are intentionally not leaked for
// transient gateway failures: callers get a semantic code and a useful next
// action while the original TaskError remains available to diagnostics and
// retry logic.
func PublicTaskError(taskErr *taskdto.TaskError, requestID string) *taskdto.TaskError {
	if taskErr == nil {
		return nil
	}
	publicErr := *taskErr
	publicErr.Message = redactTaskErrorText(strings.TrimSpace(publicErr.Message))
	status := publicErr.StatusCode
	code := strings.TrimSpace(publicErr.Code)
	message := strings.TrimSpace(publicErr.Message)

	if errors.Is(publicErr.Error, context.DeadlineExceeded) || code == "upstream_timeout" || status == http.StatusGatewayTimeout || status == http.StatusRequestTimeout {
		status = http.StatusConflict
		code = "upstream_timeout"
		message = "上游处理超时，请稍后查询任务状态或重试。"
	} else {
		switch status {
		case http.StatusServiceUnavailable:
			status = http.StatusConflict
			code = "upstream_busy"
			message = "视频上游当前繁忙，任务暂未提交，请稍后重试。"
		case http.StatusNotImplemented:
			status = http.StatusUnprocessableEntity
			code = "feature_unavailable"
			message = "当前渠道暂不支持此功能，请调整参数或更换渠道。"
		case http.StatusBadGateway:
			status = http.StatusConflict
			code = "upstream_unavailable"
			message = "视频上游暂时不可用，请稍后重试。"
		case http.StatusUnauthorized, http.StatusForbidden:
			code = "channel_credential_invalid"
			message = "渠道凭证无效或已过期，请联系管理员。"
		case http.StatusTooManyRequests:
			code = "rate_limited"
			message = "上游请求过于频繁，请稍后重试。"
		}
	}
	if code == "" {
		code = "request_failed"
	}
	if message == "" || status >= 500 {
		message = "任务请求失败，请稍后重试。"
	}
	publicErr.Code = code
	publicErr.Message = message
	publicErr.StatusCode = status
	if requestID != "" && !strings.Contains(publicErr.Message, requestID) {
		publicErr.Message = common.MessageWithRequestId(publicErr.Message, requestID)
	}
	return &publicErr
}

// TaskErrorFromAPIError 将 PreConsumeBilling 返回的 NewAPIError 转换为 TaskError。
func TaskErrorFromAPIError(apiErr *types.NewAPIError) *taskdto.TaskError {
	if apiErr == nil {
		return nil
	}
	return &taskdto.TaskError{
		Code:       string(apiErr.GetErrorCode()),
		Message:    apiErr.Err.Error(),
		StatusCode: apiErr.StatusCode,
		Error:      apiErr.Err,
	}
}
