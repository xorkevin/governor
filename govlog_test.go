package governor

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"

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

func TestJSONLogger(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		Test  string
		Level string
		Attrs []klog.Attr
	}{
		{
			Test:  "logs fields",
			Level: "INFO",
			Attrs: []klog.Attr{klog.AString("some_test_field", "some test value")},
		},
	} {
		tc := tc
		t.Run(tc.Test, func(t *testing.T) {
			t.Parallel()

			assert := require.New(t)

			var logbuf bytes.Buffer
			l := newLogger(Config{}, configLogger{
				level:  tc.Level,
				output: "TEST",
				writer: &logbuf,
			})

			l.Logger.Sublogger("sublog").Log(context.Background(), klog.LevelInfo, 0, "test message 1", tc.Attrs...)

			d := json.NewDecoder(&logbuf)
			var j struct {
				Level  string `json:"level"`
				Caller struct {
					Fn  string `json:"fn"`
					Src string `json:"src"`
				} `json:"caller"`
				Mod       string `json:"mod"`
				Msg       string `json:"msg"`
				TestField string `json:"some_test_field"`
			}
			assert.NoError(d.Decode(&j))

			assert.Equal(klog.LevelInfo.String(), j.Level)
			assert.Contains(j.Caller.Fn, "xorkevin.dev/governor.TestJSONLogger")
			assert.Contains(j.Caller.Src, "xorkevin.dev/governor/govlog_test.go")
			assert.Equal(".sublog", j.Mod)
			assert.Equal("test message 1", j.Msg)
			assert.Equal("some test value", j.TestField)
			assert.False(d.More())
		})
	}
}

func TestPlaintextLogger(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		Test  string
		Level string
		Attrs []klog.Attr
	}{
		{
			Test:  "logs fields",
			Level: "INFO",
			Attrs: []klog.Attr{klog.AString("some_test_field", "some test value")},
		},
	} {
		tc := tc
		t.Run(tc.Test, func(t *testing.T) {
			t.Parallel()

			assert := require.New(t)

			var logbuf bytes.Buffer
			l := newPlaintextLogger(configLogger{
				level:  tc.Level,
				output: "TEST",
				writer: &logbuf,
			})

			l.Logger.Sublogger("sublog").Log(context.Background(), klog.LevelInfo, 0, "test message 1", tc.Attrs...)

			t.Log(logbuf.String())
			assert.True(logbuf.Len() > 0)
		})
	}
}
