// Copyright 2023 The kubecfg authors
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

package kubecfg

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/go-jsonnet"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/kubecfg/kubecfg/internal/acquire"
	"github.com/kubecfg/kubecfg/pkg/features"
	"github.com/kubecfg/kubecfg/pkg/kubecfg/vars"
	"github.com/kubecfg/kubecfg/pkg/yakl"
	"github.com/kubecfg/kubecfg/utils"
)

type jsonnetVMOpts struct {
	gates      *features.Gates
	workingDir string
	importPath []string
	importURLs []string
	vars       []vars.Var

	resolverType          ResolverType
	resolverFailureAction ResolverFailureAction
}

type JsonnetVMOpt func(*jsonnetVMOpts)

// WithAlpha turns on every alpha feature, as --alpha does; WithAlpha(false) leaves every feature at its default.
func WithAlpha(enable bool) JsonnetVMOpt {
	return func(opts *jsonnetVMOpts) {
		opts.gates = nil
		if enable {
			opts.gates = features.AllAlpha()
		}
	}
}

func WithFeatures(gates *features.Gates) JsonnetVMOpt {
	return func(opts *jsonnetVMOpts) {
		opts.gates = gates
	}
}

func WithWorkingDir(dir string) JsonnetVMOpt {
	return func(opts *jsonnetVMOpts) {
		opts.workingDir = dir
	}
}

func WithImportPath(importPath ...string) JsonnetVMOpt {
	return func(opts *jsonnetVMOpts) {
		opts.importPath = importPath
	}
}

func WithImportURLs(importURLs ...string) JsonnetVMOpt {
	return func(opts *jsonnetVMOpts) {
		opts.importURLs = importURLs
	}
}

func WithVar(v vars.Var) JsonnetVMOpt {
	return func(opts *jsonnetVMOpts) {
		opts.vars = append(opts.vars, v)
	}
}

type ResolverType int

const (
	NoopResolver ResolverType = iota
	RegistryResolver
)

var (
	// resolverTypeValue returns the ResolverType value for a string
	resolverTypeValue = map[string]ResolverType{
		"noop":     NoopResolver,
		"registry": RegistryResolver,
	}
	// resolverTypeName returns the string value for a ResolverType
	resolverTypeName = map[ResolverType]string{
		NoopResolver:     "noop",
		RegistryResolver: "registry",
	}
)

func (t ResolverType) String() string {
	return resolverTypeName[t]
}

// AvailableResolverTypes returns the possible values of ResolverTypes
func AvailableResolverTypes() []string {
	var types []string
	for t := range resolverTypeValue {
		types = append(types, t)
	}
	return types
}

func ParseResolverType(rt string) ResolverType {
	switch rt {
	case "noop":
		return NoopResolver
	case "registry":
		return RegistryResolver
	default:
		return NoopResolver
	}
}

type ResolverFailureAction int

const (
	IgnoreResolverError ResolverFailureAction = iota
	WarnResolverError
	ReportResolverError
)

var (
	// resolverFailureActionValue returns the ResolverFailureAction value for a string
	resolverFailureActionValue = map[string]ResolverFailureAction{
		"ignore": IgnoreResolverError,
		"warn":   WarnResolverError,
		"error":  ReportResolverError,
	}
	// resolverFailureActionName returns the string value for a ResolverFailureAction
	resolverFailureActionName = map[ResolverFailureAction]string{
		IgnoreResolverError: "ignore",
		WarnResolverError:   "warn",
		ReportResolverError: "error",
	}
)

func (t ResolverFailureAction) String() string {
	return resolverFailureActionName[t]
}

// AvailableResolverFailureAction returns the possible values of ResolverFailureAction
func AvailableResolverFailureAction() []string {
	var types []string
	for t := range resolverFailureActionValue {
		types = append(types, t)
	}
	return types
}

func ParseResolverFailureAction(rfa string) ResolverFailureAction {
	switch rfa {
	case "ignore":
		return IgnoreResolverError
	case "warn":
		return WarnResolverError
	case "error":
		return ReportResolverError
	default:
		return WarnResolverError
	}
}

func WithResolver(typ ResolverType, failureMode ResolverFailureAction) JsonnetVMOpt {
	return func(opts *jsonnetVMOpts) {
		opts.resolverType = typ
		opts.resolverFailureAction = failureMode
	}
}

