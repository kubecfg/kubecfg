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
	goflag "flag"
	"fmt"
	"io"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	jsonnet "github.com/google/go-jsonnet"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"golang.org/x/crypto/ssh/terminal"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	"github.com/kubecfg/kubecfg/pkg/features"
	"github.com/kubecfg/kubecfg/pkg/kubecfg"
	"github.com/kubecfg/kubecfg/pkg/kubecfg/vars"
	"github.com/kubecfg/kubecfg/pkg/yakl"
	"github.com/kubecfg/kubecfg/utils"

	// Register auth plugins
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

const (
	flagAlpha       = "alpha"
	flagFeatures    = "features"
	flagVerbose     = "verbose"
	flagJpath       = "jpath"
	flagJUrl        = "jurl"
	flagExtVar      = "ext-str"
	flagExtVarFile  = "ext-str-file"
	flagExtCode     = "ext-code"
	flagExtCodeFile = "ext-code-file"
	flagTLAVar      = "tla-str"
	flagTLAVarFile  = "tla-str-file"
	flagTLACode     = "tla-code"
	flagTLACodeFile = "tla-code-file"
	flagResolver    = "resolve-images"
	flagResolvFail  = "resolve-images-error"
	flagQPSLimit    = "qps-limit"
)

var clientConfig clientcmd.ClientConfig

// gates are the feature gates of this invocation, set before any command runs.
var gates *features.Gates
var overrides clientcmd.ConfigOverrides

func init() {
	cobra.OnInitialize(initConfig)

	RootCmd.PersistentFlags().Bool(flagAlpha, false, "Enable all alpha features, including ones added by future releases; prefer --features")
	RootCmd.PersistentFlags().StringSlice(flagFeatures, nil, `Turn features on or off: name, name=true or name=false, comma separated. May be repeated. Run "kubecfg features" to list them.`)
	RootCmd.PersistentFlags().CountP(flagVerbose, "v", "Increase verbosity. May be given multiple times.")
	RootCmd.PersistentFlags().StringArrayP(flagJpath, "J", nil, "Additional Jsonnet library search path, appended to the ones in the KUBECFG_JPATH env var. May be repeated.")
	RootCmd.MarkPersistentFlagFilename(flagJpath)
	RootCmd.PersistentFlags().StringArrayP(flagJUrl, "U", nil, "Additional Jsonnet library search path given as a URL. May be repeated.")
	RootCmd.PersistentFlags().StringArrayP(flagExtVar, "V", nil, "Values of external variables with string values")
	RootCmd.PersistentFlags().StringArray(flagExtVarFile, nil, "Read external variables with string values from files")
	RootCmd.MarkPersistentFlagFilename(flagExtVarFile)
	RootCmd.PersistentFlags().StringArray(flagExtCode, nil, "Values of external variables with values supplied as Jsonnet code")
	RootCmd.PersistentFlags().StringArray(flagExtCodeFile, nil, "Read external variables with values supplied as Jsonnet code from files")
	RootCmd.MarkPersistentFlagFilename(flagExtCodeFile)
	RootCmd.PersistentFlags().StringArrayP(flagTLAVar, "A", nil, "Values of top level arguments with string values")
	RootCmd.PersistentFlags().StringArray(flagTLAVarFile, nil, "Read top level arguments with string values from files")
	RootCmd.MarkPersistentFlagFilename(flagTLAVarFile)
	RootCmd.PersistentFlags().StringArray(flagTLACode, nil, "Values of top level arguments with values supplied as Jsonnet code")
	RootCmd.PersistentFlags().StringArray(flagTLACodeFile, nil, "Read top level arguments with values supplied as Jsonnet code from files")
	RootCmd.MarkPersistentFlagFilename(flagTLACodeFile)
	RootCmd.PersistentFlags().String(flagResolver, kubecfg.NoopResolver.String(), fmt.Sprintf("Change implementation of resolveImage native function. One of: %s", strings.Join(kubecfg.AvailableResolverTypes(), ", ")))
	RootCmd.PersistentFlags().String(flagResolvFail, kubecfg.WarnResolverError.String(), fmt.Sprintf("Action when resolveImage fails. One of: %s", strings.Join(kubecfg.AvailableResolverFailureAction(), ", ")))
	RootCmd.PersistentFlags().Float32(flagQPSLimit, 0, "Override k8s REST client-side rate limiting; library default is 5 QPS; a negative value disables.")

	// The "usual" clientcmd/kubectl flags
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.DefaultClientConfig = &clientcmd.DefaultClientConfig
	kflags := clientcmd.RecommendedConfigOverrideFlags("")
	RootCmd.PersistentFlags().StringVar(&loadingRules.ExplicitPath, "kubeconfig", "", "Path to a kube config. Only required if out-of-cluster")
	RootCmd.MarkPersistentFlagFilename("kubeconfig")
	clientcmd.BindOverrideFlags(&overrides, RootCmd.PersistentFlags(), kflags)
	clientConfig = clientcmd.NewInteractiveDeferredLoadingClientConfig(loadingRules, &overrides, os.Stdin)

	viper.BindPFlags(RootCmd.PersistentFlags())
}

