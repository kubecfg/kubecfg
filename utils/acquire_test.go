// Copyright 2017 The kubecfg authors
//
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package utils

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestJsonWalk(t *testing.T) {
	fooObj := map[string]interface{}{
		"apiVersion": "test",
		"kind":       "Foo",
	}
	barObj := map[string]interface{}{
		"apiVersion": "test",
		"kind":       "Bar",
	}

	fooObjP := map[string]interface{}{
		"apiVersion": "test",
		"kind":       "Foo",
		"metadata": map[string]interface{}{
			"annotations": map[string]interface{}{
				AnnotationProvenancePath: "$.foo[0].quz",
			},
		},
	}
	barObjP := map[string]interface{}{
		"apiVersion": "test",
		"kind":       "Bar",
		"metadata": map[string]interface{}{
			"annotations": map[string]interface{}{
				AnnotationProvenancePath: "$.foo[1]",
			},
		},
	}
	bazObjP := map[string]interface{}{
		"apiVersion": "test",
		"kind":       "Baz",
		"metadata": map[string]interface{}{
			"annotations": map[string]interface{}{
				AnnotationProvenancePath: `$.foo[0]["self"]`,
			},
		},
	}
	baz2ObjP := map[string]interface{}{
		"apiVersion": "test",
		"kind":       "Baz",
		"metadata": map[string]interface{}{
			"annotations": map[string]interface{}{
				AnnotationProvenancePath: `$.foo[0]["1a"]`,
			},
		},
	}

	tests := []struct {
		input      string
		provenance bool
		result     []interface{}
		error      string
	}{
		{
			// nil input
			input:  `null`,
			result: []interface{}{},
		},
		{
			// single basic object
			input:  `{"apiVersion": "test", "kind": "Foo"}`,
			result: []interface{}{fooObj},
		},
		{
			// array of objects
			input:  `[{"apiVersion": "test", "kind": "Foo"}, {"apiVersion": "test", "kind": "Bar"}]`,
			result: []interface{}{barObj, fooObj},
		},
		{
			// object of objects
			input:  `{"foo": {"apiVersion": "test", "kind": "Foo"}, "bar": {"apiVersion": "test", "kind": "Bar"}}`,
			result: []interface{}{barObj, fooObj},
		},
		{
			// Deeply nested
			input:  `{"foo": [[{"apiVersion": "test", "kind": "Foo"}], {"apiVersion": "test", "kind": "Bar"}]}`,
			result: []interface{}{barObj, fooObj},
		},
		{
			// Deeply nested with provenance
			input:      `{"foo": [{"quz": {"apiVersion": "test", "kind": "Foo"}}, {"apiVersion": "test", "kind": "Bar"}]}`,
			provenance: true,
			result:     []interface{}{barObjP, fooObjP},
		},
		{
			// Deeply nested with provenance
			input:      `{"foo": [{"quz": {"apiVersion": "test", "kind": "Foo", "metadata": {}}}, {"apiVersion": "test", "kind": "Bar"}]}`,
			provenance: true,
			result:     []interface{}{barObjP, fooObjP},
		},
		{
			// Deeply nested with provenance
			input:      `{"foo": [{"quz": {"apiVersion": "test", "kind": "Foo", "metadata": {"annotations":{}}}}, {"apiVersion": "test", "kind": "Bar"}]}`,
			provenance: true,
			result:     []interface{}{barObjP, fooObjP},
		},
		{
			// Deeply nested with provenance requiring escaping
			input:      `{"foo": [{"self": {"apiVersion": "test", "kind": "Baz"}}]}`,
			provenance: true,
			result:     []interface{}{bazObjP},
		},
		{
			// Deeply nested with provenance requiring escaping (2)
			input:      `{"foo": [{"1a": {"apiVersion": "test", "kind": "Baz"}}]}`,
			provenance: true,
			result:     []interface{}{baz2ObjP},
		},
		{
			// Error: nested misplaced value
			input: `{"foo": {"bar": [null, 42]}}`,
			error: "Looking for kubernetes object at \"$.foo.bar[1]\", but instead found float64",
		},
	}

	for i, test := range tests {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Logf("%d: %s, %v", i, test.input, test.provenance)
			var top interface{}
			if err := json.Unmarshal([]byte(test.input), &top); err != nil {
				t.Fatalf("Failed to unmarshal %q: %v", test.input, err)
			}
			objs := []interface{}{}
			err := jsonWalk(&walkContext{label: "$"}, top, func(c *walkContext, obj *unstructured.Unstructured) error {
				if test.provenance {
					annotateProvenance(c, obj)
				}
				objs = append(objs, obj.Object)
				return nil
			})
			if test.error != "" {
				// expect error
				if err == nil {
					t.Fatalf("Test %d failed to fail", i)
				}
				if err.Error() != test.error {
					t.Fatalf("Test %d failed with %q but expected %q", i, err, test.error)
				}
				return
			}

			// expect success
			if err != nil {
				t.Fatalf("Test %d failed: %v", i, err)
			}
			keyFunc := func(i int) string {
				v := objs[i].(map[string]interface{})
				return v["kind"].(string)
			}
			sort.Slice(objs, func(i, j int) bool {
				return keyFunc(i) < keyFunc(j)
			})
			if !reflect.DeepEqual(objs, test.result) {
				t.Errorf("Expected %v, got %v", test.result, objs)
			}
		})
	}
}

func TestJsonnetPathAccessor(t *testing.T) {
	testCases := []struct {
		input string
		want  string
	}{
		{"foo", ".foo"},
		{"1foo", `["1foo"]`},
		{"a b", `["a b"]`},
		{"a-b", `["a-b"]`},
		{"a,b", `["a,b"]`},
		{"a:b", `["a:b"]`},
		{"a+b", `["a+b"]`},
		{"self", `["self"]`},
		{"super", `["super"]`},
		{"if", `["if"]`},
		{"local", `["local"]`},
		{"import", `["import"]`},
		{"importbin", `["importbin"]`},
		{"tailstrict", `["tailstrict"]`},
		{"function", `["function"]`},
		{"false", `["false"]`},
		{"true", `["true"]`},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Logf("%d: %q", i, tc.input)

			if got, want := jsonnetPathAccessor(tc.input), tc.want; got != want {
				t.Errorf("got: %q, want: %q", got, want)
			}
		})
	}
}
