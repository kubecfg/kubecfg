// Copyright 2023 The kubecfg authors
//
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package utils

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/StalkR/httpcache"
	"github.com/containerd/containerd/remotes"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/pkg/auth/docker"
)

const (
	OCIBundleBodyMediaType   = "application/vnd.kubecfg.bundle.tar+gzip"
	OCIBundleConfigMediaType = "application/vnd.kubecfg.bundle.config.v1+json"
)

type OCIBundleConfig struct {
	Entrypoint string `json:"entrypoint"`
}

type ociImporter struct {
	httpClient *http.Client
}

func newOCIImporter() *ociImporter {
	return &ociImporter{httpcache.NewVolatileClient(5*time.Minute, 1024)}
}

func (o *ociImporter) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := context.Background()
	pkg, path := ociSplitURL(req.URL)

	cli, err := docker.NewClient()
	if err != nil {
		return nil, err
	}
	resolver, err := cli.Resolver(ctx, o.httpClient, false)
	if err != nil {
		return nil, err
	}
	_, manifestDesc, err := resolver.Resolve(ctx, pkg)
	if err != nil {
		return nil, err
	}

	fetcher, err := resolver.Fetcher(ctx, pkg)
	if err != nil {
		return nil, err
	}

	var manifest ocispec.Manifest
	if err := fetchInto(ctx, fetcher, manifestDesc, &manifest); err != nil {
		return nil, err
	}
	var config OCIBundleConfig
	if err := fetchInto(ctx, fetcher, manifest.Config, &config); err != nil {
		return nil, err
	}
	if path == "" {
		path = config.Entrypoint
	}

	for _, l := range manifest.Layers {
		if l.MediaType != OCIBundleBodyMediaType {
			continue
		}
		r, err := fetcher.Fetch(ctx, l)
		if err != nil {
			return nil, err
		}
		f, err := extractFile(r, path)
		if err != nil {
			return nil, err
		}

		res := &http.Response{
			Request:       req,
			Proto:         "HTTP/1.0",
			ProtoMajor:    1,
			Status:        http.StatusText(http.StatusOK),
			StatusCode:    http.StatusOK,
			ContentLength: -1,
			Header:        make(http.Header),
			Close:         true,
			Body:          f,
		}
		return res, nil
	}

	return nil, fmt.Errorf("cannot find layer with mediatype %q", OCIBundleBodyMediaType)
}

func fetchInto(ctx context.Context, fetcher remotes.Fetcher, desc ocispec.Descriptor, v interface{}) error {
	c, err := fetcher.Fetch(ctx, desc)
	if err != nil {
		return err
	}
	defer c.Close()
	return json.NewDecoder(c).Decode(v)
}

func ociSplitURL(u *url.URL) (string, string) {
	_, after, _ := strings.Cut(u.Path, ":")
	_, path, _ := strings.Cut(after, "/")
	base := strings.TrimPrefix(strings.TrimSuffix(u.String(), "/"+path), "oci://")
	return base, path
}

// normalizeOCIURL adds a trailing slash if the OCI url has no path component.
// This allows correctly resolving relative imports even when the OCI package is referenced without path component, like
// oci://foo.com/my/package:v1
func normalizeOCIURL(u *url.URL) *url.URL {
	_, after, _ := strings.Cut(u.Path, ":")
	res := *u
	if !strings.Contains(after, "/") {
		res.Path += "/"
	}
	return &res
}

func extractFile(r io.Reader, path string) (io.ReadCloser, error) {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("building gzip reader: %w", err)
	}
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			return nil, fmt.Errorf("extracting file %q: %w", path, err)
		}
		if hdr.Name == path {
			return io.NopCloser(tr), nil
		}
	}
	return nil, fmt.Errorf("cannot find file %q in tarball", path)
}
