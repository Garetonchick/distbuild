//go:build !solution

package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/Garetonchick/distbuild/pkg/build"
	"go.uber.org/zap"
)

type JSONStreamWriter interface {
	SendMessage(v any) error
	Close() error
}

type jsonStreamWriterImpl struct {
	w     http.ResponseWriter
	rc    *http.ResponseController
	first bool
}

func NewJSONStreamWriter(w http.ResponseWriter) JSONStreamWriter {
	return &jsonStreamWriterImpl{w: w, rc: http.NewResponseController(w), first: true}
}

func (sw *jsonStreamWriterImpl) SendMessage(v any) error {
	if sw.first {
		sw.first = false
		_, err := sw.w.Write([]byte("[")) // start json stream
		if err != nil {
			return err
		}
	} else {
		_, err := sw.w.Write([]byte(",")) // object separator
		if err != nil {
			return err
		}
	}

	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = sw.w.Write(b)
	if err != nil {
		return err
	}
	return sw.rc.Flush()
}

func (sw *jsonStreamWriterImpl) Close() error {
	_, err := sw.w.Write([]byte("]")) // end json stream
	return err
}

type BuildHandler struct {
	l       *zap.SugaredLogger
	service Service
}

type statusWriterImpl struct {
	stream  JSONStreamWriter
	started bool
}

func newStatusWriter(stream JSONStreamWriter) *statusWriterImpl {
	return &statusWriterImpl{stream: stream, started: false}
}

func (sw *statusWriterImpl) Started(rsp *BuildStarted) error {
	sw.started = true
	return sw.stream.SendMessage(rsp)
}
func (sw *statusWriterImpl) Updated(update *StatusUpdate) error {
	return sw.stream.SendMessage(update)
}

func (h *BuildHandler) HandleStartBuild(w http.ResponseWriter, r *http.Request) {
	var err error
	logEnd := logHelper(h.l, &err, "service", "start build")
	defer logEnd()
	defer r.Body.Close()

	b, err := io.ReadAll(r.Body)
	if err != nil {
		return
	}
	var buildRequest BuildRequest
	err = json.Unmarshal(b, &buildRequest)
	if err != nil {
		return
	}

	stream := NewJSONStreamWriter(w)
	statusWriter := newStatusWriter(stream)
	defer stream.Close()

	err = h.service.StartBuild(
		r.Context(),
		&buildRequest,
		statusWriter,
	)
	if err != nil {
		if !statusWriter.started {
			h.l.Debug("send error using error-header")
			w.Header().Add(ErrorHeader, err.Error())
		} else {
			h.l.Debug("send error using status update")
			_ = statusWriter.Updated(&StatusUpdate{BuildFailed: &BuildFailed{err.Error()}})
		}
		return
	}
}

func (h *BuildHandler) HandleSignalBuild(w http.ResponseWriter, r *http.Request) {
	var err error
	logEnd := logHelper(h.l, &err, "service", "signal build")
	defer logEnd()
	defer r.Body.Close()

	encodedBuildID := r.URL.Query().Get("build_id")
	var buildID build.ID
	err = buildID.UnmarshalText([]byte(encodedBuildID))

	if err != nil {
		return
	}

	b, err := io.ReadAll(r.Body)
	if err != nil {
		return
	}

	var signalRequest SignalRequest
	err = json.Unmarshal(b, &signalRequest)
	if err != nil {
		return
	}

	signalResp, err := h.service.SignalBuild(r.Context(), buildID, &signalRequest)
	if err != nil {
		w.Header().Add(ErrorHeader, err.Error())
		return
	}

	b, err = json.Marshal(&signalResp)
	if err != nil {
		return
	}

	_, err = w.Write(b)
}

func NewBuildService(l *zap.Logger, s Service) *BuildHandler {
	return &BuildHandler{l: l.Sugar(), service: s}
}

func (h *BuildHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /build", h.HandleStartBuild)
	mux.HandleFunc("POST /signal", h.HandleSignalBuild)
}
