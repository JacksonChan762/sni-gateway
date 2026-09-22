package services

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type fakeStopper struct {
	names  []string
	failed map[string]bool
}

func (f *fakeStopper) StopContainer(ctx context.Context, name string) error {
	f.names = append(f.names, name)
	if f.failed[name] {
		return errors.New("stop failed")
	}
	return ctx.Err()
}
func TestStopAttemptsBothContainersAndReportsFailures(t *testing.T) {
	for _, failed := range []map[string]bool{nil, {"relay-a": true}, {"gluetun-a": true}, {"relay-a": true, "gluetun-a": true}} {
		fake := &fakeStopper{failed: failed}
		err := stopExitContainers(context.Background(), fake, "a")
		if !reflect.DeepEqual(fake.names, []string{"relay-a", "gluetun-a"}) {
			t.Fatalf("calls: %v", fake.names)
		}
		if (err != nil) != (len(failed) > 0) {
			t.Fatalf("unexpected error %v", err)
		}
		for name := range failed {
			if !strings.Contains(err.Error(), name) {
				t.Fatalf("lost failure for %s", name)
			}
		}
	}
}
func TestStopHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := stopExitContainers(ctx, &fakeStopper{}, "a")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

type fakeRemover struct {
	fakeStopper
	force bool
}

func (f *fakeRemover) RemoveContainer(ctx context.Context, name string, force bool) error {
	f.force = force
	return f.StopContainer(ctx, name)
}
func TestRemoveAttemptsBothContainers(t *testing.T) {
	client := &fakeRemover{fakeStopper: fakeStopper{failed: map[string]bool{"relay-a": true}}}
	err := removeExitContainers(context.Background(), client, "a")
	if err == nil || !client.force || !reflect.DeepEqual(client.names, []string{"relay-a", "gluetun-a"}) {
		t.Fatalf("removal: calls %v, error %v", client.names, err)
	}
}
