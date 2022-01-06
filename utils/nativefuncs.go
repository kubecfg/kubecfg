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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	goyaml "github.com/ghodss/yaml"

	jsonnet "github.com/google/go-jsonnet"
	jsonnetAst "github.com/google/go-jsonnet/ast"
	log "github.com/sirupsen/logrus"
	helmLoader "helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	helmEngine "helm.sh/helm/v3/pkg/engine"
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

// RegisterNativeFuncs adds kubecfg's native jsonnet functions to provided VM
func RegisterNativeFuncs(vm *jsonnet.VM, resolver Resolver, allowRelativeHelmURLs bool) {
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
		Name:   "helmTemplate",
		Params: []jsonnetAst.Identifier{"releaseName", "namespace", "chartURL", "values"},
		Func: func(args []interface{}) (interface{}, error) {
			releaseName := args[0].(string)
			namespace := args[1].(string)
			chartURL := args[2].(string)
			vals := args[3].(map[string]interface{})

			// TODO: Support URLs relative to source file.
			// We could ask the caller to pass in
			// std.thisFile, but a new `importbin` jsonnet
			// statement would be more sandboxed.  For
			// now, just hide it behind an opt-in flag.
			if u, err := url.Parse(chartURL); err != nil {
				return nil, fmt.Errorf("invalid chartURL %q: %v", chartURL, err)
			} else if !allowRelativeHelmURLs && !u.IsAbs() {
				return nil, fmt.Errorf("rejecting relative helm chart URL %q", chartURL)
			}
			cwd, err := os.Getwd()
			if err != nil {
				return nil, err
			}
			cwdURL := dirURL(cwd).String()

			// Cheating a bit here: Theoretically,
			// `contents` is a (UTF8) string, but we're
			// relying on the fact that golang doesn't
			// care.
			contents, foundAt, err := vm.ImportData(cwdURL, chartURL)
			if err != nil {
				return nil, err
			}
			reader := bytes.NewReader([]byte(contents))

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

			ret := make(map[string]interface{}, len(strobjs))

			for _, crd := range chrt.CRDObjects() {
				objs, err := unmarshalYAMLString(string(crd.File.Data))
				if err != nil {
					return nil, fmt.Errorf("failed to parse CRD in file %q from helm chart %q: %v", crd.Filename, foundAt, err)
				}

				if a, ok := objs.([]interface{}); ok && len(a) == 1 {
					ret[crd.Filename] = a[0]
				} else {
					ret[crd.Filename] = objs
				}
			}

			for key, value := range strobjs {
				if strings.HasSuffix(key, "NOTES.txt") {
					log.Debugf("NOTES:\n%s", value)
					continue
				}
				objs, err := unmarshalYAMLString(value)
				if err != nil {
					return nil, fmt.Errorf("failed to parse file %q from helm chart %q: %v", key, foundAt, err)
				}

				// Very common for helm charts to have
				// a single object per file and arrays
				// are harder to override/merge in
				// jsonnet, so optimise away the array
				// for this trivial case.
				if a, ok := objs.([]interface{}); ok && len(a) == 1 {
					ret[key] = a[0]
				} else {
					ret[key] = objs
				}
			}

			return ret, nil
		},
	})
}

func unmarshalYAMLString(yamlStr string) (interface{}, error) {
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
