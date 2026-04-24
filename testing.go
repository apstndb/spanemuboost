package spanemuboost

import (
	"log"
	"os"
	"testing"
)

// runTestMain runs tests, closes the emulator, logs any close error, and exits.
func runTestMain(m *testing.M, close func() error) {
	code := m.Run()
	if err := close(); err != nil {
		log.Printf("spanemuboost: failed to close: %v", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

// TestMain runs m.Run(), closes the emulator, and calls os.Exit with the
// appropriate code. A close failure is logged and causes a non-zero exit code.
//
// Because TestMain calls os.Exit, it must be the last statement in your
// TestMain function. If you need additional cleanup, refer to the
// source of this method and write the logic manually.
//
// Usage in TestMain:
//
//	func TestMain(m *testing.M) {
//	    var err error
//	    emulator, err = spanemuboost.RunEmulator(context.Background(),
//	        spanemuboost.EnableInstanceAutoConfigOnly(),
//	    )
//	    if err != nil { log.Fatal(err) }
//	    emulator.TestMain(m)
//	}
func (e *Emulator) TestMain(m *testing.M) {
	runTestMain(m, e.Close)
}

// TestMain runs m.Run(), closes the lazy emulator, and calls os.Exit with the
// appropriate code. A close failure is logged and causes a non-zero exit code.
// If the emulator was never started, Close is a no-op.
//
// Because TestMain calls os.Exit, it must be the last statement in your
// TestMain function. If you need additional cleanup, refer to the
// source of this method and write the logic manually.
//
// Usage in TestMain:
//
//	var lazyEmu = spanemuboost.NewLazyEmulator(spanemuboost.EnableInstanceAutoConfigOnly())
//
//	func TestMain(m *testing.M) { lazyEmu.TestMain(m) }
func (le *LazyEmulator) TestMain(m *testing.M) {
	runTestMain(m, le.Close)
}

// Setup starts the emulator on first call (thread-safe via [sync.Once]) and
// returns the cached [*Emulator] on subsequent calls.
// It calls [testing.TB.Fatal] if startup fails.
// For use with [SetupClients] or [OpenClients], you can pass [*LazyEmulator] directly
// without calling Setup.
func (le *LazyEmulator) Setup(tb testing.TB) *Emulator {
	tb.Helper()
	emu, err := le.Get(tb.Context())
	if err != nil {
		tb.Fatal(err)
	}
	return emu
}

// SetupEmulator starts a Cloud Spanner Emulator and registers cleanup via [testing.TB.Cleanup].
// It calls [testing.TB.Fatal] on setup error.
// Use [RunEmulator] if you need a [context.Context] or are not in a test.
// Note that [testing.M] does not implement [testing.TB], so use [RunEmulator] in TestMain.
func SetupEmulator(tb testing.TB, options ...Option) *Emulator {
	tb.Helper()

	emu, err := RunEmulator(tb.Context(), options...)
	if err != nil {
		tb.Fatal(err)
	}

	tb.Cleanup(func() {
		if err := emu.Close(); err != nil {
			tb.Errorf("spanemuboost: failed to close emulator: %v", err)
		}
	})

	return emu
}

func setupOmni(tb testing.TB, options ...Option) Runtime {
	tb.Helper()

	runtime, err := runOmni(tb.Context(), options...)
	if err != nil {
		tb.Fatal(err)
	}

	tb.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			tb.Errorf("spanemuboost: failed to close omni: %v", err)
		}
	})

	return runtime
}

func setupOmniWithClients(tb testing.TB, options ...Option) *RuntimeEnv {
	tb.Helper()

	env, err := runOmniWithClients(tb.Context(), options...)
	if err != nil {
		tb.Fatal(err)
	}

	tb.Cleanup(func() {
		if err := env.Close(); err != nil {
			tb.Errorf("spanemuboost: failed to close omni env: %v", err)
		}
	})

	return env
}

// SetupEmulatorWithClients starts a Cloud Spanner Emulator with clients and registers
// cleanup via [testing.TB.Cleanup]. It calls [testing.TB.Fatal] on setup error.
// Use [RunEmulatorWithClients] if you need a [context.Context] or are not in a test.
func SetupEmulatorWithClients(tb testing.TB, options ...Option) *Env {
	tb.Helper()

	env, err := RunEmulatorWithClients(tb.Context(), options...)
	if err != nil {
		tb.Fatal(err)
	}

	tb.Cleanup(func() {
		if err := env.Close(); err != nil {
			tb.Errorf("spanemuboost: failed to close env: %v", err)
		}
	})

	return env
}

// SetupClients opens Spanner clients against an existing runtime and registers
// cleanup via [testing.TB.Cleanup]. It calls [testing.TB.Fatal] on setup error.
// The runtime parameter accepts [*Emulator], [*LazyEmulator], and the [Runtime]
// returned by [Run] or [Setup] with [BackendOmni].
// When a [*LazyEmulator] is passed, the emulator is started automatically on first use.
// Use [OpenClients] if you need a [context.Context] or are not in a test.
func SetupClients(tb testing.TB, runtime abstractRuntime, options ...Option) *Clients {
	tb.Helper()

	clients, err := OpenClients(tb.Context(), runtime, options...)
	if err != nil {
		tb.Fatal(err)
	}

	tb.Cleanup(func() {
		if err := clients.Close(); err != nil {
			tb.Errorf("spanemuboost: failed to close clients: %v", err)
		}
	})

	return clients
}
