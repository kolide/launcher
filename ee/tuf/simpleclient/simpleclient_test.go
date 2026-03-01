package simpleclient

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/kolide/launcher/pkg/log/multislogger"
)

func TestDownload(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	data, err := Download(ctx, multislogger.NewNopLogger(), "launcher", runtime.GOOS, runtime.GOARCH, "stable", nil)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("expected non-empty data")
	}

	t.Logf("downloaded %d bytes", len(data))
}

func TestListTargets(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	targets, err := ListTargets(ctx, multislogger.NewNopLogger(), nil)
	if err != nil {
		t.Fatalf("ListTargets: %v", err)
	}

	if len(targets) == 0 {
		t.Fatal("expected at least one target")
	}

	found := false
	for _, name := range targets {
		if name == "launcher" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'launcher' in targets, got: %v", targets)
	}

	t.Logf("targets: %v", targets)
}