// RootCmd is the root of cobra subcommand tree
var RootCmd = &cobra.Command{
	Use:           "kubecfg",
	Short:         "Synchronise Kubernetes resources with config files",
	SilenceErrors: true,
	SilenceUsage:  true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		goflag.CommandLine.Parse([]string{})
		flags := cmd.Flags()
		out := cmd.OutOrStderr()
		log.SetOutput(out)

		logFmt := NewLogFormatter(out)
		log.SetFormatter(logFmt)

		verbosity, err := flags.GetCount(flagVerbose)
		if err != nil {
			return err
		}
		log.SetLevel(logLevel(verbosity))

		// Ask me how much I love glog/klog's interface.
		logflags := goflag.NewFlagSet(os.Args[0], goflag.ExitOnError)
		klog.InitFlags(logflags)
		logflags.Set("logtostderr", "true")
		if verbosity >= 2 {
			// Semi-arbitrary mapping to klog level.
			logflags.Set("v", fmt.Sprintf("%d", verbosity*3))
		}

		rcFeatures, err := flagsFromFile(cmd, args)
		if err != nil {
			return err
		}

		// After flagsFromFile, so an rc file's alpha counts.
		// Through viper, so KUBECFG_FEATURES is the same setting as --features.
		// --features goes first: per feature, it beats the rc file's features.
		gates, err = features.New(viper.GetBool(flagAlpha), features.Source{Name: "--" + flagFeatures, Items: viper.GetStringSlice(flagFeatures)}, rcFeatures)
		return err
	},
}

