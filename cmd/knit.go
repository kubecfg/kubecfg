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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kubecfg/kubecfg/pkg/features"
	"github.com/kubecfg/kubecfg/pkg/yakl"
	"github.com/kubecfg/kubecfg/utils"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"
	k8syaml "sigs.k8s.io/yaml"
)

const (
	flagExtend = "extend"
	flagDelete = "delete"
)

func init() {
	RootCmd.AddCommand(knitCmd)
	knitCmd.AddCommand(knitImportCmd)
	knitImportCmd.Flags().String(flagExtend, "", "Make the file an overlay of this yakl file: objects it has get ^^+: and its names")
	knitImportCmd.Flags().Bool(flagDelete, false, "With --extend, the file has every object in full: what the base has and the file doesn't is taken out")
}

var knitCmd = &cobra.Command{
	Use:   "knit",
	Short: "Tools for knit, the library that merges Kubernetes manifests in layers",
}

var knitImportCmd = &cobra.Command{
	Use:   "import <file.yaml>",
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
^^+:: and its name.`,
	Args: cobra.ExactArgs(1),
	// It evaluates the base of --extend, so it takes evaluation flags from the rc file.
	Annotations: map[string]string{evalCmdAnno: ""},
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := errors.Join(gates.Require(features.Knit), gates.Require(features.YAKL)); err != nil {
			return err
		}
		file := args[0]
		src, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		var base *knitBase
		path, err := cmd.Flags().GetString(flagExtend)
		if err != nil {
			return err
		}
		del, err := cmd.Flags().GetBool(flagDelete)
		if err != nil {
			return err
		}
		if del && path == "" {
			return fmt.Errorf("--%s needs --%s", flagDelete, flagExtend)
		}
		if path != "" {
			if base, err = newKnitBase(cmd, file, path, del); err != nil {
				return err
			}
		}
		out, err := knitImport(string(src), base)
		if err != nil {
			return fmt.Errorf("%s: %w", file, err)
		}
		fmt.Fprint(cmd.OutOrStdout(), out)
		return nil
	},
}

const knitHeader = "<<+: ${import 'knit/knit.libsonnet'}\n"

// knitFields are the fields of knit, which a file that extends it shares. A
// generated name starting with one gets a _ in front: knit rejects ^^: registry.Service, which
// would be a field of its registry. Kubernetes names never start with _.
var knitFields = map[string]bool{
	"append": true, "delete": true, "documents": true, "groupKind": true, "groupVersionKind": true, "id": true,
	"manifests": true, "mixin": true, "openapi": true, "registry": true, "render": true, "replace": true,
}

// A knitBase is the yakl file an overlay extends.
type knitBase struct {
	header string // the overlay's first document, which extends it
	del    bool   // the overlay has every object in full: it deletes what it leaves out
	// lookup tells what the base knows of objs, whose heads (apiVersion, kind, metadata) are heads.
	lookup func(heads, objs []any) (*knitLookup, error)
}

type knitLookup struct {
	Names map[string]string `json:"names"` // the base's object names, by the object's identity
	All   []string          `json:"all"`   // every name in the base, objects or not
	IDs   []string          `json:"ids"`   // the identities of the objects looked up
	// What of each object changes the base's: true for all of it, false for none, or a map (a
	// list) of the same for each field (entry).
	Changes []any `json:"changes"`
}

// newKnitBase evaluates the base at path for an overlay that will sit where file is.
func newKnitBase(cmd *cobra.Command, file, path string, del bool) (*knitBase, error) {
	from, err := filepath.Abs(filepath.Dir(file))
	if err != nil {
		return nil, err
	}
	to, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(from, to)
	if err != nil {
		return nil, err
	}
	u, err := utils.PathToURL(path)
	if err != nil {
		return nil, err
	}
	vm, err := JsonnetVM(cmd)
	if err != nil {
		return nil, err
	}
	return &knitBase{
		// Imports are URLs: #, ? and % in a file name are escaped, and ./ goes before a : in it.
		del:    del,
		header: fmt.Sprintf("<<+: ${import %s}\n", quoteJsonnet("yakl:"+(&url.URL{Path: filepath.ToSlash(rel)}).String())),
		lookup: func(heads, objs []any) (*knitLookup, error) {
			js, err := json.Marshal(heads)
			if err != nil {
				return nil, err
			}
			jo, err := json.Marshal(objs)
			if err != nil {
				return nil, err
			}
			out, err := vm.EvaluateAnonymousSnippet("knit-import", fmt.Sprintf(`
local b = import %q, c = b['$composed'];
assert std.objectHasAll(b, '$id') : %s;
{
  names: { [c.names[n]]: n for n in std.objectFields(c.names) },
  all: [d.name for d in b.documents],
  ids: [b['$id'](o) for o in %s],
  changes: [b['$changes'](o, %t) for o in %s],
}`, utils.ImportPath(u), quoteJsonnet(path+" is not a yakl file on knit"), js, del, jo))
			if err != nil {
				return nil, err
			}
			var l knitLookup
			return &l, json.Unmarshal([]byte(out), &l)
		},
	}, nil
}

// quoteJsonnet returns s as a single-quoted Jsonnet string, which reads well in YAML.
func quoteJsonnet(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, "'", `\'`).Replace(s) + "'"
}

