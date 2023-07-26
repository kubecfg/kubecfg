package cmd

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/klauspost/compress/gzip"
)

const (
	testRef  = "gcr.io/demo/test:v1"
	testRoot = "import '../body.jsonnet'"
	testBody = `{
		apiVersion: 'v1',
		kind: 'Namespace',
		metadata: {name: 'demo'},
	  }`

	testRootFilename = "dir1/demo.jsonnet"
	testBodyFilename = "body.jsonnet"
)

func prepareTestData(t *testing.T) string {
	srcDir, err := os.MkdirTemp("", "kubecfg-test-pack-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(srcDir) })

	testRootFile := filepath.Join(srcDir, testRootFilename)
	os.MkdirAll(filepath.Dir(testRootFile), 0700)
	err = os.WriteFile(testRootFile, []byte(testRoot), 0600)
	if err != nil {
		t.Fatal(err)
	}

	testBodyFile := filepath.Join(srcDir, testBodyFilename)
	err = os.WriteFile(testBodyFile, []byte(testBody), 0600)
	if err != nil {
		t.Fatal(err)
	}

	return testRootFile
}

func verifyBodyTarball(t *testing.T, r io.Reader) {
	expected := []struct {
		name string
		body string
	}{
		{testBodyFilename, testBody},
		{testRootFilename, testRoot},
	}

	gr, err := gzip.NewReader(r)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gr)

	i := 0
	for ; ; i++ {
		hdr, err := tr.Next()
		if err == io.EOF {
			break // End of archive
		}
		if err != nil {
			t.Fatal(err)
		}
		if got, want := hdr.Name, expected[i].name; got != want {
			t.Errorf("got %q, want: %q", got, want)
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(b), expected[i].body; got != want {
			t.Errorf("got %q, want: %q", got, want)
		}
	}
	if got, want := i, len(expected); got != want {
		t.Errorf("got %d, want: %d", got, want)
	}
}

func TestPack(t *testing.T) {
	testRootFile := prepareTestData(t)

	output, err := os.CreateTemp("", "kubecfg-test-*.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	output.Close()
	defer os.Remove(output.Name())

	cmdOutput(t, []string{"--alpha", "pack", testRef, testRootFile, "--output", output.Name()})

	f, err := os.Open(output.Name())
	if err != nil {
		t.Fatal(err)
	}
	verifyBodyTarball(t, f)
}

func TestPush(t *testing.T) {
	// This is basically a snapshot test that breaks as soon as the OCI client does something different.
	// The OCI spec is strict enough so that this shouldn't be a problem.

	var session atomic.Int32
	blobs := map[string][]byte{}
	var blobLock sync.Mutex

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("new request: method=%q, path=%q", r.Method, r.URL.Path)
		switch {
		case r.Method == "POST" && r.URL.Path == "/v2/demo/blobs/uploads/":
			next := session.Add(1)

			w.Header().Add("Location", fmt.Sprintf("/upload/session/%d", next))
			w.WriteHeader(http.StatusAccepted)
		case r.Method == "PUT" && strings.HasPrefix(r.URL.Path, "/upload/session/"):
			digest := r.URL.Query().Get("digest")
			t.Logf("             digest=%q", digest)
			b, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			r.Body.Close()
			blobLock.Lock()
			defer blobLock.Unlock()
			blobs[digest] = b

			w.Header().Add("Location", fmt.Sprintf("/dummy/blobs/%s", digest))
			w.WriteHeader(http.StatusCreated)
		case r.Method == "PUT" && r.URL.Path == "/v2/demo/manifests/1234":
			b, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			r.Body.Close()
			blobLock.Lock()
			defer blobLock.Unlock()
			blobs["manifest"] = b

			w.Header().Add("Location", "/dummy/blobs/manifest")
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unknown request: method=%q, path=%q", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(testServer.Close)

	testRootFile := prepareTestData(t)

	testRef := fmt.Sprintf("%s/demo:1234", strings.TrimPrefix(testServer.URL, "http://"))
	t.Logf("testRef=%q", testRef)

	cmdOutput(t, []string{"--alpha", "pack", testRef, testRootFile, "--insecure-registry"})

	blobLock.Lock()
	defer blobLock.Unlock()

	t.Logf("recorded blob keys:")
	for key := range blobs {
		t.Logf("blob: %q", key)
	}

	// these digests have been observed by running the test failing and obverving the logs
	const (
		blobDigest = "sha256:90847d8bd5ca990ebf777bf131130cdcf695ace2f8bb59dee0571129af855f00"
		confDigest = "sha256:d5e1762d319a9de4f26f64420d485611ce6f1bdd4418e4d37d0b388ebc996fc0"
	)
	getBlob := func(key string) []byte {
		got, ok := blobs[key]
		if !ok {
			t.Fatalf("can't find blob for key %q", key)
		}
		return got
	}

	verifyBodyTarball(t, bytes.NewReader(getBlob(blobDigest)))

	if got, want := string(getBlob(confDigest)), `{"entrypoint":"dir1/demo.jsonnet","metadata":{"pack.kubecfg.dev/v1alpha1":{"version":"(dev build)"}}}`; got != want {
		t.Errorf("got: %q, want: %q", got, want)
	}

}
