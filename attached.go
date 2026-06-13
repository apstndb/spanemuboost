package spanemuboost

import (
	"context"
	"fmt"

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
func NewAttachedRuntime(endpoint Endpoint) (*AttachedRuntime, error) {
	if err := endpoint.validate(); err != nil {
		return nil, err
	}
	opts, err := finalizeAttachedOptions(endpoint)
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
func NewAttachedRuntimeFromEnv() (*AttachedRuntime, error) {
	endpoint, err := LoadEndpoint()
	if err != nil {
		return nil, err
	}
	return NewAttachedRuntime(endpoint)
}

// NewLazyRuntimeOptionalEndpoint returns a [LazyRuntime] that attaches to an
// external endpoint when one is configured in the environment. Otherwise it
// starts a container on first use, preserving the existing testcontainers path.
func NewLazyRuntimeOptionalEndpoint(backend Backend, options ...Option) *LazyRuntime {
	lr := NewLazyRuntime(backend, options...)
	if endpoint, err := LoadEndpoint(); err == nil && endpoint.Backend == backend {
		lr.attachedEndpoint = &endpoint
	}
	return lr
}

func finalizeAttachedOptions(endpoint Endpoint) (*emulatorOptions, error) {
	base := &emulatorOptions{
		projectID:             endpoint.ProjectID,
		instanceID:            endpoint.InstanceID,
		disableCreateInstance: true,
	}
	switch endpoint.Backend {
	case BackendOmni:
		return finalizeOmniOptions(base)
	case BackendEmulator:
		return finalizeOptions(base)
	default:
		return nil, fmt.Errorf("unsupported backend %q", endpoint.Backend)
	}
}

func (a *AttachedRuntime) URI() string { return a.uri }

func (a *AttachedRuntime) ClientOptions() []option.ClientOption {
	if a.opts != nil && len(a.opts.clientOptionsForClient) > 0 {
		return a.opts.clientOptionsForClient
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

func (a *AttachedRuntime) ProjectID() string  { return a.opts.projectID }
func (a *AttachedRuntime) InstanceID() string { return a.opts.instanceID }
func (a *AttachedRuntime) DatabaseID() string { return a.opts.databaseID }

func (a *AttachedRuntime) ProjectPath() string  { return a.opts.ProjectPath() }
func (a *AttachedRuntime) InstancePath() string { return a.opts.InstancePath() }
func (a *AttachedRuntime) DatabasePath() string { return a.opts.DatabasePath() }

func (a *AttachedRuntime) inheritedOptions(options ...Option) (*emulatorOptions, error) {
	base := inheritedRuntimeOptions(a.opts)
	switch a.backend {
	case BackendOmni:
		return applyOmniOptionsWithBase(base, options...)
	default:
		return applyOptionsWithBase(base, options...)
	}
}

func (a *AttachedRuntime) runtimePlatform(context.Context) (string, error) {
	return "attached", nil
}
