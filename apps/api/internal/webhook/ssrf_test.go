package webhook

import (
	"testing"
)

func TestValidateURLBlocksPrivateIPs(t *testing.T) {
	blocked := []string{
		"http://10.0.0.1/webhook",
		"http://172.16.0.1/webhook",
		"http://192.168.1.1/webhook",
		"http://127.0.0.1/webhook",
		"http://169.254.169.254/webhook",
		"http://0.0.0.0/webhook",
		"http://[::1]/webhook",
		"http://[fc00::1]/webhook",
		"http://[fe80::1]/webhook",
	}

	for _, u := range blocked {
		t.Run(u, func(t *testing.T) {
			err := ValidateURL(u)
			if err == nil {
				t.Errorf("ValidateURL(%q) should be blocked", u)
			}
		})
	}
}

func TestValidateURLAllowsPublicIPs(t *testing.T) {
	// Note: these may fail in CI if DNS resolves differently, but in general
	// public IPs should pass. We test with known public DNS resolvers.
	allowed := []string{
		"https://8.8.8.8/webhook",
		"https://1.1.1.1/webhook",
	}

	for _, u := range allowed {
		t.Run(u, func(t *testing.T) {
			err := ValidateURL(u)
			if err != nil {
				t.Errorf("ValidateURL(%q) should be allowed, got %v", u, err)
			}
		})
	}
}

func TestValidateURLRejectsInvalidSchemes(t *testing.T) {
	invalid := []string{
		"ftp://example.com/file",
		"file:///etc/passwd",
		"javascript:alert(1)",
		"ssh://example.com",
		"ws://example.com/socket",
	}

	for _, u := range invalid {
		t.Run(u, func(t *testing.T) {
			err := ValidateURL(u)
			if err == nil {
				t.Errorf("ValidateURL(%q) should be rejected (invalid scheme)", u)
			}
		})
	}
}

func TestValidateURLRejectsMalformed(t *testing.T) {
	invalid := []string{
		"",
		"not-a-url",
		"http://",
		"://missing-scheme",
		"http://[invalid",
	}

	for _, u := range invalid {
		t.Run(u, func(t *testing.T) {
			err := ValidateURL(u)
			if err == nil {
				t.Errorf("ValidateURL(%q) should be rejected (malformed)", u)
			}
		})
	}
}

func TestValidateURLAllowLoopback(t *testing.T) {
	// The test variant allows loopback (for httptest servers).
	allowed := []string{
		"http://127.0.0.1:8080/webhook",
		"http://localhost:3000/webhook",
	}

	for _, u := range allowed {
		t.Run(u, func(t *testing.T) {
			err := ValidateURLAllowLoopback(u)
			if err != nil {
				t.Errorf("ValidateURLAllowLoopback(%q) should be allowed, got %v", u, err)
			}
		})
	}
}

func TestValidateURLAllowLoopbackStillBlocksMetadata(t *testing.T) {
	// Even in loopback-allowed mode, cloud metadata should be blocked.
	blocked := []string{
		"http://169.254.169.254/webhook",
		"http://10.0.0.1/webhook",
		"http://172.16.0.1/webhook",
	}

	for _, u := range blocked {
		t.Run(u, func(t *testing.T) {
			err := ValidateURLAllowLoopback(u)
			if err == nil {
				t.Errorf("ValidateURLAllowLoopback(%q) should be blocked", u)
			}
		})
	}
}
