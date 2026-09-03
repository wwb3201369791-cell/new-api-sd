package mobilecloudasset

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	DefaultBaseURL        = "https://ecloud.10086.cn"
	DefaultRunyuanBaseURL = "https://runy.yitd.cn"
	DefaultResourcePool   = "CIDC-CORE-00"
	DefaultRunyuanVersion = "2024-01-01"
)

var (
	ErrMissingCredentials         = errors.New("mobile cloud asset credentials are missing")
	ErrUnsupportedSignatureMethod = errors.New("unsupported mobile cloud signature method")
)

type Config struct {
	// Provider selects the upstream asset protocol. Empty and "mobilecloud"
	// use the Mobile Cloud query-signature API; "runyuan" uses the Volcano
	// Ark HMAC-SHA256 header signature documented by Runyuan.
	Provider        string
	BaseURL         string
	AccessKey       string
	SecretKey       string
	ResourcePool    string
	SignatureMethod string
	HTTPClient      *http.Client
}

type Client struct {
	config Config
	client *http.Client
}

type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	Provider   string
}

type ProviderError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *ProviderError) Error() string {
	if e == nil {
		return ""
	}
	if e.Code == "" {
		return fmt.Sprintf("asset provider request failed (%d): %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("asset provider request failed (%d, %s): %s", e.StatusCode, e.Code, e.Message)
}

func NewClient(config Config) (*Client, error) {
	config.Provider = strings.ToLower(strings.TrimSpace(config.Provider))
	if config.Provider == "" {
		config.Provider = "mobilecloud"
	}
	if config.Provider != "mobilecloud" && config.Provider != "runyuan" {
		return nil, fmt.Errorf("unsupported asset provider %q", config.Provider)
	}
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if config.BaseURL == "" {
		config.BaseURL = DefaultBaseURL
		if config.Provider == "runyuan" {
			config.BaseURL = DefaultRunyuanBaseURL
		}
	}
	parsed, err := url.Parse(config.BaseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return nil, errors.New("asset provider base URL must be an absolute HTTP(S) URL")
	}
	if strings.TrimSpace(config.AccessKey) == "" || strings.TrimSpace(config.SecretKey) == "" {
		return nil, ErrMissingCredentials
	}
	if strings.TrimSpace(config.ResourcePool) == "" {
		config.ResourcePool = DefaultResourcePool
	}
	if strings.TrimSpace(config.SignatureMethod) == "" && config.Provider == "mobilecloud" {
		config.SignatureMethod = "HmacSHA1"
	}
	client := config.HTTPClient
	if client == nil {
		// The Mobile Cloud asset gateway currently closes HTTP/2 requests before
		// returning a response. Keep the default client on HTTP/1.1 while still
		// preserving the standard transport proxy/TLS settings.
		transport := http.DefaultTransport
		if base, ok := http.DefaultTransport.(*http.Transport); ok {
			clone := base.Clone()
			clone.ForceAttemptHTTP2 = false
			clone.TLSNextProto = nil
			transport = clone
		}
		client = &http.Client{Transport: transport, Timeout: 60 * time.Second}
	}
	return &Client{config: config, client: client}, nil
}

func (c *Client) Do(ctx context.Context, method, path string, payload any, params map[string]string) (*Response, error) {
	if c == nil {
		return nil, errors.New("mobile cloud asset client is nil")
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodGet
	}
	if path == "" || !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "?#") {
		return nil, errors.New("mobile cloud asset path must be an absolute path without query")
	}
	body := []byte(nil)
	if payload != nil {
		var err error
		body, err = common.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal mobile cloud asset payload: %w", err)
		}
	}
	signedQuery, err := Sign(method, path, c.config.AccessKey, c.config.SecretKey, params, time.Now().UTC(), "", c.config.SignatureMethod)
	if err != nil {
		return nil, err
	}
	endpoint := c.config.BaseURL + path + "?" + signedQuery
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	result := &Response{StatusCode: response.StatusCode, Header: response.Header.Clone(), Body: data, Provider: c.config.Provider}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return result, decodeProviderError(result)
	}
	if err := decodeProviderError(result); err != nil {
		return result, err
	}
	return result, nil
}

