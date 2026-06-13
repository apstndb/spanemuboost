# SPANner EMUlator BOOtSTrapper

[![Go Reference](https://pkg.go.dev/badge/github.com/apstndb/spanemuboost.svg)](https://pkg.go.dev/github.com/apstndb/spanemuboost)

spanemuboost bootstraps Cloud Spanner Emulator and, experimentally, Spanner Omni for tests using [testcontainers-go](https://github.com/testcontainers/testcontainers-go). Review the [Spanner Omni software requirements](https://docs.cloud.google.com/spanner-omni/system-requirements#software-requirements) before enabling the Omni path in local development or CI.

It inspired by `autoConfigEmulator` of:

* [java-spanner-jdbc](https://github.com/googleapis/java-spanner-jdbc?tab=readme-ov-file#commonly-used-properties)
* [java-spanner](https://cloud.google.com/java/docs/reference/google-cloud-spanner/6.62.0/com.google.cloud.spanner.connection.ConnectionOptions.Builder#com_google_cloud_spanner_connection_ConnectionOptions_Builder_setUri_java_lang_String_)

This package doesn't have functionality of splitting statements and stripping comments.
Consider to use [memefish](https://github.com/cloudspannerecosystem/memefish) or helper packages.

## Examples

### Quick start

```go
func TestFoo(t *testing.T) {
    env := spanemuboost.SetupEmulatorWithClients(t,
        spanemuboost.WithSetupDDLs(ddls),
    )
    // env.Client, env.DatabaseClient, env.InstanceClient available
    // cleanup is automatic via t.Cleanup
}
```

For non-test usage (e.g. embedding the emulator in an application where the `testing` package is unavailable), see runnable examples on [pkg.go.dev](https://pkg.go.dev/github.com/apstndb/spanemuboost#pkg-examples).

### Shared runtime, database-per-case

For many independent test or validation cases, prefer one shared backend runtime
and one database per case. That is the recommended shape for the emulator, and
it is even more important for Omni because each Omni runtime owns a memory-heavy
container.

Use `NewLazyRuntime` plus `SetupClients` or `OpenClients` when cases should
share a runtime lazily. Use `WithRandomDatabaseID()` for per-case isolation. See
`ExampleNewLazyRuntime_validationCases` on
[pkg.go.dev](https://pkg.go.dev/github.com/apstndb/spanemuboost#pkg-examples)
for a validation-harness shape that composes shared and case-specific setup, and
separates setup failures from candidate statement results.

Random database IDs do not enable schema teardown on `Clients.Close()` by
default. The databases disappear when the runtime container is closed. For
long-lived shared runtimes, use `ForceSchemaTeardown()` or explicit cleanup if
database accumulation matters.

| Need | Entry point | Starts a new runtime container? |
|---|---|---|
| One test owns runtime and clients | `SetupWithClients(t, backend, ...)` | Yes |
| Non-test code owns runtime and clients | `RunWithClients(ctx, backend, ...)` | Yes |
| Many tests share one runtime with `testing.TB` cleanup | `NewLazyRuntime(backend, ...)` + `SetupClients` | Once, on first use |
| Many cases need explicit `context.Context` or manual client cleanup | `NewLazyRuntime(backend, ...)` + `OpenClients` | Once, on first use |
| Eager runtime startup with multiple databases | `Run(ctx, backend, ...)` + `OpenClients` | Once, when `Run` is called |

### Spanner Omni (experimental)

`Setup`, `Run`, `RunWithClients`, and `SetupWithClients` with `BackendOmni` start a Spanner Omni single-server container and use the public Spanner gRPC API on port `15000` for database creation, DDL application, DML setup, and managed client creation. This path is intended for integration tests that want a real Omni runtime without depending on the emulator.

```go
func TestOmni(t *testing.T) {
    env := spanemuboost.SetupWithClients(t, spanemuboost.BackendOmni,
        spanemuboost.WithRandomDatabaseID(),
        spanemuboost.WithSetupDDLs([]string{
            "CREATE TABLE tbl (pk STRING(MAX), col INT64) PRIMARY KEY (pk)",
        }),
        spanemuboost.WithSetupRawDMLs([]string{
            "INSERT INTO tbl (pk, col) VALUES ('foo', 1)",
        }),
    )

    err := env.Client.Single().Query(t.Context(), spanner.NewStatement(
        "SELECT col FROM tbl WHERE pk = 'foo'",
    )).Do(func(r *spanner.Row) error {
        var col int64
        return r.Column(0, &col)
    })
    if err != nil { t.Fatal(err) }
}
```

For Omni, use databases as the normal isolation unit. Do not use random project
or instance IDs for ordinary test isolation; the single-server deployment uses
fixed project and instance IDs. Use `WithRandomDatabaseID()` and share the
runtime with `NewLazyRuntime(BackendOmni, ...)` when many cases are involved.

| Omni caveat | Detail |
|---|---|
| Experimental runtime | Omni support is newer than the emulator path and should be treated as integration-test-oriented |
| Primary endpoint | The main Spanner gRPC endpoint is `15000`; the console remains separate |
| Resource use | Each started Omni runtime owns one Spanner Omni container; plan for roughly 4 GiB of memory per concurrently running Omni container |
| Recommended client config | Managed Omni clients force the `RecommendedOmniClientConfig()` transport defaults (`DisableNativeMetrics` and `IsExperimentalHost`) unless guardrails are disabled; the same helper remains the recommended base for external Go clients |
| Host and container prerequisites | Review the [Spanner Omni software requirements](https://docs.cloud.google.com/spanner-omni/system-requirements#software-requirements) before enabling Omni in local development or CI; see [Omni runtime environments](docs/omni-runtime-environments.md) for local Colima and Podman notes |
| Guardrails | Known-invalid single-server Omni settings fail fast with human-readable errors; use `DisableBackendGuardrails()` only when testing a newer backend whose constraints may have changed |

The repository's Omni integration tests are gated by `SPANEMUBOOST_ENABLE_OMNI_TESTS=1` so default test runs stay hermetic unless the environment is explicitly prepared for Omni.
Keep tests that start Omni runtimes serial unless the host has enough spare
memory for multiple Omni containers. spanemuboost does not impose a global
runtime lock; use `go test -p=1 -parallel=1` or share a runtime with
`NewLazyRuntime(BackendOmni, ...)` when memory is tight.

When running through Podman and Testcontainers-Go does not auto-detect Podman
from `DOCKER_HOST`, set `SPANEMUBOOST_TESTCONTAINERS_PROVIDER=podman` for that
command or pass `WithContainerProvider(testcontainers.ProviderPodman)`. The
environment variable affects all spanemuboost runtime containers, including the
default emulator backend. For rootful Podman machine with Ryuk enabled, the
relevant environment is:

```sh
env DOCKER_HOST=unix://<host Podman API socket> \
  TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/run/podman/podman.sock \
  TESTCONTAINERS_RYUK_CONTAINER_PRIVILEGED=true \
  SPANEMUBOOST_TESTCONTAINERS_PROVIDER=podman \
  SPANEMUBOOST_ENABLE_OMNI_TESTS=1 \
  go test -p=1 -parallel=1 ./...
```

Once a runtime is started, the shared client helpers are backend-neutral:

```go
func TestSharedHelpers(t *testing.T) {
    runtime := spanemuboost.Setup(t, spanemuboost.BackendOmni)
    clients := spanemuboost.SetupClients(t, runtime,
        spanemuboost.WithRandomDatabaseID(),
        spanemuboost.WithSetupDDLs([]string{
            "CREATE TABLE tbl (pk STRING(MAX)) PRIMARY KEY (pk)",
        }),
    )
    _ = clients
}
```

### Reusing a long-lived Omni runtime

Starting Spanner Omni through testcontainers is slow because each runtime pulls
and boots a memory-heavy container. For local development and repeated test
runs, start Omni once with the `spanemuboost` CLI and attach clients to the
published endpoint:

```sh
spanemuboost serve omni --endpoint-file /tmp/omni-endpoint.json
```

In another shell:

```sh
export SPANEMUBOOST_ENDPOINT_FILE=/tmp/omni-endpoint.json
export SPANEMUBOOST_ENABLE_OMNI_TESTS=1
go test -p=1 -parallel=1 ./...
```

Client code can use [NewLazyRuntimeFromEnvOrStart] to keep the existing
testcontainers path while automatically attaching when endpoint env vars are
set, or [NewAttachedRuntimeFromEnv] for explicit attachment:

```go
runtime, err := spanemuboost.NewLazyRuntimeFromEnvOrStart(spanemuboost.BackendOmni)
if err != nil {
    log.Fatal(err)
}
clients, err := spanemuboost.OpenClients(ctx, runtime,
    spanemuboost.WithRandomDatabaseID(),
    spanemuboost.WithSetupDDLs(ddls),
)
```

| Variable | Purpose |
|---|---|
| `SPANEMUBOOST_ENDPOINT_FILE` | JSON file written by `spanemuboost serve` |
| `SPANEMUBOOST_OMNI_URI` | Direct Omni gRPC endpoint (`host:port`) |
| `SPANEMUBOOST_OMNI_PROJECT_ID` | Optional project override (default `default`) |
| `SPANEMUBOOST_OMNI_INSTANCE_ID` | Optional instance override (default `default`) |

`[AttachedRuntime.Close]` is a no-op because the lifecycle manager owns the
container. Stop the `serve` process to tear down the shared runtime.

`Run`, `RunWithClients`, `Setup`, `SetupWithClients`, `OpenClients`, `SetupClients`, `RuntimePlatform`, and `NewLazyRuntime` work across emulator and Omni. This backend-neutral API surface is the primary stable entry point; only the `BackendOmni` backend and its specific behaviors are considered experimental. Omni does not add separate exported startup or client-opening helpers.

Use `RuntimePlatform(ctx, runtime)` when you want to surface the actual resolved container platform for a package-provided runtime handle without downcasting back to `*Emulator`. Depending on what metadata the underlying runtime exposes, that may be an `os/arch` string such as `linux/amd64`, a variant-qualified string such as `linux/arm64/v8`, or an OS-only value such as `linux`.

### Shared emulator patterns

As [recommended by the Cloud Spanner Emulator FAQ](https://github.com/GoogleCloudPlatform/cloud-spanner-emulator/blob/master/README.md#what-is-the-recommended-test-setup):

> What is the recommended test setup?
> Use a single emulator process and create a Cloud Spanner instance within it. Since creating databases is cheap in the emulator, we recommend that each test bring up and tear down its own database. This ensures hermetic testing and allows the test suite to run tests in parallel if needed.

| Pattern | Emulator lifetime | Best for |
|---|---|---|
| **Lazy** (`NewLazyRuntime(BackendEmulator, ...)` + `SetupClients`) | First `SetupClients` call &rarr; `TestMain` cleanup | Packages mixing emulator and non-emulator tests; skips startup when unused |
| **Eager** (`RunEmulator` + `SetupClients`) | `TestMain` start &rarr; `TestMain` cleanup | All tests need the emulator; fail fast on startup errors |
| **Subtests** (`SetupEmulator` + `SetupClients`) | Parent test &rarr; `t.Cleanup` | Related tests grouped under one function; supports `t.Parallel()` in subtests |

#### Lazy shared emulator (recommended)

The emulator starts only when the first test calls `SetupClients` with the `LazyRuntime`. If `go test -run TestUnit` matches only tests that never use it, the container is never started.

```go
var lazyRuntime = spanemuboost.NewLazyRuntime(
    spanemuboost.BackendEmulator,
    spanemuboost.EnableInstanceAutoConfigOnly(),
)

func TestMain(m *testing.M) { lazyRuntime.TestMain(m) }

func TestCreate(t *testing.T) {
    clients := spanemuboost.SetupClients(t, lazyRuntime,
        spanemuboost.WithRandomDatabaseID(),
        spanemuboost.WithSetupDDLs(ddls),
    )
    // use clients.Client...
}

func TestUnit(t *testing.T) {
    // Does NOT use lazyRuntime — emulator never starts
}
```

`NewLazyEmulator` remains available as a backward-compatible wrapper around the
emulator backend, but `NewLazyRuntime(BackendEmulator, ...)` can cover the same
shared-runtime patterns while also extending naturally to Omni.

When you need emulator-specific helpers such as `Container()`, call
`runtime := lazyRuntime.Setup(t)` (or `Get(ctx)`) and type assert the result back
to `*spanemuboost.Emulator` for the `BackendEmulator` case.

#### Eager shared emulator

`testing.M` does NOT implement `testing.TB`, so use `RunEmulator` directly in `TestMain`.

```go
var emulator *spanemuboost.Emulator

func TestMain(m *testing.M) {
    var err error
    emulator, err = spanemuboost.RunEmulator(context.Background(),
        spanemuboost.EnableInstanceAutoConfigOnly(),
    )
    if err != nil { log.Fatal(err) }
    emulator.TestMain(m)
}

func TestCreate(t *testing.T) {
    clients := spanemuboost.SetupClients(t, emulator,
        spanemuboost.WithRandomDatabaseID(),
        spanemuboost.WithSetupDDLs(ddls),
    )
    // use clients.Client...
}
```

#### Shared emulator with subtests

When tests are naturally related and don't need `TestMain`, you can share an emulator within subtests of a single parent test. Since each subtest creates its own database via `WithRandomDatabaseID()`, subtests can safely run in parallel with `t.Parallel()`.

```go
func TestSuite(t *testing.T) {
    lazy := spanemuboost.NewLazyRuntime(
        spanemuboost.BackendEmulator,
        spanemuboost.EnableInstanceAutoConfigOnly(),
    )
    t.Cleanup(func() { _ = lazy.Close() })
    runtime := lazy.Setup(t)

    t.Run("test1", func(t *testing.T) {
        t.Parallel()
        clients := spanemuboost.SetupClients(t, runtime,
            spanemuboost.WithRandomDatabaseID(),
            spanemuboost.WithSetupDDLs(ddls),
        )
        // use clients.Client...
    })
}
```

### `SPANNER_EMULATOR_HOST` environment variable

For serial tests with code that reads `SPANNER_EMULATOR_HOST` directly:

```go
func TestWithEnvVar(t *testing.T) {
    lazy := spanemuboost.NewLazyRuntime(
        spanemuboost.BackendEmulator,
        spanemuboost.EnableInstanceAutoConfigOnly(),
    )
    t.Cleanup(func() { _ = lazy.Close() })
    runtime := lazy.Setup(t)
    t.Setenv("SPANNER_EMULATOR_HOST", runtime.URI())
    // Code under test that reads SPANNER_EMULATOR_HOST directly
}
```

| Caveat | Detail |
|---|---|
| No `t.Parallel()` | `t.Setenv` panics if the test or an ancestor called `t.Parallel()` |
| Process-global | The env var doesn't scale to concurrent tests |
| Prefer `ClientOptions()` | Pass `runtime.ClientOptions()` or clients directly when possible |
