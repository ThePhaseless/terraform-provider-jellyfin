// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strings"
	"testing"
)

func TestUnitReadForImportRefusesResourcesWithoutABinding(t *testing.T) {
	_, _, err := ReadForImport(context.Background(), nil, "jellyfin_api_key", "id", "{}")
	if err == nil || !strings.Contains(err.Error(), "does not import a Jellyfin JSON document") {
		t.Errorf("ReadForImport(jellyfin_api_key) error = %v, want one saying it has no document to import", err)
	}
}

func TestUnitReadForImportRefusesUnknownResourceTypes(t *testing.T) {
	_, _, err := ReadForImport(context.Background(), nil, "jellyfin_no_such_resource", "id", "{}")
	if err == nil || !strings.Contains(err.Error(), "has no resource jellyfin_no_such_resource") {
		t.Errorf("ReadForImport(jellyfin_no_such_resource) error = %v, want one naming the unknown type", err)
	}
}

func TestUnitReadForImportStartsFromWhatImportStateSets(t *testing.T) {
	_, got, err := ReadForImport(context.Background(), nil, "jellyfin_scheduled_task", "task-id", `{"Id":"served-id"}`)
	if err != nil {
		t.Fatal(err)
	}
	if v := got.Attributes()["task_id"].String(); v != `"task-id"` {
		t.Errorf("task_id = %s, want the import ID", v)
	}
}
