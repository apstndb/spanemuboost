# AGENTS

## Purpose

`spanemuboost` is still primarily an emulator-focused package, but its runtime
layer is now backend-neutral. The public architecture supports both:

- **Cloud Spanner Emulator** as the default, stable path
- **Spanner Omni** as an **experimental** backend

Keep that framing in docs and code changes: emulator-first, Omni-experimental,
shared runtime/client abstractions underneath.

## Public architecture

### Backend-neutral runtime layer

These are the preferred architecture entry points:

- `BackendEmulator`, `BackendOmni`
- `Runtime`
- `Run`, `RunWithClients`
- `Setup`, `SetupWithClients`
- `OpenClients`, `SetupClients`
- `NewLazyRuntime`

`Runtime` is a started backend instance with connection metadata (`URI`,
`ClientOptions`, resource paths) and lifecycle (`Close`).

`OpenClients` and `SetupClients` intentionally accept package-provided runtime
values only: started runtimes, `*LazyRuntime`, and `*LazyEmulator`.

### Emulator compatibility layer

These remain first-class and should stay easy to use:

- `Emulator`, `Env`
- `RunEmulator`, `RunEmulatorWithClients`
- `SetupEmulator`, `SetupEmulatorWithClients`
- `NewLazyEmulator`

`NewLazyEmulator` is a backward-compatible wrapper. Do not remove it just
because `NewLazyRuntime(BackendEmulator, ...)` exists.

### Omni architecture

Omni does **not** add separate exported startup helpers. It plugs into the
backend-neutral surface instead:

- `Run(..., BackendOmni, ...)`
- `RunWithClients(..., BackendOmni, ...)`
- `Setup(..., BackendOmni, ...)`
- `SetupWithClients(..., BackendOmni, ...)`

Important Omni constraints:

- use the public Spanner gRPC endpoint on port `15000`
- keep Omni positioned as experimental
- keep backend guardrails unless a caller explicitly disables them
- use `RecommendedOmniClientConfig()` as the base for external Go clients

## Lifecycle and teardown rules

- `Close()` paths are expected to be **nil-safe** and **idempotent**
- `Setup*` helpers own cleanup via `t.Cleanup`
- `TestMain` helpers own cleanup via `os.Exit` wrappers
- fixed IDs default to schema teardown on client close
- random IDs do not default to teardown
- runtime-owned environments (`RunEmulatorWithClients`, `SetupEmulatorWithClients`,
  Omni runtime envs) disable schema teardown unless explicitly forced

Current close semantics use a shared internal `closeState` helper. For exported
types, it is kept pointer-backed to avoid exposing `sync.Once` copylock fields
in the public struct layout.

## Bootstrap and rollback rules

- bootstrap is shared across backends where possible
- `OpenClients` inherits runtime IDs and reopen semantics from the started runtime
- partial bootstrap failures must not leak created schema resources
- rollback logic is intentionally explicit about instance-vs-database teardown

When touching bootstrap behavior, read `internal.go` first.

## Lazy runtime rules

`LazyRuntime` is the backend-neutral lazy entry point.

Preserve these behaviors:

- startup runs once
- startup panics are re-surfaced on later `Get`
- typed-nil runtimes are treated as nil
- `Close` is safe before initialization and after repeated calls

## Key files

- `runtime.go`: backend-neutral runtime API and runtime resolution
- `spanemuboost.go`: clients, emulator-facing compatibility API
- `testing.go`: `Setup*` and `TestMain` helpers
- `lazy.go`: lazy runtime and lazy emulator behavior
- `omni.go`: Omni runtime, client config, guardrails
- `internal.go`: shared bootstrap, rollback, client construction
- `close.go`: shared close-state helper

## Documentation guidance

When updating docs:

- keep README emulator-centered in tone
- describe backend-neutral APIs accurately
- describe Omni as experimental, not parity-complete
- do not imply separate exported Omni helper families that do not exist
- mention `SPANEMUBOOST_ENABLE_OMNI_TESTS=1` for opt-in Omni test coverage

## Validation

For code changes, the usual local checks are:

```sh
TESTCONTAINERS_RYUK_DISABLED=true go test -race ./...
golangci-lint run
git diff --check
```
