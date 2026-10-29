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
	"encoding/json"
	"flag"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestKnitImport(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"one object", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: foo\n",
			knitHeader + "---\n^^: foo.ConfigMap\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: foo\n"},
		{"comments stay", "# top\n---\n# cm\napiVersion: v1\nkind: ConfigMap\nmetadata: {name: foo} # x\n",
			knitHeader + "# top\n---\n# cm\n^^: foo.ConfigMap\napiVersion: v1\nkind: ConfigMap\nmetadata: {name: foo} # x\n"},
		{"a dot nests", "kind: CustomResourceDefinition\nmetadata: {name: foos.example.com}\n",
			knitHeader + "---\n^^: foos.example.com.CustomResourceDefinition\nkind: CustomResourceDefinition\nmetadata: {name: foos.example.com}\n"},
		{"the namespace when two objects need it",
			"kind: ConfigMap\nmetadata: {name: foo, namespace: a}\n---\nkind: ConfigMap\nmetadata: {name: foo, namespace: b}\n---\nkind: Secret\nmetadata: {name: foo, namespace: a}\n",
			knitHeader + "---\n^^: a.foo.ConfigMap\nkind: ConfigMap\nmetadata: {name: foo, namespace: a}\n---\n^^: b.foo.ConfigMap\nkind: ConfigMap\nmetadata: {name: foo, namespace: b}\n---\n^^: foo.Secret\nkind: Secret\nmetadata: {name: foo, namespace: a}\n"},
		{"empty documents stay", "---\n---\nkind: ConfigMap\nmetadata: {name: foo}\n---\n",
			knitHeader + "---\n---\n^^: foo.ConfigMap\nkind: ConfigMap\nmetadata: {name: foo}\n---\n"},
		{"knit's fields get a _", "kind: Service\nmetadata: {name: registry}\n---\nkind: ConfigMap\nmetadata: {name: foo, namespace: manifests}\n---\nkind: ConfigMap\nmetadata: {name: foo, namespace: b}\n",
			knitHeader + "---\n^^: _registry.Service\nkind: Service\nmetadata: {name: registry}\n---\n^^: _manifests.foo.ConfigMap\nkind: ConfigMap\nmetadata: {name: foo, namespace: manifests}\n---\n^^: b.foo.ConfigMap\nkind: ConfigMap\nmetadata: {name: foo, namespace: b}\n"},
		{"line breaks of \\r\\n and \\r", "# top\r\nkind: ConfigMap\rmetadata: {name: foo}\r",
			knitHeader + "---\n# top\n^^: foo.ConfigMap\nkind: ConfigMap\nmetadata: {name: foo}\n"},
		{"a List of another group is an object", "apiVersion: example.com/v1\nkind: List\nmetadata: {name: l}\n",
			knitHeader + "---\n^^: l.List\napiVersion: example.com/v1\nkind: List\nmetadata: {name: l}\n"},
		{"a date is a string", "kind: ConfigMap\nmetadata: {name: 2026-10-28}\n",
			knitHeader + "---\n^^: 2026-10-28.ConfigMap\nkind: ConfigMap\nmetadata: {name: 2026-10-28}\n"},
		{"an indented document", "  kind: ConfigMap\n  metadata: {name: foo}\n",
			knitHeader + "---\n  ^^: foo.ConfigMap\n  kind: ConfigMap\n  metadata: {name: foo}\n"},
	} {
		got, err := knitImport(tc.src, nil)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
		} else if got != tc.want {
			t.Errorf("%s: got\n%s\nwant\n%s", tc.name, got, tc.want)
		}
	}
	for _, tc := range []struct{ src, want string }{
		{"kind: ConfigMap\nmetadata: {name: foo}\n---\nkind: ConfigMap\nmetadata: {name: foo}\n", "two objects would be named foo.ConfigMap"},
		{"--- {kind: ConfigMap, metadata: {name: foo}}\n", "line 1: ConfigMap foo needs its first key at the start of a line"},
		{"kind: ConfigMap\nmetadata: {name: foo}\ndata: {a: '${HOME}'}\n", `line 3: the string "${HOME}" would be a yakl expression; write it as ${"${HOME}"}`},
		{"kind: ConfigMap\nmetadata: {name: foo}\ndata: {a: yes}\n", "the file as yakl:"},
		{"kind: ConfigMap\nmetadata: {name: foo}\ndata: {a: ' ${\"keep me\"}'}\n", `line 3: the string " ${\"keep me\"}" would be a yakl expression`},
		{"kind: ConfigMap\nmetadata: {name: foo}\ndata: {a: \"\\n $${HOME}\"}\n", `line 3: the string "\n $${HOME}" would be a yakl expression`},
		{"kind: Thing\nmetadata: {name: foo}\nspec: {c++: 1}\n", `line 3: yakl reads the key "c++" differently; write it as ${"c++"}`},
		{"kind: Thing\nmetadata: {name: foo}\nspec: {'hidden:': 1}\n", `line 3: yakl reads the key "hidden:" differently`},
		{"kind: Thing\nmetadata: {name: foo}\nspec: {'&variable': 1}\n", `line 3: yakl reads the key "&variable" differently`},
		{"kind: Thing\nmetadata: {name: foo}\nspec: {key: &k feature+, *k : enabled}\n", `yakl reads the key "feature+" differently`},
		{"kind: Thing\nmetadata: {name: foo}\nspec: {'${a}': 1}\n", `line 3: yakl reads the key "${a}" differently`},
		{"kind: ConfigMap\nmetadata: {name: foo}\n---\na: 1\n", "line 4: a document without a kind and a metadata.name can't get a name"},
		{"kind: Job\nmetadata: {generateName: foo-}\n", "line 1: a document without a kind and a metadata.name"},
		{"kind: ConfigMap\nmetadata: {name: foo}\n--- ~\n", "line 3: a null document: delete it"},
		{"# a\u2028# b\u2028kind: ConfigMap\nmetadata: {name: foo}\n", "line 1: unsupported Unicode line separator"},
		{"kind: List\nitems: []\n", "line 1: a List: write each of its items as a document of its own"},
		{"kind: ConfigMap\nmetadata: {name: foo}\n$patch: delete\n", "line 1: ConfigMap foo has $patch: to leave out an object of the base, write a document of only ^^+::"},
	} {
		if _, err := knitImport(tc.src, nil); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: got %v, want %q", tc.src, err, tc.want)
		}
	}
}

