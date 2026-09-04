package mobilecloudasset

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestClientSignsRequestsAndAcceptsProviderEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/openapi-maas/exp/aicc/v2/asset-group/query" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if !r.Close || r.Header.Get("Connection") != "close" {
			t.Fatalf("mobile cloud request must disable keep-alive: close=%t connection=%q", r.Close, r.Header.Get("Connection"))
		}
		for _, key := range []string{"AccessKey", "Timestamp", "SignatureNonce", "SignatureVersion", "SignatureMethod", "Signature"} {
			if r.URL.Query().Get(key) == "" {
				t.Errorf("missing signed query %s", key)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"state":"OK","body":{"data":[],"total":0}}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, AccessKey: "ak", SecretKey: "sk"})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.ListAssetGroups(context.Background(), map[string]any{"pageNo": 1, "pageSize": 20})
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("request failed: response=%v err=%v", response, err)
	}
}

func TestClientSurfacesProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"state":"ERROR","errorCode":"BAD","errorMessage":"invalid request"}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, AccessKey: "ak", SecretKey: "sk"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.GetAssetGroup(context.Background(), "group-1")
	if err == nil || !strings.Contains(err.Error(), "invalid request") {
		t.Fatalf("expected provider error, got %v", err)
	}
}

func TestMobileCloudReadRetriesAClosedConnection(t *testing.T) {
	var calls int
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		if !request.Close || request.Header.Get("Connection") != "close" {
			t.Fatalf("retry request must disable keep-alive: close=%t connection=%q", request.Close, request.Header.Get("Connection"))
		}
		if calls == 1 {
			return nil, errors.New("remote end closed connection without response")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"state":"OK","body":{}}`)),
			Request:    request,
		}, nil
	})
	client, err := NewClient(Config{
		BaseURL:    "https://ecloud.example.test",
		AccessKey:  "ak",
		SecretKey:  "sk",
		HTTPClient: &http.Client{Transport: transport},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.GetAsset(context.Background(), "asset-1"); err != nil {
		t.Fatalf("read should succeed after a transient close: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected one retry, got %d calls", calls)
	}
}

func TestMobileCloudDeleteTreatsAmbiguousAlreadyAbsentAsSuccess(t *testing.T) {
	var calls int
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("remote end closed connection without response")
		}
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"state":"ERROR","errorCode":"C400999","errorMessage":"素材不存在或无权限访问"}`)),
			Request:    request,
		}, nil
	})
	client, err := NewClient(Config{
		BaseURL:    "https://ecloud.example.test",
		AccessKey:  "ak",
		SecretKey:  "sk",
		HTTPClient: &http.Client{Transport: transport},
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.DeleteAsset(context.Background(), "asset-1")
	if err != nil {
		t.Fatalf("ambiguous idempotent delete should succeed: %v", err)
	}
	if response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"body":true`) {
		t.Fatalf("unexpected normalized delete response: %#v", response)
	}
	if calls != 2 {
		t.Fatalf("expected one retry, got %d calls", calls)
	}
}

func TestMobileCloudDeletePreservesPermissionError(t *testing.T) {
	var calls int
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("remote end closed connection without response")
		}
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"state":"ERROR","errorCode":"C400999","errorMessage":"无权限访问素材"}`)),
			Request:    request,
		}, nil
	})
	client, err := NewClient(Config{
		Provider:   "mobilecloud",
		BaseURL:    "https://ecloud.example.test",
		AccessKey:  "ak",
		SecretKey:  "sk",
		HTTPClient: &http.Client{Transport: transport},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.DeleteAsset(context.Background(), "asset-1")
	if err == nil || !strings.Contains(err.Error(), "无权限访问素材") {
		t.Fatalf("permission failure must not be normalized: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected one retry, got %d calls", calls)
	}
}

func TestClientUsageAndDeductionEndpointsUseSignedPaths(t *testing.T) {
	paths := make(chan string, 5)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths <- r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"state":"OK","body":{}}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, AccessKey: "ak", SecretKey: "sk"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err = client.QueryModelTokensConsumed(ctx, map[string]any{"model": "AICC-Doubao-Seedance-2.0"}); err != nil {
		t.Fatal(err)
	}
	if _, err = client.QueryAiccCreditDeduction(ctx, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if _, err = client.CreateAiccDeductionExportTask(ctx, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if _, err = client.GetAiccDeductionExportTask(ctx, "export-1"); err != nil {
		t.Fatal(err)
	}
	if _, err = client.CancelOrder(ctx, map[string]any{"instanceIds": []any{"instance-1"}}); err != nil {
		t.Fatal(err)
	}
	close(paths)
	want := []string{
		"/api/openapi-maas/model/tokens/consumed",
		"/api/openapi-maas/model/aicc/deduction",
		"/api/openapi-maas/model/aicc/deduction/export-task",
		"/api/openapi-maas/model/aicc/deduction/export-task/export-1",
		"/api/openapi-maas/console/studio/cancel",
	}
	for _, expected := range want {
		if actual := <-paths; actual != expected {
			t.Fatalf("unexpected path: got %s want %s", actual, expected)
		}
	}
}

func TestRunyuanClientSignsArkHeadersAndMapsPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/video" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		if r.URL.Query().Get("Action") != "CreateAsset" || r.URL.Query().Get("Version") != DefaultRunyuanVersion {
			t.Fatalf("unexpected action query: %s", r.URL.RawQuery)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "HMAC-SHA256 Credential=AK/") {
			t.Fatalf("missing HMAC authorization: %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("X-Date") == "" || r.Header.Get("X-Content-Sha256") == "" || r.Host == "" {
			t.Fatalf("missing signed headers: host=%q x-date=%q hash=%q", r.Host, r.Header.Get("X-Date"), r.Header.Get("X-Content-Sha256"))
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(body)
		if got, want := r.Header.Get("X-Content-Sha256"), hex.EncodeToString(hash[:]); got != want {
			t.Fatalf("body hash mismatch: got %s want %s", got, want)
		}
		var payload map[string]any
		if err := common.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["Name"] != "hero" || payload["URL"] != "https://example.com/hero.png" || payload["GroupId"] != "group-1" {
			t.Fatalf("unexpected mapped payload: %#v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ResponseMetadata":{"RequestId":"req-1"},"Result":{"Id":"asset-1"}}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{Provider: "runyuan", BaseURL: server.URL, AccessKey: "AK", SecretKey: "SK"})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.CreateAsset(context.Background(), map[string]any{
		"assetName": "hero",
		"assetUrl":  "https://example.com/hero.png",
		"assetType": "Image",
		"groupId":   "group-1",
	})
	if err != nil {
		t.Fatalf("runyuan request failed: %v", err)
	}
	if response.Provider != "runyuan" || response.StatusCode != http.StatusOK {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestRunyuanProviderErrorFromResponseMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ResponseMetadata":{"Error":{"Code":"InvalidParameter","Message":"bad asset"}},"Result":null}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{Provider: "runyuan", BaseURL: server.URL, AccessKey: "AK", SecretKey: "SK"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.GetAsset(context.Background(), "asset-1")
	if err == nil || !strings.Contains(err.Error(), "bad asset") || !strings.Contains(err.Error(), "InvalidParameter") {
		t.Fatalf("expected Runyuan provider error, got %v", err)
	}
}

func TestRunyuanCancelOrderDoesNotUseMobileCloudEndpoint(t *testing.T) {
	client, err := NewClient(Config{Provider: "runyuan", BaseURL: "https://example.com", AccessKey: "AK", SecretKey: "SK"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.CancelOrder(context.Background(), map[string]any{"instanceIds": []any{"MAAS-TEST"}})
	if err == nil || !strings.Contains(err.Error(), "does not expose") {
		t.Fatalf("expected unsupported-operation error, got %v", err)
	}
}
