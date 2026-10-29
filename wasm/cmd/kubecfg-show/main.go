// Copyright 2026 The kubecfg authors
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

// Command kubecfg-show is the entry point of the WebAssembly (wasip1) build of kubecfg.
// Natively, use the kubecfg CLI instead.
//
// It renders the given Jsonnet/yakl/YAML/JSON files like `kubecfg show -o json` and
// writes the resulting Kubernetes objects to stdout as a JSON array. It never touches the
// network or a cluster: imports resolve only against the filesystem the host provides
// and kubecfg's embedded libraries.
//
//	GOOS=wasip1 GOARCH=wasm go build -o kubecfg-show.wasm ./wasm/cmd/kubecfg-show
//	wasmtime run --dir=/src kubecfg-show.wasm --features=yakl /src/app.yakl
package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-jsonnet"
	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/kubecfg/kubecfg/pkg/features"
	"github.com/kubecfg/kubecfg/utils"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := pflag.NewFlagSet("kubecfg-show", pflag.ContinueOnError)
	alpha := flags.Bool("alpha", false, "Enable all alpha features; prefer --features")
	featureItems := flags.StringSlice("features", nil, "Turn features on or off: name, name=true or name=false (repeatable)")
	vm := jsonnet.MakeVM()
	setters := map[string]func(string, string){
		"ext-str":  vm.ExtVar,
		"ext-code": vm.ExtCode,
		"tla-str":  vm.TLAVar,
		"tla-code": vm.TLACode,
	}
	values := map[string]*[]string{}
	for name := range setters {
		values[name] = flags.StringArray(name, nil, "Set a Jsonnet variable as name=value (repeatable)")
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() == 0 {
		return fmt.Errorf("usage: kubecfg-show [flags] FILE...")
	}

	for name, set := range setters {
		for _, entry := range *values[name] {
			k, v, ok := strings.Cut(entry, "=")
			if !ok {
				return fmt.Errorf("--%s %q: missing '='", name, entry)
			}
			set(k, v)
		}
	}
	gates, err := features.New(*alpha, features.Source{Name: "--features", Items: *featureItems})
	if err != nil {
		return err
	}
	vm.Importer(utils.MakeUniversalImporterWithFeatures([]*url.URL{{Scheme: "internal", Path: "/"}}, gates))
	utils.RegisterNativeFuncs(vm, utils.NewIdentityResolver())
	utils.RegisterFeatures(vm, gates)

	var objs []*unstructured.Unstructured
	for _, path := range flags.Args() {
		// Under wasip1 the working directory is "." unless the host sets $PWD.
		if abs, _ := filepath.Abs(path); !filepath.IsAbs(abs) {
			return fmt.Errorf("%s: relative paths need $PWD set to an absolute directory", path)
		}
		read, err := utils.Read(vm, path)
		if err != nil {
			return fmt.Errorf("error reading %s: %w", path, err)
		}
		objs = append(objs, utils.FlattenToV1(read)...)
	}
	objs, err = utils.RemoveDuplicates(objs)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(objs)
}
