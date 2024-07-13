//go:build !solution

package artifact

import (
	"net/http"

	"github.com/Garetonchick/distbuild/pkg/build"
	"github.com/Garetonchick/distbuild/pkg/tarstream"
	"go.uber.org/zap"
)

type Handler struct {
	l     *zap.SugaredLogger
	cache *Cache
}

func NewLogger(
	l *zap.SugaredLogger,
	serviceName string,
	funName string,
	h func(w http.ResponseWriter, r *http.Request) error,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l.Debugf("start: %s %s", serviceName, funName)
		defer func() {
			if err := recover(); err != nil {
				l.Debugf("%s %s panic: %v", serviceName, funName, err)
				panic(err)
			}
			l.Debugf("finish: %s %s", serviceName, funName)
		}()
		err := h(w, r)
		if err != nil {
			l.Debugf("%s %s error: %v", serviceName, funName, err)
		}
	}
}

func NewHandler(l *zap.Logger, c *Cache) *Handler {
	return &Handler{l: l.Sugar(), cache: c}
}

const ErrorHeader = "Error-Header"

func (h *Handler) HandleDownload(w http.ResponseWriter, r *http.Request) error {
	rawID := r.URL.Query().Get("id")
	var id build.ID

	if err := id.UnmarshalText([]byte(rawID)); err != nil {
		return err
	}
	artifactPath, unlock, err := h.cache.Get(id)
	if err != nil {
		w.Header().Add(ErrorHeader, err.Error())
		return err
	}
	defer unlock()

	return tarstream.Send(artifactPath, w)
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /artifact", NewLogger(h.l, "cache", "download", h.HandleDownload))
}
