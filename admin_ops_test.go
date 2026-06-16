package spanemuboost

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestWithInstanceAdminLockCancelsWhileWaiting(t *testing.T) {
	const instancePath = "projects/test/instances/test"

	acquired := make(chan struct{})
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := withInstanceAdminLock(context.Background(), instancePath, func() {
			close(acquired)
			<-release
		}); err != nil {
			t.Errorf("holder lock: %v", err)
		}
	}()
	<-acquired

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer waitCancel()
	err := withInstanceAdminLock(waitCtx, instancePath, func() {})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait err = %v, want %v", err, context.DeadlineExceeded)
	}

	close(release)
	wg.Wait()
}

func TestDropDatabaseRetryBudget(t *testing.T) {
	want := dropDatabaseTimeout*time.Duration(dropDatabaseMaxAttempts) + dropDatabaseRetryBackoff
	if got := dropDatabaseRetryBudget(); got != want {
		t.Fatalf("dropDatabaseRetryBudget() = %v, want %v", got, want)
	}
	if got := dropDatabaseRetryBudget(); got <= closeTimeout {
		t.Fatalf("drop retry budget %v must exceed close timeout %v", got, closeTimeout)
	}
}

func TestDropDatabaseComplete(t *testing.T) {
	if !dropDatabaseComplete(nil) {
		t.Fatal("dropDatabaseComplete(nil) = false, want true")
	}
	if !dropDatabaseComplete(status.Error(codes.NotFound, "missing")) {
		t.Fatal("dropDatabaseComplete(NotFound) = false, want true")
	}
	if dropDatabaseComplete(status.Error(codes.Unavailable, "retry")) {
		t.Fatal("dropDatabaseComplete(Unavailable) = true, want false")
	}
}
