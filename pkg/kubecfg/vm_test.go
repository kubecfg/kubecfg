package kubecfg

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kubecfg/kubecfg/pkg/kubecfg/vars"
	"github.com/kubecfg/kubecfg/utils"
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

func TestTracebackJamlet(t *testing.T) {
	file := filepath.Join(t.TempDir(), "t.jamlet")
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

	// A stream evaluates to an array; provenance paths index it.
	if err := os.WriteFile(file, []byte("---\na: 1\n---\nb: ${2}\n"), 0600); err != nil {
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
	if got, want := strings.TrimSpace(buf.String()), file+":4"; got != want {
		t.Errorf("stream: got %q, want %q", got, want)
	}
}

func TestJamletCodeFileVar(t *testing.T) {
	file := filepath.Join(t.TempDir(), "v.jamlet")
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
	got, err := vm.EvaluateAnonymousSnippet("eval", evalSnippet(fileURL, "$.value", []string{"r", "std"}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got) != `"hello!"` {
		t.Errorf("got %s", got)
	}
}

func TestEvalJamletStream(t *testing.T) {
	file := filepath.Join(t.TempDir(), "s.jamlet")
	if err := os.WriteFile(file, []byte("---\na: ${1}\n---\nb: 2\n"), 0600); err != nil {
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
	got, err := vm.EvaluateAnonymousSnippet("eval", evalSnippet(utils.ImportPath(fileURL), "", nil))
	if err != nil {
		t.Fatal(err)
	}
	if got = strings.Join(strings.Fields(got), ""); got != `[{"a":1},{"b":2}]` {
		t.Errorf("got %s", got)
	}
	_, err = vm.EvaluateAnonymousSnippet("eval", evalSnippet(utils.ImportPath(fileURL), "$.a", nil))
	if err == nil || !strings.Contains(err.Error(), "-e needs the file to evaluate to an object, not array") {
		t.Errorf("got error %v", err)
	}
}
