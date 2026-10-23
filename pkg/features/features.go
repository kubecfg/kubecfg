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

// Package features is the registry of kubecfg's feature gates (RFC05): features that can be
// turned on and off one at a time, each at a stage that says how far along it is.
package features

import (
	"fmt"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

type Stage int

const (
	Alpha Stage = iota
	Beta
	GA
	Removed
)

func (s Stage) String() string {
	return [...]string{"alpha", "beta", "GA", "removed"}[s]
}

// Default says whether a feature at this stage is on when nothing names it.
func (s Stage) Default() bool { return s == Beta || s == GA }

type Kind int

const (
	// Additive features add something that used to be an error, like a new command or import scheme.
	Additive Kind = iota
	// Semantic features change what existing, valid input means. They never go past alpha.
	Semantic
)

type Feature struct {
	Name  string
	Stage Stage
	Kind  Kind
	Since string // the release in which it entered its current stage
	Doc   string
}

var registry []*Feature

func register(f Feature) *Feature {
	if Lookup(f.Name) != nil {
		panic("duplicate feature " + f.Name)
	}
	registry = append(registry, &f)
	return &f
}

var (
	Eval            = register(Feature{Name: "eval", Stage: Alpha, Since: "v0.40.0", Doc: "the eval command"})
	Traceback       = register(Feature{Name: "traceback", Stage: Alpha, Since: "v0.40.0", Doc: "the traceback command"})
	Pack            = register(Feature{Name: "pack", Stage: Alpha, Since: "v0.40.0", Doc: "the pack command"})
	Httpd           = register(Feature{Name: "httpd", Stage: Alpha, Since: "v0.40.0", Doc: "the httpd command"})
	Overlay         = register(Feature{Name: "overlay", Stage: Alpha, Since: "v0.40.0", Doc: "--overlay-code, --overlay-code-file (and the deprecated --overlay)"})
	YAKL            = register(Feature{Name: "yakl", Stage: Alpha, Since: "v0.40.0", Doc: "YAML with Jsonnet expressions: .yakl files, yakl: imports, yaklfmt, eval -o jsonnet"})
	KustomizeImport = register(Feature{Name: "kustomize-import", Stage: Alpha, Since: "v0.40.0", Doc: "kustomize+https:// imports"})
	BinaryImport    = register(Feature{Name: "binary-import", Stage: Removed, Since: "v0.41.0", Doc: "the binary:// import prefix; use importbin"})
)

// All returns every registered feature, in registration order.
func All() []*Feature { return registry }

// Lookup returns the feature with the given name, or nil.
func Lookup(name string) *Feature {
	for _, f := range registry {
		if f.Name == name {
			return f
		}
	}
	return nil
}

const sourceDefault = "default"

// Source is one place features are set from.
type Source struct {
	Name  string   // where the settings came from, for kubecfg features and errors, e.g. "--features"
	Items []string // "name", "name=true" or "name=false"; an item can hold several, separated by commas
}

type setting struct {
	on     bool
	source string
}

// Gates is the set of enabled features for one kubecfg invocation, with where each setting came from.
// A nil *Gates has every feature at its default.
type Gates struct {
	set map[*Feature]setting
}

// New resolves the features. For each feature the first source that names it wins; within one
// source, the last setting of a feature wins. alpha turns on the alpha features no source names.
func New(alpha bool, sources ...Source) (*Gates, error) {
	g := &Gates{set: map[*Feature]setting{}}
	for _, src := range sources {
		named := map[*Feature]setting{}
		for _, item := range src.Items {
			for _, item := range strings.Split(item, ",") {
				item = strings.TrimSpace(item)
				if item == "" {
					continue
				}
				name, value, hasValue := strings.Cut(item, "=")
				on := true
				if hasValue {
					var err error
					if on, err = strconv.ParseBool(value); err != nil {
						return nil, fmt.Errorf("%s: %q: want name, name=true or name=false", src.Name, item)
					}
				}
				f := Lookup(name)
				switch {
				case f == nil:
					return nil, fmt.Errorf("unknown feature %q; run \"kubecfg features\" to list them", name)
				case f.Stage == Removed:
					log.Warnf("%s was removed in %s; ignoring", f.Name, f.Since)
					continue
				case f.Stage == GA && !on:
					return nil, fmt.Errorf("%s is GA since %s and can no longer be turned off; remove %q from %s", f.Name, f.Since, item, src.Name)
				case f.Stage == GA:
					log.Debugf("%s is GA since %s; it doesn't need to be turned on", f.Name, f.Since)
				}
				named[f] = setting{on, src.Name}
			}
		}
		for f, s := range named {
			if _, ok := g.set[f]; !ok {
				g.set[f] = s
			}
		}
	}
	if alpha {
		for _, f := range registry {
			if _, ok := g.set[f]; !ok && f.Stage == Alpha {
				g.set[f] = setting{true, "--alpha"}
			}
		}
	}
	return g, nil
}

// AllAlpha returns gates with every alpha feature on, as --alpha does.
func AllAlpha() *Gates {
	g, _ := New(true)
	return g
}

func (g *Gates) setting(f *Feature) setting {
	if g != nil {
		if s, ok := g.set[f]; ok {
			return s
		}
	}
	return setting{f.Stage.Default(), sourceDefault}
}

func (g *Gates) Enabled(f *Feature) bool {
	switch f.Stage {
	case GA:
		return true
	case Removed:
		return false
	}
	return g.setting(f).on
}

// Require returns the standard error for a feature that isn't on, or nil.
func (g *Gates) Require(f *Feature) error {
	if g.Enabled(f) {
		return nil
	}
	if s := g.setting(f); s.source != sourceDefault {
		return fmt.Errorf("%s is turned off by %s; turn it on with --features=%s", f.Name, s.source, f.Name)
	}
	return fmt.Errorf("%s is an %s feature; turn it on with --features=%s", f.Name, f.Stage, f.Name)
}

// Status is a feature's row in kubecfg features.
type Status struct {
	Name    string `json:"name"`
	Stage   string `json:"stage"`
	Default bool   `json:"default"`
	Enabled bool   `json:"enabled"`
	Source  string `json:"source"`
	Since   string `json:"since"`
	Doc     string `json:"doc"`
}

// List returns the status of every feature, in registration order.
func (g *Gates) List() []Status {
	var l []Status
	for _, f := range registry {
		l = append(l, Status{
			Name:    f.Name,
			Stage:   f.Stage.String(),
			Default: f.Stage.Default(),
			Enabled: g.Enabled(f),
			Source:  g.setting(f).source,
			Since:   f.Since,
			Doc:     f.Doc,
		})
	}
	return l
}
