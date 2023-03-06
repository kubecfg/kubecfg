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
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// CheckDuplicates returns error if the provided object slice contains multiple
// objects sharing the same version/kind/namespace/name combination.
func CheckDuplicates(objs []*unstructured.Unstructured) error {
	seen := map[string]string{}
	for _, o := range objs {
		k := fmt.Sprintf("%s, %q, %q", o.GroupVersionKind().GroupKind(), o.GetNamespace(), o.GetName())
		v := hash(o)
		if h, found := seen[k]; found {
			// allow but elide literal duplicates
			if h == v {
				continue
			}
			return fmt.Errorf("duplicate resource %s", k)
		}
		seen[k] = v
	}
	return nil
}

func hash(obj *unstructured.Unstructured) string {
	h := sha1.New()
	// ignore error based on the unvalidated assumption that we already have a valid, marshallable unstructured object
	json, _ := obj.MarshalJSON()
	h.Write([]byte(json))

	return hex.EncodeToString(h.Sum(nil))
}
