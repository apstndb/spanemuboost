package spanemuboost

import (
	"context"
	"errors"

	tcspanner "github.com/testcontainers/testcontainers-go/modules/gcloud/spanner"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Emulator wraps a Cloud Spanner Emulator container.
// Use [RunEmulator] or [SetupEmulator] to create one.
type Emulator struct {
	container *tcspanner.Container
	opts      *emulatorOptions
}

// URI returns the gRPC endpoint (host:port) of the emulator,
// suitable for use as SPANNER_EMULATOR_HOST.
func (e *Emulator) URI() string {
	return e.container.URI()
}

// ClientOptions returns [option.ClientOption] values configured for connecting
// to this emulator (endpoint, insecure credentials, no authentication).
func (e *Emulator) ClientOptions() []option.ClientOption {
	return []option.ClientOption{
		option.WithEndpoint(e.container.URI()),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithoutAuthentication(),
	}
}

// Close terminates the emulator container.
func (e *Emulator) Close() error {
	return e.container.Terminate(context.Background())
}

// Container returns the underlying [*tcspanner.Container] for direct access.
// Most users should use [Emulator.URI] or [Emulator.ClientOptions] instead.
func (e *Emulator) Container() *tcspanner.Container {
	return e.container
}

// ProjectID returns the project ID configured for this emulator.
func (e *Emulator) ProjectID() string { return e.opts.projectID }

// InstanceID returns the instance ID configured for this emulator.
func (e *Emulator) InstanceID() string { return e.opts.instanceID }

// DatabaseID returns the database ID configured for this emulator.
func (e *Emulator) DatabaseID() string { return e.opts.databaseID }

// ProjectPath returns the project resource path.
func (e *Emulator) ProjectPath() string { return projectPath(e.opts.projectID) }

// InstancePath returns the instance resource path.
func (e *Emulator) InstancePath() string { return instancePath(e.opts.projectID, e.opts.instanceID) }

// DatabasePath returns the database resource path.
func (e *Emulator) DatabasePath() string {
	return databasePath(e.opts.projectID, e.opts.instanceID, e.opts.databaseID)
}

// Env combines an [Emulator] with [Clients] for the single-call use case.
// Use [RunEmulatorWithClients] or [SetupEmulatorWithClients] to create one.
type Env struct {
	*Clients
	emulator *Emulator
}

// Emulator returns the underlying [Emulator].
func (e *Env) Emulator() *Emulator {
	return e.emulator
}

// Close closes the clients and then terminates the emulator.
func (e *Env) Close() error {
	return errors.Join(
		e.Clients.Close(),
		e.emulator.Close(),
	)
}
