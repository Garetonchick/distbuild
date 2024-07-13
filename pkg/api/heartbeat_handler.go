//go:build !solution

package api

import (
	"encoding/json"
	"io"
	"net/http"

	"go.uber.org/zap"
)

type HeartbeatHandler struct {
	l       *zap.SugaredLogger
	service HeartbeatService
}

func NewHeartbeatHandler(l *zap.Logger, s HeartbeatService) *HeartbeatHandler {
	return &HeartbeatHandler{l: l.Sugar(), service: s}
}

func (h *HeartbeatHandler) HandleHeartbeat(w http.ResponseWriter, r *http.Request) {
	var err error
	logEnd := logHelper(h.l, &err, "service", "heartbeat")
	defer logEnd()
	defer r.Body.Close()

	b, err := io.ReadAll(r.Body)
	if err != nil {
		return
	}

	var heartbeatRequest HeartbeatRequest
	err = json.Unmarshal(b, &heartbeatRequest)
	if err != nil {
		return
	}

	heartbeatResponse, err := h.service.Heartbeat(r.Context(), &heartbeatRequest)
	if err != nil {
		w.Header().Set(ErrorHeader, err.Error())
		return
	}
	b, err = json.Marshal(heartbeatResponse)
	if err != nil {
		return
	}
	_, err = w.Write(b)
}

func (h *HeartbeatHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /heartbeat", h.HandleHeartbeat)
}
