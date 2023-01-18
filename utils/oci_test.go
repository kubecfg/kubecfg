package utils

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestOCITransport(t *testing.T) {
	const (
		testFile1 = "guestbook.jsonnet"
		testBody1 = "dummy string"
		testFile2 = "other.jsonnet"
		testBody2 = "other dummy string"
	)

	testServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// these mock responses are based on an actual traffic grab of a test artifact I pushed on gcr.io.
		switch {
		case r.Method == "HEAD" && r.URL.Path == "/v2/mkm-cloud/hello/manifests/v1":
			w.Header().Add("Content-Type", "application/vnd.oci.image.manifest.v1+json")
			w.WriteHeader(200)
		case r.Method == "GET" && r.URL.Path == "/v2/mkm-cloud/hello/manifests/v1":
			w.Header().Add("Content-Type", "application/vnd.oci.image.manifest.v1+json")
			fmt.Fprintf(w, "{\"schemaVersion\":2,\"mediaType\":\"application/vnd.oci.image.manifest.v1+json\",\"config\":{\"mediaType\":\"application/vnd.kubecfg.bundle.config.v1+json\",\"digest\":\"sha256:f554a2f13a74f54fa95e1e128ae349bfec6dad4b2d9b836abd90949ef5ea8731\",\"size\":40},\"layers\":[{\"mediaType\":\"application/vnd.kubecfg.bundle.tar+gzip\",\"digest\":\"sha256:cf7d50c45c421eb89cd969d509550abf137220c97e9257fb483c4f3a9548b29f\",\"size\":2036,\"annotations\":{\"io.deis.oras.content.digest\":\"sha256:146e6b19cd18a48247bf8be61821879886f84db9f57eb9f29769db8a9dde816f\",\"io.deis.oras.content.unpack\":\"true\",\"org.opencontainers.image.title\":\".\"}}],\"annotations\":{\"org.opencontainers.image.created\":\"2023-01-13T10:16:18Z\"}}")
		case r.Method == "GET" && r.URL.Path == "/v2/mkm-cloud/hello/manifests/sha256:db94623ace77be069591d7b435f004ebee6c808fe6f6842df27bb1ce31e2b0ce":
			w.Header().Add("Content-Type", "application/vnd.oci.image.manifest.v1+json")
			fmt.Fprintf(w, "{\"schemaVersion\":2,\"mediaType\":\"application/vnd.oci.image.manifest.v1+json\",\"config\":{\"mediaType\":\"application/vnd.kubecfg.bundle.config.v1+json\",\"digest\":\"sha256:f554a2f13a74f54fa95e1e128ae349bfec6dad4b2d9b836abd90949ef5ea8731\",\"size\":40},\"layers\":[{\"mediaType\":\"application/vnd.kubecfg.bundle.tar+gzip\",\"digest\":\"sha256:cf7d50c45c421eb89cd969d509550abf137220c97e9257fb483c4f3a9548b29f\",\"size\":2036,\"annotations\":{\"io.deis.oras.content.digest\":\"sha256:146e6b19cd18a48247bf8be61821879886f84db9f57eb9f29769db8a9dde816f\",\"io.deis.oras.content.unpack\":\"true\",\"org.opencontainers.image.title\":\".\"}}],\"annotations\":{\"org.opencontainers.image.created\":\"2023-01-13T10:16:18Z\"}}")
		case r.Method == "GET" && r.URL.Path == "/v2/mkm-cloud/hello/blobs/sha256:f554a2f13a74f54fa95e1e128ae349bfec6dad4b2d9b836abd90949ef5ea8731":
			w.Header().Add("Content-Type", OCIBundleConfigMediaType)
			fmt.Fprintf(w, `{"entrypoint": "guestbook.jsonnet"}`)
		case r.Method == "GET" && r.URL.Path == "/v2/mkm-cloud/hello/blobs/sha256:cf7d50c45c421eb89cd969d509550abf137220c97e9257fb483c4f3a9548b29f":
			// the OCI client doesn't really check the hash of the blob, so we can return some random test data.
			w.Header().Add("Content-Type", OCIBundleBodyMediaType)
			gw := gzip.NewWriter(w)
			defer gw.Close()
			tw := tar.NewWriter(gw)

			tw.WriteHeader(&tar.Header{
				Name: testFile1,
				Mode: 0600,
				Size: int64(len(testBody1)),
			})
			tw.Write([]byte(testBody1))

			tw.WriteHeader(&tar.Header{
				Name: testFile2,
				Mode: 0600,
				Size: int64(len(testBody2)),
			})
			tw.Write([]byte(testBody2))

			tw.Close()
		default:
			http.Error(w, fmt.Sprintf("unhandled request %v", r), 500)
		}
	}))
	t.Cleanup(testServer.Close)

	tr := &http.Transport{}
	oci := newOCIImporter()
	oci.httpClient.Transport = &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if addr != "gcr.io:443" {
				return nil, fmt.Errorf("bad address %q", addr)
			}
			return net.Dial(network, testServer.Listener.Addr().String())
		},
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	tr.RegisterProtocol("oci", oci)

	cl := http.Client{Transport: tr}

	res, err := cl.Get("oci://gcr.io/mkm-cloud/hello:v1")
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := string(b), testBody1; got != want {
		t.Fatalf("got %q, want: %q", got, want)
	}

	res, err = cl.Get("oci://gcr.io/mkm-cloud/hello:v1/" + testFile2)
	if err != nil {
		t.Fatal(err)
	}
	b, err = io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := string(b), testBody2; got != want {
		t.Fatalf("got %q, want: %q", got, want)
	}
}

