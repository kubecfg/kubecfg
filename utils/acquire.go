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
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	jsonnet "github.com/google/go-jsonnet"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
)

const (
	AnnotationProvenanceFile = "kubecfg.github.com/provenance-file"
	AnnotationProvenancePath = "kubecfg.github.com/provenance-path"
)

type readOptions struct {
	showProvenance bool
	readTwice      bool
	expr           string
}

type ReadOption func(*readOptions)

func WithProvenance(show bool) ReadOption {
	return func(opts *readOptions) {
		opts.showProvenance = show
	}
}

func WithReadTwice(twice bool) ReadOption {
	return func(opts *readOptions) {
		opts.readTwice = twice
	}
}

func WithExpr(expr string) ReadOption {
	return func(opts *readOptions) {
		opts.expr = expr
	}
}

// Read fetches and decodes K8s objects by path.
// TODO: Replace this with something supporting more sophisticated
// content negotiation.
func Read(vm *jsonnet.VM, path string, opts ...ReadOption) ([]runtime.Object, error) {
	var opt readOptions
	for _, o := range opts {
		o(&opt)
	}

	if isURL(path) {
		return jsonnetReader(vm, path, opt)
	}

	switch filepath.Ext(path) {
	case ".json":
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return jsonReader(f)
	case ".yaml":
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return yamlReader(f)
	case ".jsonnet":
		return jsonnetReader(vm, path, opt)
	}
	return nil, fmt.Errorf("unknown file extension: %s", path)
}

func jsonReader(r io.Reader) ([]runtime.Object, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	obj, _, err := unstructured.UnstructuredJSONScheme.Decode(data, nil, nil)
	if err != nil {
		return nil, err
	}
	return []runtime.Object{obj}, nil
}

func yamlReader(r io.ReadCloser) ([]runtime.Object, error) {
	decoder := yaml.NewYAMLReader(bufio.NewReader(r))
	ret := []runtime.Object{}
	for {
		bytes, err := decoder.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		if len(bytes) == 0 {
			continue
		}
		jsondata, err := yaml.ToJSON(bytes)
		if err != nil {
			return nil, err
		}
		obj, _, err := unstructured.UnstructuredJSONScheme.Decode(jsondata, nil, nil)
		if err != nil {
			return nil, err
		}
		ret = append(ret, obj)
	}
	return ret, nil
}

type walkContext struct {
	parent *walkContext
	label  string
	file   string
}

func (c *walkContext) path() string {
	parent := ""
	if c.parent != nil {
		parent = c.parent.path()
	}
	return parent + c.label
}

func (c *walkContext) child(label string) *walkContext {
	return &walkContext{
		parent: c,
		label:  label,
		file:   c.file,
	}
}

func annotateProvenance(ctx *walkContext, o *unstructured.Unstructured) {
	if file := ctx.file; file != "" {
		SetMetaDataAnnotation(o, AnnotationProvenanceFile, file)
	}
	SetMetaDataAnnotation(o, AnnotationProvenancePath, ctx.path())
}

func jsonWalk(parentCtx *walkContext, obj interface{}, visitor func(c *walkContext, obj *unstructured.Unstructured) error) error {
	switch o := obj.(type) {
	case nil:
		return nil
	case map[string]interface{}:
		if o["kind"] != nil && o["apiVersion"] != nil {
			obj := unstructured.Unstructured{Object: o}
			if obj.IsList() {
				return obj.EachListItem(func(item runtime.Object) error {
					return visitor(parentCtx.child(".item"), item.(*unstructured.Unstructured))
				})
			}
			return visitor(parentCtx, &obj)
		}
		// Use consistent traversal order
		keys := make([]string, 0, len(o))
		for k := range o {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			v := o[k]
			if err := jsonWalk(parentCtx.child(fmt.Sprintf(".%s", k)), v, visitor); err != nil {
				return err
			}
		}
		return nil
	case []interface{}:
		for i, v := range o {
			err := jsonWalk(parentCtx.child(fmt.Sprintf("[%d]", i)), v, visitor)
			if err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("Looking for kubernetes object at %q, but instead found %T", parentCtx.path(), o)
	}
}

func PathToURL(path string) (string, error) {
	if isURL(path) {
		return path, nil
	}
	// if it's not an URL already, turn it into a file URL.
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}).String(), nil
}

func isURL(path string) bool {
	// TODO: figure a better way to tell filepaths and URLs apart (it also must work on windows...)
	return strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") || strings.HasPrefix(path, "oci://") || strings.HasPrefix(path, "file://")
}

func jsonnetReader(vm *jsonnet.VM, path string, opts readOptions) ([]runtime.Object, error) {
	// TODO(mkm): evaluate expressions in opts.expr

	pathURL, err := PathToURL(path)
	if err != nil {
		return nil, err
	}
	content, foundAt, err := vm.ImportData(pathURL, pathURL)
	if err != nil {
		return nil, err
	}

	jsonstr, err := vm.EvaluateSnippet(foundAt, content)
	if err != nil {
		return nil, err
	}

	log.Debugf("jsonnet result is: %s", jsonstr)

	if opts.readTwice {
		str2, err := vm.EvaluateSnippet(foundAt, content)
		if err != nil {
			return nil, fmt.Errorf("error re-reading %s: %w", foundAt, err)
		}

		if jsonstr != str2 {
			return nil, fmt.Errorf("repeat read of %s returned non-idempotent result", foundAt)
		}
	}

	var top interface{}
	if err = json.Unmarshal([]byte(jsonstr), &top); err != nil {
		return nil, err
	}

	var ret []runtime.Object
	visitor := func(c *walkContext, obj *unstructured.Unstructured) error {
		if opts.showProvenance {
			annotateProvenance(c, obj)
		}
		ret = append(ret, obj)
		return nil
	}

	if err := jsonWalk(&walkContext{file: path, label: "$"}, top, visitor); err != nil {
		return nil, err
	}

	return ret, nil
}

// FlattenToV1 expands any List-type objects into their members, and
// cooerces everything to v1.Unstructured.  Panics if coercion
// encounters an unexpected object type.
func FlattenToV1(objs []runtime.Object) []*unstructured.Unstructured {
	ret := make([]*unstructured.Unstructured, 0, len(objs))
	for _, obj := range objs {
		switch o := obj.(type) {
		case *unstructured.UnstructuredList:
			for i := range o.Items {
				ret = append(ret, &o.Items[i])
			}
		case *unstructured.Unstructured:
			ret = append(ret, o)
		default:
			panic("Unexpected unstructured object type")
		}
	}
	return ret
}
