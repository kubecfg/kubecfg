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
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	goyaml "github.com/ghodss/yaml"
	jsonnet "github.com/google/go-jsonnet"
	jsonnetAst "github.com/google/go-jsonnet/ast"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"
	log "github.com/sirupsen/logrus"
	helmLoader "helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	helmEngine "helm.sh/helm/v3/pkg/engine"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func resolveImage(resolver Resolver, image string) (string, error) {
	n, err := ParseImageName(image)
	if err != nil {
		return "", err
	}

	if err := resolver.Resolve(&n); err != nil {
		return "", err
	}

	return n.String(), nil
}

type ArrayReader struct {
	buf []interface{}
}

var errBadByte = errors.New("expected number 0-255")

func (r *ArrayReader) Read(p []byte) (int, error) {
	n := len(r.buf)
	if n == 0 {
		return 0, io.EOF
	}
	if len(p) < n {
		n = len(p)
	}

	for i := 0; i < n; i++ {
		num, ok := r.buf[i].(float64)
		if !ok || num < 0 || num > 255 {
			return i, errBadByte
		}
		p[i] = byte(num)
	}

	r.buf = r.buf[n:]
	return n, nil
}

// RegisterNativeFuncs adds kubecfg's native jsonnet functions to provided VM
func RegisterNativeFuncs(vm *jsonnet.VM, resolver Resolver) {
	// TODO(mkm): go-jsonnet 0.12.x now contains native std.parseJson; deprecate and remove this one.
	vm.NativeFunction(&jsonnet.NativeFunction{
		Name:   "parseJson",
		Params: []jsonnetAst.Identifier{"json"},
		Func: func(args []interface{}) (res interface{}, err error) {
			data := []byte(args[0].(string))
			err = json.Unmarshal(data, &res)
			return
		},
	})

	vm.NativeFunction(&jsonnet.NativeFunction{
		Name:   "parseYaml",
		Params: []jsonnetAst.Identifier{"yaml"},
		Func: func(args []interface{}) (res interface{}, err error) {
			return unmarshalYAMLString(args[0].(string))
		},
	})

	vm.NativeFunction(&jsonnet.NativeFunction{
		Name:   "manifestJson",
		Params: []jsonnetAst.Identifier{"json", "indent"},
		Func: func(args []interface{}) (res interface{}, err error) {
			value := args[0]
			indent := int(args[1].(float64))
			data, err := json.MarshalIndent(value, "", strings.Repeat(" ", indent))
			if err != nil {
				return "", err
			}
			data = append(data, byte('\n'))
			return string(data), nil
		},
	})

	vm.NativeFunction(&jsonnet.NativeFunction{
		Name:   "manifestYaml",
		Params: []jsonnetAst.Identifier{"json"},
		Func: func(args []interface{}) (res interface{}, err error) {
			value := args[0]
			output, err := goyaml.Marshal(value)
			return string(output), err
		},
	})

	vm.NativeFunction(&jsonnet.NativeFunction{
		Name:   "resolveImage",
		Params: []jsonnetAst.Identifier{"image"},
		Func: func(args []interface{}) (res interface{}, err error) {
			return resolveImage(resolver, args[0].(string))
		},
	})

	vm.NativeFunction(&jsonnet.NativeFunction{
		Name:   "escapeStringRegex",
		Params: []jsonnetAst.Identifier{"str"},
		Func: func(args []interface{}) (res interface{}, err error) {
			return regexp.QuoteMeta(args[0].(string)), nil
		},
	})

	vm.NativeFunction(&jsonnet.NativeFunction{
		Name:   "regexMatch",
		Params: []jsonnetAst.Identifier{"regex", "string"},
		Func: func(args []interface{}) (res interface{}, err error) {
			return regexp.MatchString(args[0].(string), args[1].(string))
		},
	})

	vm.NativeFunction(&jsonnet.NativeFunction{
		Name:   "regexSubst",
		Params: []jsonnetAst.Identifier{"regex", "src", "repl"},
		Func: func(args []interface{}) (res interface{}, err error) {
			regex := args[0].(string)
			src := args[1].(string)
			repl := args[2].(string)

			r, err := regexp.Compile(regex)
			if err != nil {
				return "", err
			}
			return r.ReplaceAllString(src, repl), nil
		},
	})

	vm.NativeFunction(&jsonnet.NativeFunction{
		Name:   "parseHelmChart",
		Params: []jsonnetAst.Identifier{"releaseName", "namespace", "chartData", "values"},
		Func: func(args []interface{}) (interface{}, error) {
			chartData := args[0].([]interface{})
			releaseName := args[1].(string)
			namespace := args[2].(string)
			vals := args[3].(map[string]interface{})

			reader := &ArrayReader{chartData}

			chrt, err := helmLoader.LoadArchive(reader)
			if err != nil {
				return nil, err
			}
			log.Debugf("Loaded helm chart %s/%s", chrt.Name(), chrt.AppVersion())

			options := chartutil.ReleaseOptions{
				Name:      releaseName,
				Namespace: namespace,
				Revision:  1,
				IsInstall: true,
			}
			values, err := chartutil.ToRenderValues(chrt, vals, options, nil)
			if err != nil {
				return nil, err
			}

			engine := helmEngine.Engine{}
			strobjs, err := engine.Render(chrt, values)
			if err != nil {
				return nil, err
			}

			ret := make(map[string]interface{}, len(chrt.CRDObjects())+len(strobjs))

			for _, crd := range chrt.CRDObjects() {
				objs, err := unmarshalYAMLString(string(crd.File.Data))
				if err != nil {
					return nil, fmt.Errorf("failed to parse CRD in file %q from helm chart: %v", crd.Filename, err)
				}

				ret[crd.Filename] = objs
			}

			for key, value := range strobjs {
				if strings.HasSuffix(key, "NOTES.txt") {
					log.Debugf("NOTES:\n%s", value)
					continue
				}
				objs, err := unmarshalYAMLString(value)
				if err != nil {
					return nil, fmt.Errorf("failed to parse file %q from helm chart: %v", key, err)
				}

				// helm charts often don't specify a
				// namespace - see helm/helm#5465.
				// kubecfg always specifies
				// namespaces, so fix that up here.
				for i := range objs {
					if objs[i] == nil {
						continue
					}
					if o, ok := objs[i].(map[string]interface{}); ok {
						obj := unstructured.Unstructured{Object: o}
						// Cheat and just set namespace on everything, even non-namespaced objects. Server will ignore namespace where it isn't relevant.
						if obj.GetNamespace() == "" {
							obj.SetNamespace(namespace)
						}
					} else {
						log.Debugf("Unexpected object type in helm chart: %T", objs[i])
					}
				}

				ret[key] = objs
			}

			return ret, nil
		},
	})

	vm.NativeFunction(&jsonnet.NativeFunction{
		Name:   "validateJSONSchema",
		Params: []jsonnetAst.Identifier{"obj", "schema"},
		Func: func(args []interface{}) (interface{}, error) {
			obj := args[0]
			schema := args[1]

			jSchema, err := json.Marshal(schema)
			if err != nil {
				return nil, fmt.Errorf("unable to json marshal schema: %w", err)
			}

			// No URL defaults to draft 7 of JSONSchema
			sch, err := jsonschema.CompileString("", string(jSchema))
			if err != nil {
				return false, fmt.Errorf("unable to compile jsonschema: %w", err)
			}

			err = sch.Validate(obj)
			if err != nil {
				return nil, fmt.Errorf("object is invalid against the schema: %w", err)
			}

			return true, nil
		},
	})
}

func unmarshalYAMLString(yamlStr string) ([]interface{}, error) {
	d := yaml.NewYAMLToJSONDecoder(strings.NewReader(yamlStr))
	var ret []interface{}
	for {
		var doc interface{}
		if err := d.Decode(&doc); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		ret = append(ret, doc)
	}
	return ret, nil
}

// NB: `path` is assumed to be in native-OS path separator form
func dirURL(path string) *url.URL {
	path = filepath.ToSlash(path)
	if path[len(path)-1] != '/' {
		// trailing slash is important
		path = path + "/"
	}
	return &url.URL{Scheme: "file", Path: path}
}
