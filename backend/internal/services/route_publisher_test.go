package services

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestRouteLookupDoesNotWaitForPublication(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routes.json")
	if err := os.WriteFile(path, []byte(`{"rules":[{"id":1,"pattern":"example.com","type":"exact","exit_id":7,"enabled":true}],"exits":[{"id":7,"name":"west","enabled":true}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	publisher, err := NewRoutePublisher(path)
	if err != nil {
		t.Fatal(err)
	}
	publisher.mu.Lock()
	result := make(chan string, 1)
	go func() { result <- publisher.ExitName("example.com") }()
	select {
	case got := <-result:
		if got != "west" {
			t.Fatalf("exit = %q", got)
		}
	case <-time.After(time.Second):
		publisher.mu.Unlock()
		t.Fatal("route lookup waited for publication lock")
	}
	publisher.mu.Unlock()
}
