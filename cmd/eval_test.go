package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvalJsonnet(t *testing.T) {
	file := filepath.Join(t.TempDir(), "app.yakl")
	src := "'&k': ${import 'k.libsonnet'}\n---\n^^:: config\nreplicas: 2 # overlays change it\n---\n^^: deployment\nkind: Deployment\n" +
		"metadata:\n  labels: &labels {app: web}\nspec:\n  replicas: ${$.config.replicas}\n  selector: {matchLabels: '${labels}'}\n"
	if err := os.WriteFile(file, []byte(src), 0600); err != nil {
		t.Fatal(err)
	}
	// The file isn't evaluated, so the missing import doesn't matter.
	got := cmdOutput(t, []string{"--alpha", "eval", "-o", "jsonnet", file})
	const want = `{
  local k = import 'k.libsonnet',
  config:: {
    replicas: 2,  // overlays change it
  },
  deployment: local yakl__root = {
    kind: 'Deployment',
    metadata: {
      labels: { app: 'web' },
    },
    spec: {
      replicas: $.config.replicas,
      selector: { matchLabels: labels },
    },
  }, labels = yakl__root.metadata.labels; yakl__root,
}
`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// eval reads its inputs as show does: files, -e, overlays, -x and top-level arguments. It prints
// their values instead of looking for Kubernetes objects in them.
func TestEvalInputsLikeShow(t *testing.T) {
	t.Chdir(t.TempDir())
	for name, src := range map[string]string{
		"a.jsonnet":  "{ cm: { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'a' } }, note: 'hello' }",
		"f.jsonnet":  "function(name='dflt') { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: name } }",
		"ns.jsonnet": "{ cm+: { metadata+: { namespace: 'ns' } } }",
	} {
		if err := os.WriteFile(name, []byte(src), 0600); err != nil {
			t.Fatal(err)
		}
	}

	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"a.jsonnet", "-x", "$.note"}, "---\nhello\n"},
		{[]string{"-e", "{a: 1}", "-x", "$.a"}, "---\n1\n"},
		{[]string{"a.jsonnet", "-e", "{b: 2}", "-k"}, "---\n- cm\n- note\n---\n- b\n"},
		{[]string{"a.jsonnet", "--overlay-code-file", "ns.jsonnet", "-x", "$.cm.metadata"}, "---\nname: a\nnamespace: ns\n"},
		{[]string{"a.jsonnet", "--overlay-code", "{note: 'bye'}", "-x", "$.note"}, "---\nbye\n"},
		{[]string{"f.jsonnet", "-A", "name=t", "-x", "$.metadata.name"}, "---\nt\n"},
		{[]string{"f.jsonnet", "-x", "$.metadata.name"}, "---\ndflt\n"},
		{[]string{"-o", "json", "-e", "{a: [1]}"}, "{\n  \"a\": [\n    1\n  ]\n}\n"},
	} {
		got := cmdOutput(t, append([]string{"--features=eval,overlay", "eval"}, tc.args...))
		if got != tc.want {
			t.Errorf("eval %v: got %q, want %q", tc.args, got, tc.want)
		}
	}

	// show reads the same inputs, and looks for objects in them.
	got := cmdOutput(t, []string{"--features=eval,overlay", "show", "a.jsonnet", "--overlay-code-file", "ns.jsonnet", "-x", "$.cm"})
	if want := "---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: a\n  namespace: ns\n"; got != want {
		t.Errorf("show: got %q, want %q", got, want)
	}
}

func TestEvalErrors(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("a.jsonnet", []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--features=eval", "eval"}, "eval needs a file, or code with -e"},
		{[]string{"--features=eval", "eval", "a.jsonnet", "--trace"}, "--trace: traceback is an alpha feature"},
		{[]string{"--features=eval,traceback", "eval", "-e", "{}", "--trace"}, "--trace needs one file"},
		{[]string{"--features=eval", "eval", "-o", "jsonnet", "a.jsonnet"}, "-o jsonnet: yakl is an alpha feature"},
		{[]string{"--features=eval,yakl", "eval", "-o", "jsonnet", "a.jsonnet", "-x", "$"}, "can't be combined"},
		{[]string{"show", "a.jsonnet", "-x", "$"}, "--expr: eval is an alpha feature"},
	} {
		func() {
			defer resetFlags()
			RootCmd.SetArgs(tc.args)
			if err := RootCmd.Execute(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("%v: got %v, want %q", tc.args, err, tc.want)
			}
		}()
	}
}

// The old eval -e '$.foo' now evaluates $.foo on its own, and the hint says what to use.
func TestHintDollarInExec(t *testing.T) {
	if hint := Hint(errors.New("RUNTIME ERROR: No top-level object found.")); !strings.Contains(hint, "use -x") {
		t.Errorf("got hint %q", hint)
	}
}
