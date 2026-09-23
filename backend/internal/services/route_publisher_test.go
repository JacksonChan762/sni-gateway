package services

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRoutePublisherLoadsOnlyPublishedSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "routes.json")
	if err := os.WriteFile(path, []byte(`{"rules":[{"id":1,"pattern":"example.com","type":"exact","exit_id":7,"enabled":true}],"exits":[{"id":7,"name":"west","enabled":true}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	publisher, err := NewRoutePublisher(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := publisher.ExitName("EXAMPLE.COM"); got != "west" {
		t.Fatalf("exit = %q", got)
	}
	if got := publisher.ExitName("other.example.com"); got != "" {
		t.Fatalf("unexpected exit %q", got)
	}
}
