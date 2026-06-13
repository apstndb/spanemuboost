package spanemuboost

import (
	"context"
	"fmt"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// AttachedRuntime is a [Runtime] connected to an already-running backend.
// [AttachedRuntime.Close] does not stop the remote process or container.
type AttachedRuntime struct {
	backend Backend
	opts    *emulatorOptions
	uri     string
}

func (*AttachedRuntime) spanemuboostRuntime() {}

// NewAttachedRuntime connects to endpoint without starting a container.
func NewAttachedRuntime(endpoint Endpoint, options ...Option) (*AttachedRuntime, error) {
	if err := endpoint.validate(); err != nil {
		return nil, err
	}
	opts, err := finalizeAttachedOptions(endpoint, options...)
	if err != nil {
		return nil, err
	}
	return &AttachedRuntime{
		backend: endpoint.Backend,
		opts:    opts,
		uri:     endpoint.URI,
	}, nil
}

// NewAttachedRuntimeFromEnv is a convenience wrapper around [LoadEndpoint] and
// [NewAttachedRuntime].
func NewAttachedRuntimeFromEnv(options ...Option) (*AttachedRuntime, error) {
	endpoint, err := LoadEndpoint()
	if err != nil {
		return nil, err
	}
	return NewAttachedRuntime(endpoint, options...)
}

// NewLazyRuntimeFromEnvOrStart returns a [LazyRuntime] that attaches to an
// external endpoint when one is configured for the requested backend. When no
// matching endpoint env vars are set, it defers container startup until first
// use. When endpoint env vars are set but [LoadEndpointForBackend] fails, it
// returns an error instead of falling back to cold start.
//
// Options passed to the constructor apply to both cold-start and attach paths.
// Database bootstrap options such as [WithRandomDatabaseID] and [WithSetupDDLs]
// are honored when [OpenClients] or [SetupClients] runs against the lazy handle.
func NewLazyRuntimeFromEnvOrStart(backend Backend, options ...Option) (*LazyRuntime, error) {
	lr := NewLazyRuntime(backend, options...)
	if !EndpointConfiguredForBackend(backend) {
		return lr, nil
	}
	endpoint, err := LoadEndpointForBackend(backend)
	if err != nil {
		return nil, err
	}
	lr.attachedEndpoint = &endpoint
	return lr, nil
}

func finalizeAttachedOptions(endpoint Endpoint, options ...Option) (*emulatorOptions, error) {
	base := &emulatorOptions{
		projectID:             endpoint.ProjectID,
		instanceID:            endpoint.InstanceID,
		disableCreateInstance: true,
	}
	switch endpoint.Backend {
	case BackendOmni:
		return applyOmniOptionsWithBase(base, options...)
	case BackendEmulator:
		return applyOptionsWithBase(base, options...)
	default:
		return nil, fmt.Errorf("unsupported backend %q", endpoint.Backend)
	}
}

func (a *AttachedRuntime) URI() string {
	if a == nil {
		return ""
	}
	return a.uri
}

// ClientOptions returns transport options for the attached backend. Options
// passed via [WithClientOptionsForClient] are applied in [OpenClients] and
// [SetupClients], not here.
func (a *AttachedRuntime) ClientOptions() []option.ClientOption {
	if a == nil {
		return nil
	}
	switch a.backend {
	case BackendOmni:
		return []option.ClientOption{
			option.WithEndpoint(a.uri),
			option.WithoutAuthentication(),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		}
	default:
		return []option.ClientOption{
			option.WithEndpoint("passthrough:///" + a.uri),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
			option.WithoutAuthentication(),
			internaloption.SkipDialSettingsValidation(),
		}
	}
}

// Close is a no-op for attached runtimes because this handle does not own the
// remote backend lifecycle.
func (a *AttachedRuntime) Close() error { return nil }

func (a *AttachedRuntime) ProjectID() string {
	if a == nil || a.opts == nil {
		return ""
	}
	return a.opts.projectID
}
func (a *AttachedRuntime) InstanceID() string {
	if a == nil || a.opts == nil {
		return ""
	}
	return a.opts.instanceID
}
func (a *AttachedRuntime) DatabaseID() string {
	if a == nil || a.opts == nil {
		return ""
	}
	return a.opts.databaseID
}

func (a *AttachedRuntime) ProjectPath() string {
	if a == nil || a.opts == nil {
		return ""
	}
	return a.opts.ProjectPath()
}
func (a *AttachedRuntime) InstancePath() string {
	if a == nil || a.opts == nil {
		return ""
	}
	return a.opts.InstancePath()
}
func (a *AttachedRuntime) DatabasePath() string {
	if a == nil || a.opts == nil {
		return ""
	}
	return a.opts.DatabasePath()
}

func (a *AttachedRuntime) inheritedOptions(options ...Option) (*emulatorOptions, error) {
	if a == nil || a.opts == nil {
		return nil, fmt.Errorf("spanemuboost: attached runtime or options is nil")
	}
	base := inheritedRuntimeOptions(a.opts)
	preserveAttachedBootstrapOptions(base, a.opts)
	if len(options) == 0 {
		return base, nil
	}
	switch a.backend {
	case BackendOmni:
		return applyOmniOptionsWithBase(base, options...)
	default:
		return applyOptionsWithBase(base, options...)
	}
}

func preserveAttachedBootstrapOptions(base, source *emulatorOptions) {
	if !source.disableCreateDatabase {
		base.disableCreateDatabase = false
	}
	base.randomDatabaseID = source.randomDatabaseID
	if len(source.setupDDLs) > 0 {
		base.setupDDLs = append([]string(nil), source.setupDDLs...)
	}
	if len(source.setupDMLs) > 0 {
		base.setupDMLs = append([]spanner.Statement(nil), source.setupDMLs...)
	}
}

func (a *AttachedRuntime) runtimePlatform(context.Context) (string, error) {
	return "attached", nil
}
