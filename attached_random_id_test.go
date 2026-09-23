package spanemuboost

import (
	"strings"
	"testing"
)

func TestResolvedRandomDatabaseIDAllowsUnrelatedAttachedOptions(t *testing.T) {
	for _, backend := range []Backend{BackendEmulator, BackendOmni} {
		t.Run(string(backend), func(t *testing.T) {
			endpoint := attachedEndpointForBackend(backend)
			runtime, err := NewAttachedRuntime(endpoint, WithRandomDatabaseID())
			if err != nil {
				t.Fatalf("NewAttachedRuntime() error = %v", err)
			}
			original := runtime.DatabaseID()
			inherited, err := runtime.inheritedOptions(ForceSchemaTeardown(), WithSetupDDLs([]string{"CREATE TABLE t (id INT64) PRIMARY KEY (id)"}))
			if err != nil {
				t.Fatalf("inheritedOptions() error = %v", err)
			}
			if inherited.databaseID != original {
				t.Fatalf("databaseID = %q, want preserved %q", inherited.databaseID, original)
			}
			if !inherited.randomDatabaseID || !inherited.randomDatabaseIDResolved {
				t.Fatalf("random flags = %t/%t, want resolved random identity", inherited.randomDatabaseID, inherited.randomDatabaseIDResolved)
			}
			if inherited.schemaTeardown == nil || !*inherited.schemaTeardown {
				t.Fatal("schemaTeardown was not applied from the per-call option")
			}
		})
	}
}

func TestLazyAttachResolvedRandomDatabaseIDAllowsUnrelatedOptions(t *testing.T) {
	t.Setenv(endpointFileEnv, "")
	for _, tc := range []struct {
		backend Backend
		env     string
	}{
		{BackendEmulator, emulatorURIEnv},
		{BackendOmni, omniURIEnv},
	} {
		t.Run(string(tc.backend), func(t *testing.T) {
			t.Setenv(emulatorURIEnv, "")
			t.Setenv(omniURIEnv, "")
			t.Setenv(tc.env, "127.0.0.1:15000")
			lazy, err := NewLazyRuntimeFromEnvOrStart(tc.backend, WithRandomDatabaseID())
			if err != nil {
				t.Fatalf("NewLazyRuntimeFromEnvOrStart() error = %v", err)
			}
			runtime, err := lazy.Get(t.Context())
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}
			attached, ok := runtime.(*AttachedRuntime)
			if !ok {
				t.Fatalf("Get() runtime = %T, want *AttachedRuntime", runtime)
			}
			original := attached.DatabaseID()
			inherited, err := attached.inheritedOptions(WithSetupDDLs([]string{"CREATE TABLE t (id INT64) PRIMARY KEY (id)"}))
			if err != nil {
				t.Fatalf("inheritedOptions() error = %v", err)
			}
			if inherited.databaseID != original {
				t.Fatalf("databaseID = %q, want preserved %q", inherited.databaseID, original)
			}
		})
	}
}

func TestExplicitRandomAndDatabaseIDStillConflict(t *testing.T) {
	for _, name := range []string{"emulator", "omni"} {
		t.Run(name, func(t *testing.T) {
			var err error
			if name == "omni" {
				_, err = applyOmniOptions(WithRandomDatabaseID(), WithDatabaseID("explicit-db"))
			} else {
				_, err = applyOptions(WithRandomDatabaseID(), WithDatabaseID("explicit-db"))
			}
			if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
				t.Fatalf("error = %v, want mutual exclusion", err)
			}
		})
	}
}

func TestDatabaseIDBeforeRandomStillGeneratesID(t *testing.T) {
	opts, err := applyOptions(WithDatabaseID("explicit-db"), WithRandomDatabaseID())
	if err != nil {
		t.Fatalf("applyOptions() error = %v", err)
	}
	if opts.databaseID == "" || opts.databaseID == "explicit-db" || opts.databaseID == DefaultDatabaseID {
		t.Fatalf("databaseID = %q, want a generated random ID", opts.databaseID)
	}
	omni, err := applyOmniOptions(WithDatabaseID("explicit-db"), WithRandomDatabaseID())
	if err != nil {
		t.Fatalf("applyOmniOptions() error = %v", err)
	}
	if omni.databaseID == "" || omni.databaseID == "explicit-db" || omni.databaseID == DefaultDatabaseID {
		t.Fatalf("omni databaseID = %q, want a generated random ID", omni.databaseID)
	}
}

func TestResolvedRandomDatabaseIDCanBeOverridden(t *testing.T) {
	runtime, err := NewAttachedRuntime(attachedEndpointForBackend(BackendEmulator), WithRandomDatabaseID())
	if err != nil {
		t.Fatalf("NewAttachedRuntime() error = %v", err)
	}
	inherited, err := runtime.inheritedOptions(WithDatabaseID("override-db"))
	if err != nil {
		t.Fatalf("inheritedOptions() error = %v", err)
	}
	if inherited.databaseID != "override-db" {
		t.Fatalf("databaseID = %q, want override-db", inherited.databaseID)
	}
	if inherited.randomDatabaseID || inherited.randomDatabaseIDResolved {
		t.Fatal("explicit WithDatabaseID kept the resolved random-ID flags")
	}
}

func attachedEndpointForBackend(backend Backend) Endpoint {
	if backend == BackendOmni {
		return Endpoint{
			Backend:    BackendOmni,
			URI:        "127.0.0.1:15000",
			ProjectID:  defaultOmniProjectID,
			InstanceID: defaultOmniInstanceID,
		}
	}
	return Endpoint{
		Backend:    BackendEmulator,
		URI:        "127.0.0.1:9010",
		ProjectID:  DefaultProjectID,
		InstanceID: DefaultInstanceID,
	}
}
