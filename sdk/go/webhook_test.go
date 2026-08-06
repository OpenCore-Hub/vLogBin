package vlogbin

import (
	"strconv"
	"testing"
	"time"
)

func TestVerifyWebhookSignature(t *testing.T) {
	secret := "secret"
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	payload := []byte(`{"event_type":"usage.accepted"}`)
	mac := hmacSHA256(secret, timestamp, payload)
	if !VerifyWebhookSignature(secret, timestamp, payload, mac) {
		t.Fatal("valid signature rejected")
	}
	if VerifyWebhookSignature(secret, timestamp, payload, "deadbeef") {
		t.Fatal("invalid signature accepted")
	}
}

func TestVerifyWebhookSignatureWithin(t *testing.T) {
	secret := "secret"
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	payload := []byte(`{}`)
	mac := hmacSHA256(secret, timestamp, payload)
	if !VerifyWebhookSignatureWithin(secret, timestamp, payload, mac, 5*time.Minute) {
		t.Fatal("fresh signature rejected")
	}
	old := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	oldMac := hmacSHA256(secret, old, payload)
	if VerifyWebhookSignatureWithin(secret, old, payload, oldMac, 5*time.Minute) {
		t.Fatal("stale timestamp accepted")
	}
}
