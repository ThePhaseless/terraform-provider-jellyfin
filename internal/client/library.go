// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// VirtualFolder represents a library (virtual folder) in Jellyfin.
type VirtualFolder struct {
	Name           string          `json:"Name"`
	Locations      []string        `json:"Locations"`
	CollectionType string          `json:"CollectionType"`
	ItemID         string          `json:"ItemId"`
	LibraryOptions json.RawMessage `json:"LibraryOptions,omitempty"`
}

// LibraryOptions represents the configuration for a library.
// RawJSON stores the complete JSON for flexibility.
type LibraryOptions struct {
	RawJSON string `json:"-"`
}

// MarshalJSON implements custom JSON marshaling for LibraryOptions.
func (lo *LibraryOptions) MarshalJSON() ([]byte, error) {
	if lo.RawJSON == "" {
		return []byte("{}"), nil
	}
	return []byte(lo.RawJSON), nil
}

// UnmarshalJSON implements custom JSON unmarshaling for LibraryOptions.
func (lo *LibraryOptions) UnmarshalJSON(data []byte) error {
	lo.RawJSON = string(data)
	return nil
}

// GetVirtualFolders retrieves all virtual folders (libraries).
func (c *Client) GetVirtualFolders(ctx context.Context) ([]VirtualFolder, error) {
	var folders []VirtualFolder
	if err := c.get(ctx, "/Library/VirtualFolders", func(reader io.Reader) error {
		return json.NewDecoder(reader).Decode(&folders)
	}); err != nil {
		return nil, fmt.Errorf("getting virtual folders: %w", err)
	}
	return folders, nil
}

// AddVirtualFolder creates a new virtual folder (library).
func (c *Client) AddVirtualFolder(ctx context.Context, name, collectionType string, paths []string, libraryOptions *LibraryOptions) error {
	params := url.Values{}
	params.Set("name", name)
	params.Set("collectionType", collectionType)
	params.Set("refreshLibrary", "true")
	for _, p := range paths {
		params.Add("paths", p)
	}

	apiPath := "/Library/VirtualFolders?" + params.Encode()

	var body []byte
	if libraryOptions != nil {
		requestBody := struct {
			LibraryOptions *LibraryOptions `json:"LibraryOptions"`
		}{LibraryOptions: libraryOptions}
		jsonBody, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("marshaling virtual folder %s request: %w", name, err)
		}
		body = jsonBody
	}

	if err := c.post(ctx, apiPath, body); err != nil {
		return fmt.Errorf("adding virtual folder %s: %w", name, err)
	}
	return nil
}

// RemoveVirtualFolder removes a virtual folder (library) by name.
func (c *Client) RemoveVirtualFolder(ctx context.Context, name string) error {
	params := url.Values{}
	params.Set("name", name)
	params.Set("refreshLibrary", "false")

	path := "/Library/VirtualFolders?" + params.Encode()

	if err := c.delete(ctx, path); err != nil {
		return fmt.Errorf("removing virtual folder %s: %w", name, err)
	}
	return nil
}

// UpdateVirtualFolder updates the library options for a virtual folder.
func (c *Client) UpdateVirtualFolder(ctx context.Context, name string, libraryOptions *LibraryOptions) error {
	// Build a JSON body that includes the Name field alongside the library options.
	rawOpts := "{}"
	if libraryOptions != nil && libraryOptions.RawJSON != "" {
		rawOpts = libraryOptions.RawJSON
	}

	// Parse the library options, inject the Name field, and re-serialize.
	var opts map[string]json.RawMessage
	if err := json.Unmarshal([]byte(rawOpts), &opts); err != nil {
		return fmt.Errorf("parsing library options for virtual folder %s: %w", name, err)
	}

	nameJSON, err := json.Marshal(name)
	if err != nil {
		return fmt.Errorf("marshaling name for virtual folder %s: %w", name, err)
	}
	opts["Name"] = json.RawMessage(nameJSON)

	body, err := json.Marshal(opts)
	if err != nil {
		return fmt.Errorf("marshaling library options for virtual folder %s: %w", name, err)
	}

	if err := c.postRaw(ctx, "/Library/VirtualFolders/LibraryOptions", string(body)); err != nil {
		return fmt.Errorf("updating virtual folder %s: %w", name, err)
	}
	return nil
}

// GetVirtualFolderLibraryOptions extracts the LibraryOptions from a VirtualFolder as a LibraryOptions struct.
func (vf *VirtualFolder) GetLibraryOptions() *LibraryOptions {
	return &LibraryOptions{
		RawJSON: strings.TrimSpace(string(vf.LibraryOptions)),
	}
}
