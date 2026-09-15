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
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strings"

	"github.com/containerd/containerd/remotes"
	"github.com/containerd/containerd/remotes/docker"
	"github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/config/credentials"
	ctypes "github.com/docker/cli/cli/config/types"
	"github.com/docker/docker/registry"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	OCIBundleBodyMediaType      = "application/vnd.kubecfg.bundle.tar+gzip"
	OCIBundleDocsMediaType      = "application/vnd.kubecfg.bundle.docs.tar+gzip"
	OCIBundleConfigMediaType    = "application/vnd.oci.image.config.v1+json"
	OCIBundleConfigArtifactType = "application/vnd.kubecfg.bundle.config.v1+json"
)

type OCIBundleConfig struct {
	Entrypoint string          `json:"entrypoint"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
}

type OCIBundle struct {
	manifest ocispec.Manifest
	config   OCIBundleConfig
	files    map[string][]byte
}

func NewOCIBundle(manifest ocispec.Manifest, config OCIBundleConfig, r io.ReadCloser) (*OCIBundle, error) {
	files, err := slurpTar(r)
	if err != nil {
		return nil, err
	}
	return &OCIBundle{manifest, config, files}, nil
}

func (o *OCIBundle) Open(path string) (io.ReadCloser, error) {
	b, found := o.files[path]
	if !found {
		return nil, fs.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

type ociImporter struct {
	httpClient  *http.Client
	bundleCache map[string]*OCIBundle
}

func newOCIImporter() *ociImporter {
	return &ociImporter{
		bundleCache: make(map[string]*OCIBundle),
	}
}

func (o *ociImporter) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	pkg, path := ociSplitURL(req.URL)

	bundle, found := o.bundleCache[pkg]
	if !found {
		var err error
		bundle, err = o.fetchBundle(ctx, pkg)
		if err != nil {
			return nil, err
		}
		o.bundleCache[pkg] = bundle
	}
	if path == "" {
		// cannot just redirect via HTTP here because otherwise relative jsonnet imports
		// won't be based on the entrypoint file location.
		imp := fmt.Sprintf("import %q", bundle.config.Entrypoint)

		// this prevents infinite import recursion
		if bundle.config.Entrypoint == "" {
			return nil, fmt.Errorf(`must use non-empty "entrypoint" config field if you want to render the OCI bundle root`)
		}
		return simpleHTTPResponse(req, http.StatusOK, io.NopCloser(strings.NewReader(imp))), nil
	}

	status := http.StatusOK
	r, err := bundle.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		status = http.StatusNotFound
	} else if err != nil {
		return nil, err
	}
	return simpleHTTPResponse(req, status, r), nil
}

func simpleHTTPResponse(req *http.Request, statusCode int, r io.ReadCloser) *http.Response {
	return &http.Response{
		Request:       req,
		Proto:         "HTTP/1.0",
		ProtoMajor:    1,
		Status:        http.StatusText(statusCode),
		StatusCode:    statusCode,
		ContentLength: -1,
		Header:        make(http.Header),
		Close:         true,
		Body:          r,
	}
}

type dockerClient struct {
	configs []*configfile.ConfigFile
}

func (c *dockerClient) Credential(hostname string) (string, string, error) {
	hostname = resolveHostname(hostname)
	var (
		auth ctypes.AuthConfig
		err  error
	)
	for _, cfg := range c.configs {
		auth, err = cfg.GetAuthConfig(hostname)
		if err != nil {
			// fall back to next config
			continue
		}
		if auth.IdentityToken != "" {
			return "", auth.IdentityToken, nil
		}
		if auth.Username == "" && auth.Password == "" {
			// fall back to next config
			continue
		}
		return auth.Username, auth.Password, nil
	}
	return "", "", err
}

func resolveHostname(hostname string) string {
	switch hostname {
	case registry.IndexHostname, registry.IndexName, registry.DefaultV2Registry.Host:
		return registry.IndexServer
	}
	return hostname
}

func (o *ociImporter) fetchBundle(ctx context.Context, pkg string) (*OCIBundle, error) {
	cfg, err := config.Load(config.Dir())
	if err != nil {
		return nil, err
	}
	if !cfg.ContainsAuth() {
		cfg.CredentialsStore = credentials.DetectDefaultStore(cfg.CredentialsStore)
	}

	cli := &dockerClient{
		configs: []*configfile.ConfigFile{cfg},
	}
	var resolver remotes.Resolver = docker.NewResolver(docker.ResolverOptions{
		Credentials: cli.Credential,
		Client:      o.httpClient,
		PlainHTTP:   false,
	})

	fetcher, err := resolver.Fetcher(ctx, pkg)
	if err != nil {
		return nil, err
	}

	_, manifestDesc, err := resolver.Resolve(ctx, pkg)
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

	for _, l := range manifest.Layers {
		if l.MediaType != OCIBundleBodyMediaType {
			continue
		}
		r, err := fetcher.Fetch(ctx, l)
		if err != nil {
			return nil, err
		}
		return NewOCIBundle(manifest, config, r)
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

// Read all files from a targz and return a map of file->contents
func slurpTar(r io.Reader) (map[string][]byte, error) {
	res := map[string][]byte{}
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
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		res[hdr.Name] = b
	}
	return res, nil
}
