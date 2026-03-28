// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// ScheduledTask represents a Jellyfin scheduled task.
type ScheduledTask struct {
	Name        string            `json:"Name"`
	State       string            `json:"State"`
	Id          string            `json:"Id"`
	Description string            `json:"Description"`
	Category    string            `json:"Category"`
	IsHidden    bool              `json:"IsHidden"`
	Key         string            `json:"Key"`
	Triggers    []json.RawMessage `json:"Triggers"`
}

// GetScheduledTasks retrieves all scheduled tasks.
func (c *Client) GetScheduledTasks() ([]ScheduledTask, error) {
	var tasks []ScheduledTask
	if err := c.get("/ScheduledTasks", &tasks); err != nil {
		return nil, fmt.Errorf("getting scheduled tasks: %w", err)
	}
	return tasks, nil
}

// GetScheduledTask retrieves a single scheduled task by ID.
func (c *Client) GetScheduledTask(id string) (*ScheduledTask, error) {
	var task ScheduledTask
	if err := c.get(fmt.Sprintf("/ScheduledTasks/%s", url.PathEscape(id)), &task); err != nil {
		return nil, fmt.Errorf("getting scheduled task %s: %w", id, err)
	}
	return &task, nil
}

// UpdateScheduledTaskTriggers updates the triggers for a scheduled task.
func (c *Client) UpdateScheduledTaskTriggers(id string, triggersJSON string) error {
	if err := c.postRaw(fmt.Sprintf("/ScheduledTasks/%s/Triggers", url.PathEscape(id)), triggersJSON); err != nil {
		return fmt.Errorf("updating triggers for task %s: %w", id, err)
	}
	return nil
}
