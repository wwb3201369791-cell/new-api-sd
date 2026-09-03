package mobilecloudasset

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// PercentEncode follows Mobile Cloud's V2.0 signing rules (RFC3986-style;
// spaces are %20, not '+').
func PercentEncode(value string) string {
	encoded := url.QueryEscape(value)
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	encoded = strings.ReplaceAll(encoded, "*", "%2A")
	encoded = strings.ReplaceAll(encoded, "%7E", "~")
	return encoded
}

// CanonicalQuery sorts and percent-encodes all parameters except Signature.
func CanonicalQuery(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for key := range params {
		if strings.EqualFold(key, "Signature") {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, PercentEncode(key)+"="+PercentEncode(params[key]))
	}
	return strings.Join(parts, "&")
}

// Sign creates the final signed query string. It intentionally accepts a
// clock/nonce so tests can be deterministic and operators can diagnose clock
// skew without changing the signing algorithm.
func Sign(method, path, accessKey, secretKey string, params map[string]string, now time.Time, nonce string, signatureMethod string) (string, error) {
	if strings.TrimSpace(accessKey) == "" || strings.TrimSpace(secretKey) == "" {
		return "", ErrMissingCredentials
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if strings.TrimSpace(nonce) == "" {
		nonce = strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = "GET"
	}
	signatureMethod = strings.TrimSpace(signatureMethod)
	if signatureMethod == "" {
		signatureMethod = "HmacSHA1"
	}
	params = cloneParams(params)
	params["AccessKey"] = accessKey
	// Mobile Cloud's V2 gateway follows its SDKs and expects the current
	// Beijing wall-clock time with a literal trailing Z. Sending UTC here is
	// rejected as an invalid timestamp even when the server clock is healthy.
	// Keep this conversion explicit so deployments in a UTC container behave
	// the same as the official SDKs.
	beijing := time.FixedZone("Asia/Shanghai", 8*60*60)
	params["Timestamp"] = now.In(beijing).Format("2006-01-02T15:04:05Z")
	params["SignatureNonce"] = nonce
	params["SignatureVersion"] = "V2.0"
	params["SignatureMethod"] = signatureMethod
	canonical := CanonicalQuery(params)
	hashed := sha256.Sum256([]byte(canonical))
	stringToSign := method + "\n" + PercentEncode(path) + "\n" + hex.EncodeToString(hashed[:])
	key := []byte("BC_SIGNATURE&" + secretKey)
	var signature []byte
	switch strings.ToUpper(signatureMethod) {
	case "HMACSHA1":
		mac := hmac.New(sha1.New, key)
		_, _ = mac.Write([]byte(stringToSign))
		signature = mac.Sum(nil)
	case "HMACSHA256":
		mac := hmac.New(sha256.New, key)
		_, _ = mac.Write([]byte(stringToSign))
		signature = mac.Sum(nil)
	default:
		return "", ErrUnsupportedSignatureMethod
	}
	params["Signature"] = hex.EncodeToString(signature)
	return CanonicalQueryIncludingSignature(params), nil
}

func CanonicalQueryIncludingSignature(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, PercentEncode(key)+"="+PercentEncode(params[key]))
	}
	return strings.Join(parts, "&")
}

func cloneParams(params map[string]string) map[string]string {
	clone := make(map[string]string, len(params)+5)
	for key, value := range params {
		clone[key] = value
	}
	return clone
}
