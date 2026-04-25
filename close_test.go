package spanemuboost

import (
	"errors"
	"sync"
	"testing"
)

func TestCloseStateConcurrentCalls(t *testing.T) {
	var (
		state closeState
		wg    sync.WaitGroup
		errs  = make([]error, 2)
	)

	wantErr := errors.New("close failed")
	started := make(chan struct{})
	release := make(chan struct{})
	calls := 0

	closeFn := func() error {
		calls++
		close(started)
		<-release
		return wantErr
	}

	wg.Add(2)
	go func() {
		defer wg.Done()
		errs[0] = state.close(closeFn)
	}()
	<-started
	go func() {
		defer wg.Done()
		errs[1] = state.close(closeFn)
	}()

	close(release)
	wg.Wait()

	if calls != 1 {
		t.Fatalf("close calls = %d, want 1", calls)
	}
	for i, err := range errs {
		if !errors.Is(err, wantErr) {
			t.Fatalf("errs[%d] = %v, want %v", i, err, wantErr)
		}
	}
}