// doRunyuan sends one of Runyuan's /v1/video Action requests. Unlike the
// Mobile Cloud API, all asset operations are POSTs signed with the Volcano
// Ark HMAC-SHA256 header scheme.
func (c *Client) doRunyuan(ctx context.Context, action string, payload any) (*Response, error) {
	body := []byte(nil)
	if payload != nil {
		var err error
		body, err = common.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal runyuan asset payload: %w", err)
		}
	}
	now := time.Now().UTC()
	xDate := now.Format("20060102T150405Z")
	date := now.Format("20060102")
	bodyHashBytes := sha256.Sum256(body)
	bodyHash := hex.EncodeToString(bodyHashBytes[:])
	parsed, err := url.Parse(c.config.BaseURL)
	if err != nil || parsed.Host == "" {
		return nil, errors.New("runyuan asset base URL is invalid")
	}
	host := parsed.Host
	query := "Action=" + url.QueryEscape(action) + "&Version=" + url.QueryEscape(DefaultRunyuanVersion)
	canonicalRequest := "POST\n/v1/video\n" + query + "\ncontent-type:application/json\nhost:" + host + "\nx-content-sha256:" + bodyHash + "\nx-date:" + xDate + "\n\ncontent-type;host;x-content-sha256;x-date\n" + bodyHash
	canonicalHashBytes := sha256.Sum256([]byte(canonicalRequest))
	canonicalHash := hex.EncodeToString(canonicalHashBytes[:])
	credentialScope := date + "/cn-beijing/ark/request"
	stringToSign := "HMAC-SHA256\n" + xDate + "\n" + credentialScope + "\n" + canonicalHash
	macSum := func(key, value []byte) []byte {
		mac := hmac.New(sha256.New, key)
		_, _ = mac.Write(value)
		return mac.Sum(nil)
	}
	kDate := macSum([]byte(c.config.SecretKey), []byte(date))
	kRegion := macSum(kDate, []byte("cn-beijing"))
	kService := macSum(kRegion, []byte("ark"))
	signingKey := macSum(kService, []byte("request"))
	signature := hex.EncodeToString(macSum(signingKey, []byte(stringToSign)))
	endpoint := c.config.BaseURL + "/v1/video?" + query
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Host", host)
	request.Host = host
	request.Header.Set("X-Date", xDate)
	request.Header.Set("X-Content-Sha256", bodyHash)
	request.Header.Set("Authorization", "HMAC-SHA256 Credential="+c.config.AccessKey+"/"+credentialScope+", SignedHeaders=content-type;host;x-content-sha256;x-date, Signature="+signature)
	response, err := c.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	result := &Response{StatusCode: response.StatusCode, Header: response.Header.Clone(), Body: data, Provider: c.config.Provider}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return result, decodeProviderError(result)
	}
	if err := decodeProviderError(result); err != nil {
		return result, err
	}
	return result, nil
}

func (c *Client) action(ctx context.Context, action string, body map[string]any) (*Response, error) {
	if c.config.Provider == "runyuan" {
		return c.doRunyuan(ctx, action, runyuanPayload(action, body))
	}
	return nil, errors.New("asset action is not available")
}

func decodeProviderError(response *Response) error {
	if response == nil || len(response.Body) == 0 {
		if response != nil && response.StatusCode >= 200 && response.StatusCode < 300 {
			return nil
		}
		statusCode := 0
		if response != nil {
			statusCode = response.StatusCode
		}
		message := http.StatusText(statusCode)
		if message == "" {
			message = "empty provider response"
		}
		return &ProviderError{StatusCode: statusCode, Message: message}
	}
	var envelope struct {
		State        string `json:"state"`
		ErrorCode    any    `json:"errorCode"`
		ErrorMessage string `json:"errorMessage"`
		Code         any    `json:"code"`
		Message      string `json:"message"`
		Error        *struct {
			Code    any    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		ResponseMetadata *struct {
			Error *struct {
				Code    any    `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error"`
		} `json:"ResponseMetadata"`
	}
	if err := common.Unmarshal(response.Body, &envelope); err != nil {
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			return nil
		}
		return &ProviderError{StatusCode: response.StatusCode, Message: strings.TrimSpace(string(response.Body))}
	}
	code := fmt.Sprint(envelope.ErrorCode)
	if code == "<nil>" || code == "" {
		code = fmt.Sprint(envelope.Code)
	}
	message := strings.TrimSpace(envelope.ErrorMessage)
	if message == "" {
		message = strings.TrimSpace(envelope.Message)
	}
	if envelope.Error != nil {
		if code == "" || code == "<nil>" {
			code = fmt.Sprint(envelope.Error.Code)
		}
		if message == "" {
			message = envelope.Error.Message
		}
	}
	if envelope.ResponseMetadata != nil && envelope.ResponseMetadata.Error != nil {
		if code == "" || code == "<nil>" {
			code = fmt.Sprint(envelope.ResponseMetadata.Error.Code)
		}
		if message == "" {
			message = envelope.ResponseMetadata.Error.Message
		}
	}
	if message == "" && response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}
	if message == "" {
		message = http.StatusText(response.StatusCode)
	}
	return &ProviderError{StatusCode: response.StatusCode, Code: code, Message: message}
}

