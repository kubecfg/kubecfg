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

	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/kyaml/filesys"
	k8syaml "sigs.k8s.io/yaml"
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
		{"knit's fields get a _", "kind: Service\nmetadata: {name: registry}\n---\nkind: ConfigMap\nmetadata: {name: foo, namespace: render}\n---\nkind: ConfigMap\nmetadata: {name: foo, namespace: b}\n",
			knitHeader + "---\n^^: _registry.Service\nkind: Service\nmetadata: {name: registry}\n---\n^^: _render.foo.ConfigMap\nkind: ConfigMap\nmetadata: {name: foo, namespace: render}\n---\n^^: b.foo.ConfigMap\nkind: ConfigMap\nmetadata: {name: foo, namespace: b}\n"},
		{"line breaks of \\r\\n and \\r", "# top\r\nkind: ConfigMap\rmetadata: {name: foo}\r",
			knitHeader + "---\n# top\n^^: foo.ConfigMap\nkind: ConfigMap\nmetadata: {name: foo}\n"},
		{"a List of another group is an object", "apiVersion: example.com/v1\nkind: List\nmetadata: {name: l}\n",
			knitHeader + "---\n^^: l.List\napiVersion: example.com/v1\nkind: List\nmetadata: {name: l}\n"},
		{"a date is a string", "kind: ConfigMap\nmetadata: {name: 2026-10-28}\n",
			knitHeader + "---\n^^: 2026-10-28.ConfigMap\nkind: ConfigMap\nmetadata: {name: 2026-10-28}\n"},
		{"an indented document", "  kind: ConfigMap\n  metadata: {name: foo}\n",
			knitHeader + "---\n  ^^: foo.ConfigMap\n  kind: ConfigMap\n  metadata: {name: foo}\n"},
	} {
		got, err := knitImport([]knitSource{{text: tc.src}}, nil, nil)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
		} else if got != tc.want {
			t.Errorf("%s: got\n%s\nwant\n%s", tc.name, got, tc.want)
		}
	}
	// A block scalar at the end of a file without a final line break has none in its value. It
	// keeps it out with the line break the file gets.
	for _, end := range []string{"|", "|+", "|2", ">+2"} {
		got, err := knitImport([]knitSource{{text: "kind: ConfigMap\nmetadata: {name: a}\ndata:\n  k: " + end + "\n    x"}, {text: "kind: ConfigMap\nmetadata: {name: b}\ndata:\n  k: |\n    y"}}, nil, nil)
		want := "  k: " + strings.Replace(end, "+", "", 1) + "-\n    x\n---\n"
		if !strings.HasSuffix(got, "  k: |-\n    y\n") {
			t.Errorf("got\n%s", got)
		}
		if err != nil || !strings.Contains(got, want) {
			t.Errorf("%s: got %v\n%s\nwant %q", end, err, got, want)
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
		if _, err := knitImport([]knitSource{{text: tc.src}}, nil, nil); err == nil || !strings.Contains(err.Error(), tc.want) {
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

// The .yakl files of examples/experimental/knit/import and kustomize, and the shop's redis.yakl,
// are what knit import gives for their YAML files. If the new output is right, run:
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
	// The kustomize example's knit directory.
	kust, out := filepath.Join(examples, "kustomize"), t.TempDir()
	if *update {
		out = filepath.Join(kust, "knit")
	}
	cmdOutput(t, []string{"--alpha", "knit", "import", "-k", filepath.Join(kust, "overlays", "prod"), "--out-dir", out})
	for _, f := range []string{"base.yakl", filepath.Join("components", "tracing.yakl"), filepath.Join("overlays", "prod.yakl")} {
		got, err := os.ReadFile(filepath.Join(out, f))
		if err != nil {
			t.Fatal(err)
		}
		if want, err := os.ReadFile(filepath.Join(kust, "knit", f)); err != nil {
			t.Fatal(err)
		} else if string(got) != string(want) {
			t.Errorf("%s is out of date; if the change is right, run with -update\ngot:\n%s", filepath.Join(kust, "knit", f), got)
		}
	}
}

// knitTestDeployment is a Deployment d with one container and a toleration.
const knitTestDeployment = "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: d}\nspec: {template: {spec: {containers: [{name: c, image: c}], tolerations: [{key: k}]}}}\n"

// knit import -k gives a file that renders what kustomize builds.
func TestKnitImportKustomize(t *testing.T) {
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
	// Entries of keyed lists come in another order: knit puts new ones last, kustomize first.
	var byName func(v any) any
	byName = func(v any) any {
		switch v := v.(type) {
		case map[string]any:
			for k, x := range v {
				v[k] = byName(x)
			}
		case []any:
			for i, x := range v {
				v[i] = byName(x)
			}
			name := func(x any) string { m, _ := x.(map[string]any); n, _ := m["name"].(string); return n }
			if !slices.ContainsFunc(v, func(x any) bool { return name(x) == "" }) {
				slices.SortFunc(v, func(a, b any) int { return strings.Compare(name(a), name(b)) })
			}
		}
		return v
	}
	objects := func(stream []byte) []string {
		t.Helper()
		var objs []string
		for _, doc := range strings.Split(string(stream), "\n---\n") {
			var o any
			if err := k8syaml.Unmarshal([]byte(doc), &o); err != nil {
				t.Fatal(err)
			}
			if o != nil {
				j, _ := json.Marshal(byName(o))
				objs = append(objs, string(j))
			}
		}
		slices.Sort(objs)
		return objs
	}
	// The yakl file renders the objects kustomize builds from the kustomization in kdir.
	same := func(file, kdir string) {
		t.Helper()
		m, err := krusty.MakeKustomizer(krusty.MakeDefaultOptions()).Run(filesys.MakeFsOnDisk(), kdir)
		if err != nil {
			t.Fatal(err)
		}
		want, err := m.AsYaml()
		if err != nil {
			t.Fatal(err)
		}
		if g, w := objects([]byte(cmdOutput(t, []string{"--alpha", "show", file}))), objects(want); !slices.Equal(g, w) {
			src, _ := os.ReadFile(file)
			t.Errorf("%s renders\n%s\nkustomize builds\n%s\nthe file:\n%s", file, strings.Join(g, "\n"), strings.Join(w, "\n"), src)
		}
	}
	// knit import -k gives such a file, written next to the kustomization.
	check := func(kdir string, args ...string) string {
		t.Helper()
		got := cmdOutput(t, append([]string{"--alpha", "knit", "import", "-k", filepath.Join(dir, kdir)}, args...))
		same(write(filepath.Join(kdir, "kustomization.yakl"), got), filepath.Join(dir, kdir))
		return got
	}

	// So do the examples.
	examples := filepath.Join("..", "examples", "experimental", "knit", "kustomize")
	for _, kdir := range []string{"base", filepath.Join("overlays", "prod")} {
		same(filepath.Join(examples, "knit", kdir+".yakl"), filepath.Join(examples, kdir))
	}

	write("base/app.yaml", `apiVersion: v1
kind: ConfigMap
metadata: {name: web, namespace: a}
data: {LOG_LEVEL: info}
---
apiVersion: v1
kind: ConfigMap
metadata: {name: web, namespace: b}
data: {LOG_LEVEL: info}
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: web, labels: {tier: front}}
spec:
  replicas: 1
  selector: {matchLabels: {app: web}}
  template:
    metadata: {labels: {app: web}}
    spec:
      containers:
      - {name: web, image: 'web:1', env: [{name: A, value: '1'}, {name: B, value: '2'}]}
      - {name: log, image: 'log:1'}
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: worker, annotations: {team: jobs}}
spec:
  selector: {matchLabels: {app: worker}}
  template:
    metadata: {labels: {app: worker}}
    spec:
      containers: [{name: worker, image: 'worker:1'}]
---
apiVersion: v1
kind: Service
metadata: {name: web}
spec:
  selector: {app: web}
  ports: [{port: 80}]
`)
	write("base/cm.yaml", "# b only\napiVersion: v1\nkind: ConfigMap\nmetadata: {name: web, namespace: b}\ndata: {LOG_LEVEL: debug}\n")
	write("base/web.yaml", "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: web}\nspec:\n  template:\n    spec:\n      containers:\n"+
		"      - name: web\n        image: 'web:2'\n        env: [{name: A, $patch: delete}, {name: C, value: '3'}]\n      - {name: log, $patch: delete}\n")
	write("base/kustomization.yaml", `resources: [app.yaml]
# The legacy field: a file and an inline patch.
patchesStrategicMerge:
- cm.yaml
- |-
  apiVersion: v1
  kind: Service
  metadata: {name: web}
  spec:
    selector: {$patch: replace, app: web2}
patches:
- path: web.yaml
# Targets: the name is a regular expression, and labels and annotations select.
- target: {kind: Deployment, name: w.*}
  patch: "kind: Deployment\nmetadata: {name: x, labels: {all: 'yes'}}\n"
- target: {labelSelector: tier=front}
  patch: "kind: Deployment\nmetadata: {name: x}\nspec: {replicas: 2}\n"
- target: {group: apps, annotationSelector: team=jobs}
  patch: "kind: Deployment\nmetadata: {name: x}\nspec: {replicas: 5}\n"
`)
	check("base")
	got := cmdOutput(t, []string{"--alpha", "knit", "import", "-k", filepath.Join(dir, "base")})
	// Each patch is a document of its own, in kustomize's order: the documents of a name are a
	// chain of merges.
	for _, s := range []string{"# b only\n^^+: b.web.ConfigMap\n", "^^+: web.Deployment\nmetadata: {labels: {all: 'yes'}}\n---\n^^+: worker.Deployment\n" +
		"metadata: {labels: {all: 'yes'}}\n---\n^^+: web.Deployment\nspec: {replicas: 2}\n"} {
		if !strings.Contains(got, s) {
			t.Errorf("no %q in\n%s", s, got)
		}
	}

	// An overlay with the base as a directory resource, legacy bases too: one file with the
	// base's objects and patches, and then its own. It deletes an object of the base, and one of its
	// own.
	write("prod/kustomization.yaml", `bases: [../base]
resources: [extra.yaml]
patches:
- patch: "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: web, namespace: a}\n$patch: delete\n"
- patch: "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: extra}\n$patch: delete\n"
- patch: "apiVersion: v1\nkind: Service\nmetadata: {name: web}\nspec: {ports: [{port: 80, name: http}]}\n"
`)
	write("prod/extra.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: extra}\n")
	if got := check("prod"); !strings.Contains(got, "^^:: extra.ConfigMap\n") || !strings.Contains(got, "^^:: a.web.ConfigMap\n") {
		t.Errorf("got\n%s", got)
	}

	// With --out-dir, each kustomization is a file in a tree like theirs. One with one directory
	// resource extends its file, and its patches patch the objects there too.
	write("dev/kustomization.yaml", `resources: [../base, extra.yaml]
patches:
- patch: "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: web, namespace: a}\n$patch: delete\n"
- patch: "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: extra}\ndata: {a: '1'}\n"
- target: {kind: Deployment}
  patch: "kind: Deployment\nmetadata: {name: x}\nspec: {template: {spec: {containers: [{name: debug, image: 'debug:1'}]}}}\n"
`)
	write("dev/extra.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: extra}\n")
	write("overlays/stage/kustomization.yaml", `resources: [../../dev]
patches:
- patch: "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: extra}\ndata: {b: '2'}\n"
`)
	out := filepath.Join(dir, "knit")
	if got := cmdOutput(t, []string{"--alpha", "knit", "import", "-k", filepath.Join(dir, "overlays", "stage"), "--out-dir", out}); got != strings.Join([]string{
		filepath.Join(out, "base.yakl"), filepath.Join(out, "dev.yakl"), filepath.Join(out, "overlays", "stage.yakl"), ""}, "\n") {
		t.Errorf("got %s", got)
	}
	for _, kdir := range []string{"base", "dev", filepath.Join("overlays", "stage")} {
		same(filepath.Join(out, kdir+".yakl"), filepath.Join(dir, kdir))
	}
	if src, _ := os.ReadFile(filepath.Join(out, "overlays", "stage.yakl")); !strings.HasPrefix(string(src), "<<+: ${import 'yakl:../dev.yakl'}\n---\n^^+: extra.ConfigMap\ndata: {b: '2'}\n") {
		t.Errorf("got\n%s", src)
	}
	src, _ := os.ReadFile(filepath.Join(out, "dev.yakl"))
	got = string(src)
	want := `<<+: ${import 'yakl:base.yakl'}
---
^^: extra.ConfigMap
apiVersion: v1
kind: ConfigMap
metadata: {name: extra}
---
^^+:: a.web.ConfigMap
---
^^+: extra.ConfigMap
data: {a: '1'}
---
^^+: web.Deployment
spec: {template: {spec: {containers: [{name: debug, image: 'debug:1'}]}}}
---
^^+: worker.Deployment
spec: {template: {spec: {containers: [{name: debug, image: 'debug:1'}]}}}
`
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}

	// Components add their resources and patches to the kustomization that lists them, and patch
	// its base's objects too.
	write("components/debug/kustomization.yaml", `apiVersion: kustomize.config.k8s.io/v1alpha1
kind: Component
resources: [debug.yaml]
components: [../trace]
patches:
- patch: "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: web}\nspec: {template: {spec: {containers: [{name: web, env: [{name: DEBUG, value: '1'}]}]}}}\n"
- patch: "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: debug}\ndata: {trace: 'on'}\n"
`)
	write("components/debug/debug.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: debug}\n")
	write("components/trace/kustomization.yaml", `apiVersion: kustomize.config.k8s.io/v1alpha1
kind: Component
patches:
- patch: "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: web}\nspec: {template: {spec: {containers: [{name: web, resources: {limits: {cpu: '1'}}}]}}}\n"
`)
	write("debug/kustomization.yaml", `resources: [../base]
components: [../components/debug]
patches:
- patch: "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: web}\nspec: {template: {spec: {containers: [{name: web, env: [{name: TRACE, value: '1'}, {name: DEBUG, value: '2'}]}]}}}\n"
`)
	// In one file, the component's patches and the kustomization's are documents of their own,
	// in order: the last DEBUG wins.
	got = check("debug")
	if want := "^^+: web.Deployment\nspec: {template: {spec: {containers: [{name: web, env: [{name: TRACE, value: '1'}, {name: DEBUG, value: '2'}]}]}}}\n"; !strings.HasSuffix(got, want) {
		t.Errorf("no %q at the end of\n%s", want, got)
	}

	// A patch can set what an earlier one deleted, a field or a list entry, as kustomize applies them.
	write("chain/kustomization.yaml", `resources: [d.yaml]
patches:
- patch: "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: a}\ndata: null\n"
- patch: "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: a}\ndata: {k: '2'}\n"
- patch: "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: d}\nspec: {template: {spec: {containers: [{name: c, env: [{name: A, $patch: delete}]}]}}}\n"
- patch: "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: d}\nspec: {template: {spec: {containers: [{name: c, env: [{name: A, value: '3'}]}]}}}\n"
`)
	write("chain/d.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: a}\ndata: {j: '1'}\n---\n"+
		"apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: d}\nspec: {template: {spec: {containers: [{name: c, env: [{name: A, value: '1'}, {name: B, value: '2'}]}]}}}\n")
	check("chain")
	// With --out-dir, a component is a file of its own, which the kustomization's file adds after
	// its base, and which extends its own components.
	if got := cmdOutput(t, []string{"--alpha", "knit", "import", "-k", filepath.Join(dir, "debug"), "--out-dir", out}); got != strings.Join([]string{
		filepath.Join(out, "base.yakl"), filepath.Join(out, "components", "trace.yakl"), filepath.Join(out, "components", "debug.yakl"), filepath.Join(out, "debug.yakl"), ""}, "\n") {
		t.Errorf("got %s", got)
	}
	same(filepath.Join(out, "debug.yakl"), filepath.Join(dir, "debug"))
	for file, want := range map[string]string{
		"debug.yakl":            "<<+: ${(import 'yakl:base.yakl') + import 'yakl:components/debug.yakl'}\n---\n^^+: web.Deployment\n",
		"components/debug.yakl": "# A kustomize component. A file adds it after what it extends: <<+: ${(import 'base') + import 'this'}\n<<+: ${import 'yakl:trace.yakl'}\n---\n^^: debug.ConfigMap\n",
		"components/trace.yakl": "# A kustomize component. A file adds it after what it extends: <<+: ${(import 'base') + import 'this'}\n<<+: {}\n---\n^^+: web.Deployment\n",
	} {
		if src, _ := os.ReadFile(filepath.Join(out, file)); !strings.HasPrefix(string(src), want) {
			t.Errorf("%s:\n%s\nwant it to start with\n%s", file, src, want)
		}
	}
	// A component that patches an object of the kustomization that lists it stays in its file,
	// which has those objects after the files it extends.
	write("late/kustomization.yaml", "resources: [../base, extra.yaml]\ncomponents: [../components/late]\n")
	write("late/extra.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: extra}\n")
	write("components/late/kustomization.yaml", `apiVersion: kustomize.config.k8s.io/v1alpha1
kind: Component
patches:
- patch: "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: extra}\ndata: {a: '1'}\n"
`)
	if got := cmdOutput(t, []string{"--alpha", "knit", "import", "-k", filepath.Join(dir, "late"), "--out-dir", out}); !strings.HasSuffix(got, "it patches an object of the kustomization that lists it, which the component's file can't reach\n"+
		filepath.Join(out, "base.yakl")+"\n"+filepath.Join(out, "late.yakl")+"\n") {
		t.Errorf("got %s", got)
	}
	same(filepath.Join(out, "late.yakl"), filepath.Join(dir, "late"))

	// A configMapGenerator makes a ConfigMap with kustomize's hash after its name, and references
	// to it read that name, in the resources and in the patches. An overlay's generator with
	// behavior: replace changes its data, and so the hash.
	write("gen/kustomization.yaml", `resources: [app.yaml]
generatorOptions: {labels: {made: gen}}
configMapGenerator:
- name: conf
  literals: [A=1, 'B="two words"']
  envs: [app.env]
  files: [cfg=app.ini]
- name: fixed
  literals: [C=3]
  options: {disableNameSuffixHash: true, immutable: true, annotations: {note: kept}}
secretGenerator:
- name: creds
  literals: [password=hunter2]
- name: pull
  type: kubernetes.io/dockerconfigjson
  literals: ['.dockerconfigjson={"auths":{}}']
patches:
- patch: "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: app}\nspec: {template: {spec: {initContainers: [{name: init, image: 'i:1', envFrom: [{configMapRef: {name: conf}}]}]}}}\n"
`)
	write("gen/app.env", "# comment\nD=4\n")
	write("gen/app.ini", "[a]\nb = c\n")
	write("gen/app.yaml", `apiVersion: apps/v1
kind: Deployment
metadata: {name: app}
spec:
  template:
    spec:
      containers:
      - name: app
        image: 'app:1'
        envFrom: [{configMapRef: {name: conf}}, {configMapRef: {name: fixed}}, {secretRef: {name: creds}}]
        env:
        - name: E
          valueFrom:
            configMapKeyRef:
              name: 'conf'
              key: A
      imagePullSecrets: [{name: pull}]
      volumes:
      - name: v
        projected: {sources: [{configMap: {name: "conf"}}, {secret: {name: creds}}]}
      - name: s
        secret: {secretName: creds}
`)
	write("genprod/kustomization.yaml", `resources: [../gen]
configMapGenerator:
- name: conf
  behavior: replace
  literals: [A=9]
secretGenerator:
- name: creds
  behavior: merge
  literals: [user=admin]
`)
	if got := check("gen"); !strings.Contains(got, `name: ${$.conf.ConfigMap.metadata.name}`) || !strings.Contains(got, `{name: "${$.conf.ConfigMap.metadata.name}"}`) ||
		!strings.Contains(got, `{secretName: "${$.creds.Secret.metadata.name}"}`) || !strings.Contains(got, `[{name: "${$.pull.Secret.metadata.name}"}]`) {
		t.Errorf("got\n%s", got)
	}
	check("genprod")
	cmdOutput(t, []string{"--alpha", "knit", "import", "-k", filepath.Join(dir, "genprod"), "--out-dir", out})
	same(filepath.Join(out, "genprod.yakl"), filepath.Join(dir, "genprod"))

	// JSON 6902 patches that append to a list or remove: an index into a list with keys is the
	// entry's key. A strategic merge patch before them can change an entry, as long as it doesn't
	// add or delete one.
	write("json/d.yaml", `apiVersion: apps/v1
kind: Deployment
metadata: {name: d}
spec:
  replicas: 2
  template:
    spec:
      containers:
      - {name: a, image: 'a:1', args: [x], ports: [{containerPort: 80}, {containerPort: 81}]}
      - {name: b, image: 'b:1'}
      tolerations: [{key: k, operator: Exists}]
`)
	write("json/kustomization.yaml", `resources: [d.yaml]
patches:
- patch: "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: d}\nspec: {template: {spec: {containers: [{name: a, image: 'a:2'}]}}}\n"
- target: {kind: Deployment, name: d}
  patch: |-
    # Two more arguments, and no b.
    - {op: add, path: /spec/template/spec/containers/0/args/-, value: y2}
    - {op: remove, path: /spec/template/spec/containers/1}
    - {op: add, path: /spec/template/spec/containers/0/args/-, value: z}
    - {op: remove, path: /spec/replicas}
    - {op: remove, path: /spec/template/spec/containers/0/ports/0}
- target: {kind: Deployment}
  patch: '[{"op": "add", "path": "/spec/template/spec/containers/-", "value": {"name": "c", "image": "c:1"}}, {"op": "add", "path": "/spec/template/spec/tolerations/-", "value": {"key": "j", "operator": "Exists"}}]'
`)
	if got := check("json"); !strings.Contains(got, "# Two more arguments, and no b.\n# add /spec/template/spec/containers/0/args/-\n^^+: d.Deployment\n"+
		"spec:\n  template:\n    spec:\n      containers:\n        - name: a\n          args:\n            - {$patch: append}\n            - y2\n") ||
		!strings.Contains(got, "        - {name: b, $patch: delete}\n") || !strings.Contains(got, "  replicas: {$patch: delete}\n") ||
		!strings.Contains(got, "          ports:\n            - {containerPort: 80, $patch: delete}\n") {
		t.Errorf("got\n%s", got)
	}

	for _, tc := range []struct {
		args  []string
		files map[string]string
		want  string
	}{
		{[]string{"-k", "x"}, map[string]string{"x/kustomization.yaml": "resources: [a.yaml]\nnamePrefix: p-\n"}, "x/kustomization.yaml: knit import doesn't read namePrefix yet"},
		{[]string{"-k", "x"}, map[string]string{"x/kustomization.yaml": "resources: [a.yaml]\npatches: [{patch: '[{op: remove, path: /data}]', target: {kind: ConfigMap}}]\n"}, "x/kustomization.yaml: patch 1: line 1: ConfigMap a: remove /data: no /data"},
		{[]string{"-k", "x"}, map[string]string{"x/kustomization.yaml": "resources: [a.yaml]\npatches: [{patch: '[{op: replace, path: /data, value: {}}]', target: {kind: ConfigMap}}]\n"}, "replace /data: knit import reads only add at the end of a list (/-) and remove, not replace"},
		{[]string{"-k", "x"}, map[string]string{"x/kustomization.yaml": "resources: [a.yaml]\npatches: [{patch: '[{op: remove, path: /data}]'}]\n"}, "x/kustomization.yaml: patch 1: a JSON 6902 patch needs a target"},
		{[]string{"-k", "x"}, map[string]string{"x/kustomization.yaml": "resources: [d.yaml]\npatches: [{patch: '[{op: remove, path: /spec/template/spec/tolerations/0}]', target: {kind: Deployment}}]\n", "x/d.yaml": knitTestDeployment},
			"Deployment d: remove /spec/template/spec/tolerations/0: /spec/template/spec/tolerations/0 is an entry of a list without keys: knit can only append to it"},
		{[]string{"-k", "x"}, map[string]string{"x/kustomization.yaml": "resources: [d.yaml]\npatches:\n- patch: \"apiVersion: apps/v1\\nkind: Deployment\\nmetadata: {name: d}\\nspec: {template: {spec: {containers: [{name: new, image: n}]}}}\"\n- {patch: '[{op: remove, path: /spec/template/spec/containers/0}]', target: {kind: Deployment}}\n", "x/d.yaml": knitTestDeployment},
			"a patch before it adds or deletes entries of /spec/template/spec/containers"},
		{[]string{"-k", "x"}, map[string]string{"x/kustomization.yaml": "resources: [a.yaml]\npatches: [{patch: \"kind: ConfigMap\\nmetadata: {name: a}\\n$patch: delete\"}, {path: p.yaml}]\n", "x/p.yaml": "kind: ConfigMap\nmetadata: {name: a}\n"}, "x/p.yaml: line 1: no ConfigMap a to patch"},
		{[]string{"-k", "x"}, map[string]string{"x/kustomization.yaml": "resources: [a.yaml]\npatches: [{patch: \"kind: ConfigMap\\nmetadata: {name: b}\"}]\n"}, "x/kustomization.yaml: patch 1: line 1: no ConfigMap b to patch"},
		{[]string{"-k", "x"}, map[string]string{"x/kustomization.yaml": "resources: [a.yaml]\npatches: [{path: p.yaml, options: {allowNameChange: true}}]\n"}, "patch 1: knit can't change an object's name or kind"},
		{[]string{"-k", "x"}, map[string]string{"x/kustomization.yaml": "resources: [missing.yaml]\n"}, "x/kustomization.yaml: resource missing.yaml:"},
		{[]string{"-k", "x"}, map[string]string{"x/kustomization.yaml": "kind: Component\n"}, "x/kustomization.yaml: a component, which only components can list"},
		{[]string{"-k", "x"}, map[string]string{"x/kustomization.yaml": "resources: [../y]\n", "y/kustomization.yaml": "kind: Component\n"}, "y/kustomization.yaml: a component, which only components can list"},
		{[]string{"-k", "x"}, map[string]string{"x/kustomization.yaml": "components: [../y]\n", "y/kustomization.yaml": "resources: []\n"}, "y/kustomization.yaml: components lists it, but it isn't a component"},
		{[]string{"-k", "y"}, nil, "y: no kustomization.yaml"},
		{[]string{"-k", "x", "--extend", "base.yakl"}, nil, "--extend doesn't go with -k"},
		{[]string{"--out-dir", "o", "a.yaml"}, nil, "--out-dir needs -k"},
		{[]string{"-k", "x"}, map[string]string{"x/kustomization.yaml": "resources: [../x]\n"}, "x: a kustomization that is its own resource"},
		{[]string{"-k", "x", "--out-dir", "o"}, map[string]string{"x/kustomization.yaml": "resources: [../y]\n", "y/kustomization.yaml": "resources: [../x]\n"}, "x: a kustomization that is its own resource"},
		{[]string{"-k", "x", "a.yaml"}, nil, "give a YAML file or -k and a kustomization directory"},
		{[]string{"-k", "x"}, map[string]string{"x/kustomization.yaml": "configMapGenerator: [{name: a, literals: [k=1, k=2]}]\n"}, "x/kustomization.yaml: configMapGenerator a: the key k comes twice"},
		{[]string{"-k", "x"}, map[string]string{"x/kustomization.yaml": "configMapGenerator: [{name: a, files: [../a.yaml]}]\n"}, "x/kustomization.yaml: configMapGenerator a: file sources"},
		{[]string{"-k", "x", "--out-dir", "o"}, map[string]string{"x/kustomization.yaml": "resources: [../y]\nconfigMapGenerator: [{name: a, literals: [k=1]}]\n", "y/kustomization.yaml": "resources: [d.yaml]\n",
			"y/d.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: d}\nspec: {template: {spec: {containers: [{name: c, envFrom: [{configMapRef: {name: a}}]}]}}}\n"},
			"the base's Deployment d refers to ConfigMap a, which gets a hash after its name here"},
	} {
		dir := t.TempDir()
		for name, src := range tc.files {
			if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(name)), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0600); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(dir, "x/a.yaml"), []byte("kind: ConfigMap\nmetadata: {name: a}\n"), 0600); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "base.yakl"), []byte(knitHeader), 0600); err != nil {
			t.Fatal(err)
		}
		resetFlags()
		wd, _ := os.Getwd()
		if err := os.Chdir(dir); err != nil {
			t.Fatal(err)
		}
		RootCmd.SetArgs(append([]string{"--alpha", "knit", "import"}, tc.args...))
		err := RootCmd.Execute()
		if err := os.Chdir(wd); err != nil {
			t.Fatal(err)
		}
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v: got %v, want %q", tc.args, err, tc.want)
		}
	}
	resetFlags()
}
