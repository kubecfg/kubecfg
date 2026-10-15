package utils

import (
	"net/url"
	"os"
	"reflect"
	"testing"
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
		"a.jamlet":                      "jamlet:a.jamlet",
		"dir/a.jsonnet":                 "dir/a.jsonnet",
		"jamlet:a.jamlet":               "jamlet:a.jamlet",
		"file:///x/a.jamlet?raw=true":   "jamlet:file:///x/a.jamlet?raw=true",
		"https://h/a.jamlet#fragment":   "jamlet:https://h/a.jamlet#fragment",
		"https://h/a.jsonnet?x=.jamlet": "https://h/a.jsonnet?x=.jamlet",
		"data:,{a: 1} // demo.jamlet":   "data:,{a: 1} // demo.jamlet",
		`C:\x\a.jamlet`:                 `jamlet:C:\x\a.jamlet`,
	} {
		if got := ImportPath(path); got != want {
			t.Errorf("ImportPath(%q) = %q, want %q", path, got, want)
		}
	}
}
