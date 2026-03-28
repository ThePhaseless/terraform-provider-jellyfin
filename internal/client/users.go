// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"fmt"
	"net/url"
)

// User represents a Jellyfin user.
type User struct {
	Id     string     `json:"Id"`
	Name   string     `json:"Name"`
	Policy UserPolicy `json:"Policy"`
}

// UserPolicy represents the policy/permissions for a user.
type UserPolicy struct {
	IsAdministrator  bool `json:"IsAdministrator"`
	IsDisabled       bool `json:"IsDisabled"`
	EnableAllFolders bool `json:"EnableAllFolders"`
}

// AuthResult represents the result of a user authentication.
type AuthResult struct {
	User        User   `json:"User"`
	AccessToken string `json:"AccessToken"`
	ServerId    string `json:"ServerId"`
}

// GetUsers retrieves all users.
func (c *Client) GetUsers() ([]User, error) {
	var users []User
	if err := c.get("/Users", &users); err != nil {
		return nil, fmt.Errorf("getting users: %w", err)
	}
	return users, nil
}

// GetUserByID retrieves a user by their ID.
func (c *Client) GetUserByID(id string) (*User, error) {
	var user User
	if err := c.get(fmt.Sprintf("/Users/%s", url.PathEscape(id)), &user); err != nil {
		return nil, fmt.Errorf("getting user %s: %w", id, err)
	}
	return &user, nil
}

// CreateUser creates a new user with the given name and password.
func (c *Client) CreateUser(name, password string) (*User, error) {
	body := map[string]string{
		"Name":     name,
		"Password": password,
	}
	var user User
	if err := c.postAndDecode("/Users/New", body, &user); err != nil {
		return nil, fmt.Errorf("creating user %s: %w", name, err)
	}
	return &user, nil
}

// DeleteUser deletes a user by their ID.
func (c *Client) DeleteUser(id string) error {
	if err := c.delete(fmt.Sprintf("/Users/%s", url.PathEscape(id))); err != nil {
		return fmt.Errorf("deleting user %s: %w", id, err)
	}
	return nil
}

// UpdateUser updates an existing user.
func (c *Client) UpdateUser(user *User) error {
	if err := c.post(fmt.Sprintf("/Users/%s", url.PathEscape(user.Id)), user); err != nil {
		return fmt.Errorf("updating user %s: %w", user.Id, err)
	}
	return nil
}

// UpdateUserPassword changes a user's password.
func (c *Client) UpdateUserPassword(id, currentPassword, newPassword string) error {
	body := map[string]string{
		"CurrentPw": currentPassword,
		"NewPw":     newPassword,
	}
	if err := c.post(fmt.Sprintf("/Users/%s/Password", url.PathEscape(id)), body); err != nil {
		return fmt.Errorf("updating password for user %s: %w", id, err)
	}
	return nil
}

// UpdateUserPolicy updates a user's policy/permissions.
func (c *Client) UpdateUserPolicy(id string, policy *UserPolicy) error {
	if err := c.post(fmt.Sprintf("/Users/%s/Policy", url.PathEscape(id)), policy); err != nil {
		return fmt.Errorf("updating policy for user %s: %w", id, err)
	}
	return nil
}

// AuthenticateByName authenticates a user by username and password.
func (c *Client) AuthenticateByName(username, password string) (*AuthResult, error) {
	body := map[string]string{
		"Username": username,
		"Pw":       password,
	}
	var result AuthResult
	if err := c.postAndDecode("/Users/AuthenticateByName", body, &result); err != nil {
		return nil, fmt.Errorf("authenticating user %s: %w", username, err)
	}
	return &result, nil
}
