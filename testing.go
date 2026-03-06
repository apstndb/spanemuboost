package spanemuboost

import (
	"context"
	"testing"
)

// SetupEmulator starts a Cloud Spanner Emulator and registers cleanup via [testing.TB.Cleanup].
// It calls [testing.TB.Fatal] on setup error.
// Use [RunEmulator] if you need a [context.Context] or are not in a test.
func SetupEmulator(tb testing.TB, options ...Option) *Emulator {
	tb.Helper()

	emu, err := RunEmulator(context.Background(), options...)
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

// SetupEmulatorWithClients starts a Cloud Spanner Emulator with clients and registers
// cleanup via [testing.TB.Cleanup]. It calls [testing.TB.Fatal] on setup error.
// Use [RunEmulatorWithClients] if you need a [context.Context] or are not in a test.
func SetupEmulatorWithClients(tb testing.TB, options ...Option) *Env {
	tb.Helper()

	env, err := RunEmulatorWithClients(context.Background(), options...)
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

// SetupClients opens Spanner clients against an existing [Emulator] and registers
// cleanup via [testing.TB.Cleanup]. It calls [testing.TB.Fatal] on setup error.
// Use [OpenClients] if you need a [context.Context] or are not in a test.
func SetupClients(tb testing.TB, emu *Emulator, options ...Option) *Clients {
	tb.Helper()

	clients, err := OpenClients(context.Background(), emu, options...)
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
