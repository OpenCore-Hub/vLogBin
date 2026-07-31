package zitadel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ManagementClient calls the ZITADEL Management API (v1) to create
// projects and OIDC applications for providers that enable Hosted Auth.
type ManagementClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewManagementClient creates a client for the ZITADEL Management API.
// The token is a Personal Access Token (PAT) or service account token
// with Management API permissions.
func NewManagementClient(baseURL, token string) *ManagementClient {
	return &ManagementClient{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// CreateProject creates a ZITADEL project and returns the project ID.
func (c *ManagementClient) CreateProject(ctx context.Context, name string) (string, error) {
	body := map[string]any{
		"name":                 name,
		"projectRoleAssertion": false,
		"hasProjectCheck":      false,
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := c.do(ctx, http.MethodPost, "/management/v1/projects", body, &resp); err != nil {
		return "", fmt.Errorf("create project: %w", err)
	}
	return resp.ID, nil
}

// CreateOIDCApp creates an OIDC application under a ZITADEL project and
// returns the app ID, client ID, and client secret.
func (c *ManagementClient) CreateOIDCApp(ctx context.Context, projectID, appName string, redirectURIs []string) (appID, clientID, clientSecret string, err error) {
	body := map[string]any{
		"name":              appName,
		"redirectUris":      redirectURIs,
		"responseTypes":     []string{"OIDC_RESPONSE_TYPE_CODE"},
		"grantTypes":        []string{"OIDC_GRANT_TYPE_AUTHORIZATION_CODE", "OIDC_GRANT_TYPE_REFRESH_TOKEN"},
		"appType":           "OIDC_APP_TYPE_WEB",
		"authMethodType":    "OIDC_AUTH_METHOD_TYPE_BASIC",
		"accessTokenType":   "OIDC_TOKEN_TYPE_BEARER",
		"idTokenRoleAssertion":   true,
		"accessTokenRoleAssertion": true,
	}
	var resp struct {
		AppID         string `json:"appId"`
		ClientID      string `json:"clientId"`
		ClientSecret  string `json:"clientSecret"`
	}
	path := fmt.Sprintf("/management/v1/projects/%s/apps/oidc", projectID)
	if err := c.do(ctx, http.MethodPost, path, body, &resp); err != nil {
		return "", "", "", fmt.Errorf("create oidc app: %w", err)
	}
	return resp.AppID, resp.ClientID, resp.ClientSecret, nil
}

// DeleteProject removes a ZITADEL project and all its applications.
func (c *ManagementClient) DeleteProject(ctx context.Context, projectID string) error {
	path := fmt.Sprintf("/management/v1/projects/%s", projectID)
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

// do executes an HTTP request to the ZITADEL Management API.
func (c *ManagementClient) do(ctx context.Context, method, path string, body any, out any) error {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ZITADEL API %s %s: HTTP %d: %s", method, path, resp.StatusCode, string(raw))
	}

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}
