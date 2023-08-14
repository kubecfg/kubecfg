package utils

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	jsonnet "github.com/google/go-jsonnet"
	libsonnet "github.com/kubecfg/kubecfg/lib"
	log "github.com/sirupsen/logrus"
)

var errNotFound = errors.New("Not found")

var extVarKindRE = regexp.MustCompile("^<(?:extvar|top-level-arg):.+>$")

func newInternalFS() http.FileSystem {
	return http.FS(libsonnet.Assets)
}

/*
MakeUniversalImporter creates an importer that handles resolving imports from the filesystem and HTTP/S.

In addition to the standard importer, supports:
  - URLs in import statements
  - URLs in library search paths
  - importing binary files (for local files and URLs)

A real-world example:
  - You have https://raw.githubusercontent.com/ksonnet/ksonnet-lib/master in your search URLs.
  - You evaluate a local file which calls `import "ksonnet.beta.2/k.libsonnet"`.
  - If the `ksonnet.beta.2/k.libsonnet“ is not located in the current working directory, an attempt
    will be made to follow the search path, i.e. to download
    https://raw.githubusercontent.com/ksonnet/ksonnet-lib/master/ksonnet.beta.2/k.libsonnet.
  - Since the downloaded `k.libsonnet“ file turn in contains `import "k8s.libsonnet"`, the import
    will be resolved as https://raw.githubusercontent.com/ksonnet/ksonnet-lib/master/ksonnet.beta.2/k8s.libsonnet
    and downloaded from that location.
*/
func MakeUniversalImporter(searchURLs []*url.URL, alpha bool) jsonnet.Importer {
	// Reconstructed copy of http.DefaultTransport (to avoid
	// modifying the default)
	t := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	t.RegisterProtocol("file", http.NewFileTransport(http.Dir("/")))
	t.RegisterProtocol("internal", http.NewFileTransport(newInternalFS()))
	t.RegisterProtocol("oci", newOCIImporter())
	t.RegisterProtocol("kustomize+https", &kustomizeImporter{alpha: alpha})

	return &universalImporter{
		BaseSearchURLs: searchURLs,
		HTTPClient:     &http.Client{Transport: t},
		cache:          map[string]jsonnet.Contents{},
		alpha:          alpha,
	}
}

type universalImporter struct {
	BaseSearchURLs []*url.URL
	HTTPClient     *http.Client
	cache          map[string]jsonnet.Contents
	alpha          bool // alpha features are enable only if true
}

func (importer *universalImporter) Import(importedFrom, importedPath string) (jsonnet.Contents, string, error) {
	log.Debugf("Importing %q from %q", importedPath, importedFrom)

	binary := false
	if strings.HasPrefix(importedPath, "binary://") {
		if !importer.alpha {
			log.Debugf("WARNING: `import 'binary://file.tgz'` form is now deprecated. please use `importbin './file.tgz' instead")
			return jsonnet.Contents{}, "", fmt.Errorf(`"binary://" url prefix requires the --alpha flag`)
		}
		log.Debugf("WARNING: `import 'binary://file.tgz'` form is now deprecated. please use `importbin './file.tgz' instead")
		binary = true
		importedPath = strings.TrimPrefix(importedPath, "binary://")
	}

	candidateURLs, err := importer.expandImportToCandidateURLs(importedFrom, importedPath)
	if err != nil {
		return jsonnet.Contents{}, "", fmt.Errorf("Could not get candidate URLs for when importing %s (imported from %s): %v", importedPath, importedFrom, err)
	}

	var tried []string
	for _, u := range candidateURLs {
		if u.Scheme == "oci" {
			u = normalizeOCIURL(u)
		}

		foundAt := u.String()
		// Avoid collision bug when importing same chart in the same jsonnet file using `import binary://` and `importbin`
		if binary {
			foundAt = u.String() + "##binaryImport"
		}
		if c, ok := importer.cache[foundAt]; ok {
			return c, foundAt, nil
		}

		tried = append(tried, foundAt)
		importedData, err := importer.tryImport(foundAt, binary)
		if err == nil {
			importer.cache[foundAt] = importedData
			return importedData, foundAt, nil
		} else if err != errNotFound {
			return jsonnet.Contents{}, "", err
		}
	}

	return jsonnet.Contents{}, "", fmt.Errorf("Couldn't open import %q, no match locally or in library search paths. Tried: %s",
		importedPath,
		strings.Join(tried, ";"),
	)
}

func (importer *universalImporter) tryImport(url string, binary bool) (jsonnet.Contents, error) {
	url = strings.TrimSuffix(url, "##binaryImport")
	res, err := importer.HTTPClient.Get(url)
	if err != nil {
		return jsonnet.Contents{}, err
	}
	defer res.Body.Close()
	log.Debugf("GET %q -> %s", url, res.Status)
	if res.StatusCode == http.StatusNotFound {
		return jsonnet.Contents{}, errNotFound
	} else if res.StatusCode != http.StatusOK {
		return jsonnet.Contents{}, fmt.Errorf("error reading content: %s", res.Status)
	}

	bodyBytes, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return jsonnet.Contents{}, err
	}
	if binary {
		return toIntArray(bodyBytes), nil
	}
	return jsonnet.MakeContents(string(bodyBytes)), nil
}

func toIntArray(bytes []byte) jsonnet.Contents {
	var sb strings.Builder
	sb.WriteRune('[')
	for i, ch := range bytes {
		if i > 0 {
			sb.WriteRune(',')
		}
		fmt.Fprintf(&sb, "%d", ch)
	}
	sb.WriteRune(']')

	return jsonnet.MakeContents(sb.String())
}

func (importer *universalImporter) expandImportToCandidateURLs(importedFrom, importedPath string) ([]*url.URL, error) {
	importedPathURL, err := url.Parse(importedPath)
	if err != nil {
		return nil, fmt.Errorf("Import path %q is not valid", importedPath)
	}
	if importedPathURL.IsAbs() {
		return []*url.URL{importedPathURL}, nil
	}

	importDirURL, err := url.Parse(importedFrom)
	if err != nil {
		return nil, fmt.Errorf("Invalid import dir %q: %v", importedFrom, err)
	}

	candidateURLs := make([]*url.URL, 1, len(importer.BaseSearchURLs)+1)
	candidateURLs[0] = importDirURL.ResolveReference(importedPathURL)

	for _, u := range importer.BaseSearchURLs {
		candidateURLs = append(candidateURLs, u.ResolveReference(importedPathURL))
	}

	return candidateURLs, nil
}
