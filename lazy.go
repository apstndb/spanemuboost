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
		defer func() {
			if r := recover(); r != nil {
				s.panicked = true
				s.panicValue = r
				panic(r)
			}
		}()
		s.runtime, s.err = start(ctx)
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
type LazyRuntime struct {
	state   lazyRuntimeState
	backend Backend
	opts    []Option
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
// and returns the cached [Runtime] on subsequent calls.
func (lr *LazyRuntime) Get(ctx context.Context) (Runtime, error) {
	runtime, err := lr.get(ctx)
	if err != nil {
		return nil, err
	}
	return runtime, nil
}

// Close terminates the runtime if it was started. No-op otherwise.
// Close is idempotent — subsequent calls return the result of the first call.
// Close waits for any in-progress initialization to complete before checking.
// If Close is called before any Get or Setup, the runtime will never be started.
func (lr *LazyRuntime) Close() error {
	return lr.state.close()
}
