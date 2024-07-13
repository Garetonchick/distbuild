//go:build !solution

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"go.uber.org/zap"

	"github.com/Garetonchick/distbuild/pkg/build"
)

type JSONStreamReader interface {
	Next() bool
	Message(v any) error
	Err() error
}

type jsonStreamReaderImpl struct {
	dec   *json.Decoder
	first bool
	err   error
}

func NewJSONStreamReader(r io.Reader) JSONStreamReader {
	return &jsonStreamReaderImpl{dec: json.NewDecoder(r), first: true}
}

func (s *jsonStreamReaderImpl) Next() bool {
	if s.first {
		s.first = false
		_, err := s.dec.Token() // read [
		if err != nil {
			s.err = err
			return false
		}
	}

	more := s.dec.More()

	if !more {
		_, err := s.dec.Token() // read ]
		if err != nil {
			s.err = err
		}
	}
	return more
}

func (s *jsonStreamReaderImpl) Message(v any) error {
	return s.dec.Decode(v)
}

func (s *jsonStreamReaderImpl) Err() error {
	return s.err
}

type BuildClient struct {
	l        *zap.SugaredLogger
	endpoint string
}

type statusReader struct {
	close  io.Closer
	stream JSONStreamReader
}

func (s *statusReader) Close() error {
	return s.close.Close()
}

func (s *statusReader) Next() (*StatusUpdate, error) {
	next := s.stream.Next()
	if !next {
		if err := s.stream.Err(); err != nil {
			return nil, err
		}
	}
	var statusUpdate StatusUpdate
	if err := s.stream.Message(&statusUpdate); err != nil {
		return nil, err
	}
	return &statusUpdate, nil
}

func NewBuildClient(l *zap.Logger, endpoint string) *BuildClient {
	return &BuildClient{l: l.Sugar(), endpoint: endpoint}
}

func (c *BuildClient) StartBuild(ctx context.Context, request *BuildRequest) (*BuildStarted, StatusReader, error) {
	var err error
	logEnd := logHelper(c.l, &err, "client", "start build")
	defer logEnd()

	endpoint, err := url.JoinPath(c.endpoint, "build")
	if err != nil {
		return nil, nil, err
	}
	b, err := json.Marshal(request)
	if err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return nil, nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	if errS := resp.Header.Get(ErrorHeader); errS != "" {
		return nil, nil, fmt.Errorf(errS)
	}

	stream := NewJSONStreamReader(resp.Body)

	var buildStarted BuildStarted
	readBuildStarted := func() error {
		defer func() {
			err := recover()
			if err != nil {
				resp.Body.Close()
				panic(err)
			}
		}()
		if !stream.Next() {
			if err = stream.Err(); err != nil {
				return err
			}
			return fmt.Errorf("response is empty")
		}

		if err = stream.Message(&buildStarted); err != nil {
			return err
		}
		return nil
	}

	if err = readBuildStarted(); err != nil {
		resp.Body.Close()
		return nil, nil, err
	}

	return &buildStarted, &statusReader{stream: stream, close: resp.Body}, nil
}

func (c *BuildClient) SignalBuild(ctx context.Context, buildID build.ID, signal *SignalRequest) (*SignalResponse, error) {
	var err error
	logEnd := logHelper(c.l, &err, "client", "signal build")
	defer logEnd()

	endpointNoParams, err := url.JoinPath(c.endpoint, "signal")
	if err != nil {
		return nil, err
	}
	endpoint, err := url.Parse(endpointNoParams)
	if err != nil {
		return nil, err
	}
	qValues := endpoint.Query()
	qValues.Set("build_id", buildID.String())
	endpoint.RawQuery = qValues.Encode()

	b, err := json.Marshal(signal)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if errS := resp.Header.Get(ErrorHeader); errS != "" {
		return nil, fmt.Errorf(errS)
	}

	b, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var signalResponse SignalResponse
	err = json.Unmarshal(b, &signalResponse)
	if err != nil {
		return nil, err
	}
	return &signalResponse, err
}
