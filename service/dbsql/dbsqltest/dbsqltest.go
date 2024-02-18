package dbsqltest

import (
	"fmt"
	"os"
	"testing"

	"xorkevin.dev/governor/service/dbsql"
	"xorkevin.dev/klog"
)

func NewStatic(t testing.TB) *dbsql.Static {
	t.Helper()
	s, err := dbsql.NewStatic(
		fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=disable",
			os.Getenv("GOV_TEST_POSTGRES_USERNAME"),
			os.Getenv("GOV_TEST_POSTGRES_PASSWORD"),
			os.Getenv("GOV_TEST_POSTGRES_HOST"),
			os.Getenv("GOV_TEST_POSTGRES_PORT"),
			os.Getenv("GOV_TEST_POSTGRES_DB"),
		),
		klog.Discard{},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}
