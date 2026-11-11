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
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	jsonnet "github.com/google/go-jsonnet"
	"github.com/kubecfg/kubecfg/pkg/features"
	"github.com/kubecfg/kubecfg/pkg/kubecfg"
	"github.com/kubecfg/kubecfg/pkg/yakl"
	"github.com/kubecfg/kubecfg/utils"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/kustomize/api/ifc"
	"sigs.k8s.io/kustomize/api/konfig"
	"sigs.k8s.io/kustomize/api/kv"
	"sigs.k8s.io/kustomize/api/provider"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/resid"
	kyaml "sigs.k8s.io/kustomize/kyaml/yaml"
	k8syaml "sigs.k8s.io/yaml"
)

const (
	flagExtend    = "extend"
	flagDelete    = "delete"
	flagKustomize = "kustomize"
	flagOutDir    = "out-dir"
)

func init() {
	RootCmd.AddCommand(knitCmd)
	knitCmd.AddCommand(knitImportCmd)
	knitImportCmd.Flags().String(flagExtend, "", "Make the file an overlay of this yakl file: objects it has get ^^+: and its names")
	knitImportCmd.Flags().Bool(flagDelete, false, "With --extend, the file has every object in full: what the base has and the file doesn't is taken out")
	knitImportCmd.Flags().StringP(flagKustomize, "k", "", "Import the kustomization in this directory: its resources, and its strategic merge patches as ^^+: documents")
	knitImportCmd.Flags().String(flagOutDir, "", "With -k, write a yakl file per kustomization in this directory, instead of one file with all of them")
}

var knitCmd = &cobra.Command{
	Use:   "knit",
	Short: "Tools for knit, the library that merges Kubernetes manifests in layers",
}

var knitImportCmd = &cobra.Command{
	Use:   "import <file.yaml> | -k <dir>",
	Short: "Print a YAML file as a yakl file on knit, with a name for each object",
	Long: `Print a YAML file of Kubernetes objects as a yakl file that extends knit. Each object gets
a name, ^^: <name>.<Kind>, so overlays can change it as ^^+: <name>.<Kind>. Objects that differ
only by namespace get the namespace in front: ^^: <namespace>.<name>.<Kind>. A name that would
start with one of knit's fields, like registry, gets a _ in front: ^^: _registry.Service. knit
needs a name on every object, so a document without a kind or metadata.name, like a List, is an
error. The rest of the file is kept as it is.

With --extend base.yakl, the file is an overlay of base.yakl instead. An object the base has
gets ^^+: with the base's name, so the document patches it, with only the fields that change
what the base renders. Other objects are new, and get ^^: as above.

The file is read as patches: what it leaves out stays as the base has it. With --delete, it has
every object in full instead, as kubectl get or a rendered chart gives them: a field or list entry
the base has and the file doesn't gets deleted, and an object it doesn't have is left out with
^^+:: and its name.

With -k dir, it imports the kustomization in dir instead, as one file: the objects of its
resources, a directory's (another kustomization's) first, and then a ^^+: document for each
object a strategic merge patch patches, with the patch. A patch with a target gets one document
per object the target selects. A patch with $patch: delete hides the object. A component's
resources and patches come after the resources of the kustomization that lists it, and before its
patches. Each patch is a ^^+: document of its own, in order.

A configMapGenerator or secretGenerator makes a ConfigMap or Secret, after the resources,
annotated kustomize.config.k8s.io/needs-hash: "true": knit puts a hash of its data after its name,
as kustomize does. A reference to it, like configMapRef: {name: web-config}, reads that name:
${$['web-config'].ConfigMap.metadata.name}. With behavior: merge or replace, it patches the
ConfigMap or Secret instead.

A JSON 6902 patch is a ^^+: document for each operation, of two kinds: add at the end of a list
(/-), which appends to it, and remove, which deletes a field or an entry of a list with keys, like
a container. An index names the entry by its key.

With -k dir --out-dir out, it writes a file per kustomization under out instead, in a tree like
the kustomizations': -k overlays/prod writes out/base.yakl and out/overlays/prod.yakl, which
extends out/base.yakl, its one directory resource. A component is a file of its own too, which the
file of a kustomization that lists it adds after its base:
<<+: ${(import 'yakl:../base.yakl') + import 'yakl:../components/tracing.yakl'}. A component
that patches an object of the kustomization that lists it, which comes after it in that file,
goes in that file, with the kustomization's other components. It prints the files it writes.

Only resources, bases, components, patches, patchesStrategicMerge, configMapGenerator,
secretGenerator and generatorOptions are read so far: anything else in a kustomization, and
another JSON 6902 operation, is an error.`,
	Args: cobra.MaximumNArgs(1),
	// It evaluates the base of --extend or of a kustomization, so it takes evaluation flags from the rc file.
	Annotations: map[string]string{evalCmdAnno: ""},
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := errors.Join(gates.Require(features.Knit), gates.Require(features.YAKL)); err != nil {
			return err
		}
		path, err := cmd.Flags().GetString(flagExtend)
		if err != nil {
			return err
		}
		del, err := cmd.Flags().GetBool(flagDelete)
		if err != nil {
			return err
		}
		dir, err := cmd.Flags().GetString(flagKustomize)
		if err != nil {
			return err
		}
		outDir, err := cmd.Flags().GetString(flagOutDir)
		if err != nil {
			return err
		}
		switch {
		case (dir == "") == (len(args) == 0):
			return fmt.Errorf("give a YAML file or -k and a kustomization directory")
		case del && path == "":
			return fmt.Errorf("--%s needs --%s", flagDelete, flagExtend)
		case dir != "" && path != "":
			return fmt.Errorf("--%s doesn't go with -k: with --%s, a kustomization extends the file of its base", flagExtend, flagOutDir)
		case dir == "" && outDir != "":
			return fmt.Errorf("--%s needs -k", flagOutDir)
		}
		// Errors in a kustomization tree name the file they're about.
		if dir != "" && outDir != "" {
			files, err := knitImportTree(cmd, dir, outDir)
			if err != nil {
				return err
			}
			for _, f := range files {
				fmt.Fprintln(cmd.OutOrStdout(), f)
			}
			return nil
		}
		var out string
		if dir != "" {
			var k knitKustomization
			if _, err := k.read(dir, false, nil); err != nil {
				return err
			}
			if out, err = knitImport(k.srcs, k.patches, nil); err != nil {
				return err
			}
		} else {
			file := args[0]
			src, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			var base *knitBase
			if path != "" {
				if base, err = newKnitBase(cmd, file, []string{path}, 0, del); err != nil {
					return err
				}
			}
			if out, err = knitImport([]knitSource{{text: string(src)}}, nil, base); err != nil {
				return fmt.Errorf("%s: %w", file, err)
			}
		}
		fmt.Fprint(cmd.OutOrStdout(), out)
		return nil
	},
}

const knitHeader = "<<+: ${kubecfg.knit}\n"

// knitFields are the fields of knit, which a file that extends it shares. A
// generated name starting with one gets a _ in front: knit rejects ^^: registry.Service, which
// would be a field of its registry. Kubernetes names never start with _.
var knitFields = map[string]bool{
	"append": true, "delete": true, "groupKind": true, "groupVersionKind": true, "id": true,
	"mixin": true, "openapi": true, "registry": true, "render": true, "replace": true,
}

// A knitBase is the yakl file an overlay extends.
type knitBase struct {
	header string // the overlay's first document, which extends it
	del    bool   // the overlay has every object in full: it deletes what it leaves out
	// lookup tells what the base knows of objs, whose heads (apiVersion, kind, metadata) are heads.
	lookup func(heads, objs []any) (*knitLookup, error)
	// The objects a kustomize component's file comes before, in the file that adds it: those of
	// the kustomization that lists it. Its patches can't patch them.
	late []map[string]any
}

type knitLookup struct {
	Names map[string]string `json:"names"` // the base's object names, by the object's identity
	All   []string          `json:"all"`   // every name in the base, objects or not
	IDs   []string          `json:"ids"`   // the identities of the objects looked up
	// The base's objects as it renders them, by name, for the patches of a kustomization.
	Objects map[string]map[string]any `json:"objects"`
	// What of each object changes the base's: true for all of it, false for none, or a map (a
	// list) of the same for each field (entry).
	Changes []any `json:"changes"`
}

