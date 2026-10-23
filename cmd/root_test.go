package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kubecfg/kubecfg/pkg/features"
	"github.com/kubecfg/kubecfg/utils"
	"github.com/spf13/cobra"
)

func TestReadObjsDuplicates(t *testing.T) {
	cmd := RootCmd
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatal(err)
	}

	_, err := readObjsInternal(cmd, []string{filepath.FromSlash("../testdata/duplicates.jsonnet")})
	if got, want := err.Error(), `duplicate resource ConfigMap, "myns", "foo"`; got != want {
		t.Fatalf("got: %s, want: %s", got, want)
	}
}

func TestReadObjsDuplicatesVersion(t *testing.T) {
	cmd := RootCmd
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatal(err)
	}

	_, err := readObjsInternal(cmd, []string{filepath.FromSlash("../testdata/duplicates_version.jsonnet")})
	if got, want := err.Error(), `duplicate resource Ingress.networking.k8s.io, "myns", "foo"`; got != want {
		t.Fatalf("got: %s, want: %s", got, want)
	}
}

func TestReadObjsDuplicatesLiteral(t *testing.T) {
	cmd := RootCmd
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatal(err)
	}

	_, err := readObjsInternal(cmd, []string{filepath.FromSlash("../testdata/duplicates_literal.jsonnet")})
	if err != nil {
		got := err.Error()
		t.Fatalf("got: %s, want: nil", got)
	}
}

func TestReadObjsDuplicatesLiteralShowProvenance(t *testing.T) {
	cmd := RootCmd
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatal(err)
	}

	_, err := readObjsInternal(cmd, []string{filepath.FromSlash("../testdata/duplicates_literal.jsonnet")}, utils.WithProvenance(true))
	if err != nil {
		got := err.Error()
		t.Fatalf("got: %s, want: nil", got)
	}
}

func TestFlagsFromFileInvalid(t *testing.T) {
	for _, rc := range []string{
		`{commands: {t: {'dry-run': 'yes'}}}`,
		`{commands: {t: {'no-such-flag': true}}}`,
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".kubecfgrc.jsonnet"), []byte(rc), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := &cobra.Command{Use: "t", Annotations: map[string]string{evalCmdAnno: ""}}
		cmd.Flags().Bool("dry-run", false, "")
		if _, err := flagsFromFile(cmd, []string{filepath.Join(dir, "main.jsonnet")}); err == nil {
			t.Errorf("%s: want error, got nil", rc)
		}
	}
}

func TestFlagsFromFileValues(t *testing.T) {
	dir := t.TempDir()
	rc := `{
		flags: {'ext-str-file': ['v=value.txt'], jpath: ['/abs', 'rel'], 'dry-run': true},
		commands+: {t+: {'ext-str': ['a=b']}},
	}`
	if err := os.WriteFile(filepath.Join(dir, ".kubecfgrc.jsonnet"), []byte(rc), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := &cobra.Command{Use: "t", Annotations: map[string]string{evalCmdAnno: ""}}
	for _, name := range []string{"ext-str-file", "jpath", "ext-str"} {
		cmd.Flags().StringArray(name, nil, "")
	}
	cmd.MarkFlagFilename("ext-str-file")
	cmd.MarkFlagFilename("jpath")

	// Relative input from inside a subdirectory still finds the rc file above it.
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)
	if _, err := flagsFromFile(cmd, []string{"main.jsonnet"}); err != nil {
		t.Fatal(err)
	}

	for name, want := range map[string][]string{
		"ext-str-file": {"v=" + filepath.Join(dir, "value.txt")},
		"jpath":        {"/abs", filepath.Join(dir, "rel")},
		"ext-str":      {"a=b"},
	} {
		got, _ := cmd.Flags().GetStringArray(name)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("--%s = %q, want %q", name, got, want)
		}
	}
}

func TestFlagsFromFileKubeconfigEnvWins(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".kubecfgrc.jsonnet"), []byte(`{flags: {kubeconfig: 'rc.yaml'}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for env, want := range map[string]string{"": filepath.Join(dir, "rc.yaml"), "/env.yaml": ""} {
		t.Setenv("KUBECONFIG", env)
		cmd := &cobra.Command{Use: "t", Annotations: map[string]string{evalCmdAnno: ""}}
		cmd.Flags().String("kubeconfig", "", "")
		cmd.MarkFlagFilename("kubeconfig")
		if _, err := flagsFromFile(cmd, []string{filepath.Join(dir, "main.jsonnet")}); err != nil {
			t.Fatal(err)
		}
		if got, _ := cmd.Flags().GetString("kubeconfig"); got != want {
			t.Errorf("KUBECONFIG=%q: --kubeconfig = %q, want %q", env, got, want)
		}
	}
}

func TestFlagsFromFileFeatures(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("main.jsonnet", `{a: 1}`)
	t.Chdir(dir)

	// eval is alpha: it fails unless the rc file's alpha counts.
	write(".kubecfgrc.jsonnet", `{flags: {alpha: true}}`)
	cmdOutput(t, []string{"eval", "main.jsonnet"})

	// kubecfg features evaluates no files, but reads the rc file's features,
	// and --features beats them one feature at a time.
	write(".kubecfgrc.jsonnet", `{flags+: {features+: {eval: true, pack: true}}}`)
	var list []features.Status
	if err := json.Unmarshal([]byte(cmdOutput(t, []string{"features", "-o", "json", "--features=pack=false,yakl"})), &list); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, s := range list {
		if s.Enabled {
			got[s.Name] = s.Source
		}
	}
	if want := map[string]string{"eval": ".kubecfgrc.jsonnet", "yakl": "--features"}; !reflect.DeepEqual(got, want) {
		t.Errorf("enabled features: got %v, want %v", got, want)
	}

	// Features are global, not per command.
	write(".kubecfgrc.jsonnet", `{commands+: {show+: {features: ['yakl']}}}`)
	defer resetFlags()
	RootCmd.SetArgs([]string{"show", "main.jsonnet"})
	if err := RootCmd.Execute(); err == nil || !strings.Contains(err.Error(), "set features under flags, not commands.show") {
		t.Errorf("got %v, want an error about features under commands", err)
	}
}
