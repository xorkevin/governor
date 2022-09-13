package governor

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog"
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
			Env:    "STDOUT",
			Writer: os.Stdout,
		},
		{
			Env:    "TEST",
			Writer: nil,
		},
		{
			Env:    "bogus",
			Writer: os.Stdout,
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

func TestLevelToZerologLevel(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		Test  string
		Level klog.Level
		Zero  zerolog.Level
	}{
		{
			Test:  "DEBUG",
			Level: klog.LevelDebug,
			Zero:  zerolog.DebugLevel,
		},
		{
			Test:  "INFO",
			Level: klog.LevelInfo,
			Zero:  zerolog.InfoLevel,
		},
		{
			Test:  "WARN",
			Level: klog.LevelWarn,
			Zero:  zerolog.WarnLevel,
		},
		{
			Test:  "ERROR",
			Level: klog.LevelError,
			Zero:  zerolog.ErrorLevel,
		},
		{
			Test:  "bogus",
			Level: 123,
			Zero:  zerolog.InfoLevel,
		},
	} {
		tc := tc
		t.Run(tc.Test, func(t *testing.T) {
			t.Parallel()

			assert := require.New(t)
			assert.Equal(tc.Zero, levelToZerologLevel(tc.Level))
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

			l.Logger.Sublogger("sublog", nil).Log(context.Background(), klog.LevelInfo, 1, "test message 1", tc.Fields)

			d := json.NewDecoder(&logbuf)
			var j struct {
				Level      string `json:"level"`
				Time       string `json:"time"`
				Unixtime   int64  `json:"unixtime"`
				UnixtimeUS int64  `json:"unixtimeus"`
				Caller     string `json:"caller"`
				Path       string `json:"path"`
				Msg        string `json:"msg"`
				TestField  string `json:"some_test_field"`
			}
			assert.NoError(d.Decode(&j))

			assert.Equal(klog.LevelInfo.String(), j.Level)
			ti, err := time.Parse(time.RFC3339Nano, j.Time)
			assert.NoError(err)
			assert.True(ti.After(time.Unix(0, 0)))
			assert.Equal(ti.Unix(), j.Unixtime)
			assert.Equal(ti.UnixMicro(), j.UnixtimeUS)
			assert.Contains(j.Caller, "xorkevin.dev/governor.TestZerologLogger")
			assert.Contains(j.Caller, "xorkevin.dev/governor/govlog_test.go")
			assert.Equal(".sublog", j.Path)
			assert.Equal("test message 1", j.Msg)
			assert.Equal("some test value", j.TestField)
			assert.False(d.More())
		})
	}
}