// newKnitBase evaluates the files of paths, each added to the ones before it with +, for an
// overlay that will sit where file is; "" is knit itself. The overlay extends the files from
// from on: a kustomize component's file extends only its own components, but what its patches
// patch is in the files before it.
func newKnitBase(cmd *cobra.Command, file string, paths []string, from int, del bool) (*knitBase, error) {
	dir, err := filepath.Abs(filepath.Dir(file))
	if err != nil {
		return nil, err
	}
	var header, code []string
	for i, path := range paths {
		if path == "" {
			header, code = append(header, "kubecfg.knit"), append(code, "import 'knit/knit.libsonnet'")
			continue
		}
		to, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(dir, to)
		if err != nil {
			return nil, err
		}
		u, err := utils.PathToURL(path)
		if err != nil {
			return nil, err
		}
		// Imports are URLs: #, ? and % in a file name are escaped, and ./ goes before a : in it.
		if i >= from {
			header = append(header, "import "+quoteJsonnet("yakl:"+(&url.URL{Path: filepath.ToSlash(rel)}).String()))
		}
		code = append(code, fmt.Sprintf("import %q", utils.ImportPath(u)))
	}
	b := &knitBase{del: del}
	if len(header) == 0 {
		b.header = "<<+: {}\n"
	} else {
		// An import reads everything after it as its path, so one with a + after it needs
		// parentheses: (import 'a') + import 'b'.
		for i, h := range header[:len(header)-1] {
			if strings.HasPrefix(h, "import ") {
				header[i] = "(" + h + ")"
			}
		}
		b.header = "<<+: ${" + strings.Join(header, " + ") + "}\n"
	}
	if from > 0 {
		b.header = "# A kustomize component. A file adds it after what it extends: <<+: ${(import 'base') + import 'this'}\n" + b.header
	}
	vm, err := JsonnetVM(cmd)
	if err != nil {
		return nil, err
	}
	b.lookup = func(heads, objs []any) (*knitLookup, error) {
		js, err := json.Marshal(heads)
		if err != nil {
			return nil, err
		}
		jo, err := json.Marshal(objs)
		if err != nil {
			return nil, err
		}
		out, err := vm.EvaluateAnonymousSnippet("knit-import", fmt.Sprintf(`
local b = (%s);
assert std.objectHasAll(b, '$objects') : %s;
local objs = b['$objects'], names = { [b['$id'](objs[n])]: n for n in std.objectFields(objs) };
{
  names: names,
  // The names of the objects, and of the other documents: the fields that hold no object.
  all: std.objectFields(objs) + [
    f
    for f in std.objectFieldsAll(b)
    if !std.objectHasAll(import 'knit/knit.libsonnet', f) && [n for n in std.objectFields(objs) if std.startsWith(n + '.', f + '.')] == []
  ],
  objects: objs,
  ids: [b['$id'](o) for o in %s],
  changes: [local id = b['$id'](o); if id in names then b['$changes'](o, objs[names[id]], %t) else true for o in %s],
}`, strings.Join(code, ") + ("), quoteJsonnet(strings.Join(paths, " + ")+" is not a yakl file on knit"), js, del, jo))
		if err != nil {
			return nil, err
		}
		var l knitLookup
		return &l, json.Unmarshal([]byte(out), &l)
	}
	return b, nil
}

// A knitKustomization is what knit import -k reads of a kustomization tree, in order.
type knitKustomization struct {
	srcs    []knitSource
	patches []knitPatch
	reading []string // the kustomizations being read, which can't be their own resources
	// With split, the components the kustomization lists become files of their own: read lists
	// them in comps instead of reading them. Those of a directory resource it reads stay in.
	split bool
	comps []string
}

// A knitScope is what the patches of a kustomization patch: its sources, from from on, and with
// base the objects of its base.
type knitScope struct {
	from int
	base bool
}