func (c *Client) CreateAssetGroup(ctx context.Context, body map[string]any) (*Response, error) {
	if c.config.Provider == "runyuan" {
		return c.action(ctx, "CreateAssetGroup", body)
	}
	return c.Do(ctx, http.MethodPost, "/api/openapi-maas/exp/aicc/v2/asset-group", body, nil)
}
func (c *Client) ListAssetGroups(ctx context.Context, body map[string]any) (*Response, error) {
	if c.config.Provider == "runyuan" {
		return c.action(ctx, "ListAssetGroups", body)
	}
	return c.Do(ctx, http.MethodPost, "/api/openapi-maas/exp/aicc/v2/asset-group/query", body, nil)
}
func (c *Client) GetAssetGroup(ctx context.Context, id string) (*Response, error) {
	if c.config.Provider == "runyuan" {
		return c.action(ctx, "GetAssetGroup", map[string]any{"Id": id})
	}
	return c.Do(ctx, http.MethodGet, "/api/openapi-maas/exp/aicc/v2/asset-group/"+url.PathEscape(id), nil, nil)
}
func (c *Client) UpdateAssetGroup(ctx context.Context, id string, body map[string]any) (*Response, error) {
	if c.config.Provider == "runyuan" {
		payload := cloneMap(body)
		payload["Id"] = id
		return c.action(ctx, "UpdateAssetGroup", payload)
	}
	return c.Do(ctx, http.MethodPut, "/api/openapi-maas/exp/aicc/v2/asset-group/"+url.PathEscape(id), body, nil)
}
func (c *Client) DeleteAssetGroup(ctx context.Context, id string) (*Response, error) {
	if c.config.Provider == "runyuan" {
		return c.action(ctx, "DeleteAssetGroup", map[string]any{"Id": id})
	}
	return c.Do(ctx, http.MethodDelete, "/api/openapi-maas/exp/aicc/v2/asset-group/"+url.PathEscape(id), nil, nil)
}
func (c *Client) CreateAsset(ctx context.Context, body map[string]any) (*Response, error) {
	if c.config.Provider == "runyuan" {
		return c.action(ctx, "CreateAsset", body)
	}
	return c.Do(ctx, http.MethodPost, "/api/openapi-maas/exp/aicc/v2/asset", body, nil)
}
func (c *Client) ListAssets(ctx context.Context, body map[string]any) (*Response, error) {
	if c.config.Provider == "runyuan" {
		return c.action(ctx, "ListAssets", body)
	}
	return c.Do(ctx, http.MethodPost, "/api/openapi-maas/exp/aicc/v2/asset/query", body, nil)
}
func (c *Client) GetAsset(ctx context.Context, id string) (*Response, error) {
	if c.config.Provider == "runyuan" {
		return c.action(ctx, "GetAsset", map[string]any{"Id": id})
	}
	return c.Do(ctx, http.MethodGet, "/api/openapi-maas/exp/aicc/v2/asset/"+url.PathEscape(id), nil, nil)
}
func (c *Client) UpdateAsset(ctx context.Context, id string, body map[string]any) (*Response, error) {
	if c.config.Provider == "runyuan" {
		payload := cloneMap(body)
		payload["Id"] = id
		return c.action(ctx, "UpdateAsset", payload)
	}
	return c.Do(ctx, http.MethodPut, "/api/openapi-maas/exp/aicc/v2/asset/"+url.PathEscape(id), body, nil)
}
func (c *Client) DeleteAsset(ctx context.Context, id string) (*Response, error) {
	if c.config.Provider == "runyuan" {
		return c.action(ctx, "DeleteAsset", map[string]any{"Id": id})
	}
	return c.Do(ctx, http.MethodDelete, "/api/openapi-maas/exp/aicc/v2/asset/"+url.PathEscape(id), nil, nil)
}

func (c *Client) CreateRealPersonSession(ctx context.Context, body map[string]any) (*Response, error) {
	if c.config.Provider == "runyuan" {
		return c.action(ctx, "CreateVisualValidateSession", body)
	}
	return c.Do(ctx, http.MethodPost, "/api/openapi-maas/exp/aicc/v2/real-person-auth/sessions", body, nil)
}