// The imported file outputs the objects of the YAML file, in the order of their names.
func TestKnitImportShow(t *testing.T) {
	dir := t.TempDir()
	src := "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: foo, namespace: a}\ndata: {k: v}\n---\n" +
		"apiVersion: v1\nkind: ConfigMap\nmetadata: {name: foo, namespace: b}\n---\n" +
		"apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: api}\nspec:\n  replicas: 2\n"
	yamlFile, yaklFile := filepath.Join(dir, "app.yaml"), filepath.Join(dir, "app.yakl")
	if err := os.WriteFile(yamlFile, []byte(src), 0600); err != nil {
		t.Fatal(err)
	}
	out := cmdOutput(t, []string{"--alpha", "knit", "import", yamlFile})
	if err := os.WriteFile(yaklFile, []byte(out), 0600); err != nil {
		t.Fatal(err)
	}
	sorted := func(file string) string {
		docs := strings.Split(cmdOutput(t, []string{"--alpha", "show", file}), "---\n")
		slices.Sort(docs)
		return strings.Join(docs, "---\n")
	}
	if got, want := sorted(yaklFile), sorted(yamlFile); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestKnitImportExtend(t *testing.T) {
	dir := t.TempDir()
	write := func(name, src string) string {
		t.Helper()
		file := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(src), 0600); err != nil {
			t.Fatal(err)
		}
		return file
	}
	// The base gave web's Deployment a name of its own.
	base := write("base/base.yakl", knitHeader+"---\n^^: api.deployment\napiVersion: apps/v1\nkind: Deployment\nmetadata: {name: web}\nspec: {replicas: 1}\n"+
		"---\n^^: plain.ConfigMap\napiVersion: v1\nkind: ConfigMap\nmetadata: {name: plain}\ndata: {a: '1'}\n")
	prod := write("prod/prod.yaml", "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: web}\nspec: {replicas: 3}\n"+
		"---\napiVersion: v1\nkind: ConfigMap\nmetadata: {name: plain}\ndata: {b: '2'}\n"+
		"---\napiVersion: v1\nkind: Service\nmetadata: {name: web}\n")
	got := cmdOutput(t, []string{"--alpha", "knit", "import", "--extend", base, prod})
	want := "<<+: ${import 'yakl:../base/base.yakl'}\n---\n^^+: api.deployment\nspec: {replicas: 3}\n" +
		"---\n^^+: plain.ConfigMap\ndata: {b: '2'}\n" +
		"---\n^^: web.Service\napiVersion: v1\nkind: Service\nmetadata: {name: web}\n"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	shown := cmdOutput(t, []string{"--alpha", "show", write("prod/prod.yakl", got)})
	for _, s := range []string{"replicas: 3", "a: \"1\"\n  b: \"2\"", "kind: Service"} {
		if !strings.Contains(shown, s) {
			t.Errorf("show: no %q in\n%s", s, shown)
		}
	}

	// An object the base has keeps only what changes it: a changed entry of a keyed list keeps its
	// key, and an object with no change is an empty patch. One with anchors is kept whole.
	base = write("base/full.yakl", knitHeader+"---\n^^: web\napiVersion: apps/v1\nkind: Deployment\nmetadata: {name: web}\n"+
		"spec:\n  replicas: 1\n  template:\n    spec:\n      containers:\n      - {name: web, image: 'web:1'}\n      - {name: log, image: 'log:1'}\n"+
		"---\n^^: same\napiVersion: v1\nkind: ConfigMap\nmetadata: {name: same}\ndata: {a: '1'}\n"+
		"---\n^^: anchored\napiVersion: v1\nkind: ConfigMap\nmetadata: {name: anchored}\ndata: {a: '1'}\n")
	full := write("base/full.yaml", "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: web}\n"+
		"spec:\n  replicas: 1\n  template:\n    spec:\n      containers:\n      - {name: web, image: 'web:2'}\n      - {name: log, image: 'log:1'}\n"+
		"---\n# the same\napiVersion: v1\nkind: ConfigMap\nmetadata: {name: same}\ndata: {a: '1'}\n"+
		"---\napiVersion: v1\nkind: ConfigMap\nmetadata: {name: anchored}\ndata: {a: &a '1', b: *a}\n")
	got = cmdOutput(t, []string{"--alpha", "knit", "import", "--extend", base, full})
	want = "<<+: ${import 'yakl:full.yakl'}\n---\n^^+: web\nspec:\n  template:\n    spec:\n      containers:\n        - {name: web, image: 'web:2'}\n" +
		"---\n# the same\n^^+: same\n" +
		"---\n^^+: anchored\napiVersion: v1\nkind: ConfigMap\nmetadata: {name: anchored}\ndata: {a: &a '1', b: *a}\n"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}

	// With --delete, the file has every object in full: what it leaves out is deleted.
	full = write("base/full-delete.yaml", "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: web}\n"+
		"spec:\n  template:\n    spec:\n      containers:\n      - {name: web, image: 'web:2'}\n"+
		"---\napiVersion: v1\nkind: ConfigMap\nmetadata: {name: same}\ndata: {a: '1'}\n")
	got = cmdOutput(t, []string{"--alpha", "knit", "import", "--extend", base, "--delete", full})
	want = "<<+: ${import 'yakl:full.yakl'}\n---\n^^+: web\nspec:\n  template:\n    spec:\n      containers:\n        - {name: web, image: 'web:2'}\n" +
		"        - {name: log, $patch: delete}\n  replicas: null\n" +
		"---\n^^+: same\n---\n^^+:: anchored\n"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	show := func(file string) string {
		docs := strings.Split(cmdOutput(t, []string{"--alpha", "show", file}), "---\n")
		slices.Sort(docs)
		return strings.Join(docs, "---\n")
	}
	if got, want := show(write("base/full-delete.yakl", got)), show(full); got != want {
		t.Errorf("show: got\n%s\nwant\n%s", got, want)
	}

	// The base is evaluated with the rc file's settings, like show.
	write("base/.kubecfgrc.jsonnet", "{flags: {'ext-str': ['n=fromrc']}}")
	base = write("base/ext.yakl", knitHeader+"---\n^^: cm\napiVersion: v1\nkind: ConfigMap\nmetadata: {name: '${std.extVar(\"n\")}'}\n")
	cm := write("base/ext.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: fromrc}\n")
	if got := cmdOutput(t, []string{"--alpha", "knit", "import", "--extend", base, cm}); !strings.Contains(got, "^^+: cm\n") {
		t.Errorf("got\n%s", got)
	}

	// An explicit namespace: '' isn't the base's default namespace, so it's a new object.
	base = write("base/ns.yakl", knitHeader+"---\n^^: other\napiVersion: v1\nkind: ConfigMap\nmetadata: {name: cm}\n"+
		"---\n^^+: registry\nobjects+:\n  metadata+: {namespace: prod}\n")
	cm = write("base/ns.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: cm, namespace: ''}\n")
	if got := cmdOutput(t, []string{"--alpha", "knit", "import", "--extend", base, cm}); !strings.Contains(got, "^^: cm.ConfigMap\n") {
		t.Errorf("got\n%s", got)
	}

	// The identity is read through aliases, and a base path can have a quote, #, % or : in it.
	base = write("base/it's #1 100% a:b.yakl", knitHeader+"---\n^^: x\napiVersion: v1\nkind: ConfigMap\nmetadata: {name: a, namespace: prod}\ndata: {from: base}\n")
	cm = write("base/alias.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  labels: {env: &ns prod}\n  name: a\n  namespace: *ns\n")
	got = cmdOutput(t, []string{"--alpha", "knit", "import", "--extend", base, cm})
	if !strings.Contains(got, "^^+: x\n") {
		t.Errorf("got\n%s", got)
	}
	if shown := cmdOutput(t, []string{"--alpha", "show", write("base/alias.yakl", got)}); !strings.Contains(shown, "from: base") || !strings.Contains(shown, "env: prod") {
		t.Errorf("show: got\n%s", shown)
	}

	// A new object's name can't clash with one of the base's.
	base = write("base/web.yakl", knitHeader+"---\n^^: web\napiVersion: v1\nkind: ConfigMap\nmetadata: {name: web}\n")
	defer resetFlags()
	RootCmd.SetArgs([]string{"--alpha", "knit", "import", "--extend", base, prod})
	if err := RootCmd.Execute(); err == nil || !strings.Contains(err.Error(), "Deployment web would be named web.Deployment, which clashes with ^^: web in the base") {
		t.Errorf("got %v", err)
	}

	resetFlags()
	RootCmd.SetArgs([]string{"--alpha", "knit", "import", "--delete", prod})
	if err := RootCmd.Execute(); err == nil || !strings.Contains(err.Error(), "--delete needs --extend") {
		t.Errorf("got %v", err)
	}
}

