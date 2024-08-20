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
	"net/url"
	"os"
	"path/filepath"
	"strings"

	jsonnet "github.com/google/go-jsonnet"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/crypto/ssh/terminal"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	"github.com/kubecfg/kubecfg/pkg/kubecfg"
	"github.com/kubecfg/kubecfg/pkg/kubecfg/vars"
	"github.com/kubecfg/kubecfg/utils"

	// Register auth plugins
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

const (
	flagAlpha       = "alpha"
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
)

var clientConfig clientcmd.ClientConfig
var overrides clientcmd.ConfigOverrides

func init() {
	cobra.OnInitialize(initConfig)

	RootCmd.PersistentFlags().Bool(flagAlpha, false, "Enable alpha features")
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
	RootCmd.PersistentFlags().String(flagResolver, "noop", "Change implementation of resolveImage native function. One of: noop, registry")
	RootCmd.PersistentFlags().String(flagResolvFail, "warn", "Action when resolveImage fails. One of ignore,warn,error")

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

		return nil
	},
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

	opts = append(opts, kubecfg.WithAlpha(viper.GetBool(flagAlpha)))

	withVar := func(typ vars.Type, expr vars.ExpressionType, source vars.Source) func(string, string) {
		return func(name, value string) {
			opts = append(opts, kubecfg.WithVar(vars.New(typ, expr, source, name, value)))
		}
	}

	resolverType := kubecfg.ResolverType_value[viper.GetString(flagResolver)]
	resolverFailureAction := kubecfg.ResolverFailureAction_value[viper.GetString(flagResolvFail)]
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
		paths = append(paths, utils.ToDataURL(exec))
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
		alpha := viper.GetBool(flagAlpha)
		if !alpha {
			return nil, fmt.Errorf("--%s is an alpha feature please use --%s", flagOverlayCodeFile, flagAlpha)
		}
		opts = append(opts, utils.WithOverlayURL(overlayCodeFile))
	}

	overlayCode, err := flags.GetString(flagOverlayCode)
	if err != nil {
		return nil, err
	}
	if overlayCode != "" {
		alpha := viper.GetBool(flagAlpha)
		if !alpha {
			return nil, fmt.Errorf("--%s is an alpha feature please use --%s", flagOverlayCode, flagAlpha)
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

func initConfig() {
	viper.AutomaticEnv()
	viper.SetEnvPrefix("KUBECFG")
}
