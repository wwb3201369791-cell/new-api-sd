package mobilecloudasset

import (
	"strings"
	"testing"
	"time"
)

func TestCanonicalQueryUsesMobileCloudEncoding(t *testing.T) {
	got := CanonicalQuery(map[string]string{
		"z":         "a b*~",
		"a":         "中文",
		"Signature": "ignored",
	})
	if want := "a=%E4%B8%AD%E6%96%87&z=a%20b%2A~"; got != want {
		t.Fatalf("canonical query = %q, want %q", got, want)
	}
}

func TestSignIsDeterministicAndContainsPublicParameters(t *testing.T) {
	params := map[string]string{"Version": "2016-12-05"}
	now := time.Date(2017, 1, 11, 15, 15, 11, 0, time.UTC)
	first, err := Sign("POST", "/api/v2/keypair", "testid", "testsecret", params, now, "9d81ffbeaaf7477390db5df577bb3299", "HmacSHA1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Sign("POST", "/api/v2/keypair", "testid", "testsecret", params, now, "9d81ffbeaaf7477390db5df577bb3299", "HmacSHA1")
	if err != nil {
		t.Fatal(err)
	}
	if first != second || !strings.Contains(first, "AccessKey=testid") || !strings.Contains(first, "SignatureMethod=HmacSHA1") || !strings.Contains(first, "Signature=") {
		t.Fatalf("unexpected signed query: %q", first)
	}
}

func TestSignUsesBeijingWallClockTimestamp(t *testing.T) {
	now := time.Date(2026, 9, 3, 2, 54, 0, 0, time.UTC)
	signed, err := Sign("POST", "/api/v2/keypair", "testid", "testsecret", nil, now, "nonce", "HmacSHA1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(signed, "Timestamp=2026-09-03T10%3A54%3A00Z") {
		t.Fatalf("signed query did not use Beijing wall-clock timestamp: %q", signed)
	}
}
