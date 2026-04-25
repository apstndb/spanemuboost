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
- `RuntimeHandle`
- `Runtime`, `RuntimeEnv`
- `Run`, `RunWithClients`
- `Setup`, `SetupWithClients`
- `OpenClients`, `SetupClients`
- `NewLazyRuntime`

`RuntimeHandle` is the package-owned public handle type accepted by
`OpenClients` and `SetupClients`.

`Runtime` is a started backend instance with connection metadata (`URI`,
`ClientOptions`, resource paths) and lifecycle (`Close`). Treat this
backend-neutral surface as the stable API layer; treat Omni-specific behavior
as experimental.

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

- `Run(ctx, BackendOmni, ...)`
- `RunWithClients(ctx, BackendOmni, ...)`
- `Setup(tb, BackendOmni, ...)`
- `SetupWithClients(tb, BackendOmni, ...)`

Important Omni constraints:

- use the public Spanner gRPC endpoint on port `15000`
- keep Omni positioned as experimental
- keep backend guardrails unless a caller explicitly disables them
- use `RecommendedOmniClientConfig()` as the base for external Go clients

## Lifecycle and teardown rules

- `Close()` paths are expected to be **nil-safe** and **idempotent**
- `Setup*` helpers own cleanup via `tb.Cleanup`
- `TestMain` helpers own cleanup via `os.Exit` wrappers
- auto-created resources with fixed IDs default to schema teardown on client close
- random IDs do not default to teardown
- runtime-owned environments (`RunEmulatorWithClients`, `SetupEmulatorWithClients`,
  Omni runtime envs) disable schema teardown unless explicitly forced

Current close semantics use a shared internal `closeState` helper. For exported
types that use that helper, it is kept pointer-backed to avoid exposing
`sync.Once` copylock fields in the public struct layout. `LazyRuntime` and
`LazyEmulator` use a separate internal `lazyRuntimeState` by value because
their `sync.Once` fields are part of the lazy-start state, not the exported
close-state helper. Treat them as pointer-owned handles: create them with
`NewLazyRuntime(...)` or `NewLazyEmulator(...)` and pass the returned pointers
around rather than copying them by value.

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
- `spanemuboost.go`: `Clients`, `OpenClients`, and emulator-facing compatibility API
- `emulator.go`: `Emulator`, `Env`, `LazyEmulator`, and emulator-specific runtime logic
- `options.go`: `Option` and `With*` helpers for backend configuration
- `testing.go`: `Setup*` and `TestMain` helpers
- `lazy.go`: lazy runtime behavior
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
go test -race ./...
golangci-lint run
git diff --check
```

If your local Docker setup requires Ryuk to be disabled, prefix the test
command with `TESTCONTAINERS_RYUK_DISABLED=true`.