// flagsFromFile sets cmd's flags from .kubecfgrc.jsonnet and returns the rc file's features.
// Only commands that evaluate files (evalCmdAnno) take flags from it, and they look for it from
// their input files. The feature settings, alpha and features, are global: every command takes
// them from the rc file's flags, looking for it from the current directory if it has no input files.
func flagsFromFile(cmd *cobra.Command, args []string) (features.Source, error) {
	_, evalCmd := cmd.Annotations[evalCmdAnno]
	if !evalCmd {
		args = nil // not input files
	}

	rcFile, found, err := findKubecfgRCFile(args)
	if err != nil {
		return features.Source{}, err
	}
	if !found {
		return features.Source{}, nil
	}
	// With every feature gate at its default: the rc file decides the features.
	vm, err := kubecfg.JsonnetVM()
	if err != nil {
		return features.Source{}, err
	}
	rcURL, err := utils.PathToURL(rcFile)
	if err != nil {
		return features.Source{}, err
	}

	// Each command's flags default to the shared $.flags, as the RFC describes,
	// except the feature settings, which aren't per command.
	rcStr, err := vm.EvaluateAnonymousSnippet(rcFile, fmt.Sprintf(
		`{flags: {}, commands: {[%q]: {[k]: $.flags[k] for k in std.objectFields($.flags) if k != %q && k != %q}}} + import %q`,
		cmd.Name(), flagAlpha, flagFeatures, rcURL))
	if err != nil {
		return features.Source{}, err
	}

	var rc struct {
		Flags    map[string]interface{}            `json:"flags"`
		Commands map[string]map[string]interface{} `json:"commands"`
	}
	if err := json.Unmarshal([]byte(rcStr), &rc); err != nil {
		return features.Source{}, err
	}
	for name, c := range rc.Commands {
		for _, k := range []string{flagAlpha, flagFeatures} {
			if _, ok := c[k]; ok {
				return features.Source{}, fmt.Errorf("%s: set %s under flags, not commands.%s; features apply to every command", rcFile, k, name)
			}
		}
	}

	settings := map[string]interface{}{}
	if evalCmd {
		maps.Copy(settings, rc.Commands[cmd.Name()])
	}
	if v, ok := rc.Flags[flagAlpha]; ok {
		settings[flagAlpha] = v
	}

	for k, v := range settings {
		f := cmd.Flags().Lookup(k)
		if f == nil {
			if _, shared := rc.Flags[k]; shared {
				continue // shared flags need not apply to every command
			}
			return features.Source{}, fmt.Errorf("unknown flag %q for command %q in %s", k, cmd.Name(), rcFile)
		}
		// client-go reads KUBECONFIG itself and can't tell an rc value from --kubeconfig,
		// so skip it here to keep the environment's cluster.
		if k == "kubeconfig" && os.Getenv("KUBECONFIG") != "" {
			continue
		}

		normalizeFlagValue := func(v interface{}) string {
			s := fmt.Sprint(v)

			_, isFileFlag := f.Annotations[cobra.BashCompFilenameExt]
			_, isDirFlag := f.Annotations[cobra.BashCompSubdirsInDir]
			if !isFileFlag && !isDirFlag {
				return s
			}
			resolve := func(p string) string {
				if filepath.IsAbs(p) {
					return p
				}
				return filepath.Join(filepath.Dir(rcFile), p)
			}
			// --{ext,tla}-*-file take name=path; only the path is relative to the rc file.
			if name, path, ok := strings.Cut(s, "="); ok && strings.HasSuffix(f.Name, "-file") {
				return name + "=" + resolve(path)
			}
			return resolve(s)
		}

		if !f.Changed {
			switch v := v.(type) {
			case []interface{}:
				var values []string
				for _, i := range v {
					values = append(values, normalizeFlagValue(i))
				}
				sliceValue, ok := f.Value.(pflag.SliceValue)
				if !ok {
					return features.Source{}, fmt.Errorf("flag %q is not an array flag, but instead it's a %q", k, f.Value.Type())
				}
				if err := sliceValue.Replace(values); err != nil {
					return features.Source{}, fmt.Errorf("invalid value for --%s in %s: %w", k, rcFile, err)
				}
			default:
				if err := f.Value.Set(normalizeFlagValue(v)); err != nil {
					return features.Source{}, fmt.Errorf("invalid value for --%s in %s: %w", k, rcFile, err)
				}
			}
		}
	}

	items, err := featureItems(rc.Flags[flagFeatures])
	if err != nil {
		return features.Source{}, fmt.Errorf("%s: %w", rcFile, err)
	}
	return features.Source{Name: ".kubecfgrc.jsonnet", Items: items}, nil
}

// featureItems turns the rc file's features, a list of names or an object of name: bool,
// into the items of a features.Source.
func featureItems(v interface{}) ([]string, error) {
	var items []string
	switch v := v.(type) {
	case nil:
	case []interface{}:
		for _, i := range v {
			s, ok := i.(string)
			if !ok {
				return nil, fmt.Errorf("features: want names, got %v", i)
			}
			items = append(items, s)
		}
	case map[string]interface{}:
		for _, name := range slices.Sorted(maps.Keys(v)) {
			on, ok := v[name].(bool)
			if !ok {
				return nil, fmt.Errorf("features: %s: want true or false, got %v", name, v[name])
			}
			items = append(items, fmt.Sprintf("%s=%t", name, on))
		}
	default:
		return nil, fmt.Errorf("features: want a list of names or an object of name: true/false, got %v", v)
	}
	return items, nil
}

func findKubecfgRCFile(files []string) (string, bool, error) {
	if len(files) == 0 {
		// Inline code (-e) has no file; look up from the current directory.
		files = []string{"-"}
	}

	seen := map[string]struct{}{}
	const noRcFileFound = "<no .kubecfgrc.jsonnet found>"

	for _, f := range files {
		// Absolute, so the search goes past the current directory and
		// the same rc file reached from relative and absolute inputs is one key.
		f, err := filepath.Abs(f)
		if err != nil {
			return "", false, err
		}
		path, found, err := utils.SearchUp(".kubecfgrc.jsonnet", f)
		if err != nil {
			return "", false, err
		}
		key := path
		if !found {
			key = noRcFileFound
		}
		seen[key] = struct{}{}
	}
	if len(seen) != 1 {
		var files []string
		for k := range seen {
			files = append(files, k)
		}
		return "", false, fmt.Errorf("all evaluated files should resolve to the same .kubecfgrc.jsonnet, found %q instead", files)
	}

	for k := range seen {
		if k == noRcFileFound {
			return "", false, nil
		} else {
			return k, true, nil
		}
	}
	return "", false, nil
}