// knitImport returns YAML src as yakl on knit, or as an overlay of base if it isn't nil: a first
// document that extends knit or base, and a ^^ key before the first key of each object.
func knitImport(src string, base *knitBase) (string, error) {
	type object struct {
		head, obj              map[string]any
		root                   *yaml.Node
		name, ns, kind, indent string
		line                   int    // the line of its first key, from 0
		key, full              string // ^^ or ^^+, and the name it gets
		changes                any    // what of it changes the base's, if it's written anew
	}
	var objects []*object
	// YAML also breaks lines at a lone \r, as yakl does; the output has \n only.
	src = strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(src)
	// yaml.v3 breaks lines at these too, and yakl rejects them.
	if i := strings.IndexAny(src, "\u0085\u2028\u2029"); i >= 0 {
		return "", fmt.Errorf("line %d: unsupported Unicode line separator", strings.Count(src[:i], "\n")+1)
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
			return "", fmt.Errorf("line %d: a null document: delete it, since it holds no object", root.Line)
		}
		if err := checkLiteral(root); err != nil {
			return "", err
		}
		// The identity is read from the values YAML gives, through aliases and merge keys, as
		// kubectl and yakl read them: a date like 2026-10-28 stays a string.
		var obj map[string]any
		if y, err := yaml.Marshal(root); err != nil {
			return "", fmt.Errorf("line %d: %w", root.Line, err)
		} else if j, err := k8syaml.YAMLToJSON(y); err != nil {
			return "", fmt.Errorf("line %d: %w", root.Line, err)
		} else if err := json.Unmarshal(j, &obj); err != nil {
			return "", fmt.Errorf("line %d: %w", root.Line, err)
		}
		md, _ := obj["metadata"].(map[string]any)
		apiVersion, _ := obj["apiVersion"].(string)
		kind, _ := obj["kind"].(string)
		name, _ := md["name"].(string)
		ns, hasNS := md["namespace"].(string)
		switch {
		case kind == "List" && (apiVersion == "v1" || apiVersion == ""):
			return "", fmt.Errorf("line %d: a List: write each of its items as a document of its own, for each to get a name", root.Line)
		case kind == "" || name == "":
			return "", fmt.Errorf("line %d: a document without a kind and a metadata.name can't get a name, and knit needs one on every object", root.Line)
		case obj["$patch"] != nil:
			return "", fmt.Errorf("line %d: %s %s has $patch: to leave out an object of the base, write a document of only ^^+:: and the base's name for it", root.Line, kind, name)
		}
		first := root.Content[0]
		indent := lines[first.Line-1][:first.Column-1]
		if root.Style&yaml.FlowStyle != 0 || strings.TrimSpace(indent) != "" {
			return "", fmt.Errorf("line %d: %s %s needs its first key at the start of a line, for its name to go above it", first.Line, kind, name)
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
	var b strings.Builder
	b.WriteString(header)
	if !strings.HasPrefix(strings.TrimLeft(withoutComments(src), " \t\n"), "---") {
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
	for _, n := range hide {
		b.WriteString("---\n^^+:: " + n + "\n")
	}
	out := b.String()
	if _, err := yakl.Compile(out); err != nil {
		return "", fmt.Errorf("the file as yakl: %w", err)
	}
	return out, nil
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
func checkLiteral(n *yaml.Node) error {
	for i := 0; n.Kind == yaml.MappingNode && i < len(n.Content); i += 2 {
		at, k := n.Content[i], n.Content[i]
		if k.Kind == yaml.AliasNode { // yakl reads the key an alias stands for
			k = k.Alias
		}
		if k.Kind == yaml.ScalarNode && !yakl.LiteralKey(k.Value) {
			return fmt.Errorf("line %d: yakl reads the key %q differently; write it as ${%q}", at.Line, k.Value, k.Value)
		}
	}
	// YAKL reads ${ and $${ after leading white space too.
	if t := strings.TrimLeft(n.Value, " \t\n"); n.Kind == yaml.ScalarNode && (strings.HasPrefix(t, "${") || strings.HasPrefix(t, "$${")) {
		return fmt.Errorf("line %d: the string %q would be a yakl expression; write it as ${%q}", n.Line, n.Value, n.Value)
	}
	for _, c := range n.Content {
		if err := checkLiteral(c); err != nil {
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
