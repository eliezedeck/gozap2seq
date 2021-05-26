package gozap2seq

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/tidwall/gjson"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type LogInjector struct {
	client        *http.Client
	sequrl        string
	seqtoken      string
	consolelogger *zap.Logger // this is used in case of SEQ error
	wg            *sync.WaitGroup
}

func NewLogInjector(sequrl, token string) (*LogInjector, error) {
	pu, err := url.Parse(sequrl)
	if err != nil {
		return nil, err
	}
	if pu.Hostname() == "" {
		return nil, errors.New("invalid hostname in SEQ URL")
	}

	furl := pu.Scheme + "://" + pu.Hostname() + ":" + pu.Port()
	if pu.Port() == "" {
		furl += "5341"
	}

	return &LogInjector{
		client:   &http.Client{},
		sequrl:   furl,
		seqtoken: strings.TrimSpace(token),
		wg:       &sync.WaitGroup{},
	}, nil
}

func (i *LogInjector) Build(zapconfig zap.Config) *zap.Logger {
	// Create a console logger that will be used if SEQ fails
	consoleencoder := zapcore.NewConsoleEncoder(zapconfig.EncoderConfig)
	stderrsync := zapcore.Lock(os.Stderr)
	i.consolelogger = zap.New(zapcore.NewCore(consoleencoder, stderrsync, zapconfig.Level.Level()))

	configcopy := zapconfig

	// SEQ requires that the fields and value format follow these rules
	configcopy.EncoderConfig.EncodeTime = zapcore.RFC3339NanoTimeEncoder
	configcopy.EncoderConfig.EncodeLevel = zapcore.LowercaseLevelEncoder
	configcopy.EncoderConfig.LevelKey = "@l"
	configcopy.EncoderConfig.TimeKey = "@t"
	configcopy.EncoderConfig.MessageKey = "@mt"
	configcopy.EncoderConfig.CallerKey = "caller"
	configcopy.EncoderConfig.StacktraceKey = "trace"

	jsonencoder := zapcore.NewJSONEncoder(configcopy.EncoderConfig)
	seqsync := zapcore.AddSync(i)

	return zap.New(zapcore.NewCore(jsonencoder, seqsync, configcopy.Level.Level()),
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.PanicLevel))
}

func (i *LogInjector) Write(p []byte) (n int, err error) {
	i.wg.Add(1)

	// Since we immediately return, we need to make a copy of the payload that takes time to be sent
	pcopy := make([]byte, len(p))
	copy(pcopy, p)

	req, err := http.NewRequest("POST", i.sequrl+"/api/events/raw", bytes.NewBuffer(pcopy))
	if err != nil {
		return 0, err
	}

	if i.seqtoken != "" {
		req.Header.Set("X-Seq-ApiKey", i.seqtoken)
	}
	req.Header.Set("Content-Type", "application/vnd.serilog.clef")

	go func() {
		defer i.wg.Done()

		// Get the response
		resp, err := i.client.Do(req)
		if err != nil {
			i.consolelogger.Error("Failed reading SEQ response", zap.Error(err))
			return
		}
		defer resp.Body.Close()

		// The status is supposed to be 201 (Created)
		if resp.StatusCode != 201 {
			// Parse the JSON message
			content, err := io.ReadAll(resp.Body)
			if err != nil {
				i.consolelogger.Error("Failed reading SEQ body", zap.Error(err))
				return
			}
			value := gjson.GetBytes(content, "Error")
			i.consolelogger.Error("SEQ error", zap.String("message", value.String()))
			return
		}
	}()

	return len(p), nil // always success (but it might have failed)
}

func (i *LogInjector) Wait() {
	runtime.Gosched()
	i.wg.Wait()
}
