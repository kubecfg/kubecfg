package kubecfg

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-jsonnet"
	"github.com/kubecfg/kubecfg/utils"
)

// The k8s-libsonnet examples give the kubecfg show output kept in their testdata. They import the
// library from GitHub; the test serves it from the copy in testdata, so it runs offline.
func TestK8sLibsonnetExamples(t *testing.T) {
	// Pinned in examples/k8s-libsonnet/k8s.libsonnet; update with testdata/k8s.libsonnet there.
	const lib = "https://raw.githubusercontent.com/kubecfg/k8s-libsonnet/0c6a5947b2e5b6ded6c6053c36fb675c8aa4c44d/k8s.libsonnet"
	dir, err := filepath.Abs(filepath.FromSlash("../../examples/k8s-libsonnet"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ overlay, want string }{
		{"", "shop.yaml"},
		{"dev.yakl", "shop-dev.yaml"},
		{"prod.yakl", "shop-prod.yaml"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			vm, err := JsonnetVM(WithAlpha(true))
			if err != nil {
				t.Fatal(err)
			}
			vm.Importer(offlineImporter{
				Importer: utils.MakeUniversalImporter([]*url.URL{{Scheme: "internal", Path: "/"}}, true),
				url:      lib,
				file:     (&url.URL{Scheme: "file", Path: filepath.ToSlash(filepath.Join(dir, "testdata", "k8s.libsonnet"))}).String(),
			})
			cmd := "kubecfg --alpha show shop.yakl"
			var opts []utils.ReadOption
			if tc.overlay != "" {
				cmd += " --overlay-code-file " + tc.overlay
				opts = append(opts, utils.WithOverlayURL(filepath.Join(dir, tc.overlay)))
			}
			objs, err := ReadObjects(vm, []string{filepath.Join(dir, "shop.yakl")}, opts...)
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
			want, err := os.ReadFile(filepath.Join(dir, "testdata", tc.want))
			if err != nil {
				t.Fatal(err)
			}
			if got.String() != string(want) {
				t.Errorf("output differs from testdata/%[1]s; if the change is right, in examples/k8s-libsonnet run:\n  %[2]s > testdata/%[1]s\ngot:\n%[3]s", tc.want, cmd, got.String())
			}
		})
	}
}

// offlineImporter serves url from file, and fails on any other remote import.
type offlineImporter struct {
	jsonnet.Importer
	url, file string
}

func (i offlineImporter) Import(importedFrom, importedPath string) (jsonnet.Contents, string, error) {
	if importedPath == i.url {
		return i.Importer.Import("", i.file)
	}
	if strings.HasPrefix(importedPath, "http:") || strings.HasPrefix(importedPath, "https:") {
		return jsonnet.Contents{}, "", fmt.Errorf("%s: the test runs offline", importedPath)
	}
	return i.Importer.Import(importedFrom, importedPath)
}
