package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSearchUp(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "a", "marker.txt")
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	got, found, err := SearchUp("marker.txt", filepath.Join(deep, "main.jsonnet"))
	if err != nil || !found || got != marker {
		t.Errorf("SearchUp = %q, %v, %v; want %q, true, nil", got, found, err, marker)
	}

	_, found, err = SearchUp("does-not-exist-kubecfg.txt", filepath.Join(deep, "main.jsonnet"))
	if err != nil || found {
		t.Errorf("SearchUp missing file: found=%v err=%v", found, err)
	}
}
