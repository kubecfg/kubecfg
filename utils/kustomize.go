package utils

import (
	"bytes"
	"io"
	"net/http"
	"strings"

	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

// kustomizeImporter satifies the http.RoundTripper interface
type kustomizeImporter struct{}

// RoundTrip performs a HTTP transaction for an `import kustomize+https://<url>` statement
// by calling a simple Kustomize run against it, returning the rendered manifests.
func (k *kustomizeImporter) RoundTrip(req *http.Request) (*http.Response, error) {

	// We know the scheme is kustomize+https, so simply grab the URL
    // for kustomize to use
	url := strings.Split(req.URL.String(), "+")[1]

	kustomizer := krusty.MakeKustomizer(krusty.MakeDefaultOptions())
	fs := filesys.MakeFsOnDisk()
	m, err := kustomizer.Run(fs, url)
	if err != nil {
		return nil, err
	}

	yamlDocs, err := m.AsYaml()
	if err != nil {
		return nil, err
	}
	r := io.NopCloser(bytes.NewReader(yamlDocs))
	return simpleHTTPResponse(req, http.StatusOK, r), nil
}
