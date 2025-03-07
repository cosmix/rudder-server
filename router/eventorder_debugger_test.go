package router

import (
	"context"
	"testing"

	"github.com/ory/dockertest/v3"
	"github.com/stretchr/testify/require"

	"github.com/rudderlabs/rudder-go-kit/config"
	"github.com/rudderlabs/rudder-go-kit/testhelper/docker/resource"
	"github.com/rudderlabs/rudder-server/jobsdb"
	migrator "github.com/rudderlabs/rudder-server/services/sql-migrator"
)

func TestEventOrderDebugInfo(t *testing.T) {
	pool, err := dockertest.NewPool("")
	require.NoError(t, err)
	postgres, err := resource.SetupPostgres(pool, t)
	require.NoError(t, err)

	m := &migrator.Migrator{
		Handle:                     postgres.DB,
		MigrationsTable:            "node_migrations",
		ShouldForceSetLowerVersion: config.GetBool("SQLMigrator.forceSetLowerVersion", true),
	}
	require.NoError(t, m.Migrate("node"))

	jdb := jobsdb.NewForReadWrite("rt", jobsdb.WithDBHandle(postgres.DB))
	require.NoError(t, jdb.Start())
	defer jdb.Stop()

	err = jdb.Store(context.Background(), []*jobsdb.JobT{{
		UserID:       "user1",
		WorkspaceId:  "workspace1",
		Parameters:   []byte(`{"destination_id": "destination1"}`),
		EventCount:   1,
		EventPayload: []byte(`{"type": "track", "event": "test_event", "properties": {"key": "value"}}`),
		CustomVal:    "dummy",
	}})
	require.NoError(t, err)

	jobs, err := jdb.GetJobs(context.Background(), []string{jobsdb.Unprocessed.State}, jobsdb.GetQueryParams{JobsLimit: 1})
	require.NoError(t, err)
	require.Len(t, jobs.Jobs, 1)
	job := jobs.Jobs[0]
	require.NoError(t, jdb.UpdateJobStatus(context.Background(), []*jobsdb.JobStatusT{{
		WorkspaceId:   "workspace1",
		JobID:         job.JobID,
		JobState:      jobsdb.Executing.State,
		ErrorResponse: []byte("{}"),
		Parameters:    []byte(`{}`),
		JobParameters: []byte(`{"destination_id", "destination1"}`),
	}}, nil, nil))
	require.NoError(t, jdb.UpdateJobStatus(context.Background(), []*jobsdb.JobStatusT{{
		WorkspaceId:   "workspace1",
		JobID:         job.JobID,
		JobState:      jobsdb.Succeeded.State,
		ErrorResponse: []byte("{}"),
		Parameters:    []byte(`{}`),
		JobParameters: []byte(`{"destination_id", "destination1"}`),
	}}, nil, nil))

	rt := &Handle{
		jobsDB: jdb,
	}

	debugInfo := rt.eventOrderDebugInfo("user1:destination1")
	require.Equal(t,
		` |  id| job_id| job_state| attempt|                     exec_time| error_code| parameters| error_response|
 | ---|    ---|       ---|     ---|                           ---|        ---|        ---|            ---|
 |   1|      1| executing|       0| 0001-01-01 00:00:00 +0000 UTC|           |         {}|             {}|
 |   2|      1| succeeded|       0| 0001-01-01 00:00:00 +0000 UTC|           |         {}|             {}|
`, debugInfo)
}
