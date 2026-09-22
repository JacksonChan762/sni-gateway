package services

import (
	"net/http"
	"net/http/httptest"
	"sni-gateway/internal/models"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFetchSubscription(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		body    string
		wantErr bool
	}{
		{"valid", 200, "example.com\n", false},
		{"http error", 503, "unavailable", true},
		{"at limit", 200, strings.Repeat("a", maxSubscriptionBytes), false},
		{"over limit", 200, strings.Repeat("a", maxSubscriptionBytes+1), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			body, err := fetchSubscription(NewSubscriptionManager(nil).client, server.URL)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
			}
			if !tc.wantErr && string(body) != tc.body {
				t.Fatal("download was truncated")
			}
		})
	}
}

func TestFetchSubscriptionTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	client := &http.Client{Timeout: 50 * time.Millisecond}
	if _, err := fetchSubscription(client, server.URL); err == nil {
		t.Fatal("expected timeout")
	}
}

func TestSubscriptionSchedulesConcurrentUpdates(t *testing.T) {
	manager := NewSubscriptionManager(nil)
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			manager.Schedule(models.Subscription{ID: 1, Enabled: true, IntervalMinutes: 5})
			manager.Unschedule(1)
		}()
	}
	wg.Wait()
	manager.Schedule(models.Subscription{ID: 1, Enabled: true, IntervalMinutes: 5})
	if len(manager.cron.Entries()) != 1 {
		t.Fatal("expected exactly one scheduled job")
	}
	manager.Schedule(models.Subscription{ID: 1, Enabled: false, IntervalMinutes: 5})
	manager.Schedule(models.Subscription{ID: 2, Enabled: true, IntervalMinutes: -1})
	if len(manager.cron.Entries()) != 0 {
		t.Fatal("disabled or invalid subscription was scheduled")
	}
}
