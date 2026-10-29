package cmd

import (
	"os"
	"path/filepath"
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

func TestEvalInputs(t *testing.T) {
	run := func(args ...string) error {
		defer resetFlags()
		RootCmd.SetArgs(append([]string{"--features=eval", "eval"}, args...))
		return RootCmd.Execute()
	}
	// -e is code on its own, as in show and jsonnet; -x is an expression against the input.
	if err := run("-e", "{a: 1}", "-x", "$.a"); err != nil {
		t.Errorf("-e: %v", err)
	}
	for _, args := range [][]string{{"app.jsonnet", "-e", "$.a"}, {"a.jsonnet", "b.jsonnet"}, {}} {
		if err := run(args...); err == nil {
			t.Errorf("%v: want an error", args)
		}
	}
}
