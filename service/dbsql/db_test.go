package dbsql

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/governortest"
	"xorkevin.dev/klog"
)

func TestService(t *testing.T) {
	t.Parallel()

	assert := require.New(t)

	server := governortest.NewTestServer(strings.NewReader(`
{
  "database": {
    "auth": "dbauth",
    "dbname": "`+os.Getenv("GOV_TEST_POSTGRES_DB")+`",
    "host": "`+os.Getenv("GOV_TEST_POSTGRES_HOST")+`",
    "port": "`+os.Getenv("GOV_TEST_POSTGRES_PORT")+`"
  }
}
`), strings.NewReader(`
{
  "data": {
    "dbauth": {
      "username": "`+os.Getenv("GOV_TEST_POSTGRES_USERNAME")+`",
      "password": "`+os.Getenv("GOV_TEST_POSTGRES_PASSWORD")+`"
    }
  }
}
`))

	d := New()
	server.Register("database", "/null/db", d)
	assert.NoError(server.Init(context.Background(), governor.Flags{}, klog.Discard{}))

	t.Cleanup(func() {
		server.Stop(context.Background())
	})

	db, err := d.DB(context.Background())
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
		}()
	}

	{
		var res string
		assert.NoError(db.QueryRowContext(context.Background(), "SELECT $1;", "ok").Scan(&res))
		assert.Equal("ok", res)
	}
}
