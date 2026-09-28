// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// User represents a Jellyfin user.
type User struct {
	ID     string     `json:"Id"`
	Name   string     `json:"Name"`
	Policy UserPolicy `json:"Policy"`
}

// UserPolicy represents the policy/permissions for a user.
type UserPolicy struct {
	IsAdministrator          bool   `json:"IsAdministrator"`
	IsDisabled               bool   `json:"IsDisabled"`
	EnableAllFolders         bool   `json:"EnableAllFolders"`
	AuthenticationProviderID string `json:"AuthenticationProviderId"`
	PasswordResetProviderID  string `json:"PasswordResetProviderId"`
}

// errBlankUserID stops a user update before it is sent: Jellyfin applies one
// whose userId is blank to the signed-in user, the provider's own account.
var errBlankUserID = errors.New("user id is blank")

// AuthResult represents the result of a user authentication.
type AuthResult struct {
	AccessToken string `json:"AccessToken"`
	ServerID    string `json:"ServerId"`
}

// GetUsers retrieves all users.
func (c *Client) GetUsers(ctx context.Context) ([]User, error) {
	var users []User
	if err := c.getJSON(ctx, "/Users", &users); err != nil {
		return nil, fmt.Errorf("getting users: %w", err)
	}
	return users, nil
}

// CreateUser creates a new user with the given name and password.
func (c *Client) CreateUser(ctx context.Context, name, password string) (*User, error) {
	body := map[string]string{
		"Name":     name,
		"Password": password,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling create user request for %s: %w", name, err)
	}
	var user User
	if err := c.postJSON(ctx, "/Users/New", jsonBody, &user); err != nil {
		return nil, fmt.Errorf("creating user %s: %w", name, err)
	}
	return &user, nil
}

// DeleteUser deletes a user by their ID.
func (c *Client) DeleteUser(ctx context.Context, id string) error {
	if err := c.delete(ctx, fmt.Sprintf("/Users/%s", url.PathEscape(id))); err != nil {
		return fmt.Errorf("deleting user %s: %w", id, err)
	}
	return nil
}

// GetUserRaw returns the raw JSON of GET /Users/{id}.
func (c *Client) GetUserRaw(ctx context.Context, id string) (string, error) {
	raw, err := c.getRaw(ctx, fmt.Sprintf("/Users/%s", url.PathEscape(id)))
	if err != nil {
		return "", fmt.Errorf("getting user %s: %w", id, err)
	}
	return raw, nil
}

// UpdateUserRaw POSTs a raw user JSON to /Users?userId={id}.
// The server replaces the user's Configuration with the one in the body, so
// the body must carry the Configuration read from GetUserRaw or the user's
// settings are reset to defaults.
func (c *Client) UpdateUserRaw(ctx context.Context, id, userJSON string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("updating user: %w", errBlankUserID)
	}
	if err := c.postRaw(ctx, fmt.Sprintf("/Users?userId=%s", url.QueryEscape(id)), userJSON); err != nil {
		return fmt.Errorf("updating user %s: %w", id, err)
	}
	return nil
}

// UpdateUserPassword changes a user's password.
func (c *Client) UpdateUserPassword(ctx context.Context, id, currentPassword, newPassword string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("updating password: %w", errBlankUserID)
	}
	body := map[string]string{
		"CurrentPw": currentPassword,
		"NewPw":     newPassword,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshaling password update request for user %s: %w", id, err)
	}
	if err := c.post(ctx, fmt.Sprintf("/Users/Password?userId=%s", url.QueryEscape(id)), jsonBody); err != nil {
		return fmt.Errorf("updating password for user %s: %w", id, err)
	}
	return nil
}

// GetUserPolicyRaw returns the raw JSON of the user's Policy object (extracted from GET /Users/{id}).
// The policy is not decoded into UserPolicy: the server resets every field
// missing from a policy update, so fields the struct lacks (such as
// MaxParentalSubRating) must survive the read-modify-write untouched.
func (c *Client) GetUserPolicyRaw(ctx context.Context, id string) (string, error) {
	raw, err := c.getRaw(ctx, fmt.Sprintf("/Users/%s", url.PathEscape(id)))
	if err != nil {
		return "", fmt.Errorf("getting user %s for policy: %w", id, err)
	}

	var user struct {
		Policy json.RawMessage `json:"Policy"`
	}
	if err := json.Unmarshal([]byte(raw), &user); err != nil {
		return "", fmt.Errorf("parsing user %s for policy: %w", id, err)
	}
	if len(user.Policy) == 0 || string(user.Policy) == "null" {
		return "", fmt.Errorf("user %s has no policy", id)
	}

	return string(user.Policy), nil
}

// UpdateUserPolicyRaw POSTs a raw policy JSON to /Users/{id}/Policy.
func (c *Client) UpdateUserPolicyRaw(ctx context.Context, id, policyJSON string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("updating policy: %w", errBlankUserID)
	}
	if err := c.postRaw(ctx, fmt.Sprintf("/Users/%s/Policy", url.PathEscape(id)), policyJSON); err != nil {
		return fmt.Errorf("updating policy for user %s: %w", id, err)
	}
	return nil
}

// AuthenticateByName authenticates a user by username and password.
// This endpoint requires a special MediaBrowser header with client info, not a token.
func (c *Client) AuthenticateByName(ctx context.Context, username, password string) (*AuthResult, error) {
	body := map[string]string{
		"Username": username,
		"Pw":       password,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling auth request: %w", err)
	}

	const path = "/Users/AuthenticateByName"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("creating auth request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", `MediaBrowser Client="Terraform", Device="Provider", DeviceId="terraform-provider-jellyfin", Version="1.0.0"`)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing auth request: %w", err)
	}
	defer resp.Body.Close()

	if err := checkStatus(http.MethodPost, path, resp); err != nil {
		return nil, fmt.Errorf("authentication failed for user %s: %w", username, err)
	}

	var result AuthResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding auth response: %w", err)
	}

	return &result, nil
}
