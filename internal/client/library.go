// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// VirtualFolder represents a library (virtual folder) in Jellyfin.
type VirtualFolder struct {
	Name           string          `json:"Name"`
	Locations      []string        `json:"Locations"`
	CollectionType string          `json:"CollectionType"`
	ItemId         string          `json:"ItemId"`
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
func (c *Client) GetVirtualFolders() ([]VirtualFolder, error) {
	var folders []VirtualFolder
	if err := c.get("/Library/VirtualFolders", &folders); err != nil {
		return nil, fmt.Errorf("getting virtual folders: %w", err)
	}
	return folders, nil
}

// AddVirtualFolder creates a new virtual folder (library).
func (c *Client) AddVirtualFolder(name, collectionType string, paths []string, libraryOptions *LibraryOptions) error {
	params := url.Values{}
	params.Set("name", name)
	params.Set("collectionType", collectionType)
	params.Set("refreshLibrary", "false")
	for _, p := range paths {
		params.Add("paths", p)
	}

	path := "/Library/VirtualFolders?" + params.Encode()

	var body interface{}
	if libraryOptions != nil {
		body = libraryOptions
	}

	if err := c.post(path, body); err != nil {
		return fmt.Errorf("adding virtual folder %s: %w", name, err)
	}
	return nil
}

// RemoveVirtualFolder removes a virtual folder (library) by name.
func (c *Client) RemoveVirtualFolder(name string) error {
	params := url.Values{}
	params.Set("name", name)
	params.Set("refreshLibrary", "false")

	path := "/Library/VirtualFolders?" + params.Encode()

	if err := c.delete(path); err != nil {
		return fmt.Errorf("removing virtual folder %s: %w", name, err)
	}
	return nil
}

// UpdateVirtualFolder updates the library options for a virtual folder.
func (c *Client) UpdateVirtualFolder(name string, libraryOptions *LibraryOptions) error {
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

	if err := c.postRaw("/Library/VirtualFolders/LibraryOptions", string(body)); err != nil {
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
