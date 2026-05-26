// Package spanemuboost starts Cloud Spanner Emulator and experimental Spanner
// Omni runtimes for tests, then bootstraps instances, databases, schema, and
// Spanner clients.
//
// The emulator path is the default and stable path. Use
// [SetupEmulatorWithClients] for simple tests. For many independent cases
// against either backend, share one runtime with [NewLazyRuntime] and create one
// database per case with [WithRandomDatabaseID].
//
// The Omni path uses [BackendOmni] through the backend-neutral APIs such as
// [Setup], [Run], [SetupWithClients], [RunWithClients], [OpenClients],
// [SetupClients], and [NewLazyRuntime]. Omni support is experimental. The
// shared-runtime pattern is especially important for Omni because each started
// Omni runtime owns one Spanner Omni container.
package spanemuboost