// JsonnetVM constructs a new jsonnet.VM, according to command line
// flags
func JsonnetVM(opt ...JsonnetVMOpt) (*jsonnet.VM, error) {
	vm := jsonnet.MakeVM()

	var opts jsonnetVMOpts
	for _, o := range opt {
		o(&opts)
	}

	var searchUrls []*url.URL
	for _, p := range opts.importPath {
		p, err := filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		searchUrls = append(searchUrls, dirURL(p))
	}

	sURLs := opts.importURLs
	// Special URL scheme used to find embedded content
	sURLs = append(sURLs, "internal:///")

	for _, ustr := range sURLs {
		u, err := url.Parse(ustr)
		if err != nil {
			return nil, err
		}
		if u.Path[len(u.Path)-1] != '/' {
			u.Path = u.Path + "/"
		}
		searchUrls = append(searchUrls, u)
	}

	for _, u := range searchUrls {
		log.Debugln("Jsonnet search path:", u)
	}

	if opts.workingDir == "" {
		var err error
		opts.workingDir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("unable to determine current working directory: %w", err)
		}
	}

	cwd := opts.workingDir
	for _, v := range opts.vars {
		name, value := v.Name, v.Value

		if v.Source == vars.File {
			// Ensure that the import path we construct here is absolute, so that our Importer
			// won't try to glean from an extVar or TLA reference the context necessary to
			// resolve a relative path.
			path := value
			if !filepath.IsAbs(path) {
				path = filepath.Join(cwd, path)
			}
			target := (&url.URL{Scheme: "file", Path: path}).String()
			imp := "importstr"
			if v.Expr == vars.Code {
				imp, target = "import", utils.ImportPath(target)
			}

			value = fmt.Sprintf("%s @'%s'", imp, strings.ReplaceAll(target, "'", "''"))
		} else if v.Expr == vars.Code {
			value = yakl.PredefineKubecfg(value)
		}

		v.Setter()(vm, name, value)
	}

	vm.Importer(utils.MakeUniversalImporterWithFeatures(searchUrls, opts.gates))

	resolver, err := buildResolver(&opts)
	if err != nil {
		return nil, err
	}
	utils.RegisterNativeFuncs(vm, resolver)
	utils.RegisterFeatures(vm, opts.gates)

	return vm, nil
}

func buildResolver(opts *jsonnetVMOpts) (utils.Resolver, error) {
	ret := resolverErrorWrapper{}

	switch action := opts.resolverFailureAction; action {
	case IgnoreResolverError:
		ret.OnErr = func(error) error { return nil }
	case WarnResolverError:
		ret.OnErr = func(err error) error {
			log.Warning(err.Error())
			return nil
		}
	case ReportResolverError:
		ret.OnErr = func(err error) error { return err }
	default:
		return nil, fmt.Errorf("bad value %d for resolver failure mode", action)
	}

	switch resolver := opts.resolverType; resolver {
	case NoopResolver:
		ret.Inner = utils.NewIdentityResolver()
	case RegistryResolver:
		ret.Inner = utils.NewRegistryResolver()
	default:
		return nil, fmt.Errorf("bad value %d for resolver tyoe", resolver)
	}

	return &ret, nil
}

type resolverErrorWrapper struct {
	Inner utils.Resolver
	OnErr func(error) error
}

