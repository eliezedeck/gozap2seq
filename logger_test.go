package gozap2seq

import (
	"errors"
	"testing"

	"go.uber.org/zap"
)

func TestBadUrl(t *testing.T) {
	_, err := NewLogInjector("http://///////", "boooo")
	if err == nil {
		t.Error("not handling bad URL properly")
	}
}

func TestInjectionIntegration(t *testing.T) {
	injector, err := NewLogInjector("http://localhost:5341", "")
	if err != nil {
		t.Error(err)
	}

	loggerConfig := zap.NewDevelopmentConfig()
	logger := injector.Build(loggerConfig)

	go logger.Debug("Debug message", zap.String("level", "debug"), zap.Bool("ok", true))
	go logger.Info("Info message", zap.String("level", "info"), zap.Binary("binary", []byte("hello")), zap.String("original", "hello"))
	go logger.Warn("Warning message", zap.String("newline", "{\n    \"hello\": \"world\"\n}"))
	go logger.Error("Error message", zap.Error(errors.New("oh no!")))

	injector.Wait()
}
