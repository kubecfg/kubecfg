package kubecfg

import (
	"bytes"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-jsonnet"
	"github.com/kubecfg/kubecfg/utils"
)

var update = flag.Bool("update", false, "rewrite the golden files in testdata/examples")

// Every example in examples/ gives the output kept in testdata/examples, so a change that
// breaks an example fails here. If the new output is right, run:
//
//	go test ./pkg/kubecfg -run TestExamples -update
func TestExamples(t *testing.T) {
	for _, tc := range []struct {
		// file is a path under examples/; golden is a path under testdata/examples.
		file, golden string
		// request is the Jsonnet of a metacontroller hook's request; the hook is evaluated
		// with it instead of shown.
		request string
	}{
		{file: "guestbook.jsonnet", golden: "guestbook.yaml"},
		{file: "guestbook-overlay.jsonnet", golden: "guestbook-overlay.yaml"},
		{file: "experimental/yakl/guestbook.yakl", golden: "experimental/yakl/guestbook.yaml"},
		{file: "experimental/knit/guestbook.jsonnet", golden: "experimental/knit/guestbook.yaml"},
		{file: "experimental/knit/shop/shop.yakl", golden: "experimental/knit/shop/shop.yaml"},
		{file: "experimental/knit/shop/dev.yakl", golden: "experimental/knit/shop/dev.yaml"},
		{file: "experimental/knit/shop/prod.yakl", golden: "experimental/knit/shop/prod.yaml"},
		{file: "experimental/knit/shop/prod-advanced.yakl", golden: "experimental/knit/shop/prod-advanced.yaml"},
		{file: "experimental/knit/import/base.yakl", golden: "experimental/knit/import/base.yaml"},
		{file: "experimental/knit/import/prod.yakl", golden: "experimental/knit/import/prod.yaml"},
		{
			file:    "metacontroller/jsonnet/sync.jsonnet",
			golden:  "metacontroller/sync.json",
			request: `{parent: {metadata: {generation: 3}, spec: {name: 'demo'}}, children: {}}`,
		},
	} {
		t.Run(tc.golden, func(t *testing.T) {
			got := runExample(t, tc.file, tc.request)
			golden := filepath.Join("testdata", "examples", filepath.FromSlash(tc.golden))
			if *update {
				if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("output differs from %s; if the change is right, run with -update\ngot:\n%s", golden, got)
			}
		})
	}
}

func runExample(t *testing.T, file, request string) []byte {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "examples", filepath.FromSlash(file)))
	if err != nil {
		t.Fatal(err)
	}
	vm, err := JsonnetVM(WithAlpha(true))
	if err != nil {
		t.Fatal(err)
	}
	vm.Importer(offlineImporter{utils.MakeUniversalImporter([]*url.URL{{Scheme: "internal", Path: "/"}}, true)})
	if request != "" {
		vm.TLACode("request", request)
		out, err := vm.EvaluateFile((&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String())
		if err != nil {
			t.Fatal(err)
		}
		return []byte(out)
	}
	objs, err := ReadObjects(vm, []string{path})
	if err != nil {
		t.Fatal(err)
	}
	show, err := NewShowCmd("yaml", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	var got bytes.Buffer
	if err := show.Run(objs, &got); err != nil {
		t.Fatal(err)
	}
	return got.Bytes()
}

// remote are the pinned libraries the examples import, served from their copies in
// testdata/remote, so that the tests run offline.
var remote = map[string]string{
	"https://github.com/bitnami-labs/kube-libsonnet/raw/52ba963ca44f7a4960aeae9ee0fbee44726e481f/kube.libsonnet": "kube-libsonnet/kube.libsonnet",
}

// offlineImporter serves the remote libraries from testdata, and fails on any other remote import.
type offlineImporter struct{ jsonnet.Importer }

func (i offlineImporter) Import(importedFrom, importedPath string) (jsonnet.Contents, string, error) {
	if f, ok := remote[importedPath]; ok {
		path, err := filepath.Abs(filepath.Join("testdata", "remote", filepath.FromSlash(f)))
		if err != nil {
			return jsonnet.Contents{}, "", err
		}
		return i.Importer.Import("", (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String())
	}
	if strings.HasPrefix(importedPath, "http:") || strings.HasPrefix(importedPath, "https:") {
		return jsonnet.Contents{}, "", fmt.Errorf("%s: the test runs offline; add a copy to testdata/remote", importedPath)
	}
	return i.Importer.Import(importedFrom, importedPath)
}
