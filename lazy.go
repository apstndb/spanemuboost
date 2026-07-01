package spanemuboost

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type lazyRuntimeState struct {
	once    sync.Once
	runtime runtimeInstance
	err     error

	panicked   bool
	panicValue any

	closeOnce sync.Once
	closeErr  error
}

func (s *lazyRuntimeState) get(ctx context.Context, start func(context.Context) (runtimeInstance, error), missingMessage string) (runtimeInstance, error) {
	s.once.Do(func() {
		panicking := true
		defer func() {
			// Track whether start panicked separately so panic(nil) is not
			// mistaken for a normal return and later translated into missingMessage.
			if panicking {
				s.panicked = true
				s.panicValue = recover()
				panic(s.panicValue)
			}
		}()
		s.runtime, s.err = start(ctx)
		panicking = false
	})
	if s.panicked {
		panic(s.panicValue)
	}
	if (s.runtime == nil || isNilRuntimeValue(s.runtime)) && s.err == nil {
		return nil, errors.New(missingMessage)
	}
	if s.runtime == nil || isNilRuntimeValue(s.runtime) {
		return nil, s.err
	}
	return s.runtime, s.err
}

func (s *lazyRuntimeState) close() error {
	s.once.Do(func() {})
	s.closeOnce.Do(func() {
		if s.runtime != nil && !isNilRuntimeValue(s.runtime) {
			s.closeErr = s.runtime.Close()
		}
	})
	return s.closeErr
}

// LazyRuntime defers startup of the selected backend until first use.
// Use [NewLazyRuntime] in a package-level var, then pass it directly to
// [SetupClients] or [OpenClients]. Call [LazyRuntime.Setup] or [LazyRuntime.Get]
// for standalone access, and pair it with [LazyRuntime.Close] or [LazyRuntime.TestMain]
// when you need the lazy handle to own lifecycle cleanup. [LazyRuntime.Close] is
// safe to call even if the runtime was never started (no-op).
//
// Concurrent first use and cleanup are serialized. If [LazyRuntime.Get] or
// [LazyRuntime.Setup] starts initialization before [LazyRuntime.Close], Close
// waits for initialization to finish and then closes the started runtime; that
// concurrent first-use call may still return the runtime that Close is closing
// or has closed. If Close runs before initialization starts, the runtime is
// never started and later first-use calls fail. Callers should coordinate so
// returned runtimes are not used after Close begins.
type LazyRuntime struct {
	state   lazyRuntimeState
	backend Backend
	opts    []Option

	attachedEndpoint *Endpoint
}

func (*LazyRuntime) spanemuboostRuntime() {}

// NewLazyRuntime creates a [LazyRuntime] that will start the selected backend
// with the given options on first use.
func NewLazyRuntime(backend Backend, options ...Option) *LazyRuntime {
	return &LazyRuntime{
		backend: backend,
		opts:    options,
	}
}

func (lr *LazyRuntime) get(ctx context.Context) (runtimeInstance, error) {
	return lr.state.get(ctx, func(ctx context.Context) (runtimeInstance, error) {
		if lr.attachedEndpoint != nil {
			return NewAttachedRuntime(*lr.attachedEndpoint, lr.opts...)
		}
		runtime, err := Run(ctx, lr.backend, lr.opts...)
		if err != nil {
			return nil, err
		}
		instance, ok := runtime.(runtimeInstance)
		if !ok {
			return nil, fmt.Errorf("spanemuboost: lazy runtime backend %q returned unexpected runtime type %T", lr.backend, runtime)
		}
		return instance, nil
	}, "spanemuboost: lazy runtime used after Close was called before initialization")
}

// Get starts the selected backend on first call (thread-safe via [sync.Once])
// and returns the cached [Runtime] on subsequent calls. It may run concurrently
// with [LazyRuntime.Close]; see [LazyRuntime] for the lifecycle invariant.
func (lr *LazyRuntime) Get(ctx context.Context) (Runtime, error) {
	runtime, err := lr.get(ctx)
	if err != nil {
		return nil, err
	}
	return runtime, nil
}

// Close terminates the runtime if it was started. No-op otherwise.
// Close is nil-safe and idempotent — subsequent calls return the result of the first call.
// Close waits for any in-progress initialization to complete before checking.
// If Close is called before any Get or Setup, the runtime will never be started.
// If the first Get or Setup is already initializing, Close closes the runtime
// after initialization finishes; that concurrent first-use call may still
// return the runtime.
func (lr *LazyRuntime) Close() error {
	if lr == nil {
		return nil
	}
	return lr.state.close()
}
