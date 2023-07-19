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

package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	// see pkg/show.go for the rationale of using a fork of go-yaml
	"github.com/kubecfg/yaml/v2"
)

func resetFlags() {
	resetFlagsOf(RootCmd)
}

func resetFlagsOf(cmd *cobra.Command) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			sv.Replace(nil)
		} else {
			f.Value.Set(f.DefValue)
		}
		f.Changed = false
	})
	for _, c := range cmd.Commands() {
		resetFlagsOf(c)
	}
}

func cmdOutput(t *testing.T, args []string) string {
	defer resetFlags()

	var buf bytes.Buffer
	RootCmd.SetOutput(&buf)
	defer RootCmd.SetOutput(nil)

	t.Log("Running args", args)
	RootCmd.SetArgs(args)

	if err := RootCmd.Execute(); err != nil {
		t.Fatal("command failed:", err)
	}

	return buf.String()
}

func TestShow(t *testing.T) {
	formats := map[string]func(string) (interface{}, error){
		"json": func(text string) (ret interface{}, err error) {
			err = json.Unmarshal([]byte(text), &ret)
			return
		},
		"yaml": func(text string) (ret interface{}, err error) {
			err = yaml.Unmarshal([]byte(text), &ret)
			return
		},
	}

	// Use the fact that JSON is also valid YAML ..
	expected := `
{
  "apiVersion": "v0alpha1",
  "kind": "TestObject",
  "nil": null,
  "bool": true,
  "number": 42,
  "string": "bar",
  "notAVal": "aVal",
  "notAnotherVal": "aVal2",
  "filevar": "foo\n",
  "array": ["one", 2, [3]],
  "object": {"foo": "bar"},
  "extcode": {"foo": 1, "bar": "test"},
  "extVarEnvDefined": "DEFINED",
  "extVarEnvUnDefined": "FOO"
}
`

	for format, parser := range formats {
		expected, err := parser(expected)
		if err != nil {
			t.Fatalf("error parsing *expected* value: %v", err)
		}

		os.Setenv("anVar", "aVal2")
		os.Setenv("extVarEnvDefined", "DEFINED")
		defer os.Unsetenv("anVar")
		defer os.Unsetenv("extVarEnvDefined")

		output := cmdOutput(t, []string{"show",
			"-J", filepath.FromSlash("../testdata/lib"),
			"-o", format,
			filepath.FromSlash("../testdata/test.jsonnet"),
			"-V", "aVar=aVal",
			"-V", "anVar",
			"--ext-str-file", "filevar=" + filepath.FromSlash("../testdata/extvar.file"),
			"--ext-code", `extcode={foo: 1, bar: "test"}`,
			"--ext-str-env", "extVarEnvDefined=FOO",
			"--ext-str-env", "extVarEnvUnDefined=FOO",
		})

		t.Log("output is", output)
		actual, err := parser(output)
		if err != nil {
			t.Errorf("error parsing output of format %s: %v", format, err)
		} else if !reflect.DeepEqual(expected, actual) {
			t.Errorf("format %s expected != actual: %s != %s", format, expected, actual)
		}
	}
}

