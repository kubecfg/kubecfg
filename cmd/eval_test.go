package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEvalJsonnet(t *testing.T) {
	file := filepath.Join(t.TempDir(), "app.yakl")
	src := "'&k': ${import 'k.libsonnet'}\n--- !config::\nreplicas: 2 # overlays change it\n--- !deployment\nkind: Deployment\n" +
		"metadata:\n  labels: &labels {app: web}\nspec:\n  replicas: ${config.replicas}\n  selector: {matchLabels: '${labels}'}\n"
	if err := os.WriteFile(file, []byte(src), 0600); err != nil {
		t.Fatal(err)
	}
	// The file isn't evaluated, so the missing import doesn't matter.
	got := cmdOutput(t, []string{"--alpha", "eval", "-o", "jsonnet", file})
	const want = `{
  local k = import 'k.libsonnet',
  local config = self.config,
  config:: {
    replicas: 2,  // overlays change it
  },
  local deployment = self.deployment,
  deployment: local yakl__root = {
    kind: 'Deployment',
    metadata: {
      labels: { app: 'web' },
    },
    spec: {
      replicas: config.replicas,
      selector: { matchLabels: labels },
    },
  }, labels = yakl__root.metadata.labels; yakl__root,
}
`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
