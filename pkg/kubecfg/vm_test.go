package kubecfg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/kubecfg/kubecfg/internal/acquire"
	"github.com/kubecfg/kubecfg/pkg/kubecfg/vars"
	"github.com/kubecfg/kubecfg/utils"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func tempFile(t *testing.T, body string) (filename string) {
	t.Helper()

	tmpfile, err := os.CreateTemp(t.TempDir(), "kubecfg-test-*.jsonnet")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(tmpfile, body); err != nil {
		tmpfile.Close()
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}
	return tmpfile.Name()
}

func renderReadObject(t *testing.T, opts ...utils.ReadOption) map[string]string {
	vm, err := JsonnetVM()
	if err != nil {
		t.Fatal(err)
	}

	input := tempFile(t, `{a:{b:{apiVersion:'v1',kind:'ConfigMap'}}}`)
	objs, err := ReadObjects(vm, []string{input}, opts...)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := len(objs), 1; got != want {
		t.Fatalf("got: %d, want: %d", got, want)
	}
	obj := objs[0]
	t.Logf("obj: %q", obj)

	if got, want := obj.GetObjectKind().GroupVersionKind().Kind, "ConfigMap"; got != want {
		t.Fatalf("got: %q, want: %q", got, want)
	}
	return obj.GetAnnotations()
}

func TestReadObject(t *testing.T) {
	annos := renderReadObject(t)
	if got, want := len(annos), 0; got != want {
		t.Fatalf("got: %d, want: %d", got, want)
	}

	outerOverlay := `{a+:{b+:{metadata+:{annotations+:{foo:'bar'}}}}}`
	innerOverlay := `{metadata+:{annotations+:{foo:'bar'}}}`
	outerOverlayFile := tempFile(t, outerOverlay)
	innerOverlayFile := tempFile(t, innerOverlay)

	testCases := []struct {
		option utils.ReadOption
	}{
		{utils.WithOverlayCode(outerOverlay)},
		{utils.WithOverlayCode("a.b=" + innerOverlay)},
		{utils.WithOverlayURL(outerOverlayFile)},
		{utils.WithOverlayURL("a.b=" + innerOverlayFile)},
		{utils.WithOverlayURL("file://" + outerOverlayFile)},
		{utils.WithOverlayURL("a.b=" + "file://" + innerOverlayFile)},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			annos = renderReadObject(t, tc.option)
			if got, want := len(annos), 1; got != want {
				t.Fatalf("got: %d, want: %d", got, want)
			}
			if got, want := annos["foo"], "bar"; got != want {
				t.Fatalf("got: %q, want: %q", got, want)
			}
		})
	}
}

func TestParseResolverFlags(t *testing.T) {
	for _, s := range AvailableResolverTypes() {
		if got := ParseResolverType(s).String(); got != s {
			t.Errorf("ParseResolverType(%q).String() = %q", s, got)
		}
	}
	if got := ParseResolverType("bogus"); got != NoopResolver {
		t.Errorf("ParseResolverType(bogus) = %v, want noop", got)
	}

	for _, s := range AvailableResolverFailureAction() {
		if got := ParseResolverFailureAction(s).String(); got != s {
			t.Errorf("ParseResolverFailureAction(%q).String() = %q", s, got)
		}
	}
	if got := ParseResolverFailureAction("bogus"); got != WarnResolverError {
		t.Errorf("ParseResolverFailureAction(bogus) = %v, want warn", got)
	}
}

