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

package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestYaklfmt(t *testing.T) {
	file := filepath.Join(t.TempDir(), "app.yakl")
	if err := os.WriteFile(file, []byte("a: ${1+2}\nb: ${ {c:1} }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	const want = "a: ${1 + 2}\nb: '${ { c: 1 } }'\n"
	if got := cmdOutput(t, []string{"--alpha", "yaklfmt", file}); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := cmdOutput(t, []string{"--alpha", "yaklfmt", "-i", file}); got != "" {
		t.Errorf("--in-place printed %q", got)
	}
	if got, err := os.ReadFile(file); err != nil || string(got) != want {
		t.Errorf("rewrote %q, %v, want %q", got, err, want)
	}
}
