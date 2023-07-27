package kubecfg

import (
	"fmt"
	"io"
	"os"
	"testing"

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
