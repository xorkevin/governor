package governor

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"xorkevin.dev/klog"
)

func TestLogOutputFromString(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		Env    string
		Writer io.Writer
	}{
		{
			Env:    "STDERR",
			Writer: os.Stderr,
		},
		{
			Env:    "STDOUT",
			Writer: os.Stdout,
		},
		{
			Env:    "TEST",
			Writer: nil,
		},
		{
			Env:    "bogus",
			Writer: os.Stderr,
		},
	} {
		tc := tc
		t.Run(tc.Env, func(t *testing.T) {
			t.Parallel()

			assert := require.New(t)
			assert.Equal(tc.Writer, logOutputFromString(tc.Env))
		})
	}
}

func TestZerologLogger(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		Test   string
		Level  string
		Fields klog.Fields
	}{
		{
			Test:  "logs fields",
			Level: "INFO",
			Fields: klog.Fields{
				"some_test_field": "some test value",
			},
		},
	} {
		tc := tc
		t.Run(tc.Test, func(t *testing.T) {
			t.Parallel()

			assert := require.New(t)

			logbuf := bytes.Buffer{}
			l := newLogger(Config{
				logLevel:  tc.Level,
				logOutput: "TEST",
				logWriter: &logbuf,
			})

			klog.Sub(l.Logger, "sublog", nil).Log(context.Background(), klog.LevelInfo, "", 1, "test message 1", tc.Fields)

			d := json.NewDecoder(&logbuf)
			var j struct {
				Level          string `json:"level"`
				Time           string `json:"time"`
				UnixtimeUS     int64  `json:"unixtimeus"`
				MonoTime       string `json:"monotime"`
				MonoUnixtimeUS int64  `json:"monounixtimeus"`
				Caller         string `json:"caller"`
				Path           string `json:"path"`
				Msg            string `json:"msg"`
				TestField      string `json:"some_test_field"`
			}
			assert.NoError(d.Decode(&j))

			assert.Equal(klog.LevelInfo.String(), j.Level)
			ti, err := time.Parse(time.RFC3339Nano, j.Time)
			assert.NoError(err)
			assert.True(ti.After(time.Unix(0, 0)))
			assert.Equal(ti.UnixMicro(), j.UnixtimeUS)
			mt, err := time.Parse(time.RFC3339Nano, j.MonoTime)
			assert.NoError(err)
			assert.True(mt.After(time.Unix(0, 0)))
			assert.Equal(mt.UnixMicro(), j.MonoUnixtimeUS)
			assert.Contains(j.Caller, "xorkevin.dev/governor.TestZerologLogger")
			assert.Contains(j.Caller, "xorkevin.dev/governor/govlog_test.go")
			assert.Equal(".sublog", j.Path)
			assert.Equal("test message 1", j.Msg)
			assert.Equal("some test value", j.TestField)
			assert.False(d.More())
		})
	}
}