// clientConfig.Namespace() is broken in client-go 3.0:
// namespace in config erroneously overrides explicit --namespace
func defaultNamespace(c clientcmd.ClientConfig) (string, error) {
	if overrides.Context.Namespace != "" {
		return overrides.Context.Namespace, nil
	}
	ns, _, err := c.Namespace()
	return ns, err
}

func logLevel(verbosity int) log.Level {
	switch verbosity {
	case 0:
		return log.InfoLevel
	default:
		return log.DebugLevel
	}
}

type logFormatter struct {
	escapes  *terminal.EscapeCodes
	colorise bool
}

// NewLogFormatter creates a new log.Formatter customised for writer
func NewLogFormatter(out io.Writer) log.Formatter {
	var ret = logFormatter{}
	if f, ok := out.(*os.File); ok {
		ret.colorise = terminal.IsTerminal(int(f.Fd()))
		ret.escapes = terminal.NewTerminal(f, "").Escape
	}
	return &ret
}

func (f *logFormatter) levelEsc(level log.Level) []byte {
	switch level {
	case log.DebugLevel:
		return []byte{}
	case log.WarnLevel:
		return f.escapes.Yellow
	case log.ErrorLevel, log.FatalLevel, log.PanicLevel:
		return f.escapes.Red
	default:
		return f.escapes.Blue
	}
}

func (f *logFormatter) Format(e *log.Entry) ([]byte, error) {
	buf := bytes.Buffer{}
	if f.colorise {
		buf.Write(f.levelEsc(e.Level))
		fmt.Fprintf(&buf, "%-5s ", strings.ToUpper(e.Level.String()))
		buf.Write(f.escapes.Reset)
	}

	buf.WriteString(strings.TrimSpace(e.Message))
	buf.WriteString("\n")

	return buf.Bytes(), nil
}

// NB: `path` is assumed to be in native-OS path separator form
func dirURL(path string) *url.URL {
	path = filepath.ToSlash(path)
	if path[len(path)-1] != '/' {
		// trailing slash is important
		path = path + "/"
	}
	return &url.URL{Scheme: "file", Path: path}
}

// JsonnetVM constructs a new jsonnet.VM, according to command line
// flags
func JsonnetVM(cmd *cobra.Command) (*jsonnet.VM, error) {
	var opts []kubecfg.JsonnetVMOpt

	flags := cmd.Flags()

	jpath := filepath.SplitList(os.Getenv("KUBECFG_JPATH"))

	jpathArgs, err := flags.GetStringArray(flagJpath)
	if err != nil {
		return nil, err
	}
	jpath = append(jpath, jpathArgs...)
	opts = append(opts, kubecfg.WithImportPath(jpath...))

	sURLs, err := flags.GetStringArray(flagJUrl)
	if err != nil {
		return nil, err
	}
	opts = append(opts, kubecfg.WithImportURLs(sURLs...))

	opts = append(opts, kubecfg.WithFeatures(gates))

	withVar := func(typ vars.Type, expr vars.ExpressionType, source vars.Source) func(string, string) {
		return func(name, value string) {
			opts = append(opts, kubecfg.WithVar(vars.New(typ, expr, source, name, value)))
		}
	}

	resolverType := kubecfg.ParseResolverType(viper.GetString(flagResolver))
	resolverFailureAction := kubecfg.ParseResolverFailureAction(viper.GetString(flagResolvFail))
	opts = append(opts, kubecfg.WithResolver(resolverType, resolverFailureAction))

	for _, spec := range []struct {
		flagName string
		fromFile bool
		setter   func(string, string)
	}{
		{flagExtVar, false, withVar(vars.Ext, vars.String, vars.Literal)},
		{flagExtVarFile, true, withVar(vars.Ext, vars.String, vars.File)},
		{flagExtCode, false, withVar(vars.Ext, vars.Code, vars.Literal)},
		{flagExtCodeFile, true, withVar(vars.Ext, vars.Code, vars.File)},
		{flagTLAVar, false, withVar(vars.TLA, vars.String, vars.Literal)},
		{flagTLAVarFile, true, withVar(vars.TLA, vars.String, vars.File)},
		{flagTLACode, false, withVar(vars.TLA, vars.Code, vars.Literal)},
		{flagTLACodeFile, true, withVar(vars.TLA, vars.Code, vars.File)},
	} {
		entries, err := flags.GetStringArray(spec.flagName)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			kv := strings.SplitN(entry, "=", 2)
			if spec.fromFile {
				if len(kv) != 2 {
					return nil, fmt.Errorf("Failed to parse %s: missing '=' in %s", spec.flagName, entry)
				}
				spec.setter(kv[0], kv[1])
			} else {
				switch len(kv) {
				case 1:
					if v, present := os.LookupEnv(kv[0]); present {
						spec.setter(kv[0], v)
					} else {
						return nil, fmt.Errorf("Missing environment variable: %s", kv[0])
					}
				case 2:
					spec.setter(kv[0], kv[1])
				}
			}
		}
	}

	return kubecfg.JsonnetVM(opts...)
}

