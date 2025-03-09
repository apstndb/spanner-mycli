package main

import (
	"context"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/apstndb/spanemuboost"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func createTestSession(t *testing.T, endpoint string) *Session {
	ctx := context.Background()
	sysVars := &systemVariables{
		Project:  "test-project",
		Instance: "test-instance",
		Database: "test-database",
	}

	opts := []option.ClientOption{
		option.WithEndpoint(endpoint),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	}

	session, err := NewSession(ctx, sysVars, opts...)
	if err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}
	return session
}

func TestSession_queryOptions(t *testing.T) {
	emulator, teardown, err := spanemuboost.NewEmulator(context.Background(),
		spanemuboost.WithProjectID("test-project"),
		spanemuboost.WithInstanceID("test-instance"),
		spanemuboost.WithDatabaseID("test-database"),
	)
	if err != nil {
		t.Fatalf("failed to start emulator: %v", err)
	}
	defer teardown()

	session := createTestSession(t, emulator.URI)
	mode := sppb.ExecuteSqlRequest_PROFILE
	opts := session.queryOptions(&mode)

	if opts.Priority != session.systemVariables.RPCPriority {
		t.Errorf("Expected priority to be %v, but got %v", session.systemVariables.RPCPriority, opts.Priority)
	}
	if opts.Options.OptimizerVersion != session.systemVariables.OptimizerVersion {
		t.Errorf("Expected OptimizerVersion to be %v, but got %v", session.systemVariables.OptimizerVersion, opts.Options.OptimizerVersion)
	}
	if opts.Options.OptimizerStatisticsPackage != session.systemVariables.OptimizerStatisticsPackage {
		t.Errorf("Expected OptimizerStatisticsPackage to be %v, but got %v", session.systemVariables.OptimizerStatisticsPackage, opts.Options.OptimizerStatisticsPackage)
	}
}
