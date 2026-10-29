// The knit tests: the edge cases in testdata/cases.jsonnet, the expected errors in
// testdata/fail, and the golden examples in testdata/examples. They import the copy of
// knit embedded in kubecfg, as 'knit/knit.libsonnet'.
package knit

import (
	"bytes"
	"flag"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-jsonnet"
	"github.com/kubecfg/kubecfg/pkg/kubecfg"
	"github.com/kubecfg/kubecfg/utils"
)

var update = flag.Bool("update", false, "rewrite testdata/examples/*/expected.yaml")

func newVM(t *testing.T) *jsonnet.VM {
	t.Helper()
	vm, err := kubecfg.JsonnetVM(kubecfg.WithAlpha(true))
	if err != nil {
		t.Fatal(err)
	}
	vm.Importer(utils.MakeUniversalImporter([]*url.URL{{Scheme: "internal", Path: "/"}}, true))
	return vm
}

func fileURL(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}).String()
}

// Each case in cases.jsonnet is a boolean; the file asserts that all of them hold.
func TestCases(t *testing.T) {
	if _, err := newVM(t).EvaluateFile(fileURL(t, "testdata/cases.jsonnet")); err != nil {
		t.Fatal(err)
	}
}

// Each file in testdata/fail must fail, with an error that contains the text on its
// first line, after "// expect: ".
func TestFail(t *testing.T) {
	files, err := filepath.Glob("testdata/fail/*.jsonnet")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		t.Run(strings.TrimSuffix(filepath.Base(f), ".jsonnet"), func(t *testing.T) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			first, _, _ := strings.Cut(string(src), "\n")
			want, ok := strings.CutPrefix(first, "// expect: ")
			if !ok {
				t.Fatalf("first line must be // expect: <part of the error>")
			}
			_, err = newVM(t).EvaluateFile(fileURL(t, f))
			if err == nil {
				t.Fatal("rendered without error")
			}
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error does not mention %q:\n%v", want, err)
			}
		})
	}
}

// Each example renders its expected.yaml: main.jsonnet as a YAML stream, or a YAKL
// example's prod.yakl, which extends the others, as kubecfg show does. If the new output
// is right, run: go test ./lib/knit -run TestExamples -update
func TestExamples(t *testing.T) {
	dirs, err := filepath.Glob("testdata/examples/*")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dirs {
		t.Run(filepath.Base(d), func(t *testing.T) {
			var got string
			if _, err := os.Stat(filepath.Join(d, "prod.yakl")); err == nil {
				got = showYAKL(t, filepath.Join(d, "prod.yakl"))
			} else {
				got = renderJsonnet(t, filepath.Join(d, "main.jsonnet"))
			}
			golden := filepath.Join(d, "expected.yaml")
			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Errorf("output differs from %s; if the change is right, run with -update\ngot:\n%s", golden, got)
			}
		})
	}
}

func renderJsonnet(t *testing.T, path string) string {
	t.Helper()
	vm := newVM(t)
	vm.StringOutput = true
	out, err := vm.EvaluateAnonymousSnippet("render.jsonnet", `local x = import '`+fileURL(t, path)+`';
		if std.isArray(x) then std.manifestYamlStream(x, quote_keys=false) else std.manifestYamlDoc(x, quote_keys=false)`)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimRight(out, "\n") + "\n"
}

func showYAKL(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	objs, err := kubecfg.ReadObjects(newVM(t), []string{abs})
	if err != nil {
		t.Fatal(err)
	}
	show, err := kubecfg.NewShowCmd("yaml", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	var got bytes.Buffer
	if err := show.Run(objs, &got); err != nil {
		t.Fatal(err)
	}
	return got.String()
}