// read reads the kustomization in dir: the text of its resources, a directory's resources and
// patches first, then its components, and its strategic merge patches. With extend, a
// kustomization with one directory resource extends it instead: read returns it as base, whose
// objects the patches can patch too. A component of the kustomization of scope in adds to it, as
// kustomize does: its patches patch what that one has so far, its base's objects too.
func (k *knitKustomization) read(dir string, extend bool, in *knitScope) (base string, err error) {
	if slices.Contains(k.reading, filepath.Clean(dir)) {
		return "", fmt.Errorf("%s: a kustomization that is its own resource", dir)
	}
	k.reading = append(k.reading, filepath.Clean(dir))
	defer func() { k.reading = k.reading[:len(k.reading)-1] }()
	var file string
	var raw []byte
	for _, n := range konfig.RecognizedKustomizationFileNames() {
		var err error
		if raw, err = os.ReadFile(filepath.Join(dir, n)); err == nil {
			file = filepath.Join(dir, n)
			break
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
	}
	if file == "" {
		return "", fmt.Errorf("%s: no kustomization.yaml", dir)
	}
	var fields map[string]any
	if err := k8syaml.Unmarshal(raw, &fields); err != nil {
		return "", fmt.Errorf("%s: %w", file, err)
	}
	for _, f := range slices.Sorted(maps.Keys(fields)) {
		switch f {
		case "apiVersion", "kind", "metadata", "resources", "bases", "components", "patches", "patchesStrategicMerge",
			"configMapGenerator", "secretGenerator", "generatorOptions":
		default:
			return "", fmt.Errorf("%s: knit import doesn't read %s yet", file, f)
		}
	}
	var kust types.Kustomization
	if err := k8syaml.UnmarshalStrict(raw, &kust); err != nil {
		return "", fmt.Errorf("%s: %w", file, err)
	}
	switch comp := kust.Kind == types.ComponentKind; {
	case comp && in == nil:
		return "", fmt.Errorf("%s: a component, which only components can list", file)
	case !comp && in != nil:
		return "", fmt.Errorf("%s: components lists it, but it isn't a component (kind: Component)", file)
	}
	kust.FixKustomization() // bases are resources
	var dirs []string
	for _, r := range kust.Resources {
		path := filepath.Join(dir, r)
		info, err := os.Stat(path)
		if err != nil {
			return "", fmt.Errorf("%s: resource %s: %w", file, r, err)
		}
		if info.IsDir() {
			dirs = append(dirs, path)
		}
	}
	if extend && len(dirs) == 1 {
		base = dirs[0]
	}
	split := k.split && len(k.reading) == 1
	// The files of split components come before the kustomization's, after its base.
	scope := knitScope{from: len(k.srcs), base: base != "" || split && len(kust.Components) > 0}
	if in != nil {
		scope = *in
	}
	for _, r := range kust.Resources {
		switch path := filepath.Join(dir, r); {
		case path == base:
		case slices.Contains(dirs, path):
			if _, err := k.read(path, false, nil); err != nil {
				return "", err
			}
		default:
			text, err := os.ReadFile(path)
			if err != nil {
				return "", err
			}
			k.srcs = append(k.srcs, knitSource{path, string(text)})
		}
	}
	// kustomize runs generators after reading resources, before components.
	for _, g := range kust.ConfigMapGenerator {
		if err := k.generate(file, dir, "ConfigMap", g.GeneratorArgs, "", kust.GeneratorOptions, scope); err != nil {
			return "", err
		}
	}
	for _, g := range kust.SecretGenerator {
		if err := k.generate(file, dir, "Secret", g.GeneratorArgs, cmp.Or(g.Type, "Opaque"), kust.GeneratorOptions, scope); err != nil {
			return "", err
		}
	}
	for _, c := range kust.Components {
		if split {
			k.comps = append(k.comps, filepath.Join(dir, c))
		} else if _, err := k.read(filepath.Join(dir, c), false, &scope); err != nil {
			return "", err
		}
	}
	// kustomize applies patchesStrategicMerge first. Each is a file, or else a patch.
	var patches []types.Patch
	for _, p := range kust.PatchesStrategicMerge {
		if _, err := os.Stat(filepath.Join(dir, string(p))); err == nil {
			patches = append(patches, types.Patch{Path: string(p)})
		} else {
			patches = append(patches, types.Patch{Patch: string(p)})
		}
	}
	for i, p := range append(patches, kust.Patches...) {
		if p.Options != nil && (p.Options.AllowNameChange || p.Options.AllowKindChange) {
			return "", fmt.Errorf("%s: patch %d: knit can't change an object's name or kind", file, i+1)
		}
		name, text := fmt.Sprintf("%s: patch %d", file, i+1), p.Patch
		if p.Path != "" {
			name = filepath.Join(dir, p.Path)
			raw, err := os.ReadFile(name)
			if err != nil {
				return "", err
			}
			text = string(raw)
		}
		dec := yaml.NewDecoder(strings.NewReader(text))
		for {
			var doc yaml.Node
			if err := dec.Decode(&doc); err == io.EOF {
				break
			} else if err != nil {
				return "", fmt.Errorf("%s: %w", name, err)
			}
			if len(doc.Content) == 0 || doc.Content[0].Tag == "!!null" {
				continue
			}
			if doc.Content[0].Kind == yaml.SequenceNode && p.Target == nil {
				return "", fmt.Errorf("%s: a JSON 6902 patch needs a target", name)
			}
			k.patches = append(k.patches, knitPatch{file: name, doc: &doc, target: p.Target, from: scope.from, to: len(k.srcs), base: scope.base})
		}
	}
	return base, nil
}

// knitNeedsHash is the annotation that puts kustomize's hash of an object's data after its name,
// in knit as in what kustomize's generator plugins make.
const knitNeedsHash = "kustomize.config.k8s.io/needs-hash"

// generate adds what the configMapGenerator or secretGenerator g of the kustomization file in dir
// makes: a ConfigMap or Secret (kind) after the resources, with the needs-hash annotation unless
// its options disable the hash, or with behavior merge or replace, a patch of the one the
// kustomization has so far. A Secret has secretType, and its data in base64.
func (k *knitKustomization) generate(file, dir, kind string, g types.GeneratorArgs, secretType string, global *types.GeneratorOptions, scope knitScope) error {
	gen := strings.ToLower(kind[:1]) + kind[1:] + "Generator"
	at := fmt.Sprintf("%s: %s %s", file, gen, g.Name)
	// Files are read as kustomize reads them: from the kustomization's directory, and not above it.
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	kvl := kv.NewLoader(knitFiles{root: root}, provider.NewDefaultDepProvider().GetFieldValidator())
	pairs, err := kvl.Load(g.KvPairSources)
	if err != nil {
		return fmt.Errorf("%s: %w", at, err)
	}
	data := map[string]string{}
	for _, p := range pairs {
		if err := kvl.Validator().ErrIfInvalidKey(p.Key); err != nil {
			return fmt.Errorf("%s: %w", at, err)
		}
		if _, ok := data[p.Key]; ok {
			return fmt.Errorf("%s: the key %s comes twice", at, p.Key)
		}
		data[p.Key] = p.Value
	}
	if g.Name == "" {
		return fmt.Errorf("%s: a %s needs a name", file, gen)
	}
	rn, err := kyaml.Parse("apiVersion: v1\nkind: " + kind + "\n")
	if err != nil {
		return err
	}
	if _, err := rn.Pipe(kyaml.SetK8sName(g.Name)); err != nil {
		return err
	}
	if g.Namespace != "" {
		if _, err := rn.Pipe(kyaml.SetK8sNamespace(g.Namespace)); err != nil {
			return err
		}
	}
	opts := types.MergeGlobalOptionsIntoLocal(g.Options, global)
	if opts == nil {
		opts = &types.GeneratorOptions{}
	}
	for _, l := range slices.Sorted(maps.Keys(opts.Labels)) {
		if _, err := rn.Pipe(kyaml.SetLabel(l, opts.Labels[l])); err != nil {
			return err
		}
	}
	for _, a := range slices.Sorted(maps.Keys(opts.Annotations)) {
		if _, err := rn.Pipe(kyaml.SetAnnotation(a, opts.Annotations[a])); err != nil {
			return err
		}
	}
	behavior := types.NewGenerationBehavior(g.Behavior)
	create := behavior == types.BehaviorUnspecified || behavior == types.BehaviorCreate
	if create && !opts.DisableNameSuffixHash {
		if _, err := rn.Pipe(kyaml.SetAnnotation(knitNeedsHash, "true")); err != nil {
			return err
		}
	}
	if kind == "Secret" {
		if err := rn.PipeE(kyaml.SetField("type", kyaml.NewStringRNode(secretType))); err != nil {
			return err
		}
		if err := rn.LoadMapIntoSecretData(data); err != nil {
			return err
		}
	} else if err := rn.LoadMapIntoConfigMapData(data); err != nil {
		return err
	}
	if opts.Immutable {
		if err := rn.PipeE(kyaml.SetField("immutable", kyaml.NewRNode(&kyaml.Node{Kind: kyaml.ScalarNode, Tag: kyaml.NodeTagBool, Value: "true"}))); err != nil {
			return err
		}
	}
	// kyaml quotes the labels and annotations it sets; YAML needs quotes only on some.
	var plainStyle func(n *yaml.Node)
	plainStyle = func(n *yaml.Node) {
		if n.Style == yaml.SingleQuotedStyle {
			n.Style = 0
		}
		for _, c := range n.Content {
			plainStyle(c)
		}
	}
	plainStyle(rn.YNode())
	if create {
		text, err := rn.String()
		if err != nil {
			return err
		}
		k.srcs = append(k.srcs, knitSource{at, "# Made by " + gen + ". knit puts a hash of the data after its name.\n" + text})
		return nil
	}
	// replace drops the data the ConfigMap had; merge keeps what this doesn't set.
	if behavior == types.BehaviorReplace {
		d, err := rn.Pipe(kyaml.LookupCreate(kyaml.MappingNode, kyaml.DataField))
		if err != nil {
			return err
		}
		patch := []*yaml.Node{{Kind: yaml.ScalarNode, Value: "$patch"}, {Kind: yaml.ScalarNode, Value: "replace"}}
		d.YNode().Content = append(patch, d.YNode().Content...)
	}
	doc := &yaml.Node{Kind: yaml.DocumentNode, HeadComment: "# " + gen + " " + g.Name + ", behavior: " + g.Behavior, Content: []*yaml.Node{rn.YNode()}}
	k.patches = append(k.patches, knitPatch{file: at, doc: doc, from: scope.from, to: len(k.srcs), base: scope.base})
	return nil
}

// knitFiles reads the files of a generator. kv's loader only calls Load.
type knitFiles struct {
	ifc.Loader
	root *os.Root
}

func (l knitFiles) Load(path string) ([]byte, error) { return l.root.ReadFile(path) }

// errKnitLate is the error of a kustomize component's patch of an object of the kustomization
// that lists it, whose file adds the component before those objects.
var errKnitLate = errors.New("it patches an object of the kustomization that lists it, which the component's file can't reach")

// knitImportTree writes the kustomization in dir as yakl files under out, one per kustomization
// and component, in a tree like theirs: a kustomization with one directory resource extends that
// one's file, and its components' files after it. It returns the files, each after those it
// extends.
func knitImportTree(cmd *cobra.Command, dir, out string) ([]string, error) {
	type step struct {
		dir string
		k   knitKustomization
	}
	var chain []step
	for d := dir; d != ""; {
		if slices.ContainsFunc(chain, func(s step) bool { return s.dir == d }) {
			return nil, fmt.Errorf("%s: a kustomization that is its own resource", d)
		}
		s := step{dir: d, k: knitKustomization{split: true}}
		base, err := s.k.read(d, true, nil)
		if err != nil {
			return nil, err
		}
		chain, d = append(chain, s), base
	}
	// The tree starts at the directory that holds them all: overlays/prod and base give
	// overlays/prod.yakl and base.yakl.
	var dirs []string
	for _, s := range chain {
		dirs = append(dirs, s.dir)
	}
	for _, s := range chain {
		for _, c := range s.k.comps {
			if !slices.Contains(dirs, c) {
				dirs = append(dirs, c)
			}
		}
	}
	for i := len(chain); i < len(dirs); i++ {
		k := knitKustomization{split: true}
		if _, err := k.read(dirs[i], false, &knitScope{base: true}); err != nil {
			return nil, err
		}
		for _, c := range k.comps {
			if c == dirs[i] {
				return nil, fmt.Errorf("%s: a component that is its own component", c)
			} else if !slices.Contains(dirs, c) {
				dirs = append(dirs, c)
			}
		}
	}
	root, err := filepath.Abs(filepath.Dir(dir))
	if err != nil {
		return nil, err
	}
	for _, s := range dirs {
		d, err := filepath.Abs(filepath.Dir(s))
		if err != nil {
			return nil, err
		}
		for rel, err := filepath.Rel(root, d); err != nil || strings.HasPrefix(rel, ".."); rel, err = filepath.Rel(root, d) {
			root = filepath.Dir(root)
		}
	}
	var files []string
	written := map[string]string{}
	// write writes the file of the kustomization or component (comp) in dir, which comes after
	// the files of ctx, and before the objects late of the kustomization that lists a component.
	// A component that patches one of those stays in that kustomization's file.
	var write func(dir string, k knitKustomization, ctx []string, comp bool, late []map[string]any) (string, error)
	write = func(dir string, k knitKustomization, ctx []string, comp bool, late []map[string]any) (string, error) {
		d, err := filepath.Abs(dir)
		if err != nil {
			return "", err
		}
		rel, err := filepath.Rel(root, d)
		if err != nil {
			return "", err
		}
		file := filepath.Join(out, rel+".yakl")
		own, err := knitObjects(k.srcs)
		if err != nil {
			return "", err
		}
		paths := ctx
		for _, c := range k.comps {
			ck := knitKustomization{split: true}
			if _, err := ck.read(c, false, &knitScope{base: true}); err != nil {
				return "", err
			}
			f, err := write(c, ck, paths, true, append(slices.Clone(late), own...))
			if errors.Is(err, errKnitLate) {
				fmt.Fprintf(cmd.ErrOrStderr(), "%s: its components (%s) go in its file, not files of their own: %v\n", dir, strings.Join(k.comps, ", "), err)
				k.split, k.comps, k.srcs, k.patches = false, nil, nil, nil
				var in *knitScope
				if comp {
					in = &knitScope{base: true}
				}
				if _, err := k.read(dir, !comp, in); err != nil {
					return "", err
				}
				paths = ctx
				break
			} else if err != nil {
				return "", err
			}
			paths = append(slices.Clone(paths), f)
		}
		var base *knitBase
		if comp || len(paths) > 1 || paths[0] != "" {
			from := 0
			if comp {
				from = len(ctx)
			}
			if base, err = newKnitBase(cmd, file, paths, from, false); err != nil {
				return "", err
			}
			base.late = late
		}
		src, err := knitImport(k.srcs, k.patches, base)
		if err != nil {
			return "", err
		}
		if w, ok := written[file]; ok && w != src {
			return "", fmt.Errorf("%s: the component's file would differ for the kustomizations that list it, as its patches select other objects", dir)
		} else if !ok {
			if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
				return "", err
			}
			if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
				return "", err
			}
			written[file] = src
			files = append(files, file)
		}
		return file, nil
	}
	prev := ""
	for i := len(chain) - 1; i >= 0; i-- {
		if prev, err = write(chain[i].dir, chain[i].k, []string{prev}, false, nil); err != nil {
			return nil, err
		}
	}
	return files, nil
}

