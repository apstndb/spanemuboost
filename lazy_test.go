package spanemuboost

import (
	"context"
	"sync/atomic"
	"testing"

	"google.golang.org/api/option"
)

func TestLazyEmulatorCloseAfterFailedGet(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	lazy := NewLazyEmulator()
	if _, err := lazy.Get(ctx); err == nil {
		t.Fatal("Get() error = nil, want non-nil")
	}

	if err := lazy.Close(); err != nil {
		t.Fatalf("Close() after failed Get() error = %v, want nil", err)
	}
}

func TestLazyCloseNilReceivers(t *testing.T) {
	var lazyRuntime *LazyRuntime
	if err := lazyRuntime.Close(); err != nil {
		t.Fatalf("(*LazyRuntime)(nil).Close() error = %v, want nil", err)
	}

	var lazyEmulator *LazyEmulator
	if err := lazyEmulator.Close(); err != nil {
		t.Fatalf("(*LazyEmulator)(nil).Close() error = %v, want nil", err)
	}
}

func TestLazyRuntimeStateGetRepanicsAfterStartPanic(t *testing.T) {
	var state lazyRuntimeState
	const want = "boom"

	assertPanic := func(call int) {
		t.Helper()

		defer func() {
			if got := recover(); got != want {
				t.Fatalf("call %d: panic = %v, want %q", call, got, want)
			}
		}()

		_, _ = state.get(t.Context(), func(context.Context) (runtimeInstance, error) {
			panic(want)
		}, "unused")
		t.Fatalf("call %d: get() returned normally, want panic", call)
	}

	assertPanic(1)
	assertPanic(2)
}

func TestLazyRuntimeStateCloseAfterStartPanic(t *testing.T) {
	var state lazyRuntimeState
	const want = "boom"

	assertPanic := func(call string, start func(context.Context) (runtimeInstance, error)) {
		t.Helper()

		defer func() {
			if got := recover(); got != want {
				t.Fatalf("%s: panic = %v, want %q", call, got, want)
			}
		}()

		_, _ = state.get(t.Context(), start, "unused")
		t.Fatalf("%s: get() returned normally, want panic", call)
	}

	assertPanic("first get", func(context.Context) (runtimeInstance, error) {
		panic(want)
	})
	if err := state.close(); err != nil {
		t.Fatalf("Close() after panicking Get() error = %v, want nil", err)
	}
	assertPanic("second get", func(context.Context) (runtimeInstance, error) {
		t.Fatal("start function called after initial panic")
		return nil, nil
	})
}

func TestLazyRuntimeStateGetRepanicsAfterNilPanic(t *testing.T) {
	var state lazyRuntimeState

	assertPanics := func(call int) {
		t.Helper()

		panicking := true
		defer func() {
			_ = recover()
			if !panicking {
				t.Fatalf("call %d: get() returned normally, want panic", call)
			}
		}()

		_, _ = state.get(t.Context(), func(context.Context) (runtimeInstance, error) {
			panic(nil)
		}, "unused")
		panicking = false
	}

	assertPanics(1)
	assertPanics(2)
}

func TestLazyRuntimeStateCloseBeforeGetPreventsStart(t *testing.T) {
	var state lazyRuntimeState
	var starts atomic.Int32

	if err := state.close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}

	runtime, err := state.get(t.Context(), func(context.Context) (runtimeInstance, error) {
		starts.Add(1)
		return &fakeRuntimeInstance{}, nil
	}, "closed before start")
	if err == nil {
		t.Fatal("get() error = nil, want non-nil")
	}
	if got := err.Error(); got != "closed before start" {
		t.Fatalf("get() error = %q, want %q", got, "closed before start")
	}
	if runtime != nil {
		t.Fatalf("get() runtime = %T, want nil", runtime)
	}
	if got := starts.Load(); got != 0 {
		t.Fatalf("start calls = %d, want 0", got)
	}
}

func TestLazyRuntimeStateConcurrentGetAndCloseGetWins(t *testing.T) {
	var state lazyRuntimeState
	runtime := &fakeRuntimeInstance{}
	started := make(chan struct{})
	releaseStart := make(chan struct{})

	defer func() {
		select {
		case <-releaseStart:
		default:
			close(releaseStart)
		}
	}()

	type getResult struct {
		runtime runtimeInstance
		err     error
	}
	ctx := t.Context()
	getDone := make(chan getResult, 1)
	go func() {
		got, err := state.get(ctx, func(context.Context) (runtimeInstance, error) {
			close(started)
			<-releaseStart
			return runtime, nil
		}, "unused")
		getDone <- getResult{runtime: got, err: err}
	}()

	<-started

	closeDone := make(chan error, 1)
	go func() {
		closeDone <- state.close()
	}()

	select {
	case err := <-closeDone:
		t.Fatalf("Close() returned before in-progress Get() finished: %v", err)
	default:
	}

	close(releaseStart)

	result := <-getDone
	if result.err != nil {
		t.Fatalf("get() error = %v, want nil", result.err)
	}
	if result.runtime != runtime {
		t.Fatalf("get() runtime = %T, want %T", result.runtime, runtime)
	}
	if err := <-closeDone; err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
	if got := runtime.closeCalls.Load(); got != 1 {
		t.Fatalf("runtime Close() calls = %d, want 1", got)
	}
}

type fakeRuntimeInstance struct {
	closeCalls atomic.Int32
}

func (*fakeRuntimeInstance) spanemuboostRuntime() {}

func (*fakeRuntimeInstance) URI() string { return "localhost:9010" }

func (*fakeRuntimeInstance) ClientOptions() []option.ClientOption { return nil }

func (r *fakeRuntimeInstance) Close() error {
	r.closeCalls.Add(1)
	return nil
}

func (*fakeRuntimeInstance) ProjectID() string { return "project" }

func (*fakeRuntimeInstance) InstanceID() string { return "instance" }

func (*fakeRuntimeInstance) DatabaseID() string { return "database" }

func (*fakeRuntimeInstance) ProjectPath() string { return "projects/project" }

func (*fakeRuntimeInstance) InstancePath() string {
	return "projects/project/instances/instance"
}

func (*fakeRuntimeInstance) DatabasePath() string {
	return "projects/project/instances/instance/databases/database"
}

func (*fakeRuntimeInstance) inheritedOptions(...Option) (*emulatorOptions, error) {
	return &emulatorOptions{}, nil
}

func (*fakeRuntimeInstance) runtimePlatform(context.Context) (string, error) {
	return "fake", nil
}
