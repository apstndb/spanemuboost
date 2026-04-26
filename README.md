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

| Omni caveat | Detail |
|---|---|
| Experimental runtime | Omni support is newer than the emulator path and should be treated as integration-test-oriented |
| Primary endpoint | The main Spanner gRPC endpoint is `15000`; the console remains separate |
| Recommended client config | Managed Omni clients force the `RecommendedOmniClientConfig()` transport defaults (`DisableNativeMetrics` and `IsExperimentalHost`) unless guardrails are disabled; the same helper remains the recommended base for external Go clients |
| Host and container prerequisites | Review the [Spanner Omni software requirements](https://docs.cloud.google.com/spanner-omni/system-requirements#software-requirements) before enabling Omni in local development or CI |
| Guardrails | Known-invalid single-server Omni settings fail fast with human-readable errors; use `DisableBackendGuardrails()` only when testing a newer backend whose constraints may have changed |

The repository's Omni integration tests are gated by `SPANEMUBOOST_ENABLE_OMNI_TESTS=1` so default test runs stay hermetic unless the environment is explicitly prepared for Omni.

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

`Run`, `RunWithClients`, `Setup`, `SetupWithClients`, `OpenClients`, `SetupClients`, `RuntimePlatform`, and `NewLazyRuntime` work across emulator and Omni. This backend-neutral API surface is the primary stable entry point; only the `BackendOmni` backend and its specific behaviors are considered experimental. Omni does not add separate exported startup or client-opening helpers.

Use `RuntimePlatform(ctx, runtime)` when you want to surface the actual resolved container platform for a package-provided runtime handle without downcasting back to `*Emulator`.

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
