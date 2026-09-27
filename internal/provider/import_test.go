// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"
)

func TestUnitReadForImportRefusesResourcesWithoutABinding(t *testing.T) {
	for _, resourceType := range []string{"jellyfin_api_key", "jellyfin_no_such_resource"} {
		if _, _, err := ReadForImport(context.Background(), resourceType, "id", "{}"); err == nil {
			t.Errorf("ReadForImport(%s) returned no error", resourceType)
		}
	}
}

func TestUnitReadForImportStartsFromWhatImportStateSets(t *testing.T) {
	_, got, err := ReadForImport(context.Background(), "jellyfin_scheduled_task", "task-id", `{"Id":"served-id"}`)
	if err != nil {
		t.Fatal(err)
	}
	if v := got.Attributes()["task_id"].String(); v != `"task-id"` {
		t.Errorf("task_id = %s, want the import ID", v)
	}
}
