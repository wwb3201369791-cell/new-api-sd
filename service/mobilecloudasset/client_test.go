package mobilecloudasset

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientSignsRequestsAndAcceptsProviderEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/openapi-maas/exp/aicc/v2/asset-group/query" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
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

func TestClientUsageAndDeductionEndpointsUseSignedPaths(t *testing.T) {
	paths := make(chan string, 4)
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
	close(paths)
	want := []string{
		"/api/openapi-maas/model/tokens/consumed",
		"/api/openapi-maas/model/aicc/deduction",
		"/api/openapi-maas/model/aicc/deduction/export-task",
		"/api/openapi-maas/model/aicc/deduction/export-task/export-1",
	}
	for _, expected := range want {
		if actual := <-paths; actual != expected {
			t.Fatalf("unexpected path: got %s want %s", actual, expected)
		}
	}
}
