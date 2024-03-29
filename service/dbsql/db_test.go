package dbsql_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/governortest"
	"xorkevin.dev/governor/service/dbsql"
	"xorkevin.dev/governor/service/dbsql/dbsqltest"
	"xorkevin.dev/klog"
)

func TestDatabase(t *testing.T) {
	if testing.Short() {
		t.Skip("relies on db")
	}

	t.Parallel()

	assert := require.New(t)

	server := governortest.NewTestServer(t, map[string]any{
		"database": map[string]any{
			"auth":   "dbauth",
			"dbname": os.Getenv("GOV_TEST_POSTGRES_DB"),
			"host":   os.Getenv("GOV_TEST_POSTGRES_HOST"),
			"port":   os.Getenv("GOV_TEST_POSTGRES_PORT"),
		},
	}, map[string]any{
		"data": map[string]any{
			"dbauth": map[string]any{
				"username": os.Getenv("GOV_TEST_POSTGRES_USERNAME"),
				"password": os.Getenv("GOV_TEST_POSTGRES_PASSWORD"),
			},
		},
	}, nil)

	d := dbsql.New()
	server.Register("database", "/null/db", d)
	assert.NoError(server.Start(context.Background(), governor.Flags{}, klog.Discard{}))

	static := dbsqltest.NewStatic(t)

	for _, tc := range []struct {
		Name string
		DB   dbsql.Database
	}{
		{
			Name: "Service",
			DB:   d,
		},
		{
			Name: "Static",
			DB:   static,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			assert := require.New(t)

			db, err := tc.DB.DB(context.Background())
			assert.NoError(err)

			assert.NoError(db.PingContext(context.Background()))
			{
				_, err = db.ExecContext(context.Background(), "SELECT $1;", "ok")
				assert.NoError(err)
			}
			{
				func() {
					rows, err := db.QueryContext(context.Background(), "SELECT * FROM (VALUES ($1), ($2)) AS t(id);", "r1", "r2")
					assert.NoError(err)
					defer func() {
						assert.NoError(rows.Close())
					}()
					var res []string
					for rows.Next() {
						var r string
						assert.NoError(rows.Scan(&r))
						res = append(res, r)
					}
					assert.NoError(rows.Err())
					assert.Equal([]string{"r1", "r2"}, res)
				}()
			}
			{
				var res string
				assert.NoError(db.QueryRowContext(context.Background(), "SELECT $1;", "ok").Scan(&res))
				assert.Equal("ok", res)
			}
		})
	}
}
