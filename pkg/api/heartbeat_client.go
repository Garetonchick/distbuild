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
)

type HeartbeatClient struct {
	l        *zap.SugaredLogger
	endpoint string
}

func NewHeartbeatClient(l *zap.Logger, endpoint string) *HeartbeatClient {
	return &HeartbeatClient{l: l.Sugar(), endpoint: endpoint}
}

func (c *HeartbeatClient) Heartbeat(ctx context.Context, req *HeartbeatRequest) (*HeartbeatResponse, error) {
	var err error
	logEnd := logHelper(c.l, &err, "worker", "heartbeat")
	defer logEnd()

	endpoint, err := url.JoinPath(c.endpoint, "heartbeat")
	if err != nil {
		return nil, err
	}

	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	creq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer creq.Body.Close()

	resp, err := http.DefaultClient.Do(creq)
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
	var heartbeatResponse HeartbeatResponse
	err = json.Unmarshal(b, &heartbeatResponse)
	if err != nil {
		return nil, err
	}
	return &heartbeatResponse, nil
}
