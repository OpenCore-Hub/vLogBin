package vlogbin

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"
)

// VerifyWebhookSignature checks the HMAC-SHA256 signature of
// timestamp||payload under the endpoint secret.
func VerifyWebhookSignature(secret, timestamp string, payload []byte, signature string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write(payload)
	expected := mac.Sum(nil)
	given, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	return hmac.Equal(expected, given)
}

// VerifyWebhookSignatureWithin checks the signature and the timestamp
// freshness window. The timestamp is the Unix epoch in seconds.
func VerifyWebhookSignatureWithin(
	secret, timestamp string,
	payload []byte,
	signature string,
	maxAge time.Duration,
) bool {
	secs, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	sent := time.Unix(secs, 0)
	if time.Since(sent) > maxAge || sent.After(time.Now().Add(maxAge)) {
		return false
	}
	return VerifyWebhookSignature(secret, timestamp, payload, signature)
}