func TestOCISplitURL(t *testing.T) {
	testCases := []struct {
		url  string
		base string
		path string
	}{
		{"oci://gcr.io/foo/bar:v1", "gcr.io/foo/bar:v1", ""},
		{"oci://gcr.io/foo/bar:v1/file.json", "gcr.io/foo/bar:v1", "file.json"},
		{"oci://gcr.io/foo/bar:v1/dir/file.json", "gcr.io/foo/bar:v1", "dir/file.json"},
		{"oci://gcr.io/foo/bar:v1@sha256:ac21f6480f177a804794f4bb90146d4d950a7b0826c530d6ba50948e68e77f13", "gcr.io/foo/bar:v1@sha256:ac21f6480f177a804794f4bb90146d4d950a7b0826c530d6ba50948e68e77f13", ""},
		{"oci://gcr.io/foo/bar:v1@sha256:ac21f6480f177a804794f4bb90146d4d950a7b0826c530d6ba50948e68e77f13/file.json", "gcr.io/foo/bar:v1@sha256:ac21f6480f177a804794f4bb90146d4d950a7b0826c530d6ba50948e68e77f13", "file.json"},
		{"oci://gcr.io/foo/bar:v1@sha256:ac21f6480f177a804794f4bb90146d4d950a7b0826c530d6ba50948e68e77f13/dir/file.json", "gcr.io/foo/bar:v1@sha256:ac21f6480f177a804794f4bb90146d4d950a7b0826c530d6ba50948e68e77f13", "dir/file.json"},
		{"oci://gcr.io/foo/bar@sha256:ac21f6480f177a804794f4bb90146d4d950a7b0826c530d6ba50948e68e77f13", "gcr.io/foo/bar@sha256:ac21f6480f177a804794f4bb90146d4d950a7b0826c530d6ba50948e68e77f13", ""},
		{"oci://gcr.io/foo/bar@sha256:ac21f6480f177a804794f4bb90146d4d950a7b0826c530d6ba50948e68e77f13/file.json", "gcr.io/foo/bar@sha256:ac21f6480f177a804794f4bb90146d4d950a7b0826c530d6ba50948e68e77f13", "file.json"},
		{"oci://gcr.io/foo/bar@sha256:ac21f6480f177a804794f4bb90146d4d950a7b0826c530d6ba50948e68e77f13/dir/file.json", "gcr.io/foo/bar@sha256:ac21f6480f177a804794f4bb90146d4d950a7b0826c530d6ba50948e68e77f13", "dir/file.json"},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			u, err := url.Parse(tc.url)
			if err != nil {
				t.Fatal(err)
			}
			base, path := ociSplitURL(u)
			if got, want := base, tc.base; got != want {
				t.Errorf("got: %q, want: %q", got, want)
			}
			if got, want := path, tc.path; got != want {
				t.Errorf("got: %q, want: %q", got, want)
			}
		})
	}
}

func TestNormalizeOCIURL(t *testing.T) {
	testCases := []struct {
		in   string
		want string
	}{
		{"oci://gcr.io/foo/bar:v1", "oci://gcr.io/foo/bar:v1/"},
		{"oci://gcr.io/foo/bar:v1/", "oci://gcr.io/foo/bar:v1/"},
		{"oci://gcr.io/foo/bar:v1/file.jsonnet", "oci://gcr.io/foo/bar:v1/file.jsonnet"},
		{"oci://gcr.io/foo/bar:v1/dir/file.jsonnet", "oci://gcr.io/foo/bar:v1/dir/file.jsonnet"},
		{"oci://gcr.io/foo/bar:v1/dir/file.jsonnet/", "oci://gcr.io/foo/bar:v1/dir/file.jsonnet/"},
	}
	for i, tc := range testCases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			u, err := url.Parse(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := normalizeOCIURL(u).String(), tc.want; got != want {
				t.Errorf("got: %q, want: %q", got, want)
			}
		})
	}
}
