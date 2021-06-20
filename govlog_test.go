package governor

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestEnvToLevel(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		Env   string
		Level int
	}{
		{
			Env:   "DEBUG",
			Level: levelDebug,
		},
		{
			Env:   "INFO",
			Level: levelInfo,
		},
		{
			Env:   "WARN",
			Level: levelWarn,
		},
		{
			Env:   "ERROR",
			Level: levelError,
		},
		{
			Env:   "FATAL",
			Level: levelFatal,
		},
		{
			Env:   "PANIC",
			Level: levelPanic,
		},
		{
			Env:   "bogus",
			Level: levelInfo,
		},
	} {
		t.Run(tc.Env, func(t *testing.T) {
			tc := tc
			t.Parallel()

			assert := require.New(t)
			assert.Equal(tc.Level, envToLevel(tc.Env))
		})
	}
}

func TestEnvToLogOutput(t *testing.T) {
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
			Env:    "bogus",
			Writer: os.Stdout,
		},
	} {
		t.Run(tc.Env, func(t *testing.T) {
			tc := tc
			t.Parallel()

			assert := require.New(t)
			assert.Equal(tc.Writer, envToLogOutput(tc.Env))
		})
	}
}

func TestLevelToZerologLevel(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		Test  string
		Level int
		Zero  zerolog.Level
	}{
		{
			Test:  "DEBUG",
			Level: levelDebug,
			Zero:  zerolog.DebugLevel,
		},
		{
			Test:  "INFO",
			Level: levelInfo,
			Zero:  zerolog.InfoLevel,
		},
		{
			Test:  "WARN",
			Level: levelWarn,
			Zero:  zerolog.WarnLevel,
		},
		{
			Test:  "ERROR",
			Level: levelError,
			Zero:  zerolog.ErrorLevel,
		},
		{
			Test:  "FATAL",
			Level: levelFatal,
			Zero:  zerolog.FatalLevel,
		},
		{
			Test:  "PANIC",
			Level: levelPanic,
			Zero:  zerolog.PanicLevel,
		},
		{
			Test:  "bogus",
			Level: 123,
			Zero:  zerolog.InfoLevel,
		},
	} {
		t.Run(tc.Test, func(t *testing.T) {
			tc := tc
			t.Parallel()

			assert := require.New(t)
			assert.Equal(tc.Zero, levelToZerologLevel(tc.Level))
		})
	}
}

func TestLogger(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		Test    string
		Env     string
		Subtree []string
		Data    map[string]string
		Count   int
	}{
		{
			Test:  "ignores debug logs at the info level",
			Env:   "INFO",
			Count: 3,
		},
		{
			Test:  "ignores info logs at the warn level",
			Env:   "WARN",
			Count: 2,
		},
		{
			Test:  "ignores warn logs at the error level",
			Env:   "ERROR",
			Count: 1,
		},
		{
			Test:    "logs in a subtree",
			Env:     "INFO",
			Subtree: []string{"test", "subtree"},
			Count:   3,
		},
		{
			Test:    "logs additional data",
			Env:     "INFO",
			Subtree: []string{"subtree"},
			Data: map[string]string{
				"some_test_field": "some test value",
			},
			Count: 3,
		},
	} {
		t.Run(tc.Test, func(t *testing.T) {
			tc := tc
			t.Parallel()

			assert := require.New(t)

			logbuf := bytes.Buffer{}
			l := newLogger(Config{
				logLevel:  envToLevel(tc.Env),
				logOutput: &logbuf,
			})

			modulename := "root"
			if len(tc.Subtree) != 0 {
				modulename = strings.Join(tc.Subtree, ".")
				for _, i := range tc.Subtree {
					l = l.Subtree(i)
				}
			}

			if len(tc.Data) != 0 {
				l = l.WithData(tc.Data)
			}

			l.Debug("test message 0", map[string]string{
				"test_field": "test value 0",
			})
			l.Info("test message 1", map[string]string{
				"test_field": "test value 1",
			})
			l.Warn("test message 2", map[string]string{
				"test_field": "test value 2",
			})
			l.Error("test message 3", map[string]string{
				"test_field": "test value 3",
			})
			total := 4
			s := strings.Split(strings.TrimSpace(logbuf.String()), "\n")
			assert.Len(s, tc.Count)

			offset := total - tc.Count
			for n, i := range s {
				msgnum := strconv.Itoa(offset + n)
				logjson := struct {
					Level     string `json:"level"`
					Module    string `json:"module"`
					Msg       string `json:"msg"`
					Time      string `json:"time"`
					UnixTime  string `json:"unixtime"`
					TestField string `json:"test_field"`
				}{}
				assert.NoError(json.Unmarshal([]byte(i), &logjson))
				assert.Equal(modulename, logjson.Module)
				assert.Equal("test message "+msgnum, logjson.Msg)
				ti, err := time.Parse(time.RFC3339, logjson.Time)
				assert.NoError(err)
				assert.True(ti.After(time.Unix(0, 0)))
				ut, err := strconv.ParseInt(logjson.UnixTime, 10, 64)
				assert.NoError(err)
				assert.True(time.Unix(ut, 0).After(time.Unix(0, 0)))
				assert.Equal("test value "+msgnum, logjson.TestField)
				jsonmap := map[string]string{}
				assert.NoError(json.Unmarshal([]byte(i), &jsonmap))
				for k, v := range tc.Data {
					assert.Equal(v, jsonmap[k])
				}
			}
		})
	}
}
