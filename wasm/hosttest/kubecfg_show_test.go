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

// Package hosttest runs the WebAssembly build of kubecfg (../cmd/kubecfg-show) the way an
// embedding Go program would, with wazero. It is a separate module so that kubecfg itself
// doesn't depend on wazero.
package hosttest

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

func TestKubecfgShow(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and compiles a large wasm module")
	}
	bin := filepath.Join(t.TempDir(), "kubecfg-show.wasm")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = "../cmd/kubecfg-show" // built within the kubecfg module
	build.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	wasm, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	yakl, err := os.ReadFile("../../examples/guestbook.yakl")
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	var stdout, stderr bytes.Buffer
	_, err = r.InstantiateWithConfig(ctx, wasm, wazero.NewModuleConfig().
		WithArgs("kubecfg-show", "--features=yakl", "app.yakl").
		WithEnv("PWD", "/src").
		WithFSConfig(wazero.NewFSConfig().WithFSMount(fstest.MapFS{
			"app.yakl": {Data: yakl},
		}, "/src")).
		WithStdout(&stdout).
		WithStderr(&stderr))
	if err != nil {
		t.Fatalf("run: %v\nstderr: %s", err, stderr.String())
	}

	var objs []struct{ Kind string }
	if err := json.Unmarshal(stdout.Bytes(), &objs); err != nil {
		t.Fatalf("%v: %s", err, stdout.String())
	}
	if len(objs) != 3 || objs[0].Kind != "Deployment" {
		t.Errorf("unexpected objects: %+v", objs)
	}
}