// knitFields must list knit's fields.
func TestKnitFields(t *testing.T) {
	want, err := json.Marshal(slices.Sorted(maps.Keys(knitFields)))
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "fields.jsonnet")
	src := fmt.Sprintf(`local got = [f for f in std.objectFieldsAll(import 'knit/knit.libsonnet') if f[0] != '$'];
assert std.set(got) == %s : 'knitFields should be %%s' %% [std.set(got)];
{}`, want)
	if err := os.WriteFile(file, []byte(src), 0600); err != nil {
		t.Fatal(err)
	}
	cmdOutput(t, []string{"--alpha", "eval", file})
}

var update = flag.Bool("update", false, "rebuild the .yakl files that knit import makes for the examples")

// The .yakl files of examples/experimental/knit/import, and the shop's redis.yakl, are what knit
// import gives for their YAML files. If the new output is right, run:
//
//	go test ./cmd -run TestKnitImportExamples -update
func TestKnitImportExamples(t *testing.T) {
	examples := filepath.Join("..", "examples", "experimental", "knit")
	imp := filepath.Join(examples, "import")
	for _, tc := range []struct {
		out  string
		args []string
	}{
		{filepath.Join(imp, "base.yakl"), []string{filepath.Join(imp, "base.yaml")}},
		{filepath.Join(imp, "prod.yakl"), []string{"--extend", filepath.Join(imp, "base.yakl"), filepath.Join(imp, "prod.yaml")}},
		{filepath.Join(examples, "shop", "redis.yakl"), []string{filepath.Join(examples, "shop", "redis.yaml")}},
	} {
		got := cmdOutput(t, append([]string{"--alpha", "knit", "import"}, tc.args...))
		if *update {
			if err := os.WriteFile(tc.out, []byte(got), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if want, err := os.ReadFile(tc.out); err != nil {
			t.Fatal(err)
		} else if got != string(want) {
			t.Errorf("%s is out of date; if the change is right, run with -update\ngot:\n%s", tc.out, got)
		}
	}
}