// knitObjects returns the objects of srcs.
func knitObjects(srcs []knitSource) ([]map[string]any, error) {
	var objs []map[string]any
	for _, s := range srcs {
		dec := yaml.NewDecoder(strings.NewReader(s.text))
		for {
			var doc yaml.Node
			if err := dec.Decode(&doc); err == io.EOF {
				break
			} else if err != nil {
				return nil, fmt.Errorf("%s: %w", s.file, err)
			}
			if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
				continue
			}
			o, err := nodeMap(doc.Content[0])
			if err != nil {
				return nil, fmt.Errorf("%s: %w", s.file, err)
			}
			objs = append(objs, o)
		}
	}
	return objs, nil
}

// stripLastBlock returns src, which doesn't end in a line break, with the block scalar it ends in,
// if any, marked to strip its final line break (|-): the value has none, and with the line break
// the next document needs, | and |+ would read one in.
func stripLastBlock(src string) string {
	var last *yaml.Node
	for dec := yaml.NewDecoder(strings.NewReader(src)); ; {
		var doc yaml.Node
		if dec.Decode(&doc) != nil {
			break
		}
		last = &doc
	}
	for last != nil && len(last.Content) > 0 {
		last = last.Content[len(last.Content)-1]
	}
	if last == nil || last.Style&(yaml.LiteralStyle|yaml.FoldedStyle) == 0 || strings.HasSuffix(last.Value, "\n") {
		return src
	}
	lines := strings.SplitAfter(src, "\n")
	// The node's column, from 1, is the indicator's, in characters.
	l := []rune(lines[last.Line-1])
	i := last.Column
	for i < len(l) && (l[i] >= '0' && l[i] <= '9' || l[i] == '+') {
		if l[i] == '+' {
			l = slices.Delete(l, i, i+1)
		} else {
			i++
		}
	}
	lines[last.Line-1] = string(slices.Insert(l, i, '-'))
	return strings.Join(lines, "")
}

// quoteJsonnet returns s as a single-quoted Jsonnet string, which reads well in YAML.
func quoteJsonnet(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, "'", `\'`).Replace(s) + "'"
}

// A knitSource is the text of a YAML file, which errors name by file unless it's "".
type knitSource struct{ file, text string }

// A knitPatch is a strategic merge patch of a kustomization: of the objects of sources[from:to]
// and, with base, of the base's. It patches the objects target selects, or else the object of its
// own identity.
type knitPatch struct {
	file     string
	doc      *yaml.Node
	target   *types.Selector
	from, to int
	base     bool
}