func TestShowUsingExtVarFiles(t *testing.T) {
	expectedText := `
{
  "apiVersion": "v1",
  "kind": "ConfigMap",
  "metadata": {
    "name": "sink"
  },
  "data": {
    "input": {
      "greeting": "Hello!",
      "helper": true,
      "top": true
    },
    "var": "I'm a var!"
  }
}
`
	var expected interface{}
	if err := json.Unmarshal([]byte(expectedText), &expected); err != nil {
		t.Fatalf("error parsing *expected* value: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}
	if err := os.Chdir("../testdata/extvars/feed"); err != nil {
		t.Fatalf("failed to change to target directory: %v", err)
	}
	defer os.Chdir(cwd)

	output := cmdOutput(t, []string{"show",
		"top.jsonnet",
		"-o", "json",
		"--tla-code-file", "input=input.jsonnet",
		"--tla-code-file", "sink=sink.jsonnet",
		"--ext-str-file", "filevar=var.txt",
	})

	t.Log("output is", output)
	var actual interface{}
	err = json.Unmarshal([]byte(output), &actual)
	if err != nil {
		t.Errorf("error parsing output: %v", err)
	} else if !reflect.DeepEqual(expected, actual) {
		t.Errorf("expected != actual: %s != %s", expected, actual)
	}
}

func TestShowUsingURLs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{
			apiVersion: "v1",
			kind: "ConfigMap",
			metadata: { name: "foo"},
			data: { "foo": "bar" }
}`)
	}))
	t.Cleanup(server.Close)

	got := cmdOutput(t, []string{"show", server.URL, "-o", "json"})

	want := `{
  "apiVersion": "v1",
  "data": {
    "foo": "bar"
  },
  "kind": "ConfigMap",
  "metadata": {
    "name": "foo"
  }
}
`
	if got != want {
		t.Fatalf("got: %q, want: %q", got, want)
	}
}

func TestShowExec(t *testing.T) {
	const (
		input = `{
	apiVersion: "v1",
	kind: "ConfigMap",
	metadata: { name: "foo"},
	data: { "foo": "bar" }
}`

		want = `{
  "apiVersion": "v1",
  "data": {
    "foo": "bar"
  },
  "kind": "ConfigMap",
  "metadata": {
    "name": "foo"
  }
}
`
	)

	got := cmdOutput(t, []string{"show", "-e", input, "-o", "json"})

	if got != want {
		t.Fatalf("got: %q, want: %q", got, want)
	}
}

func TestShowOverlayCodeFile(t *testing.T) {
	// TODO(mkm): fix the reset flags utilities.
	// This hack ensures that any global state left over from other tests doesn't affect this test
	// and that any global state left over from this test doesn't affect other tests
	resetFlagsOf(RootCmd)
	resetFlagsOf(showCmd)
	defer resetFlagsOf(RootCmd)
	defer resetFlagsOf(showCmd)

	got := cmdOutput(t, []string{
		"--alpha",
		"show",
		filepath.FromSlash("../testdata/configmap.jsonnet"),
		filepath.FromSlash("../testdata/secret.jsonnet"),
	})
	want := `---
apiVersion: v1
kind: ConfigMap
metadata:
  name: test
---
apiVersion: v1
kind: Secret
metadata:
  name: test
`

	if got != want {
		t.Fatalf("got: %q, want: %q", got, want)
	}

	got = cmdOutput(t, []string{
		"--alpha",
		"show",
		filepath.FromSlash("../testdata/configmap.jsonnet"),
		filepath.FromSlash("../testdata/secret.jsonnet"),
		"--overlay-code-file",
		filepath.FromSlash("../testdata/namespace-overlay.jsonnet")})
	want = `---
apiVersion: v1
kind: ConfigMap
metadata:
  name: test
  namespace: myns
---
apiVersion: v1
kind: Secret
metadata:
  name: test
  namespace: myns
`

	if got != want {
		t.Fatalf("got: %q, want: %q", got, want)
	}
}

func TestShowOverlayCode(t *testing.T) {
	// TODO(mkm): fix the reset flags utilities.
	// This hack ensures that any global state left over from other tests doesn't affect this test
	// and that any global state left over from this test doesn't affect other tests
	resetFlagsOf(RootCmd)
	resetFlagsOf(showCmd)
	defer resetFlagsOf(RootCmd)
	defer resetFlagsOf(showCmd)

	got := cmdOutput(t, []string{
		"--alpha",
		"show",
		filepath.FromSlash("../testdata/configmap.jsonnet"),
		filepath.FromSlash("../testdata/secret.jsonnet"),
	})
	want := `---
apiVersion: v1
kind: ConfigMap
metadata:
  name: test
---
apiVersion: v1
kind: Secret
metadata:
  name: test
`

	if got != want {
		t.Fatalf("got: %q, want: %q", got, want)
	}

	got = cmdOutput(t, []string{
		"--alpha",
		"show",
		filepath.FromSlash("../testdata/configmap.jsonnet"),
		filepath.FromSlash("../testdata/secret.jsonnet"),
		"--overlay-code",
		`{metadata+:{namespace: 'myns'}}`})
	want = `---
apiVersion: v1
kind: ConfigMap
metadata:
  name: test
  namespace: myns
---
apiVersion: v1
kind: Secret
metadata:
  name: test
  namespace: myns
`

	if got != want {
		t.Fatalf("got: %q, want: %q", got, want)
	}
}
