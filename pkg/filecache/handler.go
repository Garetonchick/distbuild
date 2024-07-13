//go:build !solution

package filecache

import (
	"errors"
	"io"
	"net/http"
	"os"

	"github.com/Garetonchick/distbuild/pkg/build"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

type Handler struct {
	l      *zap.SugaredLogger
	cache  *Cache
	single singleflight.Group
}

func (h *Handler) HandleUpload(w http.ResponseWriter, r *http.Request) {
	rawID := r.URL.Query().Get("id")
	var id build.ID
	if err := id.UnmarshalText([]byte(rawID)); err != nil {
		return
	}
	type Result struct {
		cacheWriter io.WriteCloser
		abort       func() error
	}

	errCtxDone := errors.New("request context done")

	res, err, _ := h.single.Do(rawID, func() (any, error) {
		select {
		case <-r.Context().Done():
			return Result{}, errCtxDone
		default:
		}
		cacheWriter, abort, err := h.cache.Write(id)
		return Result{cacheWriter: cacheWriter, abort: abort}, err
	})
	if err != nil {
		return
	}
	tres := res.(Result)
	cacheWriter, abort := tres.cacheWriter, tres.abort

	closed := true
	defer func() {
		if !closed {
			_ = abort()
		}
	}()

	_, err = io.Copy(cacheWriter, r.Body)
	if err != nil {
		return
	}
	err = cacheWriter.Close()
	if err != nil {
		return
	}
	closed = true
}

func (h *Handler) HandleDownload(w http.ResponseWriter, r *http.Request) {
	rawID := r.URL.Query().Get("id")
	var id build.ID
	if err := id.UnmarshalText([]byte(rawID)); err != nil {
		return
	}
	filepath, unlock, err := h.cache.Get(id)
	if err != nil {
		return
	}
	defer unlock()

	file, err := os.Open(filepath)
	if err != nil {
		return
	}

	_, err = io.Copy(w, file)
	if err != nil {
		return
	}
}

func NewHandler(l *zap.Logger, cache *Cache) *Handler {
	return &Handler{l: l.Sugar(), cache: cache}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /file", h.HandleDownload)
	mux.HandleFunc("PUT /file", h.HandleUpload)
}
