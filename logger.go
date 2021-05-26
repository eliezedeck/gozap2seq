package gozap2seq

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"sync"

	"github.com/tidwall/gjson"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type LogInjector struct {
	client   *http.Client
	sequrl   string
	seqtoken string
	wg       *sync.WaitGroup
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
		seqtoken: token,
		wg:       &sync.WaitGroup{},
	}, nil
}

func (i *LogInjector) Build(zapconfig zap.Config) *zap.Logger {
	// SEQ requires that the fields and value format follow these rules
	zapconfig.EncoderConfig.EncodeTime = zapcore.RFC3339NanoTimeEncoder
	zapconfig.EncoderConfig.EncodeLevel = zapcore.LowercaseLevelEncoder
	zapconfig.EncoderConfig.LevelKey = "@l"
	zapconfig.EncoderConfig.TimeKey = "@t"
	zapconfig.EncoderConfig.MessageKey = "@mt"

	jsonencoder := zapcore.NewJSONEncoder(zapconfig.EncoderConfig)
	seqsync := zapcore.AddSync(i)

	return zap.New(zapcore.NewCore(jsonencoder, seqsync, zapconfig.Level.Level()))
}

func (i *LogInjector) Write(p []byte) (n int, err error) {
	i.wg.Add(1)
	defer i.wg.Done()

	req, err := http.NewRequest("POST", i.sequrl+"/api/events/raw", bytes.NewBuffer(p))
	if err != nil {
		return 0, err
	}

	if i.seqtoken != "" {
		req.Header.Set("X-Seq-ApiKey", i.seqtoken)
	}
	req.Header.Set("Content-Type", "application/vnd.serilog.clef")

	// Get the response
	resp, err := i.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	// The status is supposed to be 201 (Created)
	if resp.StatusCode != 201 {
		// Parse the JSON message
		content, err := io.ReadAll(resp.Body)
		if err != nil {
			return 0, err
		}
		value := gjson.GetBytes(content, "Error")
		return 0, errors.New("SEQ error: " + value.String()) // error (with message)
	}
	return len(p), nil // success
}

func (i *LogInjector) Wait() {
	runtime.Gosched()
	i.wg.Wait()
}
