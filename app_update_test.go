package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"ssh-tunnel-manager/internal/prefs"
	"ssh-tunnel-manager/internal/updater"
)

func TestUpdateResultFromPreviousChannelIsDiscarded(t *testing.T) {
	prefsStore, err := prefs.NewStoreWithPath(filepath.Join(t.TempDir(), "prefs.json"))
	if err != nil {
		t.Fatalf("creating prefs store: %v", err)
	}

	requestStarted := make(chan struct{})
	finishRequest := make(chan struct{})
	app := &App{
		prefs:         prefsStore,
		updateChannel: updater.ChannelStable,
		updateChecker: func(_ context.Context, _, _ string, channel updater.Channel) (*updater.UpdateInfo, error) {
			if channel != updater.ChannelStable {
				t.Errorf("request channel = %q, want stable", channel)
			}
			close(requestStarted)
			<-finishRequest
			return &updater.UpdateInfo{LatestVersion: "2.0.0"}, nil
		},
	}

	type result struct {
		info *updater.UpdateInfo
		err  error
	}
	resultCh := make(chan result, 1)
	go func() {
		info, err := app.checkForUpdate(context.Background())
		resultCh <- result{info: info, err: err}
	}()

	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("update request did not start")
	}

	if err := app.SetUpdateChannel("beta"); err != nil {
		t.Fatalf("SetUpdateChannel: %v", err)
	}
	close(finishRequest)

	select {
	case got := <-resultCh:
		if got.err != nil || got.info != nil {
			t.Fatalf("stale check = %#v, %v; want nil, nil", got.info, got.err)
		}
	case <-time.After(time.Second):
		t.Fatal("update request did not finish")
	}

	if app.pendingUpdate != nil {
		t.Fatalf("pending update = %#v, want nil", app.pendingUpdate)
	}
	if got := app.GetUpdateChannel(); got != "beta" {
		t.Fatalf("channel = %q, want beta", got)
	}
	if got := prefsStore.Get().UpdateChannel; got != "beta" {
		t.Fatalf("persisted channel = %q, want beta", got)
	}
}

func TestSetUpdateChannelRejectsUnknownValue(t *testing.T) {
	app := &App{updateChannel: updater.ChannelStable}
	if err := app.SetUpdateChannel("nightly"); err == nil {
		t.Fatal("SetUpdateChannel(nightly) should return an error")
	}
	if got := app.GetUpdateChannel(); got != "stable" {
		t.Fatalf("channel = %q, want stable", got)
	}
}