func (r *resolverErrorWrapper) Resolve(image *utils.ImageName) error {
	err := r.Inner.Resolve(image)
	if err != nil {
		err = r.OnErr(err)
	}
	return err
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

type overlayFormat int

const (
	jsonnetCode overlayFormat = iota
	jsonnetFile
)

// parse returns the code of an overlay. root says whether it applies to the whole file, rather than
// to a field that the flag names.
func (f overlayFormat) parse(src string, root bool) string {
	switch f {
	case jsonnetCode:
		return src
	case jsonnetFile:
		switch ext := path.Ext(src); ext {
		case ".yml", ".yaml", ".json":
			// JSON is a proper subset of YAML (for our intents and purposes)
			return fmt.Sprintf("(import 'kubecfg.libsonnet').toOverlay(std.parseYaml(importstr %q))", src)
		case yakl.Ext:
			// A yakl overlay spells its own +: and ::, so it applies as written.
			if !root {
				return fmt.Sprintf("import %q", utils.ImportPath(src))
			}
			// The assert sees the base as super: a whole object without a name would replace a
			// base with names.
			return fmt.Sprintf(`(local o = import %q; if !std.isObject(o) then o else {`+
				`assert !(std.objectHas(o, 'apiVersion') && std.objectHas(o, 'kind') && !('kind' in super)) : %q} + o)`,
				utils.ImportPath(src), src+": a whole object without a name, overlaid on a file with names; name it with ^^: name to add it as a document")
		default:
			return fmt.Sprintf("import %q", src)
		}
	default:
		panic(fmt.Sprintf("unhandled format %d", f))
	}
}

func buildOverlayObject(format overlayFormat, flag string) string {
	re := regexp.MustCompile(`^([^$=]*)=(.*)$`)
	parts := re.FindStringSubmatch(flag)

	if len(parts) > 0 {
		path, value := parts[1], parts[2]
		wrapper := format.parse(value, false)
		fields := strings.Split(path, ".")
		// we can't use slices.Reverse(s) since we must build on Go 1.21
		for i, j := 0, len(fields)-1; i < j; i, j = i+1, j-1 {
			fields[i], fields[j] = fields[j], fields[i]
		}
		for _, p := range fields {
			wrapper = fmt.Sprintf("{%s+:%s}", p, wrapper)
		}
		return wrapper
	} else {
		return format.parse(flag, true)
	}
}

// ReadObjects evaluates all jsonnet files in paths and return all the k8s objects found in it.
// Unlike utils.Read this checks for duplicates and flattens the v1 Lists.
func ReadObjects(vm *jsonnet.VM, paths []string, opts ...utils.ReadOption) ([]*unstructured.Unstructured, error) {
	res := []*unstructured.Unstructured{}
	err := readInputs(paths, opts, func(in string) error {
		objs, err := utils.Read(vm, in, opts...)
		res = append(res, utils.FlattenToV1(objs)...)
		return err
	})
	if err != nil {
		return nil, err
	}
	deduped, err := utils.RemoveDuplicates(res)
	if err != nil {
		return nil, err
	}
	return deduped, nil
}

// readInputs calls read with what to evaluate for each of paths. show and eval read their inputs
// through it, so they only differ in what they do with the values.
func readInputs(paths []string, opts []utils.ReadOption, read func(in string) error) error {
	opt := acquire.MakeReadOptions(opts)
	for _, path := range paths {
		in, err := input(path, opt)
		if err != nil {
			return err
		}
		if err := read(in); err != nil {
			return fmt.Errorf("error reading %s: %v", path, err)
		}
	}
	return nil
}

// input returns what to evaluate for the input at path: path itself, or, when opt adds an overlay or
// an expression, a data: URL with the code that does it.
func input(path string, opt acquire.ReadOptions) (string, error) {
	if opt.OverlayURL != "" && opt.OverlayCode != "" {
		return "", fmt.Errorf("--overlay-code and --overlay-file are mutually exclusive")
	}
	var overlay string
	if opt.OverlayURL != "" {
		overlay = buildOverlayObject(jsonnetFile, opt.OverlayURL)
	}
	if opt.OverlayCode != "" {
		overlay = buildOverlayObject(jsonnetCode, opt.OverlayCode)
	}
	if overlay == "" && opt.Expr == "" {
		return path, nil
	}

	root := fmt.Sprintf("(import %q)", utils.ImportPath(path))
	if code, ok := strings.CutPrefix(path, "data:,"); ok { // the code of -e, which can't be imported
		code, err := url.PathUnescape(code)
		if err != nil {
			return "", err
		}
		// A newline, so that a // comment at the end of the code doesn't hide the parenthesis.
		root = "(" + code + "\n)"
	}
	return utils.ToDataURL(inputCode(root, overlay, opt.Expr, opt.TLA)), nil
}

// inputCode returns Jsonnet evaluating expr, with `$` being the result of the code root, called with
// the top-level arguments tla, plus overlay. Without expr, it evaluates to that result. Like all
// code from the command line, it imports relative to the current directory.
func inputCode(root, overlay, expr string, tla []string) string {
	pairs := make([]string, len(tla))
	for i := range tla {
		pairs[i] = fmt.Sprintf("%s=%s", tla[i], tla[i])
	}
	// A function is called with the top-level arguments, as jsonnet does with a file, e.g.
	// `function(foo,bar) ... (import %q)(foo=foo,bar=bar)`. The actuals are keyworded so that the
	// order of TLA flags doesn't matter.
	root = fmt.Sprintf("(local Z____in__Z_ = %s; if Z____std__Z_.isFunction(Z____in__Z_) then Z____in__Z_(%s) else Z____in__Z_)",
		root, strings.Join(pairs, ","))
	if overlay != "" {
		root = fmt.Sprintf("%s + (%s)", root, overlay)
	}
	body := root
	if expr != "" {
		body = fmt.Sprintf("local Z____root__Z_ = %s; if Z____std__Z_.isObject(Z____root__Z_) then (Z____root__Z_ { Z____expr__Z_:: %s }).Z____expr__Z_ "+
			"else error '-x and -k need the input to evaluate to an object, not ' + Z____std__Z_.type(Z____root__Z_)", root, expr)
	}
	if len(tla) > 0 {
		body = fmt.Sprintf("function(%s) %s", strings.Join(tla, ","), body)
	}
	// std is bound before a top-level argument can shadow it. Kubecfg is predefined in the whole
	// code, so that a top-level argument named kubecfg declares it; files are imported, so they
	// don't see it.
	return yakl.PredefineKubecfg("local Z____std__Z_ = std; " + body)
}
