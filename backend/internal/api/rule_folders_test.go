package api

import (
	"encoding/json"
	"strings"
	"testing"

	"sni-gateway/internal/models"
)

func TestRuleFolderNameValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		want string
	}{
		{"  工作  ", "工作"},
		{"\t ", ""},
		{"work\nprojects", ""},
		{strings.Repeat("組", 101), ""},
	} {
		body, err := json.Marshal(map[string]string{"name": tc.name})
		if err != nil {
			t.Fatal(err)
		}
		name, err := decodeRuleFolderName(body)
		if name != tc.want || (err == nil) != (tc.want != "") {
			t.Errorf("decodeRuleFolderName(%q) = %q, %v; want %q", tc.name, name, err, tc.want)
		}
	}
}

func TestFolderMoveDoesNotChangeRoutingFields(t *testing.T) {
	exitID, sameExitID, folderID := uint(4), uint(4), uint(5)
	before := models.Rule{Pattern: "example.com", Type: "exact", Priority: 100, Enabled: true, ExitID: &exitID}
	after := before
	after.ExitID = &sameExitID
	after.FolderID = &folderID
	if changes := changedRoutingFields(before, after); len(changes) != 0 {
		t.Fatalf("folder move changed routing fields: %v", changes)
	}
	after.Enabled = false
	after.ExitID = nil
	changes := changedRoutingFields(before, after)
	if len(changes) != 2 || changes["exit_id"] != (*uint)(nil) || changes["enabled"] != false {
		t.Fatalf("routing edit did not retain explicit cleared exit/disabled state: %v", changes)
	}
}
