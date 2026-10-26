package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	jsonnet "github.com/google/go-jsonnet"
	"github.com/kubecfg/kubecfg/pkg/features"
)

func TestInternalFS(t *testing.T) {
	fs := newInternalFS()
	if _, err := fs.Open("kubecfg.libsonnet"); err != nil {
		t.Errorf("opening kubecfg.libsonnet failed! %v", err)
	}
	if _, err := fs.Open("noexist"); !os.IsNotExist(err) {
		t.Errorf("Incorrect noexist error: %v", err)
	}
	if _, err := fs.Open("noexist/foo"); !os.IsNotExist(err) {
		t.Errorf("Incorrect noexist dir error: %v", err)
	}

	// This test really belongs somewhere else, but it's easiest
	// to do here.
	if _, err := fs.Open("kubecfg_test.jsonnet"); err == nil {
		t.Errorf("kubecfg_test.jsonnet should not have been embedded")
	}
}

func TestExpandImportToCandidateURLs(t *testing.T) {
	importer := universalImporter{
		BaseSearchURLs: []*url.URL{
			{Scheme: "file", Path: "/first/base/search/"},
		},
	}

	t.Run("Absolute URL in import statement yields a single candidate", func(t *testing.T) {
		urls, _ := importer.expandImportToCandidateURLs("dir", "http://absolute.com/import/path")
		expected := []*url.URL{
			{Scheme: "http", Host: "absolute.com", Path: "/import/path"},
		}
		if !reflect.DeepEqual(urls, expected) {
			t.Errorf("Expected %v, got %v", expected, urls)
		}
	})

	t.Run("Absolute URL in import dir is searched before BaseSearchURLs", func(t *testing.T) {
		urls, _ := importer.expandImportToCandidateURLs("file:///abs/import/dir/", "relative/file.libsonnet")
		expected := []*url.URL{
			{Scheme: "file", Host: "", Path: "/abs/import/dir/relative/file.libsonnet"},
			{Scheme: "file", Host: "", Path: "/first/base/search/relative/file.libsonnet"},
		}
		if !reflect.DeepEqual(urls, expected) {
			t.Errorf("Expected %v, got %v", expected, urls)
		}
	})

	t.Run("An import dir that isn't a URL is relative to the current directory", func(t *testing.T) {
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		// "" is the directory of --ext-code and --tla-code; "dir/" that of eval -e on dir/app.jsonnet.
		for dir, want := range map[string]string{"": "/file.libsonnet", "dir/": "/dir/file.libsonnet"} {
			urls, err := importer.expandImportToCandidateURLs(dir, "file.libsonnet")
			if err != nil {
				t.Fatal(err)
			}
			expected := []*url.URL{
				{Scheme: "file", Path: filepath.ToSlash(cwd) + want},
				{Scheme: "file", Path: "/first/base/search/file.libsonnet"},
			}
			if !reflect.DeepEqual(urls, expected) {
				t.Errorf("%q: Expected %v, got %v", dir, expected, urls)
			}
		}
	})
}

