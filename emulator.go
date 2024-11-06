package main

import (
	"context"
	"fmt"
	"log"

	"github.com/testcontainers/testcontainers-go"

	database "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	instance "cloud.google.com/go/spanner/admin/instance/apiv1"
	"cloud.google.com/go/spanner/admin/instance/apiv1/instancepb"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/samber/lo"
	"github.com/testcontainers/testcontainers-go/modules/gcloud"
)

type noopLogger struct{}

// Printf implements testcontainers.Logging.
func (n noopLogger) Printf(string, ...interface{}) {
}

func newEmulator(ctx context.Context, opts spannerOptions) (container *gcloud.GCloudContainer, teardown func(), err error) {
	// Workaround to suppress log output with `-v`.
	testcontainers.Logger = &noopLogger{}
	container, err = gcloud.RunSpanner(ctx, lo.CoalesceOrEmpty(opts.EmulatorImage, defaultEmulatorImage), testcontainers.WithLogger(&noopLogger{}))
	if err != nil {
		return nil, nil, err
	}
	return container, func() {
		err := container.Terminate(ctx)
		if err != nil {
			log.Printf("failed to terminate Cloud Spanner Emulator: %v", err)
		}
	}, nil
}

func setUpEmptyInstanceAndDatabaseForEmulator(ctx context.Context, sysVars *systemVariables) error {
	clientOpts := sliceOf(
		option.WithEndpoint(sysVars.Endpoint),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	)

	instanceCli, err := instance.NewInstanceAdminClient(ctx, clientOpts...)
	if err != nil {
		return err
	}

	createInstanceOp, err := instanceCli.CreateInstance(ctx, &instancepb.CreateInstanceRequest{
		Parent:     sysVars.ProjectPath(),
		InstanceId: sysVars.Instance,
		Instance: &instancepb.Instance{
			Name:        sysVars.InstancePath(),
			Config:      "emulator-config",
			DisplayName: sysVars.Instance,
		},
	})
	if err != nil {
		return err
	}
	_, err = createInstanceOp.Wait(ctx)
	if err != nil {
		return err
	}

	dbCli, err := database.NewDatabaseAdminClient(ctx, clientOpts...)
	if err != nil {
		return err
	}

	createDBOp, err := dbCli.CreateDatabase(ctx, &databasepb.CreateDatabaseRequest{
		Parent:          sysVars.InstancePath(),
		CreateStatement: fmt.Sprintf("CREATE DATABASE `%v`", sysVars.Database),
	})
	if err != nil {
		return err
	}

	_, err = createDBOp.Wait(ctx)
	if err != nil {
		return err
	}

	return nil
}
