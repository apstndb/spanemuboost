package spanemuboost

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/api/option"
)

func TestLoadEndpointFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "endpoint.json")
	endpoint := Endpoint{
		Backend:    BackendOmni,
		URI:        "127.0.0.1:15000",
		ProjectID:  defaultOmniProjectID,
		InstanceID: defaultOmniInstanceID,
	}
	if err := SaveEndpoint(path, endpoint); err != nil {
		t.Fatalf("SaveEndpoint() error = %v", err)
	}

	t.Setenv(endpointFileEnv, path)
	t.Setenv(omniURIEnv, "")

	got, err := LoadEndpoint()
	if err != nil {
		t.Fatalf("LoadEndpoint() error = %v", err)
	}
	if got != endpoint {
		t.Fatalf("LoadEndpoint() = %#v, want %#v", got, endpoint)
	}
}

func TestLoadEndpointFromOmniEnv(t *testing.T) {
	t.Setenv(endpointFileEnv, "")
	t.Setenv(omniURIEnv, "127.0.0.1:15000")
	t.Setenv(omniProjectIDEnv, "proj")
	t.Setenv(omniInstanceIDEnv, "inst")

	got, err := LoadEndpoint()
	if err != nil {
		t.Fatalf("LoadEndpoint() error = %v", err)
	}
	want := Endpoint{
		Backend:    BackendOmni,
		URI:        "127.0.0.1:15000",
		ProjectID:  "proj",
		InstanceID: "inst",
	}
	if got != want {
		t.Fatalf("LoadEndpoint() = %#v, want %#v", got, want)
	}
}

func TestEndpointConfigured(t *testing.T) {
	t.Setenv(endpointFileEnv, "")
	t.Setenv(omniURIEnv, "")
	t.Setenv(emulatorURIEnv, "")
	if EndpointConfigured() {
		t.Fatal("EndpointConfigured() = true, want false")
	}
	t.Setenv(omniURIEnv, "127.0.0.1:15000")
	if !EndpointConfigured() {
		t.Fatal("EndpointConfigured() = false, want true")
	}
}

