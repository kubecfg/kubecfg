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

package features

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the feature list in the docs")

// withFeature registers f for the duration of the test.
func withFeature(t *testing.T, f Feature) *Feature {
	saved := registry
	t.Cleanup(func() { registry = saved })
	registry = append(registry[:len(registry):len(registry)], &f)
	return &f
}

func TestNew(t *testing.T) {
	beta := withFeature(t, Feature{Name: "test-beta", Stage: Beta, Since: "v0.41.0"})
	ga := withFeature(t, Feature{Name: "test-ga", Stage: GA, Since: "v0.42.0"})
	gone := withFeature(t, Feature{Name: "test-removed", Stage: Removed, Since: "v0.43.0"})
	flags := func(items ...string) Source { return Source{Name: "--features", Items: items} }
	rc := func(items ...string) Source { return Source{Name: ".kubecfgrc.jsonnet", Items: items} }

	type want struct {
		f      *Feature
		on     bool
		source string
	}
	for _, tc := range []struct {
		name    string
		alpha   bool
		sources []Source
		want    []want
		err     string
	}{
		{name: "defaults", want: []want{{YAKL, false, "default"}, {beta, true, "default"}, {ga, true, "default"}, {gone, false, "default"}}},
		{name: "alpha", alpha: true, want: []want{{YAKL, true, "--alpha"}, {beta, true, "default"}}},
		{name: "named beats alpha", alpha: true, sources: []Source{flags("httpd=false")}, want: []want{{Httpd, false, "--features"}, {Eval, true, "--alpha"}}},
		{name: "comma separated", sources: []Source{flags("yakl, pack", "eval=true")}, want: []want{{YAKL, true, "--features"}, {Pack, true, "--features"}, {Eval, true, "--features"}}},
		{name: "last wins in a source", sources: []Source{flags("yakl", "yakl=false")}, want: []want{{YAKL, false, "--features"}}},
		{name: "first source wins", sources: []Source{flags("yakl=false"), rc("yakl", "pack")}, want: []want{{YAKL, false, "--features"}, {Pack, true, ".kubecfgrc.jsonnet"}}},
		{name: "beta off", sources: []Source{flags("test-beta=false")}, want: []want{{beta, false, "--features"}}},
		{name: "GA on", sources: []Source{flags("test-ga")}, want: []want{{ga, true, "--features"}}},
		{name: "removed", sources: []Source{flags("test-removed")}, want: []want{{gone, false, "default"}}},
		{name: "empty", sources: []Source{flags("", ",")}, want: []want{{YAKL, false, "default"}}},
		{name: "unknown", sources: []Source{flags("knti")}, err: `unknown feature "knti"; run "kubecfg features" to list them`},
		{name: "bad value", sources: []Source{flags("yakl=maybe")}, err: `--features: "yakl=maybe": want name, name=true or name=false`},
		{name: "GA off", sources: []Source{rc("test-ga=false")}, err: `test-ga is GA since v0.42.0 and can no longer be turned off; remove "test-ga=false" from .kubecfgrc.jsonnet`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, err := New(tc.alpha, tc.sources...)
			if tc.err != "" {
				if err == nil || err.Error() != tc.err {
					t.Fatalf("got error %v, want %q", err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range tc.want {
				if got := g.Enabled(w.f); got != w.on {
					t.Errorf("%s: enabled %v, want %v", w.f.Name, got, w.on)
				}
				if got := g.setting(w.f).source; got != w.source {
					t.Errorf("%s: source %q, want %q", w.f.Name, got, w.source)
				}
			}
		})
	}
}

func TestRequire(t *testing.T) {
	var none *Gates
	if err := none.Require(YAKL); err == nil || err.Error() != "yakl is an alpha feature; turn it on with --features=yakl" {
		t.Errorf("got %v", err)
	}
	g, err := New(true, Source{Name: "--features", Items: []string{"yakl=false"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Require(YAKL); err == nil || err.Error() != "yakl is turned off by --features; turn it on with --features=yakl" {
		t.Errorf("got %v", err)
	}
	if err := g.Require(Eval); err != nil {
		t.Errorf("got %v", err)
	}
}

func TestRegistry(t *testing.T) {
	kebab := regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)
	for _, f := range All() {
		if !kebab.MatchString(f.Name) {
			t.Errorf("%q isn't lowercase kebab-case", f.Name)
		}
		if f.Kind == Semantic && (f.Stage == Beta || f.Stage == GA) {
			t.Errorf("%s changes what existing input means, so it can't be %s; it needs an edition", f.Name, f.Stage)
		}
		if f.Since == "" || f.Doc == "" {
			t.Errorf("%s needs Since and Doc", f.Name)
		}
	}
}

// A GA feature is always on, so nothing checks it anymore.
func TestGAHasNoChecks(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "features.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var vars []string // Go names of GA and removed features
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || len(spec.Values) != 1 {
			return true
		}
		ast.Inspect(spec.Values[0], func(n ast.Node) bool {
			if kv, ok := n.(*ast.KeyValueExpr); ok && fmt.Sprint(kv.Key) == "Stage" {
				if s := fmt.Sprint(kv.Value); s == "GA" || s == "Removed" {
					vars = append(vars, spec.Names[0].Name)
				}
			}
			return true
		})
		return true
	})
	if len(vars) == 0 {
		return
	}
	use := regexp.MustCompile(`\bfeatures\.(` + strings.Join(vars, "|") + `)\b`)
	err = filepath.WalkDir("../..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "vendor" || strings.HasPrefix(d.Name(), ".")) && path != "../.." {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if m := use.Find(src); m != nil {
			t.Errorf("%s: %s is GA or removed; delete the check", path, m)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

const docsPage = "../../docs/Advanced-Usage/feature-gates.md"

func markdown() string {
	var b strings.Builder
	b.WriteString("| Name | Stage | Since | What it turns on |\n| --- | --- | --- | --- |\n")
	for _, f := range All() {
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", f.Name, f.Stage, f.Since, strings.ReplaceAll(f.Doc, "|", `\|`))
	}
	return b.String()
}

// The docs page lists every feature, generated from the registry.
func TestDocs(t *testing.T) {
	src, err := os.ReadFile(docsPage)
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`(?s)(<!-- BEGIN features[^>]*-->\n).*?(<!-- END features -->)`)
	if !re.Match(src) {
		t.Fatalf("%s: missing the BEGIN/END features markers", docsPage)
	}
	got := re.ReplaceAll(src, []byte("${1}"+strings.ReplaceAll(markdown(), "$", "$$")+"${2}"))
	if *update {
		if err := os.WriteFile(docsPage, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if string(got) != string(src) {
		t.Errorf("%s is out of date; run: go test ./pkg/features -update", docsPage)
	}
}
