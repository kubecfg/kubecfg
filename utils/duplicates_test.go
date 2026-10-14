package utils

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func dupObj(name, data, provenance string) *unstructured.Unstructured {
	o := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]interface{}{"name": name, "namespace": "ns"},
		"data":       map[string]interface{}{"k": data},
	}}
	if provenance != "" {
		o.SetAnnotations(map[string]string{
			AnnotationProvenanceFile: provenance,
			AnnotationProvenancePath: "$." + provenance,
		})
	}
	return o
}

func TestRemoveDuplicates(t *testing.T) {
	tests := []struct {
		name    string
		objs    []*unstructured.Unstructured
		want    int
		wantErr bool
	}{
		{"distinct", []*unstructured.Unstructured{dupObj("a", "1", ""), dupObj("b", "1", "")}, 2, false},
		{"literal duplicate elided", []*unstructured.Unstructured{dupObj("a", "1", ""), dupObj("a", "1", "")}, 1, false},
		{"provenance ignored", []*unstructured.Unstructured{dupObj("a", "1", "x.jsonnet"), dupObj("a", "1", "y.jsonnet")}, 1, false},
		{"conflicting duplicate", []*unstructured.Unstructured{dupObj("a", "1", ""), dupObj("a", "2", "")}, 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := RemoveDuplicates(tc.objs)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if len(got) != tc.want {
				t.Errorf("got %d objects, want %d", len(got), tc.want)
			}
		})
	}

	// hashing must not mutate the caller's annotations
	o := dupObj("a", "1", "x.jsonnet")
	if _, err := RemoveDuplicates([]*unstructured.Unstructured{o}); err != nil {
		t.Fatal(err)
	}
	if o.GetAnnotations()[AnnotationProvenanceFile] != "x.jsonnet" {
		t.Errorf("provenance annotation was stripped from input object")
	}
}
