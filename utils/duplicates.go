// Copyright 2021 The kubecfg authors
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
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type objectHashPair struct {
	object *unstructured.Unstructured
	hash   string
}

// RemoveDuplicates returns error if the provided object slice contains multiple
// objects sharing the same version/kind/namespace/name combination that are not literal matches.
func RemoveDuplicates(objs []*unstructured.Unstructured) ([]*unstructured.Unstructured, error) {
	seen := map[string]objectHashPair{}
	for _, o := range objs {
		k := fmt.Sprintf("%s, %q, %q", o.GroupVersionKind().GroupKind(), o.GetNamespace(), o.GetName())
		v := objectHashPair{o, hash(o)}
		if h, found := seen[k]; found {
			// allow but elide literal duplicates
			if h.hash == v.hash {
				continue
			}
			return nil, fmt.Errorf("duplicate resource %s", k)
		}
		seen[k] = v
	}

	ret := make([]*unstructured.Unstructured, 0, len(seen))
	for _, v := range seen {
		ret = append(ret, v.object)
	}
	return ret, nil
}

func hash(obj *unstructured.Unstructured) string {
	h := sha1.New()
	// strip provenance annotations as they have the potential to make every object unique
	if err := json.NewEncoder(h).Encode(withoutProvenanceAnnotations(obj)); err != nil {
		panic(fmt.Errorf("unexpected error encoding unstructured object as json: %w", err))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func withoutProvenanceAnnotations(obj *unstructured.Unstructured) *unstructured.Unstructured {
	copy := obj.DeepCopy()

	annotations := copy.GetAnnotations()
	if annotations == nil {
		return copy
	}

	if _, ok := annotations[AnnotationProvenanceFile]; ok {
		delete(annotations, AnnotationProvenanceFile)
	}
	if _, ok := annotations[AnnotationProvenancePath]; ok {
		delete(annotations, AnnotationProvenancePath)
	}

	copy.SetAnnotations(annotations)

	return copy
}
