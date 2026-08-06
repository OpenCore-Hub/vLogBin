// Package vlogbin is the official Go SDK for the vLogBin platform API.
package vlogbin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// APIError is the standardized public error envelope.
type APIError struct {
	Status     int
	Code       string
	Message    string
	RequestID  string
	RetryAfter string
	Details    json.RawMessage
}

func (e *APIError) Error() string {
	if e.RequestID != "" {
		return fmt.Sprintf("%s: %s (request_id=%s)", e.Code, e.Message, e.RequestID)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Client talks to the vLogBin platform API with API-key authentication.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a client for a platform base URL (e.g.
// https://api.vlogbin.com/v1) and a provider API key.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// RequestOptions carries per-request headers and query parameters.
type RequestOptions struct {
	IdempotencyKey string
	Query          url.Values
}

// Do sends an authenticated request and decodes a 2xx JSON body into out.
func (c *Client) Do(ctx context.Context, method, path string, opts RequestOptions, body, out any) error {
	u := c.baseURL + path
	if len(opts.Query) > 0 {
		u += "?" + opts.Query.Encode()
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	if opts.IdempotencyKey != "" {
		req.Header.Set("Idempotency-Key", opts.IdempotencyKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if out == nil {
			return nil
		}
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return decodeAPIError(resp)
}

func decodeAPIError(resp *http.Response) error {
	var envelope struct {
		Error struct {
			Code       string          `json:"code"`
			Message    string          `json:"message"`
			RequestID  string          `json:"request_id"`
			RetryAfter string          `json:"retry_after"`
			Details    json.RawMessage `json:"details"`
		} `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&envelope)
	err := &APIError{
		Status:     resp.StatusCode,
		Code:       envelope.Error.Code,
		Message:    envelope.Error.Message,
		RequestID:  envelope.Error.RequestID,
		RetryAfter: envelope.Error.RetryAfter,
		Details:    envelope.Error.Details,
	}
	if err.Code == "" {
		err.Code = "api_error"
	}
	if err.Message == "" {
		err.Message = fmt.Sprintf("request failed with status %d", resp.StatusCode)
	}
	return err
}
