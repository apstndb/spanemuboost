package spanemuboost

import (
	"context"
	"testing"
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
