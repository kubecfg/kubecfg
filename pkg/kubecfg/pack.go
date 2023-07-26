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

package kubecfg

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/google/go-jsonnet"
	"github.com/kubecfg/kubecfg/pkg/oci"
	"github.com/kubecfg/kubecfg/pkg/version"
	"github.com/kubecfg/kubecfg/utils"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
)

const (
	packMetadataField = "_kubecfg_pack_metadata"
	packMetadataKey   = "pack.kubecfg.dev/v1alpha1"
)

// PackCmd represents the eval subcommand
type PackCmd struct {
	OutputFile       string
	InsecureRegistry bool // use HTTP if true
}

func (c PackCmd) Run(ctx context.Context, vm *jsonnet.VM, ociPackage string, rootFile string) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("packing %q: %w", rootFile, err)
		}
	}()

	rootURLString, err := utils.PathToURL(rootFile)
	if err != nil {
		return err
	}
	rootURL, err := url.Parse(rootURLString)
	if err != nil {
		return err
	}

	var bodyBlob bytes.Buffer
	shortEntrypoint, err := bundleAllDependencies(&bodyBlob, vm, rootURL)
	if err != nil {
		return err
	}

	if c.OutputFile != "" {
		return os.WriteFile(c.OutputFile, bodyBlob.Bytes(), 0666)
	}

	metadata, err := bundleConfigMetadata(vm, rootURL)
	if err != nil {
		return err
	}

	return c.pushOCIBundle(ctx, ociPackage, bodyBlob.Bytes(), shortEntrypoint, metadata)
}

func bundleConfigMetadata(vm *jsonnet.VM, rootURL *url.URL) (json.RawMessage, error) {
	packMetadata := map[string]struct {
		Version string `json:"version"`
	}{
		packMetadataKey: {
			Version: version.Get(),
		},
	}
	base, err := json.Marshal(packMetadata)
	if err != nil {
		return nil, err
	}

	metadataExpr := fmt.Sprintf(`%s + std.get(import %q, %q, {})`, string(base), rootURL, packMetadataField)
	metadata, err := vm.EvaluateAnonymousSnippet("", metadataExpr)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(metadata), nil
}

// Writes a targz to w, containing rootURL and all files transitively imported from rootURL.
// The path names are trimmed to remove the common prefix. The so trimmed rootURL is returned.
func bundleAllDependencies(w io.Writer, vm *jsonnet.VM, rootURL *url.URL) (string, error) {
	urls, err := urlsToBePackaged(vm, rootURL)
	if err != nil {
		return "", err
	}

	short, shortEntrypoint := shortNames(urls, rootURL)

	fgz := gzip.NewWriter(w)
	defer fgz.Close()
	tw := tar.NewWriter(fgz)
	defer tw.Close()

	for i := range short {
		content, _, err := vm.ImportData(".", urls[i].String())
		if err != nil {
			return "", err
		}
		b := []byte(content)
		if err := tw.WriteHeader(&tar.Header{Name: short[i], Size: int64(len(b)), Mode: 0666}); err != nil {
			return "", err
		}
		if _, err := tw.Write(b); err != nil {
			return "", err
		}
	}
	return shortEntrypoint, nil
}

func (c PackCmd) pushOCIBundle(ctx context.Context, ref string, bodyBlob []byte, entryPoint string, bundleConfigMetadata json.RawMessage) error {
	repo, err := oci.NewAuthenticatedRepository(ref)
	if err != nil {
		return err
	}
	repo.PlainHTTP = c.InsecureRegistry

	bodyDesc := content.NewDescriptorFromBytes(utils.OCIBundleBodyMediaType, bodyBlob)
	if err := repo.Push(ctx, bodyDesc, bytes.NewReader(bodyBlob)); err != nil {
		return err
	}

	bundleConfig := utils.OCIBundleConfig{
		Entrypoint: entryPoint,
		Metadata:   bundleConfigMetadata,
	}
	configBlob, err := json.Marshal(bundleConfig)
	if err != nil {
		return err
	}
	configDesc := content.NewDescriptorFromBytes(utils.OCIBundleConfigMediaType, configBlob)
	if err := repo.Push(ctx, configDesc, bytes.NewReader(configBlob)); err != nil {
		return err
	}

	manifest := ocispec.Manifest{
		Config:    configDesc,
		Layers:    []ocispec.Descriptor{bodyDesc},
		Versioned: specs.Versioned{SchemaVersion: 2},
		Annotations: map[string]string{
			// compatibility with fluxcd ocirepo source
			"org.opencontainers.image.created":  "1970-01-01T00:00:00Z",
			"org.opencontainers.image.revision": "unknown",
			"org.opencontainers.image.source":   "kubecfg pack",
		},
	}
	manifestBlob, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	manifestDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, manifestBlob)
	if err := repo.PushReference(ctx, manifestDesc, bytes.NewReader(manifestBlob), ref); err != nil {
		return err
	}

	return nil
}

// returns rootFile parsed as a file URL along with a sorted list of files imported by rootFile
// The url slice includes the rootFile itself.
func urlsToBePackaged(vm *jsonnet.VM, rootURL *url.URL) ([]*url.URL, error) {
	deps, err := vm.FindDependencies(".", []string{rootURL.String()})
	if err != nil {
		return nil, err
	}

	var urls []*url.URL
	for _, d := range deps {
		u, err := url.Parse(d)
		if err != nil {
			return nil, err
		}
		if u.Scheme == "internal" {
			continue
		}
		if u.Scheme != "file" {
			return nil, fmt.Errorf("unsupported scheme: %s", d)
		}
		urls = append(urls, u)
	}

	urls = append(urls, rootURL)
	sort.Slice(urls, func(i, j int) bool { return urls[i].String() < urls[j].String() })
	return urls, nil

}

func shortNames(urls []*url.URL, rootURL *url.URL) ([]string, string) {
	s := make([]string, len(urls))
	for i := range urls {
		s[i] = urls[i].Path
	}
	prefix := findCommonPathPrefix(s)
	for i := range s {
		s[i] = strings.TrimPrefix(s[i], prefix)
	}
	return s, strings.TrimPrefix(rootURL.Path, prefix)
}

// returns common directory part between various paths, including the trailing '/'.
// If only one path is given, return its directory path
func findCommonPathPrefix(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	if len(paths) == 1 {
		return path.Dir(paths[0]) + "/"
	}

	first, last := paths[0], paths[len(paths)-1]

	i, lastSlash := 0, 0
	for ; i < len(first) && i < len(last); i++ {
		if first[i] == '/' {
			lastSlash = i
		}
		if first[i] != last[i] {
			break
		}
	}
	return first[:lastSlash+1]
}
