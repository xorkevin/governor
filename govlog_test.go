package governor

import (
	"bytes"
	"encoding/json"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"os"
	"strings"
	"testing"
)

func TestEnvToLevel(t *testing.T) {
	assert := assert.New(t)

	assert.Equal(levelDebug, envToLevel("DEBUG"))
	assert.Equal(levelInfo, envToLevel("INFO"))
	assert.Equal(levelWarn, envToLevel("WARN"))
	assert.Equal(levelError, envToLevel("ERROR"))
	assert.Equal(levelFatal, envToLevel("FATAL"))
	assert.Equal(levelPanic, envToLevel("PANIC"))
	assert.Equal(levelInfo, envToLevel("bogus"), "default level should be info")
}

func TestEnvToLogOutput(t *testing.T) {
	assert := assert.New(t)

	assert.Equal(os.Stdout, envToLogOutput("STDOUT"))
	assert.Equal(os.Stdout, envToLogOutput("bogus"))
}

func TestLogrusLevelToLog(t *testing.T) {
	assert := assert.New(t)

	assert.Equal(logrus.DebugLevel, logrusLevelToLog(levelDebug))
	assert.Equal(logrus.InfoLevel, logrusLevelToLog(levelInfo))
	assert.Equal(logrus.WarnLevel, logrusLevelToLog(levelWarn))
	assert.Equal(logrus.ErrorLevel, logrusLevelToLog(levelError))
	assert.Equal(logrus.FatalLevel, logrusLevelToLog(levelFatal))
	assert.Equal(logrus.PanicLevel, logrusLevelToLog(levelPanic))
	assert.Equal(logrus.InfoLevel, logrusLevelToLog(123), "default level should be info")
}

func TestNewLogger(t *testing.T) {
	assert := assert.New(t)

	{
		logbuf := bytes.Buffer{}
		config := Config{
			LogLevel:  envToLevel("INFO"),
			LogOutput: &logbuf,
		}
		l := NewLogger(config)

		k := l.(*govlogger)
		assert.Equal(levelInfo, k.level, "log level should be set")
		assert.Equal(&logbuf, k.logger.Out, "log output should be set from the config")
		assert.IsType(&logrus.JSONFormatter{}, k.logger.Formatter, "log format should be JSON when not DEBUG")
	}

	{
		logbuf := bytes.Buffer{}
		config := Config{
			LogLevel:  envToLevel("DEBUG"),
			LogOutput: &logbuf,
		}
		l := NewLogger(config)

		k := l.(*govlogger)
		assert.Equal(levelDebug, k.level, "log level should be set")
		assert.Equal(&logbuf, k.logger.Out, "log output should be set from the config")
		assert.IsType(&logrus.TextFormatter{}, k.logger.Formatter, "log format should be text when DEBUG")
	}
}

func TestLogger_Log(t *testing.T) {
	assert := assert.New(t)

	{
		logbuf := bytes.Buffer{}
		config := Config{
			LogLevel:  envToLevel("INFO"),
			LogOutput: &logbuf,
		}
		l := NewLogger(config)

		l.Debug("test message 1", map[string]string{
			"test field 1": "test value 1",
		})
		l.Info("test message 2", map[string]string{
			"test field 2": "test value 2",
		})
		l.Warn("test message 3", map[string]string{
			"test field 3": "test value 3",
		})
		l.Error("test message 4", map[string]string{
			"test field 4": "test value 4",
		})

		s := strings.Split(strings.TrimSpace(logbuf.String()), "\n")
		assert.Equal(3, len(s), "only messages above warn should be logged")

		{
			logjson := map[string]interface{}{}
			err := json.Unmarshal([]byte(s[0]), &logjson)
			assert.NoError(err, "log output must be json format")
			assert.Equal("test message 2", logjson["msg"], "message should be logged")
			assert.Equal("test value 2", logjson["test field 2"], "message fields should be logged")
		}

		{
			logjson := map[string]interface{}{}
			err := json.Unmarshal([]byte(s[1]), &logjson)
			assert.NoError(err, "log output must be json format")
			assert.Equal("test message 3", logjson["msg"], "message should be logged")
			assert.Equal("test value 3", logjson["test field 3"], "message fields should be logged")
		}

		{
			logjson := map[string]interface{}{}
			err := json.Unmarshal([]byte(s[2]), &logjson)
			assert.NoError(err, "log output must be json format")
			assert.Equal("test message 4", logjson["msg"], "message should be logged")
			assert.Equal("test value 4", logjson["test field 4"], "message fields should be logged")
		}
	}

	{
		logbuf := bytes.Buffer{}
		config := Config{
			LogLevel:  envToLevel("WARN"),
			LogOutput: &logbuf,
		}
		l := NewLogger(config)

		l.Debug("test message 1", map[string]string{
			"test field 1": "test value 1",
		})
		l.Info("test message 2", map[string]string{
			"test field 2": "test value 2",
		})
		l.Warn("test message 3", map[string]string{
			"test field 3": "test value 3",
		})
		l.Error("test message 4", map[string]string{
			"test field 4": "test value 4",
		})

		s := strings.Split(strings.TrimSpace(logbuf.String()), "\n")
		assert.Equal(2, len(s), "only messages above warn should be logged")

		{
			logjson := map[string]interface{}{}
			err := json.Unmarshal([]byte(s[0]), &logjson)
			assert.NoError(err, "log output must be json format")
			assert.Equal("test message 3", logjson["msg"], "message should be logged")
			assert.Equal("test value 3", logjson["test field 3"], "message fields should be logged")
		}

		{
			logjson := map[string]interface{}{}
			err := json.Unmarshal([]byte(s[1]), &logjson)
			assert.NoError(err, "log output must be json format")
			assert.Equal("test message 4", logjson["msg"], "message should be logged")
			assert.Equal("test value 4", logjson["test field 4"], "message fields should be logged")
		}
	}
}