// knitImport returns the YAML of srcs as one yakl file on knit, or as an overlay of base if it
// isn't nil: a first document that extends knit or base, a ^^ key before the first key of each
// object, and then a ^^+: document for each object each patch patches.
func knitImport(srcs []knitSource, patches []knitPatch, base *knitBase) (string, error) {
	type object struct {
		head, obj              map[string]any
		root                   *yaml.Node
		name, ns, kind, indent string
		line                   int    // the line of its first key, from 0
		key, full              string // ^^ or ^^+, and the name it gets
		changes                any    // what of it changes the base's, if it's written anew
		hide                   bool   // a patch deletes it: ^^:: hides it
	}
	var objects []*object
	var src string
	var starts []int // the line of src each source starts at, from 0
	for _, s := range srcs {
		// YAML also breaks lines at a lone \r, as yakl does; the output has \n only.
		t := strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(s.text)
		if t != "" && !strings.HasSuffix(t, "\n") {
			t = stripLastBlock(t) + "\n"
		}
		if src != "" && !strings.HasPrefix(strings.TrimLeft(withoutComments(t), " \t\n"), "---") {
			src += "---\n"
		}
		starts = append(starts, strings.Count(src, "\n"))
		src += t
	}
	// srcOf returns the source that line l of src, from 1, is in, and at where the line is.
	srcOf := func(l int) int {
		i := len(starts) - 1
		for i > 0 && l-1 < starts[i] {
			i--
		}
		return i
	}
	at := func(l int) string {
		i := srcOf(l)
		if srcs[i].file == "" {
			return fmt.Sprintf("line %d", l-starts[i])
		}
		return fmt.Sprintf("%s: line %d", srcs[i].file, l-starts[i])
	}
	// yaml.v3 breaks lines at these too, and yakl rejects them.
	if i := strings.IndexAny(src, "\u0085\u2028\u2029"); i >= 0 {
		return "", fmt.Errorf("%s: unsupported Unicode line separator", at(strings.Count(src[:i], "\n")+1))
	}
	lines := strings.SplitAfter(src, "\n")
	dec := yaml.NewDecoder(strings.NewReader(src))
	for {
		var doc yaml.Node
		if err := dec.Decode(&doc); err == io.EOF {
			break
		} else if err != nil {
			return "", err
		}
		if len(doc.Content) == 0 {
			continue
		}
		root := doc.Content[0]
		if root.Tag == "!!null" {
			// yakl skips an empty document, as kubectl does, but not one that says null.
			if root.Value == "" && root.Style == 0 && root.Anchor == "" {
				continue
			}
			return "", fmt.Errorf("%s: a null document: delete it, since it holds no object", at(root.Line))
		}
		if err := checkLiteral(root, at); err != nil {
			return "", err
		}
		obj, err := nodeMap(root)
		if err != nil {
			return "", fmt.Errorf("%s: %w", at(root.Line), err)
		}
		md, _ := obj["metadata"].(map[string]any)
		apiVersion, _ := obj["apiVersion"].(string)
		kind, _ := obj["kind"].(string)
		name, _ := md["name"].(string)
		ns, hasNS := md["namespace"].(string)
		switch {
		case kind == "List" && (apiVersion == "v1" || apiVersion == ""):
			return "", fmt.Errorf("%s: a List: write each of its items as a document of its own, for each to get a name", at(root.Line))
		case kind == "" || name == "":
			return "", fmt.Errorf("%s: a document without a kind and a metadata.name can't get a name, and knit needs one on every object", at(root.Line))
		case obj["$patch"] != nil:
			return "", fmt.Errorf("%s: %s %s has $patch: to leave out an object of the base, write a document of only ^^+:: and the base's name for it", at(root.Line), kind, name)
		}
		first := root.Content[0]
		indent := lines[first.Line-1][:first.Column-1]
		if root.Style&yaml.FlowStyle != 0 || strings.TrimSpace(indent) != "" {
			return "", fmt.Errorf("%s: %s %s needs its first key at the start of a line, for its name to go above it", at(first.Line), kind, name)
		}
		head := map[string]any{"apiVersion": apiVersion, "kind": kind, "metadata": map[string]any{"name": name}}
		// An explicit namespace: '' is the object's own: the base's defaults don't apply to it.
		if hasNS {
			head["metadata"].(map[string]any)["namespace"] = ns
		}
		objects = append(objects, &object{head: head, obj: obj, root: root, name: name, ns: ns, kind: kind, indent: indent, line: first.Line - 1, key: "^^"})
	}
	header, taken := knitHeader, []string(nil) // taken: the base's names
	var hide []string                          // the base's objects the file leaves out, with --delete
	var baseObjects []knitObject               // for patches
	if base != nil {
		heads, objs := make([]any, len(objects)), make([]any, len(objects))
		for i, o := range objects {
			heads[i], objs[i] = o.head, o.obj
		}
		l, err := base.lookup(heads, objs)
		if err != nil {
			return "", err
		}
		for i, o := range objects {
			if n, ok := l.Names[l.IDs[i]]; ok {
				o.key, o.full = "^^+", n
				// ponytail: with anchors or merge keys, the object is kept whole.
				if plain(o.root) {
					o.changes = l.Changes[i]
				}
			}
		}
		header, taken = base.header, l.All
		for _, n := range slices.Sorted(maps.Keys(l.Objects)) {
			baseObjects = append(baseObjects, knitObject{name: n, obj: l.Objects[n]})
		}
		if base.del {
			for id, n := range l.Names {
				if !slices.Contains(l.IDs, id) {
					hide = append(hide, n)
				}
			}
			slices.Sort(hide)
		}
	}
	count := map[string]int{} // new objects by name.Kind
	for _, o := range objects {
		if o.key == "^^" {
			count[o.name+"."+o.kind]++
		}
	}
	byLine := map[int]*object{}
	named := map[string]bool{}
	for _, o := range objects {
		byLine[o.line] = o
		if o.key != "^^" {
			continue
		}
		o.full = o.name + "." + o.kind
		if count[o.full] > 1 && o.ns != "" {
			o.full = o.ns + "." + o.full
		}
		if first, _, _ := strings.Cut(o.full, "."); knitFields[first] {
			o.full = "_" + o.full
		}
		if named[o.full] {
			return "", fmt.Errorf("two objects would be named %s: give them different names, or namespaces", o.full)
		}
		named[o.full] = true
		for _, t := range taken {
			if t == o.full || strings.HasPrefix(o.full, t+".") || strings.HasPrefix(t, o.full+".") {
				return "", fmt.Errorf("%s %s would be named %s, which clashes with ^^: %s in the base", o.kind, o.name, o.full, t)
			}
		}
	}
	// The patches' documents, after the objects'. They can hide a new object.
	patched := knitPatched{gone: map[string]bool{}}
	for _, p := range patches {
		var objs []knitObject
		if p.base {
			objs = append(objs, baseObjects...)
			for _, o := range base.late {
				objs = append(objs, knitObject{obj: o, late: true})
			}
		}
		for _, o := range objects {
			if s := srcOf(o.line + 1); o.key == "^^" && s >= p.from && s < p.to {
				objs = append(objs, knitObject{name: o.full, obj: o.obj, hide: &o.hide})
			}
		}
		if err := p.apply(objs, &patched); err != nil {
			return "", err
		}
	}
	// A reference to a ConfigMap or Secret that gets a hash after its name reads the name through
	// $, as kustomize rewrites references to what it generates.
	hashed := map[string]string{} // the names, by kind/metadata.name
	addHashed := func(name string, obj map[string]any) error {
		md, _ := obj["metadata"].(map[string]any)
		a, _ := md["annotations"].(map[string]any)
		n, _ := md["name"].(string)
		kind, _ := obj["kind"].(string)
		if kind != "ConfigMap" && kind != "Secret" || a[knitNeedsHash] != "true" {
			return nil
		}
		if h, ok := hashed[kind+"/"+n]; ok && h != name {
			return fmt.Errorf("%s and %s are both %ss %s with a hash after the name: knit import can't tell which one a reference means", h, name, kind, n)
		}
		hashed[kind+"/"+n] = name
		return nil
	}
	for _, o := range baseObjects {
		if err := addHashed(o.name, o.obj); err != nil {
			return "", err
		}
	}
	own := map[string]bool{}
	for _, o := range objects {
		if o.key == "^^" {
			if err := addHashed(o.full, o.obj); err != nil {
				return "", err
			}
			own[o.kind+"/"+o.name] = hashed[o.kind+"/"+o.name] == o.full
		}
	}
	for _, o := range baseObjects {
		if err := knitRefs(o.obj, func(kind, n string) error {
			if own[kind+"/"+n] {
				md, _ := o.obj["metadata"].(map[string]any)
				return fmt.Errorf("the base's %v %v refers to %s %s, which gets a hash after its name here: knit import can't change the base's reference yet; make the %s in the base, or disable its hash", o.obj["kind"], md["name"], kind, n, kind)
			}
			return nil
		}); err != nil {
			return "", err
		}
	}
	// A reference is written anew in the objects' text, rightmost first, and in their nodes,
	// which a patch of the base's object is written from.
	type edit struct {
		n    *yaml.Node
		text string
	}
	var edits []edit
	refNodes := func(root *yaml.Node, f func(n *yaml.Node, text string)) {
		knitRefNodes(root, func(kind string, n, parent *yaml.Node) {
			if h, ok := hashed[kind+"/"+n.Value]; ok {
				expr := knitRef(h)
				if parent.Style&yaml.FlowStyle != 0 {
					f(n, `"`+expr+`"`)
					n.Style = yaml.DoubleQuotedStyle
				} else {
					f(n, expr)
					n.Style = 0
				}
				n.Value = expr
			}
		})
	}
	for _, o := range objects {
		refNodes(o.root, func(n *yaml.Node, text string) {
			edits = append(edits, edit{&yaml.Node{Line: n.Line, Column: n.Column, Style: n.Style, Value: n.Value}, text})
		})
	}
	slices.SortFunc(edits, func(a, b edit) int { return cmp.Or(a.n.Line-b.n.Line, b.n.Column-a.n.Column) })
	for _, e := range edits {
		l := []rune(lines[e.n.Line-1])
		start := e.n.Column - 1
		lines[e.n.Line-1] = string(l[:start]) + e.text + string(l[start+scalarLen(l[start:], e.n):])
	}
	for _, d := range patched.docs {
		if d.body != nil {
			refNodes(d.body, func(*yaml.Node, string) {})
		}
	}
	var b strings.Builder
	b.WriteString(header)
	if src != "" && !strings.HasPrefix(strings.TrimLeft(withoutComments(src), " \t\n"), "---") {
		b.WriteString("---\n")
	}
	for i := 0; i < len(lines); i++ {
		o := byLine[i]
		if o == nil {
			b.WriteString(lines[i])
			continue
		}
		if o.changes == nil {
			key, _ := yaml.Marshal(map[string]string{o.key: o.full})
			if o.hide {
				key = append([]byte("^^:"), key[2:]...)
			}
			b.WriteString(o.indent + string(key) + lines[i])
			continue
		}
		// A patch of the base's object, with only what changes it, written anew from its first key
		// to the end of its document. The comments above its first key stay as they are.
		o.root.Content[0].HeadComment = ""
		if err := prune(o.root, o.changes); err != nil {
			return "", err
		}
		str := func(s string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s} }
		o.root.Content = append([]*yaml.Node{str(o.key), str(o.full)}, o.root.Content...)
		enc := yaml.NewEncoder(&b)
		enc.SetIndent(2)
		if err := errors.Join(enc.Encode(o.root), enc.Close()); err != nil {
			return "", err
		}
		for i+1 < len(lines) && !docMarker(lines[i+1]) {
			i++
		}
	}
	if !strings.HasSuffix(b.String(), "\n") {
		b.WriteString("\n")
	}
	str := func(s string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s} }
	for _, d := range patched.docs {
		b.WriteString("---\n")
		if d.body == nil {
			if d.comment != "" {
				b.WriteString(d.comment + "\n")
			}
			b.WriteString("^^+:: " + d.name + "\n")
			continue
		}
		key := str("^^+")
		key.HeadComment = d.comment
		enc := yaml.NewEncoder(&b)
		enc.SetIndent(2)
		doc := &yaml.Node{Kind: yaml.MappingNode, Content: append([]*yaml.Node{key, str(d.name)}, d.body.Content...)}
		if err := errors.Join(enc.Encode(doc), enc.Close()); err != nil {
			return "", err
		}
	}
	for _, n := range hide {
		b.WriteString("---\n^^+:: " + n + "\n")
	}
	out := b.String()
	if _, err := yakl.Compile(out); err != nil {
		return "", fmt.Errorf("the file as yakl: %w", err)
	}
	return out, nil
}