func (c *Client) GetAssetGroupByBytedToken(ctx context.Context, body map[string]any) (*Response, error) {
	if c.config.Provider == "runyuan" {
		return c.action(ctx, "GetVisualValidateResult", body)
	}
	return c.Do(ctx, http.MethodPost, "/api/openapi-maas/exp/aicc/v2/real-person-auth/asset-group/by-byted-token", body, nil)
}

func cloneMap(input map[string]any) map[string]any {
	clone := make(map[string]any, len(input)+1)
	for key, value := range input {
		clone[key] = value
	}
	return clone
}

// runyuanPayload translates the gateway's stable camelCase asset contract to
// the PascalCase field names used by Runyuan's Ark-style Action API.
func runyuanPayload(action string, input map[string]any) map[string]any {
	out := make(map[string]any, len(input)+2)
	for key, value := range input {
		mapped := map[string]string{
			"groupName": "Name", "groupType": "GroupType", "groupId": "GroupId", "groupIds": "GroupIds",
			"assetName": "Name", "assetUrl": "URL", "assetType": "AssetType", "assetId": "Id",
			"pageNo": "PageNumber", "pageSize": "PageSize", "sortBy": "SortBy", "sortOrder": "SortOrder",
			"callbackUrl": "CallbackURL", "bytedToken": "BytedToken", "projectName": "ProjectName",
			"description": "Description", "statuses": "Statuses",
		}[key]
		if mapped == "" {
			mapped = key
		}
		out[mapped] = value
	}
	if filter, ok := out["filter"]; ok {
		if _, exists := out["Filter"]; !exists {
			if filterMap, isMap := filter.(map[string]any); isMap {
				normalizedFilter := make(map[string]any, len(filterMap))
				for key, value := range filterMap {
					mapped := map[string]string{"groupType": "GroupType", "name": "Name", "groupIds": "GroupIds", "assetType": "AssetType", "statuses": "Statuses"}[key]
					if mapped == "" {
						mapped = key
					}
					normalizedFilter[mapped] = value
				}
				out["Filter"] = normalizedFilter
			} else {
				out["Filter"] = filter
			}
		}
		delete(out, "filter")
	}
	if action == "ListAssetGroups" || action == "ListAssets" {
		filter := map[string]any{}
		for _, key := range []string{"GroupType", "Name", "GroupIds", "AssetType", "Statuses"} {
			if value, ok := out[key]; ok {
				filter[key] = value
				delete(out, key)
			}
		}
		if len(filter) > 0 {
			out["Filter"] = filter
		}
	}
	return out
}

// QueryModelTokensConsumed and the deduction methods expose the optional
// Mobile Cloud account/usage APIs through the same signed client. They are
// intentionally separate from gateway task billing: New API still owns the
// customer quota calculation, while these methods are for operator visibility
// into the upstream resource package.
func (c *Client) QueryModelTokensConsumed(ctx context.Context, body map[string]any) (*Response, error) {
	if c.config.Provider == "runyuan" {
		return nil, errors.New("runyuan does not expose the Mobile Cloud token-consumption API")
	}
	return c.Do(ctx, http.MethodPost, "/api/openapi-maas/model/tokens/consumed", body, nil)
}

func (c *Client) QueryAiccCreditDeduction(ctx context.Context, body map[string]any) (*Response, error) {
	if c.config.Provider == "runyuan" {
		return nil, errors.New("runyuan does not expose the Mobile Cloud credit-deduction API")
	}
	return c.Do(ctx, http.MethodPost, "/api/openapi-maas/model/aicc/deduction", body, nil)
}

func (c *Client) CreateAiccDeductionExportTask(ctx context.Context, body map[string]any) (*Response, error) {
	if c.config.Provider == "runyuan" {
		return nil, errors.New("runyuan does not expose the Mobile Cloud deduction-export API")
	}
	return c.Do(ctx, http.MethodPost, "/api/openapi-maas/model/aicc/deduction/export-task", body, nil)
}

func (c *Client) GetAiccDeductionExportTask(ctx context.Context, taskID string) (*Response, error) {
	if c.config.Provider == "runyuan" {
		return nil, errors.New("runyuan does not expose the Mobile Cloud deduction-export API")
	}
	return c.Do(ctx, http.MethodGet, "/api/openapi-maas/model/aicc/deduction/export-task/"+url.PathEscape(taskID), nil, nil)
}
