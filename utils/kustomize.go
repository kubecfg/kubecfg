package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

// kustomizeImporter satifies the http.RoundTripper interface
type kustomizeImporter struct {
	alpha bool
}

// RoundTrip performs a HTTP transaction for an `import kustomize+https://<url>` statement
// by calling a simple Kustomize run against it, returning the rendered manifests.
func (k *kustomizeImporter) RoundTrip(req *http.Request) (*http.Response, error) {

	if !k.alpha {
		return nil, fmt.Errorf("kustomize+https:// prefix is an alpha feature, please use the --alpha flag")
	}

	// We know the scheme is kustomize+https, so simply grab the URL
	// for kustomize to use
	url := strings.Split(req.URL.String(), "+")[1]

	kustomizer := krusty.MakeKustomizer(krusty.MakeDefaultOptions())
	fs := filesys.MakeFsOnDisk()
	m, err := kustomizer.Run(fs, url)
	if err != nil {
		return nil, err
	}

	// As we cannot simply convert directly to JSON,
	// we pull out the resources first and append them
	// individually to be marshaled together later into
	// a JSON array.
	var resources []map[string]interface{}
	for _, r := range m.Resources() {
		m, err := r.Map()
		if err != nil {
			return nil, err
		}
		resources = append(resources, m)
	}

	// Convert our map to JSON so that we can simply import it directly.
	data, err := json.Marshal(resources)
	if err != nil {
		return nil, err
	}

	r := io.NopCloser(bytes.NewReader(data))
	return simpleHTTPResponse(req, http.StatusOK, r), nil
}