func TestTracebackYAKL(t *testing.T) {
	file := filepath.Join(t.TempDir(), "t.yakl")
	if err := os.WriteFile(file, []byte("a: 1\nb: ${1 + 1}\nc: 3\n"), 0600); err != nil {
		t.Fatal(err)
	}
	vm, err := JsonnetVM(WithAlpha(true))
	if err != nil {
		t.Fatal(err)
	}
	fileURL, err := utils.PathToURL(file)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := traceback(&buf, vm, utils.ImportPath(fileURL), "$.c", false); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimSpace(buf.String()), file+":3"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// A file that evaluates to an array: provenance paths index it.
	file = filepath.Join(t.TempDir(), "t.jsonnet")
	if err := os.WriteFile(file, []byte("[\n  {a: 1},\n  {b: 1 + 1},\n]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if fileURL, err = utils.PathToURL(file); err != nil {
		t.Fatal(err)
	}
	vm, err = JsonnetVM(WithAlpha(true))
	if err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if err := traceback(&buf, vm, utils.ImportPath(fileURL), "$[1].b", false); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimSpace(buf.String()), file+":3"; got != want {
		t.Errorf("array: got %q, want %q", got, want)
	}
}

func TestYAKLCodeFileVar(t *testing.T) {
	file := filepath.Join(t.TempDir(), "v.yakl")
	if err := os.WriteFile(file, []byte("name: ${'world'}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	vm, err := JsonnetVM(WithAlpha(true),
		WithVar(vars.New(vars.Ext, vars.Code, vars.File, "code", file)),
		WithVar(vars.New(vars.Ext, vars.String, vars.File, "str", file)))
	if err != nil {
		t.Fatal(err)
	}
	got, err := vm.EvaluateAnonymousSnippet("x", "[std.extVar('code').name, std.extVar('str')]")
	if err != nil {
		t.Fatal(err)
	}
	if got = strings.Join(strings.Fields(got), ""); got != `["world","name:${'world'}\n"]` {
		t.Errorf("got %s", got)
	}
}

func TestEvalTLANamedLikeInternals(t *testing.T) {
	vm, err := JsonnetVM()
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "f.jsonnet")
	if err := os.WriteFile(file, []byte("function(r, std) {value: r + std}"), 0600); err != nil {
		t.Fatal(err)
	}
	fileURL, err := utils.PathToURL(file)
	if err != nil {
		t.Fatal(err)
	}
	vm.TLAVar("r", "hello")
	vm.TLAVar("std", "!")
	in, err := input(fileURL, acquire.ReadOptions{Expr: "$.value", TLA: []string{"r", "std"}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := utils.Evaluate(vm, in)
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello!" {
		t.Errorf("got %v", got)
	}
}

func TestEvalYAKLStream(t *testing.T) {
	file := filepath.Join(t.TempDir(), "s.yakl")
	if err := os.WriteFile(file, []byte("---\n^^: a\nv: ${1}\n---\n^^: b\nv: 2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	vm, err := JsonnetVM(WithAlpha(true))
	if err != nil {
		t.Fatal(err)
	}
	fileURL, err := utils.PathToURL(file)
	if err != nil {
		t.Fatal(err)
	}
	got, err := utils.Evaluate(vm, fileURL)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := json.Marshal(got); string(b) != `{"a":{"v":1},"b":{"v":2}}` {
		t.Errorf("got %s", b)
	}
	in, err := input(fileURL, acquire.ReadOptions{Expr: "$.b.v"})
	if err != nil {
		t.Fatal(err)
	}
	if got, err = utils.Evaluate(vm, in); err != nil {
		t.Fatal(err)
	}
	if got != 2.0 {
		t.Errorf("got %v", got)
	}
}

// A yakl overlay applies as written (base + overlay), not through toOverlay.
func TestYAKLOverlay(t *testing.T) {
	dir, err := filepath.Abs(filepath.FromSlash("../yakl/testdata/overlay"))
	if err != nil {
		t.Fatal(err)
	}
	read := func(overlay string) ([]string, map[string]any, error) {
		vm, err := JsonnetVM(WithAlpha(true))
		if err != nil {
			t.Fatal(err)
		}
		objs, err := ReadObjects(vm, []string{filepath.Join(dir, "web.yakl")}, utils.WithOverlayURL(overlay))
		if err != nil {
			return nil, nil, err
		}
		var kinds []string
		var deployment map[string]any
		for _, o := range objs {
			kinds = append(kinds, o.GetKind())
			if o.GetKind() == "Deployment" {
				deployment = o.Object
			}
		}
		sort.Strings(kinds)
		return kinds, deployment, nil
	}

	kinds, deployment, err := read(filepath.Join(dir, "prod.yakl"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(kinds, ","), "ConfigMap,Deployment,NetworkPolicy,PodDisruptionBudget,Service,ServiceAccount"; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	if got, _, _ := unstructured.NestedFieldNoCopy(deployment, "spec", "replicas"); fmt.Sprint(got) != "5" {
		t.Errorf("got %v replicas, want 5 from the hidden config the overlay changed", got)
	}

	whole := filepath.Join(t.TempDir(), "whole.yakl")
	if err := os.WriteFile(whole, []byte("apiVersion: v1\nkind: ServiceAccount\nmetadata:\n  name: x\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := read(whole); err == nil || !strings.Contains(err.Error(), "a whole object without a name, overlaid on a file with names") {
		t.Errorf("got error %v", err)
	}
	// Named by the flag, it's a new document.
	kinds, _, err = read("account=" + whole)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(kinds, ","), "ConfigMap,Deployment,NetworkPolicy,Service,ServiceAccount"; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}