// A knitObject is an object of the file or its base, and its name. hide is nil for the base's.
type knitObject struct {
	name string
	obj  map[string]any
	hide *bool
	late bool // of the kustomization that lists the file's component: see errKnitLate
}

// knitPatched is what the patches of a file do: a ^^+: document for each object each patch
// patches, in order, and the objects they delete.
type knitPatched struct {
	docs []*knitPatchDoc
	gone map[string]bool
	// For JSON patches: the objects as the JSON patches before leave them, by name, and a VM that
	// asks knit how their lists merge.
	cur map[string]map[string]any
	vm  *jsonnet.VM
}

type knitPatchDoc struct {
	name, comment string
	body          *yaml.Node // the fields, or nil for a document of only ^^+::, which hides it
	json          bool       // from a JSON patch, which cur has
}

// apply adds what p does to the objects of objs it patches, without the identity a patch doesn't
// have to repeat. A patch that deletes an object hides it: a new one with ^^::, one of the base's
// with a document of only ^^+::.
func (p *knitPatch) apply(objs []knitObject, s *knitPatched) error {
	root := p.doc.Content[0]
	at := fmt.Sprintf("%s: line %d", p.file, root.Line)
	if root.Kind == yaml.SequenceNode {
		return p.applyJSON(objs, s, at)
	}
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("%s: a strategic merge patch is a map", at)
	}
	if err := checkLiteral(root, func(l int) string { return fmt.Sprintf("%s: line %d", p.file, l) }); err != nil {
		return err
	}
	obj, err := nodeMap(root)
	if err != nil {
		return fmt.Errorf("%s: %w", at, err)
	}
	// Like kustomize, a patch can't patch an object an earlier one deleted.
	targets, err := p.targets(obj, slices.DeleteFunc(slices.Clone(objs), func(o knitObject) bool { return !o.late && s.gone[o.name] }))
	if err != nil {
		return fmt.Errorf("%s: %w", at, err)
	}
	for _, o := range targets {
		if o.late {
			md, _ := o.obj["metadata"].(map[string]any)
			return fmt.Errorf("%s: %v %v: %w", at, o.obj["kind"], md["name"], errKnitLate)
		}
	}
	// The comments above the patch go above its documents, on the ^^+: key.
	comment := strings.TrimSpace(p.doc.HeadComment + "\n\n" + root.Content[0].HeadComment)
	root.Content[0].HeadComment = ""
	body := &yaml.Node{Kind: yaml.MappingNode}
	for i := 0; i < len(root.Content); i += 2 {
		k, v := root.Content[i], root.Content[i+1]
		switch {
		case k.Value == "apiVersion" || k.Value == "kind":
			continue
		case k.Value == "metadata" && v.Kind == yaml.MappingNode:
			md := *v
			md.Content = nil
			for j := 0; j < len(v.Content); j += 2 {
				if n := v.Content[j].Value; n != "name" && n != "namespace" {
					md.Content = append(md.Content, v.Content[j], v.Content[j+1])
				}
			}
			if len(md.Content) == 0 {
				continue
			}
			v = &md
		}
		body.Content = append(body.Content, k, v)
	}
	for _, o := range targets {
		switch {
		case obj["$patch"] != "delete":
			s.docs = append(s.docs, &knitPatchDoc{name: o.name, comment: comment, body: copyNode(body)})
		case o.hide != nil:
			s.gone[o.name] = true
			*o.hide = true
		default:
			// The base's object: its patches here go, since all its documents in a file share the
			// colons of the one that hides it.
			s.gone[o.name] = true
			s.docs = append(slices.DeleteFunc(s.docs, func(d *knitPatchDoc) bool { return d.name == o.name }),
				&knitPatchDoc{name: o.name, comment: comment})
		}
	}
	return nil
}

// knitRefFields are the fields that refer to a ConfigMap or Secret by name, as Kubernetes names
// them: the field, its field that holds the name, and the kind. A volume's secret has a
// secretName, a projected one a name; imagePullSecrets is a list of references.
//
// ponytail: by field name, not kustomize's list of paths for each kind, which also has an
// Ingress's tls secretName and the references of custom resources a kustomization configures.
var knitRefFields = []struct{ field, name, kind string }{
	{"configMap", "name", "ConfigMap"}, {"configMapRef", "name", "ConfigMap"}, {"configMapKeyRef", "name", "ConfigMap"},
	{"secret", "secretName", "Secret"}, {"secret", "name", "Secret"}, {"secretRef", "name", "Secret"},
	{"secretKeyRef", "name", "Secret"}, {"imagePullSecrets", "name", "Secret"},
}

// knitRefs calls f with the kind and name of each reference to a ConfigMap or Secret in v, wherever
// it is.
func knitRefs(v any, f func(kind, name string) error) error {
	switch v := v.(type) {
	case map[string]any:
		for k, x := range v {
			refs := []any{x}
			if xs, ok := x.([]any); ok {
				refs = xs
			}
			for _, rf := range knitRefFields {
				for _, r := range refs {
					if r, ok := r.(map[string]any); ok && rf.field == k {
						if n, ok := r[rf.name].(string); ok {
							if err := f(rf.kind, n); err != nil {
								return err
							}
						}
					}
				}
			}
			if err := knitRefs(x, f); err != nil {
				return err
			}
		}
	case []any:
		for _, x := range v {
			if err := knitRefs(x, f); err != nil {
				return err
			}
		}
	}
	return nil
}

// knitRefNodes is knitRefs for YAML nodes: it calls f with each name's node, and the mapping
// that holds it.
func knitRefNodes(n *yaml.Node, f func(kind string, name, parent *yaml.Node)) {
	for i := 0; n.Kind == yaml.MappingNode && i+1 < len(n.Content); i += 2 {
		k, x := n.Content[i].Value, n.Content[i+1]
		refs := []*yaml.Node{x}
		if x.Kind == yaml.SequenceNode {
			refs = x.Content
		}
		for _, rf := range knitRefFields {
			for _, r := range refs {
				for j := 0; rf.field == k && r.Kind == yaml.MappingNode && j+1 < len(r.Content); j += 2 {
					if r.Content[j].Value == rf.name && r.Content[j+1].Kind == yaml.ScalarNode {
						f(rf.kind, r.Content[j+1], r)
					}
				}
			}
		}
	}
	for _, c := range n.Content {
		knitRefNodes(c, f)
	}
}

// knitRef returns the yakl expression of the name of the object named name in a file on knit:
// ${$['web-config'].ConfigMap.metadata.name} for web-config.ConfigMap.
func knitRef(name string) string {
	ref := "$"
	for _, s := range strings.Split(name, ".") {
		if yakl.IsIdentifier(s) {
			ref += "." + s
		} else {
			ref += "[" + quoteJsonnet(s) + "]"
		}
	}
	return "${" + ref + ".metadata.name}"
}

// scalarLen returns how many characters of l, from the start of the scalar n, it takes as
// written: its quotes too.
func scalarLen(l []rune, n *yaml.Node) int {
	switch n.Style {
	case yaml.SingleQuotedStyle:
		for i := 1; i < len(l); i++ {
			if l[i] == '\'' {
				if i+1 < len(l) && l[i+1] == '\'' {
					i++
				} else {
					return i + 1
				}
			}
		}
	case yaml.DoubleQuotedStyle:
		for i := 1; i < len(l); i++ {
			if l[i] == '\\' {
				i++
			} else if l[i] == '"' {
				return i + 1
			}
		}
	}
	return len([]rune(n.Value))
}

