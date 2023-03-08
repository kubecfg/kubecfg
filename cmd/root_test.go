package cmd

import (
	"path/filepath"
	"testing"

	"github.com/kubecfg/kubecfg/utils"
)

func TestReadObjsDuplicates(t *testing.T) {
	cmd := RootCmd
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatal(err)
	}

	_, err := readObjsInternal(cmd, []string{filepath.FromSlash("../testdata/duplicates.jsonnet")})
	if got, want := err.Error(), `duplicate resource ConfigMap, "myns", "foo"`; got != want {
		t.Fatalf("got: %s, want: %s", got, want)
	}
}

func TestReadObjsDuplicatesVersion(t *testing.T) {
	cmd := RootCmd
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatal(err)
	}

	_, err := readObjsInternal(cmd, []string{filepath.FromSlash("../testdata/duplicates_version.jsonnet")})
	if got, want := err.Error(), `duplicate resource Ingress.networking.k8s.io, "myns", "foo"`; got != want {
		t.Fatalf("got: %s, want: %s", got, want)
	}
}

func TestReadObjsDuplicatesLiteral(t *testing.T) {
	cmd := RootCmd
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatal(err)
	}

	_, err := readObjs(cmd, []string{filepath.FromSlash("../testdata/duplicates_literal.jsonnet")})
	if err != nil {
		got := err.Error()
		t.Fatalf("got: %s, want: nil", got)
	}
}

func TestReadObjsDuplicatesLiteralShowProvenance(t *testing.T) {
	cmd := RootCmd
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatal(err)
	}

	_, err := readObjs(cmd, []string{filepath.FromSlash("../testdata/duplicates_literal.jsonnet")}, utils.WithProvenance(true))
	if err != nil {
		got := err.Error()
		t.Fatalf("got: %s, want: nil", got)
	}
}