func readObjs(cmd *cobra.Command, paths []string, opts ...utils.ReadOption) ([]*unstructured.Unstructured, error) {
	flags := cmd.Flags()

	exec, err := flags.GetString(flagExec)
	if err != nil {
		return nil, err
	}
	if exec != "" {
		paths = append(paths, utils.ToDataURL(yakl.PredefineKubecfg(exec)))
	}

	overlayCodeFile, err := flags.GetString(flagOverlayCodeFile)
	if err != nil {
		return nil, err
	}

	overlay, err := flags.GetString(flagOverlay)
	if err != nil {
		return nil, err
	}
	if overlay != "" {
		// deprecated. pflag will print a warning
		overlayCodeFile = overlay
	}

	if overlayCodeFile != "" {
		if err := gates.Require(features.Overlay); err != nil {
			return nil, fmt.Errorf("--%s: %w", flagOverlayCodeFile, err)
		}
		opts = append(opts, utils.WithOverlayURL(overlayCodeFile))
	}

	overlayCode, err := flags.GetString(flagOverlayCode)
	if err != nil {
		return nil, err
	}
	if overlayCode != "" {
		if err := gates.Require(features.Overlay); err != nil {
			return nil, fmt.Errorf("--%s: %w", flagOverlayCode, err)
		}
		opts = append(opts, utils.WithOverlayCode(overlayCode))
	}

	return readObjsInternal(cmd, paths, opts...)
}

func readObjsInternal(cmd *cobra.Command, paths []string, opts ...utils.ReadOption) ([]*unstructured.Unstructured, error) {
	vm, err := JsonnetVM(cmd)
	if err != nil {
		return nil, err
	}
	return kubecfg.ReadObjects(vm, paths, opts...)

}

// For debugging
func dumpJSON(v interface{}) string {
	buf := bytes.NewBuffer(nil)
	enc := json.NewEncoder(buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err.Error()
	}
	return string(buf.Bytes())
}

func getDynamicClients(cmd *cobra.Command) (dynamic.Interface, meta.RESTMapper, discovery.DiscoveryInterface, error) {
	conf, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("Unable to read kubectl config: %v", err)
	}

	if qps, err := cmd.Flags().GetFloat32(flagQPSLimit); err != nil {
		return nil, nil, nil, err
	} else {
		conf.QPS = qps
	}

	disco, err := discovery.NewDiscoveryClientForConfig(conf)
	if err != nil {
		return nil, nil, nil, err
	}
	discoCache := utils.NewMemcachedDiscoveryClient(disco)

	mapper := restmapper.NewDeferredDiscoveryRESTMapper(discoCache)

	cl, err := dynamic.NewForConfig(conf)
	if err != nil {
		return nil, nil, nil, err
	}

	return cl, mapper, discoCache, nil
}

var unknownKubecfg = regexp.MustCompile(`Unknown variable: kubecfg\b`)

// Hint returns advice to print after err, or "".
func Hint(err error) string {
	if !unknownKubecfg.MatchString(err.Error()) {
		return ""
	}
	// Elsewhere it's predefined (RFC06), so the code that failed is in a Jsonnet file.
	return "hint: kubecfg is not predefined in Jsonnet files. Add this line at the top of the file:\n" +
		"  local kubecfg = import 'kubecfg.libsonnet';"
}

func initConfig() {
	viper.AutomaticEnv()
	viper.SetEnvPrefix("KUBECFG")
	// --resolve-images-error is KUBECFG_RESOLVE_IMAGES_ERROR: shells can't export names with dashes.
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
}
