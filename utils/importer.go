package utils

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing/fstest"
	"time"

	jsonnet "github.com/google/go-jsonnet"
	libsonnet "github.com/kubecfg/kubecfg/lib"
	"github.com/kubecfg/kubecfg/pkg/features"
	"github.com/kubecfg/kubecfg/pkg/yakl"
	log "github.com/sirupsen/logrus"
)

var errNotFound = errors.New("Not found")

var extVarKindRE = regexp.MustCompile("^<(?:extvar|top-level-arg):.+>$")

// streamDir is http.Dir that also serves non-seekable files such as pipes,
// so `import 'file:///dev/stdin'` works when stdin is piped.
type streamDir struct{ http.Dir }

func (d streamDir) Open(name string) (http.File, error) {
	f, err := d.Dir.Open(name)
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err == nil && (fi.Mode().IsRegular() || fi.IsDir()) {
		return f, nil
	}
	defer f.Close()
	if err != nil {
		return nil, err
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	// MapFS serves the bytes seekable and with the buffered Size(), which http.FileServer trusts.
	return http.FS(fstest.MapFS{"f": {Data: b}}).Open("f")
}

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
	if alpha {
		return MakeUniversalImporterWithFeatures(searchURLs, features.AllAlpha())
	}
	return MakeUniversalImporterWithFeatures(searchURLs, nil)
}

// MakeUniversalImporterWithFeatures is MakeUniversalImporter with the feature gates that decide
// which import schemes are on; nil leaves every feature at its default.
func MakeUniversalImporterWithFeatures(searchURLs []*url.URL, gates *features.Gates) jsonnet.Importer {
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

	t.RegisterProtocol("file", http.NewFileTransport(streamDir{http.Dir("/")}))
	t.RegisterProtocol("internal", http.NewFileTransport(newInternalFS()))
	t.RegisterProtocol("oci", newOCIImporter())
	t.RegisterProtocol("kustomize+https", &kustomizeImporter{gates: gates})

	return &universalImporter{
		BaseSearchURLs: searchURLs,
		HTTPClient:     &http.Client{Transport: t},
		cache:          map[string]jsonnet.Contents{},
		gates:          gates,
	}
}

// YAKLScheme prefixes imports that compile a yakl file to Jsonnet: `import 'yakl:foo.yakl'`.
// Without it a .yakl file is imported as is, which keeps importstr and importbin returning its source:
// go-jsonnet caches contents by location for all three, so compiled code needs a location of its own.
const YAKLScheme = "yakl:"

// YAMLScheme prefixes imports that parse a YAML file: `import 'yaml:foo.yaml'` is
// `std.parseYaml(importstr 'foo.yaml')`, and importstr still returns the file as is.
const YAMLScheme = "yaml:"

// ImportPath returns what to import to evaluate the file at path: .yakl files need the yakl: scheme.
func ImportPath(path string) string {
	if strings.HasPrefix(path, "data:") { // inline code, not a file
		return path
	}
	p := path
	if u, err := url.Parse(path); err == nil && u.Opaque == "" {
		p = u.Path // ignore a URL's query and fragment
	}
	if strings.HasSuffix(p, yakl.Ext) && !strings.HasPrefix(path, YAKLScheme) {
		return YAKLScheme + path
	}
	return path
}

// DocsScheme prefixes imports that wrap the imported value v in an object, {docs: v}, for tools
// that need an object where a yakl stream is an array.
const DocsScheme = "kubecfg+docs:"

type universalImporter struct {
	BaseSearchURLs []*url.URL
	HTTPClient     *http.Client
	cache          map[string]jsonnet.Contents
	gates          *features.Gates
}

func (importer *universalImporter) Import(importedFrom, importedPath string) (jsonnet.Contents, string, error) {
	log.Debugf("Importing %q from %q", importedPath, importedFrom)

	if path, ok := strings.CutPrefix(importedPath, DocsScheme); ok {
		if c, ok := importer.cache[importedPath]; ok {
			return c, importedPath, nil
		}
		c := jsonnet.MakeContents(fmt.Sprintf("{docs: import %q}", path))
		importer.cache[importedPath] = c
		return c, importedPath, nil
	}

	// Code compiled from a yakl file imports relative to the file itself.
	importedFrom = strings.TrimPrefix(importedFrom, YAKLScheme)
	if path, ok := strings.CutPrefix(importedPath, YAKLScheme); ok {
		return importer.importYAKL(importedFrom, path)
	}
	if path, ok := strings.CutPrefix(importedPath, YAMLScheme); ok {
		return importer.importCompiled(importedFrom, path, YAMLScheme, func(_ jsonnet.Contents, foundAt string) (string, error) {
			return fmt.Sprintf("std.parseYaml(importstr %q)", foundAt), nil
		})
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
		if c, ok := importer.cache[foundAt]; ok {
			return c, foundAt, nil
		}

		tried = append(tried, foundAt)
		importedData, err := importer.tryImport(foundAt)
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

func (importer *universalImporter) importYAKL(importedFrom, importedPath string) (jsonnet.Contents, string, error) {
	if err := importer.gates.Require(features.YAKL); err != nil {
		return jsonnet.Contents{}, "", fmt.Errorf("%s: %w", importedPath, err)
	}
	return importer.importCompiled(importedFrom, importedPath, YAKLScheme, func(src jsonnet.Contents, _ string) (string, error) {
		return yakl.Compile(src.String())
	})
}

// importCompiled imports the code that compile makes from a file, at the file's location prefixed with scheme.
func (importer *universalImporter) importCompiled(importedFrom, importedPath, scheme string, compile func(src jsonnet.Contents, foundAt string) (string, error)) (jsonnet.Contents, string, error) {
	src, foundAt, err := importer.Import(importedFrom, importedPath)
	if err != nil {
		return jsonnet.Contents{}, "", err
	}
	compiledAt := scheme + foundAt
	if c, ok := importer.cache[compiledAt]; ok {
		return c, compiledAt, nil
	}
	code, err := compile(src, foundAt)
	if err != nil {
		return jsonnet.Contents{}, "", fmt.Errorf("%s: %w", foundAt, err)
	}
	c := jsonnet.MakeContents(code)
	importer.cache[compiledAt] = c
	return c, compiledAt, nil
}

func (importer *universalImporter) tryImport(url string) (jsonnet.Contents, error) {
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
	return jsonnet.MakeContents(string(bodyBytes)), nil
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
	if err != nil || importDirURL.Scheme == "" {
		// The directory of code whose name isn't a URL: "" for --ext-code and --tla-code, or the
		// directory of the file that eval -e evaluates. As in the jsonnet command, it's relative
		// to the current directory.
		dir, err := filepath.Abs(importedFrom)
		if err != nil {
			return nil, fmt.Errorf("Invalid import dir %q: %v", importedFrom, err)
		}
		importDirURL = &url.URL{Scheme: "file", Path: strings.TrimSuffix(filepath.ToSlash(dir), "/") + "/"}
	}

	candidateURLs := make([]*url.URL, 1, len(importer.BaseSearchURLs)+1)
	candidateURLs[0] = importDirURL.ResolveReference(importedPathURL)

	for _, u := range importer.BaseSearchURLs {
		candidateURLs = append(candidateURLs, u.ResolveReference(importedPathURL))
	}

	return candidateURLs, nil
}