// A knitList is how a list merges in knit: list is map (keyed by keys), set or atomic.
type knitList struct {
	List string   `json:"list"`
	Keys []string `json:"keys"`
}

// applyJSON adds what the JSON 6902 patch p does to the objects of objs its target selects: a
// ^^+: document for each operation. knit import reads two kinds: add at the end of a list (a path
// ending in /-), which appends with - $patch: append, or adds an entry to a list with keys; and
// remove, which deletes a field with $patch: delete, or an entry of a list with keys, by its key.
// An index goes into a list with keys, by the key of the entry it names.
//
// ponytail: an index names an entry of the object as its file has it, or as the base renders it,
// after the JSON patches before; a strategic merge patch before that adds or deletes entries of
// the list is an error here. In the base, kustomize puts the entries such a patch adds first, and
// knit last, so an index into the base's list can name another entry.
func (p *knitPatch) applyJSON(objs []knitObject, s *knitPatched, at string) error {
	root := p.doc.Content[0]
	if err := checkLiteral(root, func(l int) string { return fmt.Sprintf("%s: line %d", p.file, l) }); err != nil {
		return err
	}
	y, err := yaml.Marshal(root)
	if err != nil {
		return err
	}
	j, err := k8syaml.YAMLToJSON(y)
	if err != nil {
		return fmt.Errorf("%s: %w", at, err)
	}
	var ops []struct {
		Op, Path string
		Value    any
	}
	if err := json.Unmarshal(j, &ops); err != nil {
		return fmt.Errorf("%s: a JSON 6902 patch: %w", at, err)
	}
	targets, err := p.targets(nil, slices.DeleteFunc(slices.Clone(objs), func(o knitObject) bool { return !o.late && s.gone[o.name] }))
	if err != nil {
		return fmt.Errorf("%s: %w", at, err)
	}
	comment := strings.TrimSpace(p.doc.HeadComment + "\n\n" + root.HeadComment)
	if len(root.Content) > 0 {
		comment = strings.TrimSpace(comment + "\n\n" + root.Content[0].HeadComment)
	}
	for _, o := range targets {
		md, _ := o.obj["metadata"].(map[string]any)
		if o.late {
			return fmt.Errorf("%s: %v %v: %w", at, o.obj["kind"], md["name"], errKnitLate)
		}
		w, ok := s.cur[o.name]
		if !ok {
			if err := json.Unmarshal(must(json.Marshal(o.obj)), &w); err != nil {
				return err
			}
		}
		for i, op := range ops {
			body, err := s.jsonOp(o.name, w, op.Op, op.Path, op.Value)
			if err != nil {
				return fmt.Errorf("%s: %v %v: %s %s: %w", at, o.obj["kind"], md["name"], op.Op, op.Path, err)
			}
			// The operation stays as a comment, for who reads the file next to the kustomization.
			d := &knitPatchDoc{name: o.name, body: body, json: true, comment: "# " + op.Op + " " + op.Path}
			if i == 0 && comment != "" {
				d.comment = comment + "\n" + d.comment
			}
			s.docs = append(s.docs, d)
		}
		if s.cur == nil {
			s.cur = map[string]map[string]any{}
		}
		s.cur[o.name] = w
	}
	return nil
}

// must returns v, and panics on err: for values that can't fail to encode.
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// jsonOp returns the fields of the ^^+: document of the object named name that does the JSON
// patch operation op at path, and does it to w, the object as the operations before leave it.
func (s *knitPatched) jsonOp(name string, w map[string]any, op, path string, value any) (*yaml.Node, error) {
	if op != "add" && op != "remove" {
		return nil, fmt.Errorf("knit import reads only add at the end of a list (/-) and remove, not %s", op)
	}
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("a path starts with /")
	}
	segs := strings.Split(path[1:], "/")
	for i, seg := range segs {
		segs[i] = strings.NewReplacer("~1", "/", "~0", "~").Replace(seg)
	}
	if op == "add" && segs[len(segs)-1] != "-" {
		return nil, fmt.Errorf("knit import reads add only at the end of a list (/-), which appends")
	}
	if s.vm == nil {
		vm, err := kubecfg.JsonnetVM(kubecfg.WithFeatures(gates))
		if err != nil {
			return nil, err
		}
		s.vm = vm
	}
	out, err := s.vm.EvaluateAnonymousSnippet("knit-import", fmt.Sprintf("(import 'knit/knit.libsonnet')['$lists'](%s, %s)",
		must(json.Marshal(w)), must(json.Marshal(segs))))
	if err != nil {
		return nil, err
	}
	var lists map[string]knitList
	if err := json.Unmarshal([]byte(out), &lists); err != nil {
		return nil, err
	}
	str := func(v string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v} }
	val := func(v any) (*yaml.Node, error) {
		n := &yaml.Node{}
		return n, n.Encode(v)
	}
	// patch returns the patch of v, the value at segs[:i], and does op to v: it returns v as op
	// leaves it.
	var patch func(v any, i int) (*yaml.Node, any, error)
	patch = func(v any, i int) (*yaml.Node, any, error) {
		seg, last, where := segs[i], i == len(segs)-1, "/"+strings.Join(segs[:i+1], "/")
		switch v := v.(type) {
		case map[string]any:
			x, ok := v[seg]
			if !ok {
				return nil, nil, fmt.Errorf("no %s", where)
			}
			var child *yaml.Node
			if last {
				child = &yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle, Content: []*yaml.Node{str("$patch"), str("delete")}}
				delete(v, seg)
			} else {
				var err error
				if child, v[seg], err = patch(x, i+1); err != nil {
					return nil, nil, err
				}
			}
			return &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{str(seg), child}}, v, nil
		case []any:
			l := lists[strconv.Itoa(i)]
			if seg == "-" {
				if !last || op != "add" {
					return nil, nil, fmt.Errorf("- is the end of a list, not of %s", where)
				}
				n, err := val(value)
				if err != nil {
					return nil, nil, err
				}
				seq := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{n}}
				if l.List == "atomic" {
					seq.Content = append([]*yaml.Node{{Kind: yaml.MappingNode, Style: yaml.FlowStyle, Content: []*yaml.Node{str("$patch"), str("append")}}}, n)
				}
				return seq, append(v, value), nil
			}
			n, err := strconv.Atoi(seg)
			if err != nil || n < 0 || n >= len(v) {
				return nil, nil, fmt.Errorf("no %s", where)
			}
			e, ok := v[n].(map[string]any)
			if l.List != "map" || !ok {
				return nil, nil, fmt.Errorf("%s is an entry of a list without keys: knit can only append to it", where)
			}
			if err := s.keepsEntries(name, segs[:i], w, lists); err != nil {
				return nil, nil, err
			}
			entry := &yaml.Node{Kind: yaml.MappingNode}
			for _, k := range l.Keys {
				if kv, ok := e[k]; ok {
					n, err := val(kv)
					if err != nil {
						return nil, nil, err
					}
					entry.Content = append(entry.Content, str(k), n)
				}
			}
			if last {
				entry.Style = yaml.FlowStyle
				entry.Content = append(entry.Content, str("$patch"), str("delete"))
				return &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{entry}}, slices.Delete(v, n, n+1), nil
			}
			child, x, err := patch(e, i+1)
			if err != nil {
				return nil, nil, err
			}
			v[n] = x
			entry.Content = append(entry.Content, child.Content...)
			return &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{entry}}, v, nil
		}
		return nil, nil, fmt.Errorf("no %s", where)
	}
	body, _, err := patch(w, 0)
	return body, err
}

