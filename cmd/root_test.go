package cmd

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kubecfg/kubecfg/pkg/features"
	"github.com/kubecfg/kubecfg/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
	if want := map[string]string{"eval": ".kubecfgrc.jsonnet", "yakl": "--features", "pinned-imports": "default"}; !reflect.DeepEqual(got, want) {
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

// RFC06: kubecfg is predefined in code that isn't in a Jsonnet file, and imports work there.
func TestPredefinedKubecfg(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"app.jsonnet":              `{a: 1}`,
		"cm.jsonnet":               `{apiVersion: 'v1', kind: 'ConfigMap', metadata: {name: 'x'}}`,
		"f.jsonnet":                `function(v) v`,
		"g.jsonnet":                `function(kubecfg) {}`,
		"cm.yakl":                  "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: ${kubecfg.parseYaml('x')[0]}\n",
		"file.jsonnet":             `kubecfg.parseYaml('x')[0]`,
		"vendor/kubecfg.libsonnet": `{isK8sObject(o):: 'vendored'}`,
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)
	run := func(args ...string) error {
		defer resetFlags()
		RootCmd.SetOutput(io.Discard)
		defer RootCmd.SetOutput(nil)
		RootCmd.SetArgs(append([]string{"--features=eval,overlay,yakl"}, args...))
		return RootCmd.Execute()
	}

	const imp = "(import 'kubecfg.libsonnet').parseYaml('x')[0]"
	for name, args := range map[string][]string{
		"eval -x":         {"eval", "-o", "json", "app.jsonnet", "-x", "kubecfg.parseYaml('x')[0]"},
		"eval --trace":    {"eval", "-o", "json", "cm.jsonnet", "--trace", "-x", "kubecfg.toOverlay({}) + $.metadata"},
		"eval -e":         {"eval", "-o", "json", "-e", "kubecfg.parseYaml('x')[0]"},
		"--exec":          {"show", "-e", "{apiVersion: 'v1', kind: 'ConfigMap', metadata: {name: kubecfg.parseYaml('x')[0]}}"},
		"--overlay-code":  {"show", "cm.jsonnet", "--overlay-code", "{metadata+: {name: kubecfg.parseYaml('y')[0]}}"},
		"--ext-code":      {"eval", "-o", "json", "app.jsonnet", "-x", "std.extVar('v')", "--ext-code", "v=kubecfg.parseYaml('x')[0]"},
		"--tla-code":      {"eval", "-o", "json", "f.jsonnet", "--tla-code", "v=kubecfg.parseYaml('x')[0]"},
		"a .yakl file":    {"show", "cm.yakl"},
		"a TLA named so":  {"eval", "-o", "json", "g.jsonnet", "--tla-code", "kubecfg=2", "-x", "kubecfg + 1"},
		"an object local": {"eval", "-o", "json", "app.jsonnet", "-x", "{local kubecfg = 1, v: kubecfg + 1}"},
		// Imports from code whose name isn't a URL.
		"an import in eval -x":    {"eval", "-o", "json", "app.jsonnet", "-x", imp},
		"an import in --ext-code": {"eval", "-o", "json", "app.jsonnet", "-x", "std.extVar('v')", "--ext-code", "v=" + imp},
		"an import in --tla-code": {"eval", "-o", "json", "f.jsonnet", "--tla-code", "v=" + imp},
	} {
		if err := run(args...); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}

	// The predefined kubecfg is always the built-in copy; an explicit import follows the search path.
	show := func(code string) string {
		return cmdOutput(t, []string{"show", "-J", "vendor", "-o", "json", "-e",
			code + "{apiVersion: 'v1', kind: 'ConfigMap', metadata: {name: std.toString(kubecfg.isK8sObject({}))}}"})
	}
	if got := show(""); !strings.Contains(got, `"name": "false"`) {
		t.Errorf("predefined: got %s", got)
	}
	if got := show("local kubecfg = import 'kubecfg.libsonnet'; "); !strings.Contains(got, `"name": "vendored"`) {
		t.Errorf("imported: got %s", got)
	}

	// A Jsonnet file still needs the import, and the hint says so.
	err := run("eval", "file.jsonnet")
	if err == nil || !strings.Contains(err.Error(), "Unknown variable: kubecfg") {
		t.Fatalf("got %v, want Unknown variable: kubecfg", err)
	}
	if hint := Hint(err); !strings.Contains(hint, "local kubecfg = import 'kubecfg.libsonnet';") {
		t.Errorf("got hint %q", hint)
	}
	if hint := Hint(errors.New("Unknown variable: kubecfgx")); hint != "" {
		t.Errorf("another variable: got hint %q", hint)
	}
}

func TestEnvVarUnderscores(t *testing.T) {
	t.Setenv("KUBECFG_RESOLVE_IMAGES_ERROR", "error")
	initConfig()
	if got := viper.GetString(flagResolvFail); got != "error" {
		t.Errorf("got %q, want %q", got, "error")
	}
}
