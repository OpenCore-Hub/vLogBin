package vlogbin

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

func hmacSHA256(secret, timestamp string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}