// keepsEntries returns an error if a strategic merge patch of the object named name, before the
// JSON patch, adds or deletes entries of the list at path segs in w: an index into it would name
// another entry in kustomize, which puts new entries first.
func (s *knitPatched) keepsEntries(name string, segs []string, w map[string]any, lists map[string]knitList) error {
	keyOf := func(e map[string]any, keys []string) string {
		var ks []string
		for _, k := range keys {
			ks = append(ks, fmt.Sprint(e[k]))
		}
		return strings.Join(ks, "/")
	}
	for _, d := range s.docs {
		if d.name != name || d.json || d.body == nil {
			continue
		}
		n, v := d.body, any(w)
		for i := 0; n != nil && i < len(segs); i++ {
			switch x := v.(type) {
			case map[string]any:
				var next *yaml.Node
				for j := 0; n.Kind == yaml.MappingNode && j+1 < len(n.Content); j += 2 {
					if n.Content[j].Value == segs[i] {
						next = n.Content[j+1]
					}
				}
				n, v = next, x[segs[i]]
			case []any:
				idx, _ := strconv.Atoi(segs[i])
				e, _ := x[idx].(map[string]any)
				keys := lists[strconv.Itoa(i)].Keys
				var next *yaml.Node
				for _, c := range n.Content {
					if m, err := nodeMap(c); err == nil && c.Kind == yaml.MappingNode && keyOf(m, keys) == keyOf(e, keys) {
						next = c
					}
				}
				n, v = next, e
			}
		}
		list, _ := v.([]any)
		if n == nil || n.Kind != yaml.SequenceNode || list == nil {
			continue
		}
		keys := lists[strconv.Itoa(len(segs))].Keys
		have := map[string]bool{}
		for _, e := range list {
			if e, ok := e.(map[string]any); ok {
				have[keyOf(e, keys)] = true
			}
		}
		for _, c := range n.Content {
			m, err := nodeMap(c)
			if err != nil || m["$patch"] != nil || !have[keyOf(m, keys)] {
				return fmt.Errorf("a patch before it adds or deletes entries of /%s, so an index into it can name another entry than in kustomize", strings.Join(segs, "/"))
			}
		}
	}
	return nil
}

// copyNode returns a copy of n, which changes don't share.
func copyNode(n *yaml.Node) *yaml.Node {
	c := *n
	c.Content = make([]*yaml.Node, len(n.Content))
	for i, x := range n.Content {
		c.Content[i] = copyNode(x)
	}
	return &c
}

// targets returns the objects of objs that p patches, as kustomize selects them: by
// its target, or else the one object of the patch's own group, kind, namespace and name.
//
// ponytail: the target selects objects as their files have them, not as the patches before
// it leave them; kustomize sees the labels an earlier patch adds.
func (p *knitPatch) targets(patch map[string]any, objs []knitObject) ([]knitObject, error) {
	fields := func(o map[string]any) (g, v, kind, ns, name string, md map[string]any) {
		md, _ = o["metadata"].(map[string]any)
		api, _ := o["apiVersion"].(string)
		g, v = resid.ParseGroupVersion(api)
		kind, _ = o["kind"].(string)
		name, _ = md["name"].(string)
		// kustomize: no namespace is the default one.
		if ns, _ = md["namespace"].(string); ns == "" {
			ns = "default"
		}
		return
	}
	var targets []knitObject
	if p.target == nil {
		pg, _, pk, pns, pn, _ := fields(patch)
		for _, o := range objs {
			if g, _, k, ns, n, _ := fields(o.obj); g == pg && k == pk && ns == pns && n == pn {
				return []knitObject{o}, nil
			}
		}
		return nil, fmt.Errorf("no %s %s to patch", pk, pn)
	}
	sel, err := types.NewSelectorRegex(p.target)
	if err != nil {
		return nil, err
	}
	ls, err := labels.Parse(p.target.LabelSelector)
	if err != nil {
		return nil, err
	}
	as, err := labels.Parse(p.target.AnnotationSelector)
	if err != nil {
		return nil, err
	}
	set := func(v any) labels.Set {
		s := labels.Set{}
		m, _ := v.(map[string]any)
		for k, x := range m {
			s[k] = fmt.Sprint(x)
		}
		return s
	}
	for _, o := range objs {
		g, v, k, ns, n, md := fields(o.obj)
		if sel.MatchGvk(resid.Gvk{Group: g, Version: v, Kind: k}) && sel.MatchName(n) && sel.MatchNamespace(ns) &&
			ls.Matches(set(md["labels"])) && as.Matches(set(md["annotations"])) {
			targets = append(targets, o)
		}
	}
	return targets, nil
}

// nodeMap returns the values YAML gives n, through aliases and merge keys, as kubectl and yakl
// read them: a date like 2026-10-28 stays a string.
func nodeMap(n *yaml.Node) (map[string]any, error) {
	var m map[string]any
	y, err := yaml.Marshal(n)
	if err != nil {
		return nil, err
	}
	j, err := k8syaml.YAMLToJSON(y)
	if err != nil {
		return nil, err
	}
	return m, json.Unmarshal(j, &m)
}

// prune drops what changes doesn't keep of n: changes is true to keep it whole, or a map (a list)
// of the same for each of its fields (entries). {$value: x} writes x in its place: a field n
// doesn't have, after its fields, or an entry, after its entries.
func prune(n *yaml.Node, changes any) error {
	if m, ok := changes.(map[string]any); ok && len(m) == 1 {
		if v, ok := m["$value"]; ok {
			if err := n.Encode(v); err != nil {
				return err
			}
			// Short values, like {name: web, $patch: delete}, read better on one line, with the
			// $patch last.
			if n.Kind == yaml.MappingNode && len(n.Content) <= 4 {
				n.Style = yaml.FlowStyle
				if len(n.Content) > 2 && n.Content[0].Value == "$patch" {
					n.Content = append(n.Content[2:], n.Content[:2]...)
				}
			}
			return nil
		}
	}
	var kept []*yaml.Node
	switch cs := changes.(type) {
	case map[string]any:
		for i := 0; i < len(n.Content); i += 2 {
			if c, ok := cs[n.Content[i].Value]; ok && c != false {
				if err := prune(n.Content[i+1], c); err != nil {
					return err
				}
				kept = append(kept, n.Content[i], n.Content[i+1])
				delete(cs, n.Content[i].Value)
			}
		}
		for _, k := range slices.Sorted(maps.Keys(cs)) {
			v := &yaml.Node{}
			if err := prune(v, cs[k]); err != nil {
				return err
			}
			kept = append(kept, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k}, v)
		}
	case []any:
		for i, c := range cs {
			if i >= len(n.Content) {
				n.Content = append(n.Content, &yaml.Node{})
			}
			if c != false {
				if err := prune(n.Content[i], c); err != nil {
					return err
				}
				kept = append(kept, n.Content[i])
			}
		}
	case bool:
		if cs {
			return nil
		}
	}
	n.Content = kept
	return nil
}

// plain tells if n has no anchors, aliases or merge keys: each of its values is where it's written.
func plain(n *yaml.Node) bool {
	if n.Anchor != "" || n.Kind == yaml.AliasNode || n.Tag == "!!merge" {
		return false
	}
	for _, c := range n.Content {
		if !plain(c) {
			return false
		}
	}
	return true
}

// docMarker tells if line l starts or ends a YAML document.
func docMarker(l string) bool {
	l = strings.TrimRight(l, "\n")
	return l == "---" || l == "..." || strings.HasPrefix(l, "--- ") || strings.HasPrefix(l, "... ")
}

// checkLiteral returns an error for a key or string that yakl would read differently from YAML:
// keys like c++, a: and &x, and strings starting with ${, an expression, or $${, which loses a $.
// at tells where a line is.
func checkLiteral(n *yaml.Node, at func(int) string) error {
	for i := 0; n.Kind == yaml.MappingNode && i < len(n.Content); i += 2 {
		key, k := n.Content[i], n.Content[i]
		if k.Kind == yaml.AliasNode { // yakl reads the key an alias stands for
			k = k.Alias
		}
		if k.Kind == yaml.ScalarNode && !yakl.LiteralKey(k.Value) {
			return fmt.Errorf("%s: yakl reads the key %q differently; write it as ${%q}", at(key.Line), k.Value, k.Value)
		}
	}
	// YAKL reads ${ and $${ after leading white space too.
	if t := strings.TrimLeft(n.Value, " \t\n"); n.Kind == yaml.ScalarNode && (strings.HasPrefix(t, "${") || strings.HasPrefix(t, "$${")) {
		return fmt.Errorf("%s: the string %q would be a yakl expression; write it as ${%q}", at(n.Line), n.Value, n.Value)
	}
	for _, c := range n.Content {
		if err := checkLiteral(c, at); err != nil {
			return err
		}
	}
	return nil
}

// withoutComments returns src without its leading comment lines.
func withoutComments(src string) string {
	for strings.HasPrefix(strings.TrimLeft(src, " \t\n"), "#") {
		src = strings.TrimLeft(src, " \t\n")
		_, src, _ = strings.Cut(src, "\n")
	}
	return src
}
