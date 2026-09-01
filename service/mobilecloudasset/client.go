package mobilecloudasset

import (
	"bytes"
	"context"
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
	DefaultBaseURL      = "https://ecloud.10086.cn"
	DefaultResourcePool = "CIDC-CORE-00"
)

var (
	ErrMissingCredentials         = errors.New("mobile cloud asset credentials are missing")
	ErrUnsupportedSignatureMethod = errors.New("unsupported mobile cloud signature method")
)

type Config struct {
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
		return fmt.Sprintf("mobile cloud asset request failed (%d): %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("mobile cloud asset request failed (%d, %s): %s", e.StatusCode, e.Code, e.Message)
}

func NewClient(config Config) (*Client, error) {
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if config.BaseURL == "" {
		config.BaseURL = DefaultBaseURL
	}
	parsed, err := url.Parse(config.BaseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return nil, errors.New("mobile cloud asset base URL must be an absolute HTTP(S) URL")
	}
	if strings.TrimSpace(config.AccessKey) == "" || strings.TrimSpace(config.SecretKey) == "" {
		return nil, ErrMissingCredentials
	}
	if strings.TrimSpace(config.ResourcePool) == "" {
		config.ResourcePool = DefaultResourcePool
	}
	if strings.TrimSpace(config.SignatureMethod) == "" {
		config.SignatureMethod = "HmacSHA1"
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
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
	result := &Response{StatusCode: response.StatusCode, Header: response.Header.Clone(), Body: data}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return result, decodeProviderError(result)
	}
	if err := decodeProviderError(result); err != nil {
		return result, err
	}
	return result, nil
}

func decodeProviderError(response *Response) error {
	if response == nil || len(response.Body) == 0 {
		if response != nil && response.StatusCode >= 200 && response.StatusCode < 300 {
			return nil
		}
		return &ProviderError{StatusCode: response.StatusCode, Message: http.StatusText(response.StatusCode)}
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
	}
	if err := common.Unmarshal(response.Body, &envelope); err != nil {
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			return nil
		}
		return &ProviderError{StatusCode: response.StatusCode, Message: strings.TrimSpace(string(response.Body))}
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 && (envelope.State == "" || strings.EqualFold(envelope.State, "OK") || strings.EqualFold(envelope.State, "SUCCESS")) {
		return nil
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
	if message == "" && response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}
	if message == "" {
		message = http.StatusText(response.StatusCode)
	}
	return &ProviderError{StatusCode: response.StatusCode, Code: code, Message: message}
}

func (c *Client) CreateAssetGroup(ctx context.Context, body map[string]any) (*Response, error) {
	return c.Do(ctx, http.MethodPost, "/api/openapi-maas/exp/aicc/v2/asset-group", body, nil)
}
func (c *Client) ListAssetGroups(ctx context.Context, body map[string]any) (*Response, error) {
	return c.Do(ctx, http.MethodPost, "/api/openapi-maas/exp/aicc/v2/asset-group/query", body, nil)
}
func (c *Client) GetAssetGroup(ctx context.Context, id string) (*Response, error) {
	return c.Do(ctx, http.MethodGet, "/api/openapi-maas/exp/aicc/v2/asset-group/"+url.PathEscape(id), nil, nil)
}
func (c *Client) UpdateAssetGroup(ctx context.Context, id string, body map[string]any) (*Response, error) {
	return c.Do(ctx, http.MethodPut, "/api/openapi-maas/exp/aicc/v2/asset-group/"+url.PathEscape(id), body, nil)
}
func (c *Client) DeleteAssetGroup(ctx context.Context, id string) (*Response, error) {
	return c.Do(ctx, http.MethodDelete, "/api/openapi-maas/exp/aicc/v2/asset-group/"+url.PathEscape(id), nil, nil)
}
func (c *Client) CreateAsset(ctx context.Context, body map[string]any) (*Response, error) {
	return c.Do(ctx, http.MethodPost, "/api/openapi-maas/exp/aicc/v2/asset", body, nil)
}
func (c *Client) ListAssets(ctx context.Context, body map[string]any) (*Response, error) {
	return c.Do(ctx, http.MethodPost, "/api/openapi-maas/exp/aicc/v2/asset/query", body, nil)
}
func (c *Client) GetAsset(ctx context.Context, id string) (*Response, error) {
	return c.Do(ctx, http.MethodGet, "/api/openapi-maas/exp/aicc/v2/asset/"+url.PathEscape(id), nil, nil)
}
func (c *Client) UpdateAsset(ctx context.Context, id string, body map[string]any) (*Response, error) {
	return c.Do(ctx, http.MethodPut, "/api/openapi-maas/exp/aicc/v2/asset/"+url.PathEscape(id), body, nil)
}
func (c *Client) DeleteAsset(ctx context.Context, id string) (*Response, error) {
	return c.Do(ctx, http.MethodDelete, "/api/openapi-maas/exp/aicc/v2/asset/"+url.PathEscape(id), nil, nil)
}

func (c *Client) CreateRealPersonSession(ctx context.Context, body map[string]any) (*Response, error) {
	return c.Do(ctx, http.MethodPost, "/api/openapi-maas/exp/aicc/v2/real-person-auth/sessions", body, nil)
}

func (c *Client) GetAssetGroupByBytedToken(ctx context.Context, body map[string]any) (*Response, error) {
	return c.Do(ctx, http.MethodPost, "/api/openapi-maas/exp/aicc/v2/real-person-auth/asset-group/by-byted-token", body, nil)
}
