package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	jsonnet "github.com/google/go-jsonnet"
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