func TestImportPath(t *testing.T) {
	for path, want := range map[string]string{
		"a.yakl":                      "yakl:a.yakl",
		"dir/a.jsonnet":               "dir/a.jsonnet",
		"yakl:a.yakl":                 "yakl:a.yakl",
		"file:///x/a.yakl?raw=true":   "yakl:file:///x/a.yakl?raw=true",
		"https://h/a.yakl#fragment":   "yakl:https://h/a.yakl#fragment",
		"https://h/a.jsonnet?x=.yakl": "https://h/a.jsonnet?x=.yakl",
		"data:,{a: 1} // demo.yakl":   "data:,{a: 1} // demo.yakl",
		`C:\x\a.yakl`:                 `yakl:C:\x\a.yakl`,
	} {
		if got := ImportPath(path); got != want {
			t.Errorf("ImportPath(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestImportFromPipe(t *testing.T) {
	for name, tc := range map[string]struct {
		data  string
		delay time.Duration
	}{
		"delayed": {data: "{a: 1}", delay: 50 * time.Millisecond},
		"large":   {data: strings.Repeat("x", 1<<20)}, // larger than the pipe buffer
	} {
		t.Run(name, func(t *testing.T) {
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			go func() {
				time.Sleep(tc.delay)
				w.WriteString(tc.data)
				w.Close()
			}()

			path := fmt.Sprintf("file:///dev/fd/%d", r.Fd())
			contents, _, err := MakeUniversalImporter(nil, false).Import("", path)
			if err != nil {
				t.Fatalf("importing %s: %v", path, err)
			}
			if got := contents.String(); got != tc.data {
				t.Errorf("got %d bytes, want %d", len(got), len(tc.data))
			}
		})
	}
}

func TestImportHashes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.jsonnet")
	if err := os.WriteFile(path, []byte("{a: 1}"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Hashes of {a: 1}.
	const (
		sha256 = "sha256:5c41c5087643e28ad46363c644cc5e3ad8a0aec629d514f0a672482f74e1effb"
		sha512 = "sha512:aaf558218ccae466271e6690d940c621cd8c4c969c173e36f419654571bbcbf6e8781e3f0ee5ca89d31fbee23f4e3b61d0209104ebc17670c80396c2c5b50e0d"
	)
	off, err := features.New(false, features.Source{Name: "test", Items: []string{"pinned-imports=false"}})
	if err != nil {
		t.Fatal(err)
	}
	for fragment, wantErr := range map[string]string{
		"":                                     "",
		"#":                                    "",
		"#L42":                                 "", // not for kubecfg
		"#a;b":                                 "",
		"#hash=" + sha256:                      "",
		"#hash=" + sha512:                      "",
		"#hash=" + strings.ToUpper(sha256[7:]): "invalid hash",
		"#hash=sha256:" + strings.ToUpper(sha256[7:]): "",
		"#hash=" + sha256 + "&hash=" + sha512:         "",
		"#hash=" + sha256 + "&hash=blake3:x":          "",                      // unknown algorithms are skipped
		"#hash=" + sha256 + "&hash=" + sha512[:20]:    "content hash mismatch", // every known one must match
		"#hash=" + sha256[:20]:                        "content hash mismatch",
		"#hash=sha256:":                               "content hash mismatch",
		"#hash=blake3:x":                              "no hash this kubecfg can check",
		"#hash=sha265:" + sha256[7:]:                  "no hash this kubecfg can check",
		"#hash=" + sha256 + "&hash=" + sha256:         "more than once",
		"#hash=" + sha256 + "&x=1":                    "unknown key",
		"#hash=" + sha256 + "&a;b":                    "invalid URL fragment",
	} {
		url := "file://" + filepath.ToSlash(path) + fragment
		_, _, err := MakeUniversalImporter(nil, false).Import("", url)
		if wantErr == "" && err != nil || wantErr != "" && (err == nil || !strings.Contains(err.Error(), wantErr)) {
			t.Errorf("importing %s: got error %v, want %q", url, err, wantErr)
		}
		if _, _, err := MakeUniversalImporterWithFeatures(nil, off).Import("", url); err != nil {
			t.Errorf("importing %s with pinned-imports off: %v", url, err)
		}
	}
}

func TestImportYAML(t *testing.T) {
	dir := t.TempDir()
	for name, src := range map[string]string{
		"one.yaml":    "a: ${x}\n",
		"stream.yaml": "a: 1\n---\nb: 2\n",
		"main.jsonnet": `[
			import 'yaml:one.yaml', import 'yaml:stream.yaml', importstr 'one.yaml',
			std.parseYaml(importstr 'stream.yaml') == import 'yaml:stream.yaml',
		]`,
	} {
		if err := os.WriteFile(dir+"/"+name, []byte(src), 0600); err != nil {
			t.Fatal(err)
		}
	}
	vm := jsonnet.MakeVM()
	vm.Importer(MakeUniversalImporter(nil, false))
	got, err := vm.EvaluateAnonymousSnippet("", fmt.Sprintf("import 'file://%s/main.jsonnet'", dir))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(got)); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), `[{"a":"${x}"},[{"a":1},{"b":2}],"a: ${x}\n",true]`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}
