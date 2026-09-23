package spanemuboost

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"cloud.google.com/go/spanner"
	database "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestAttachedConstructorPoliciesSurviveOpenClients(t *testing.T) {
	ctx := t.Context()
	emu, err := RunEmulator(ctx, EnableInstanceAutoConfigOnly())
	if err != nil {
		t.Fatalf("RunEmulator() error = %v", err)
	}
	t.Cleanup(func() {
		if err := emu.Close(); err != nil {
			t.Errorf("emulator Close() error = %v", err)
		}
	})
	endpoint, err := EndpointFromRuntime(emu)
	if err != nil {
		t.Fatalf("EndpointFromRuntime() error = %v", err)
	}
	clientOpts := emu.ClientOptions()

	t.Run("skip teardown keeps fixed database", func(t *testing.T) {
		assertAttachedDatabaseAfterClose(t, ctx, endpoint, clientOpts, codes.OK,
			WithDatabaseID("review-skip-fixed"), SkipSchemaTeardown())
	})
	t.Run("force teardown removes random database", func(t *testing.T) {
		assertAttachedDatabaseAfterClose(t, ctx, endpoint, clientOpts, codes.NotFound,
			WithRandomDatabaseID(), ForceSchemaTeardown())
	})
	t.Run("per-call force overrides constructor skip", func(t *testing.T) {
		attached, err := NewAttachedRuntime(endpoint, WithDatabaseID("review-call-force"), SkipSchemaTeardown())
		if err != nil {
			t.Fatalf("NewAttachedRuntime() error = %v", err)
		}
		clients, err := OpenClients(ctx, attached, ForceSchemaTeardown())
		if err != nil {
			t.Fatalf("OpenClients() error = %v", err)
		}
		dbPath := clients.DatabasePath()
		if err := clients.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
		assertDatabaseCode(t, ctx, clientOpts, dbPath, codes.NotFound)
	})
}

func TestAttachedConstructorClientOptionsReachDataClient(t *testing.T) {
	ctx := t.Context()
	emu, err := RunEmulator(ctx, EnableInstanceAutoConfigOnly())
	if err != nil {
		t.Fatalf("RunEmulator() error = %v", err)
	}
	t.Cleanup(func() {
		if err := emu.Close(); err != nil {
			t.Errorf("emulator Close() error = %v", err)
		}
	})
	endpoint, err := EndpointFromRuntime(emu)
	if err != nil {
		t.Fatalf("EndpointFromRuntime() error = %v", err)
	}

	var calls atomic.Int64
	interceptor := WithClientOptionsForClient(option.WithGRPCDialOption(grpc.WithUnaryInterceptor(
		func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			calls.Add(1)
			return invoker(ctx, method, req, reply, cc, opts...)
		},
	)))
	attached, err := NewAttachedRuntime(endpoint, WithDatabaseID("review-interceptor"), interceptor)
	if err != nil {
		t.Fatalf("NewAttachedRuntime() error = %v", err)
	}
	clients, err := OpenClients(ctx, attached)
	if err != nil {
		t.Fatalf("OpenClients() error = %v", err)
	}
	t.Cleanup(func() { _ = clients.Close() })
	if err := clients.Client.Single().Query(ctx, spanner.NewStatement("SELECT 1")).Do(func(*spanner.Row) error {
		return nil
	}); err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if calls.Load() == 0 {
		t.Fatal("constructor WithClientOptionsForClient interceptor saw 0 calls")
	}
}

func TestAttachedDisabledGuardrailsSurviveDatabaseOverride(t *testing.T) {
	attached, err := NewAttachedRuntime(Endpoint{
		Backend:    BackendOmni,
		URI:        "127.0.0.1:1",
		ProjectID:  "custom-project",
		InstanceID: "custom-instance",
	}, DisableBackendGuardrails())
	if err != nil {
		t.Fatalf("NewAttachedRuntime() error = %v", err)
	}
	_, err = OpenClients(t.Context(), attached, WithDatabaseID("custom-database"))
	if err == nil {
		t.Fatal("OpenClients() error = nil, want dial failure")
	}
	if strings.Contains(err.Error(), "DisableBackendGuardrails") || strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("OpenClients() reapplied guardrails: %v", err)
	}
}

func assertAttachedDatabaseAfterClose(t *testing.T, ctx context.Context, endpoint Endpoint, clientOpts []option.ClientOption, want codes.Code, options ...Option) {
	t.Helper()
	attached, err := NewAttachedRuntime(endpoint, options...)
	if err != nil {
		t.Fatalf("NewAttachedRuntime() error = %v", err)
	}
	clients, err := OpenClients(ctx, attached)
	if err != nil {
		t.Fatalf("OpenClients() error = %v", err)
	}
	dbPath := clients.DatabasePath()
	t.Cleanup(func() {
		dropCtx, cancel := context.WithTimeout(context.Background(), closeTimeout)
		defer cancel()
		admin, err := database.NewDatabaseAdminClient(dropCtx, clientOpts...)
		if err != nil {
			return
		}
		defer func() { _ = admin.Close() }()
		_ = admin.DropDatabase(dropCtx, &databasepb.DropDatabaseRequest{Database: dbPath})
	})
	if err := clients.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	assertDatabaseCode(t, ctx, clientOpts, dbPath, want)
}

func assertDatabaseCode(t *testing.T, ctx context.Context, clientOpts []option.ClientOption, dbPath string, want codes.Code) {
	t.Helper()
	admin, err := database.NewDatabaseAdminClient(ctx, clientOpts...)
	if err != nil {
		t.Fatalf("NewDatabaseAdminClient() error = %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	_, err = admin.GetDatabase(ctx, &databasepb.GetDatabaseRequest{Name: dbPath})
	if got := status.Code(err); got != want {
		t.Fatalf("GetDatabase() code = %s, want %s (err=%v)", got, want, err)
	}
}
