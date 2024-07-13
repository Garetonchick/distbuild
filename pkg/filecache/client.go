//go:build !solution

package filecache

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"

	"go.uber.org/zap"

	"github.com/Garetonchick/distbuild/pkg/build"
)

type Client struct {
	l        *zap.SugaredLogger
	endpoint string
}

func NewClient(l *zap.Logger, endpoint string) *Client {
	return &Client{l: l.Sugar(), endpoint: endpoint}
}

func (c *Client) Upload(ctx context.Context, id build.ID, localPath string) error {
	uploadPath, err := url.JoinPath(c.endpoint, "file")
	if err != nil {
		return err
	}
	fileR, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer fileR.Close()

	req, err := http.NewRequestWithContext(ctx, "PUT", uploadPath, fileR)
	if err != nil {
		return err
	}
	query := req.URL.Query()
	query.Set("id", id.String())
	req.URL.RawQuery = query.Encode()

	_, err = http.DefaultClient.Do(req)
	return err
}

func (c *Client) Download(ctx context.Context, localCache *Cache, id build.ID) error {
	downloadPath, err := url.JoinPath(c.endpoint, "file")
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", downloadPath, nil)
	if err != nil {
		return err
	}
	query := req.URL.Query()
	query.Set("id", id.String())
	req.URL.RawQuery = query.Encode()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	w, abort, err := localCache.Write(id)
	if err != nil {
		return err
	}
	defer func() {
		err := recover()
		if err != nil {
			_ = abort()
			panic(err)
		}
	}()

	_, err = io.Copy(w, resp.Body)
	if err != nil {
		_ = abort()
		return err
	}
	return w.Close()
}