func TestEndpointConfiguredWithBrokenFile(t *testing.T) {
	t.Setenv(endpointFileEnv, filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv(omniURIEnv, "")
	if !EndpointConfigured() {
		t.Fatal("EndpointConfigured() = false, want true when endpoint file env is set")
	}
}

func TestSaveEndpointRejectsInvalidBackend(t *testing.T) {
	err := SaveEndpoint(filepath.Join(t.TempDir(), "endpoint.json"), Endpoint{
		Backend:    Backend("bad"),
		URI:        "127.0.0.1:1",
		ProjectID:  "p",
		InstanceID: "i",
	})
	if err == nil {
		t.Fatal("SaveEndpoint() error = nil, want non-nil")
	}
}

func TestParseServeArgs(t *testing.T) {
	cfg, err := ParseServeArgs([]string{"omni", "--endpoint-file", "/tmp/omni.json"})
	if err != nil {
		t.Fatalf("ParseServeArgs() error = %v", err)
	}
	if cfg.Backend != BackendOmni || cfg.EndpointFile != "/tmp/omni.json" {
		t.Fatalf("ParseServeArgs() = %#v, want omni + /tmp/omni.json", cfg)
	}
}

func TestLoadEndpointMissingEnvMentionsEmulatorURI(t *testing.T) {
	t.Setenv(endpointFileEnv, "")
	t.Setenv(omniURIEnv, "")
	t.Setenv(emulatorURIEnv, "")

	_, err := LoadEndpoint()
	if err == nil {
		t.Fatal("LoadEndpoint() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), emulatorURIEnv) {
		t.Fatalf("LoadEndpoint() error = %v, want mention of %s", err, emulatorURIEnv)
	}
}

func TestRuntimePlatformAttached(t *testing.T) {
	runtime, err := NewAttachedRuntime(Endpoint{
		Backend:    BackendOmni,
		URI:        "127.0.0.1:15000",
		ProjectID:  defaultOmniProjectID,
		InstanceID: defaultOmniInstanceID,
	})
	if err != nil {
		t.Fatalf("NewAttachedRuntime() error = %v", err)
	}
	got, err := RuntimePlatform(context.Background(), runtime)
	if err != nil {
		t.Fatalf("RuntimePlatform() error = %v", err)
	}
	if got != "attached" {
		t.Fatalf("RuntimePlatform() = %q, want attached", got)
	}
}

func TestNewAttachedRuntimeCloseIsNoOp(t *testing.T) {
	runtime, err := NewAttachedRuntime(Endpoint{
		Backend:    BackendOmni,
		URI:        "127.0.0.1:15000",
		ProjectID:  defaultOmniProjectID,
		InstanceID: defaultOmniInstanceID,
	})
	if err != nil {
		t.Fatalf("NewAttachedRuntime() error = %v", err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("second Close() error = %v, want nil", err)
	}
}

func TestNewLazyRuntimeFromEnvOrStartUsesEnv(t *testing.T) {
	t.Setenv(endpointFileEnv, "")
	t.Setenv(omniURIEnv, "127.0.0.1:15000")

	lazy, err := NewLazyRuntimeFromEnvOrStart(BackendOmni)
	if err != nil {
		t.Fatalf("NewLazyRuntimeFromEnvOrStart() error = %v", err)
	}
	if lazy.attachedEndpoint == nil {
		t.Fatal("attachedEndpoint = nil, want non-nil")
	}
	if lazy.attachedEndpoint.URI != "127.0.0.1:15000" {
		t.Fatalf("attachedEndpoint.URI = %q, want 127.0.0.1:15000", lazy.attachedEndpoint.URI)
	}
}

func TestNewLazyRuntimeFromEnvOrStartColdStartWithoutEnv(t *testing.T) {
	t.Setenv(endpointFileEnv, "")
	t.Setenv(omniURIEnv, "")
	t.Setenv(emulatorURIEnv, "")

	lazy, err := NewLazyRuntimeFromEnvOrStart(BackendOmni)
	if err != nil {
		t.Fatalf("NewLazyRuntimeFromEnvOrStart() error = %v, want nil", err)
	}
	if lazy.attachedEndpoint != nil {
		t.Fatal("attachedEndpoint = non-nil, want nil")
	}
}

func TestNewLazyRuntimeFromEnvOrStartErrorsOnBrokenEndpointFile(t *testing.T) {
	t.Setenv(endpointFileEnv, filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv(omniURIEnv, "")

	lazy, err := NewLazyRuntimeFromEnvOrStart(BackendOmni)
	if err == nil {
		t.Fatal("NewLazyRuntimeFromEnvOrStart() error = nil, want non-nil")
	}
	if lazy != nil {
		t.Fatal("NewLazyRuntimeFromEnvOrStart() runtime = non-nil, want nil")
	}
}

func TestNewLazyRuntimeFromEnvOrStartRejectsBackendMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "endpoint.json")
	endpoint := Endpoint{
		Backend:    BackendOmni,
		URI:        "127.0.0.1:15000",
		ProjectID:  defaultOmniProjectID,
		InstanceID: defaultOmniInstanceID,
	}
	if err := SaveEndpoint(path, endpoint); err != nil {
		t.Fatalf("SaveEndpoint() error = %v", err)
	}
	t.Setenv(endpointFileEnv, path)
	t.Setenv(omniURIEnv, "")

	_, err := NewLazyRuntimeFromEnvOrStart(BackendEmulator)
	if err == nil {
		t.Fatal("NewLazyRuntimeFromEnvOrStart() error = nil, want non-nil")
	}
}

func TestEndpointConfiguredForBackend(t *testing.T) {
	t.Setenv(endpointFileEnv, "")
	t.Setenv(omniURIEnv, "")
	t.Setenv(emulatorURIEnv, "")

	if EndpointConfiguredForBackend(BackendOmni) {
		t.Fatal("EndpointConfiguredForBackend(Omni) = true, want false")
	}
	t.Setenv(emulatorURIEnv, "127.0.0.1:9010")
	if EndpointConfiguredForBackend(BackendOmni) {
		t.Fatal("EndpointConfiguredForBackend(Omni) with emulator URI = true, want false")
	}
	if !EndpointConfiguredForBackend(BackendEmulator) {
		t.Fatal("EndpointConfiguredForBackend(Emulator) = false, want true")
	}
}

func TestLoadEndpointForBackendSelectsMatchingURIEnv(t *testing.T) {
	t.Setenv(endpointFileEnv, "")
	t.Setenv(omniURIEnv, "127.0.0.1:15000")
	t.Setenv(emulatorURIEnv, "127.0.0.1:9010")

	omni, err := LoadEndpointForBackend(BackendOmni)
	if err != nil {
		t.Fatalf("LoadEndpointForBackend(Omni) error = %v", err)
	}
	if omni.URI != "127.0.0.1:15000" {
		t.Fatalf("Omni URI = %q, want 127.0.0.1:15000", omni.URI)
	}

	emulator, err := LoadEndpointForBackend(BackendEmulator)
	if err != nil {
		t.Fatalf("LoadEndpointForBackend(Emulator) error = %v", err)
	}
	if emulator.URI != "127.0.0.1:9010" {
		t.Fatalf("Emulator URI = %q, want 127.0.0.1:9010", emulator.URI)
	}
}

func TestAttachedRuntimeInheritedOptionsPreservesConstructorBootstrap(t *testing.T) {
	runtime, err := NewAttachedRuntime(Endpoint{
		Backend:    BackendOmni,
		URI:        "127.0.0.1:15000",
		ProjectID:  defaultOmniProjectID,
		InstanceID: defaultOmniInstanceID,
	},
		WithRandomDatabaseID(),
		WithSetupDDLs([]string{"CREATE TABLE tbl (pk STRING(MAX)) PRIMARY KEY (pk)"}),
		WithSetupRawDMLs([]string{"INSERT INTO tbl (pk) VALUES ('x')"}),
	)
	if err != nil {
		t.Fatalf("NewAttachedRuntime() error = %v", err)
	}
	opts, err := runtime.inheritedOptions()
	if err != nil {
		t.Fatalf("inheritedOptions() error = %v", err)
	}
	if opts.disableCreateDatabase {
		t.Fatal("disableCreateDatabase = true, want false for WithRandomDatabaseID")
	}
	if len(opts.setupDDLs) != 1 {
		t.Fatalf("len(setupDDLs) = %d, want 1", len(opts.setupDDLs))
	}
	if len(opts.setupDMLs) != 1 {
		t.Fatalf("len(setupDMLs) = %d, want 1", len(opts.setupDMLs))
	}
}

func TestParseStopArgs(t *testing.T) {
	cfg, err := ParseStopArgs([]string{"--endpoint-file", "/tmp/omni.json"})
	if err != nil {
		t.Fatalf("ParseStopArgs() error = %v", err)
	}
	if cfg.EndpointFile != "/tmp/omni.json" {
		t.Fatalf("EndpointFile = %q, want /tmp/omni.json", cfg.EndpointFile)
	}
}

func TestParseServeArgsRequiresBackend(t *testing.T) {
	_, err := ParseServeArgs([]string{"--endpoint-file", "/tmp/omni.json"})
	if err == nil {
		t.Fatal("ParseServeArgs() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "usage:") {
		t.Fatalf("ParseServeArgs() error = %v, want usage error", err)
	}
}

func TestNewAttachedRuntimeWithRandomDatabaseID(t *testing.T) {
	runtime, err := NewAttachedRuntime(Endpoint{
		Backend:    BackendOmni,
		URI:        "127.0.0.1:15000",
		ProjectID:  defaultOmniProjectID,
		InstanceID: defaultOmniInstanceID,
	}, WithRandomDatabaseID())
	if err != nil {
		t.Fatalf("NewAttachedRuntime() error = %v", err)
	}
	if runtime.DatabaseID() == "" || runtime.DatabaseID() == DefaultDatabaseID {
		t.Fatalf("DatabaseID() = %q, want generated random ID", runtime.DatabaseID())
	}
}

func TestNewLazyRuntimeFromEnvOrStartAttachPathWithRandomDatabaseID(t *testing.T) {
	t.Setenv(endpointFileEnv, "")
	t.Setenv(omniURIEnv, "127.0.0.1:15000")

	lazy, err := NewLazyRuntimeFromEnvOrStart(BackendOmni, WithRandomDatabaseID())
	if err != nil {
		t.Fatalf("NewLazyRuntimeFromEnvOrStart() error = %v", err)
	}
	if lazy.attachedEndpoint == nil {
		t.Fatal("attachedEndpoint = nil, want non-nil")
	}
	runtime, err := NewAttachedRuntime(*lazy.attachedEndpoint, lazy.opts...)
	if err != nil {
		t.Fatalf("NewAttachedRuntime() via lazy attach path error = %v", err)
	}
	if runtime.DatabaseID() == "" || runtime.DatabaseID() == DefaultDatabaseID {
		t.Fatalf("DatabaseID() = %q, want generated random ID", runtime.DatabaseID())
	}
}

func TestAttachedRuntimeClientOptionsReturnsTransportOptions(t *testing.T) {
	runtime, err := NewAttachedRuntime(Endpoint{
		Backend:    BackendOmni,
		URI:        "127.0.0.1:15000",
		ProjectID:  defaultOmniProjectID,
		InstanceID: defaultOmniInstanceID,
	}, WithClientOptionsForClient(option.WithQuotaProject("client-only")))
	if err != nil {
		t.Fatalf("NewAttachedRuntime() error = %v", err)
	}
	if got := len(runtime.ClientOptions()); got != 3 {
		t.Fatalf("ClientOptions() returned %d options, want 3 transport options", got)
	}
}

func TestNewAttachedRuntimeAppliesOptions(t *testing.T) {
	runtime, err := NewAttachedRuntime(Endpoint{
		Backend:    BackendOmni,
		URI:        "127.0.0.1:15000",
		ProjectID:  defaultOmniProjectID,
		InstanceID: defaultOmniInstanceID,
	}, WithDatabaseID("attached-db"))
	if err != nil {
		t.Fatalf("NewAttachedRuntime() error = %v", err)
	}
	if got := runtime.DatabaseID(); got != "attached-db" {
		t.Fatalf("DatabaseID() = %q, want attached-db", got)
	}
}

func TestSaveEndpointCreatesParentDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	path := filepath.Join(dir, "endpoint.json")
	endpoint := Endpoint{
		Backend:    BackendOmni,
		URI:        "127.0.0.1:15000",
		ProjectID:  defaultOmniProjectID,
		InstanceID: defaultOmniInstanceID,
	}
	if err := SaveEndpoint(path, endpoint); err != nil {
		t.Fatalf("SaveEndpoint() error = %v", err)
	}
	got, err := ReadEndpointFile(path)
	if err != nil {
		t.Fatalf("ReadEndpointFile() error = %v", err)
	}
	if got != endpoint {
		t.Fatalf("ReadEndpointFile() = %#v, want %#v", got, endpoint)
	}
}

func TestEndpointFromRuntimeRejectsInvalidEndpoint(t *testing.T) {
	runtime := &invalidEndpointRuntime{}
	_, err := EndpointFromRuntime(runtime)
	if err == nil {
		t.Fatal("EndpointFromRuntime() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "project_id") {
		t.Fatalf("EndpointFromRuntime() error = %v, want project_id validation error", err)
	}
}

type invalidEndpointRuntime struct{}

func (*invalidEndpointRuntime) spanemuboostRuntime()                 {}
func (*invalidEndpointRuntime) URI() string                          { return "127.0.0.1:1" }
func (*invalidEndpointRuntime) ClientOptions() []option.ClientOption { return nil }
func (*invalidEndpointRuntime) Close() error                         { return nil }
func (*invalidEndpointRuntime) ProjectID() string                    { return "" }
func (*invalidEndpointRuntime) InstanceID() string                   { return "" }
func (*invalidEndpointRuntime) DatabaseID() string                   { return "" }
func (*invalidEndpointRuntime) ProjectPath() string                  { return "" }
func (*invalidEndpointRuntime) InstancePath() string                 { return "" }
func (*invalidEndpointRuntime) DatabasePath() string                 { return "" }

func TestAttachedRuntimeNilReceiverSafe(t *testing.T) {
	var runtime *AttachedRuntime
	if got := runtime.URI(); got != "" {
		t.Fatalf("URI() = %q, want empty", got)
	}
	if got := runtime.ClientOptions(); got != nil {
		t.Fatalf("ClientOptions() = %v, want nil", got)
	}
	if got := runtime.ProjectID(); got != "" {
		t.Fatalf("ProjectID() = %q, want empty", got)
	}
	if got := runtime.InstanceID(); got != "" {
		t.Fatalf("InstanceID() = %q, want empty", got)
	}
	if got := runtime.DatabaseID(); got != "" {
		t.Fatalf("DatabaseID() = %q, want empty", got)
	}
	if got := runtime.ProjectPath(); got != "" {
		t.Fatalf("ProjectPath() = %q, want empty", got)
	}
	if got := runtime.InstancePath(); got != "" {
		t.Fatalf("InstancePath() = %q, want empty", got)
	}
	if got := runtime.DatabasePath(); got != "" {
		t.Fatalf("DatabasePath() = %q, want empty", got)
	}
}

func TestEndpointFromRuntimeAttached(t *testing.T) {
	runtime, err := NewAttachedRuntime(Endpoint{
		Backend:    BackendOmni,
		URI:        "127.0.0.1:15000",
		ProjectID:  defaultOmniProjectID,
		InstanceID: defaultOmniInstanceID,
	})
	if err != nil {
		t.Fatalf("NewAttachedRuntime() error = %v", err)
	}
	got, err := EndpointFromRuntime(runtime)
	if err != nil {
		t.Fatalf("EndpointFromRuntime() error = %v", err)
	}
	want := Endpoint{
		Backend:    BackendOmni,
		URI:        "127.0.0.1:15000",
		ProjectID:  defaultOmniProjectID,
		InstanceID: defaultOmniInstanceID,
	}
	if got != want {
		t.Fatalf("EndpointFromRuntime() = %#v, want %#v", got, want)
	}
}

func TestReadEndpointFileMissing(t *testing.T) {
	_, err := ReadEndpointFile(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil {
		t.Fatal("ReadEndpointFile() error = nil, want non-nil")
	}
	if !os.IsNotExist(err) && !strings.Contains(err.Error(), "read endpoint file") {
		t.Fatalf("ReadEndpointFile() error = %v", err)
	}
}
