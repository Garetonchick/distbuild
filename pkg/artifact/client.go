//go:build !solution

package artifact

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/Garetonchick/distbuild/pkg/build"
	"github.com/Garetonchick/distbuild/pkg/tarstream"
)

// Download artifact from remote cache into local cache.
func Download(ctx context.Context, endpoint string, c *Cache, artifactID build.ID) error {
	downloadPath, err := url.JoinPath(endpoint, "artifact")
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", downloadPath, nil)
	if err != nil {
		return err
	}
	query := req.URL.Query()
	query.Set("id", artifactID.String())
	req.URL.RawQuery = query.Encode()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := resp.Header.Get(ErrorHeader); err != "" {
		return fmt.Errorf(err)
	}

	download := func(artifactID build.ID, r io.Reader) error {
		artifactPath, commit, abort, err := c.Create(artifactID)
		if err != nil {
			_ = abort()
			return err
		}
		defer func() {
			err := recover()
			if err != nil {
				_ = abort()
				panic(err)
			}
		}()
		err = tarstream.Receive(artifactPath, r)
		if err != nil {
			_ = abort()
			return err
		}
		return commit()
	}

	return download(artifactID, resp.Body)
}
