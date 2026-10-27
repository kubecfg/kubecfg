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

package yakl

// Selectors: list[@.name == 'api'] is the one entry of list for which the condition is true, @
// being the entry. @ alone isn't Jsonnet (it starts a verbatim string, @'...'), so no Jsonnet
// code means something else by it. It becomes the variable itVar, which lets the code parse, and
// each selector becomes Jsonnet that filters the list.

import (
	"fmt"
	"slices"
	"strings"

	jsonnet "github.com/google/go-jsonnet"
	"github.com/google/go-jsonnet/ast"
	"github.com/google/go-jsonnet/toolutils"
)

// itVar is what @ becomes in Jsonnet.
const itVar = "yakl__it"

// pick is the function the code of a selector calls. Using kubecfgVar makes Compile declare it.
const pick = kubecfgVar + ".pick"

// withIt returns Jsonnet code with each @ that doesn't start a verbatim string replaced by itVar.
// An @ next to a letter or digit, like @0, is left for Jsonnet to report: as itVar, it would join
// them in another name.
func withIt(code string) string {
	var b strings.Builder
	prev := 0
	words(code, func(i int, w string) {
		if w == "@" && !(i > 0 && isWordChar(code[i-1])) && !(i+1 < len(code) && isWordChar(code[i+1])) {
			b.WriteString(code[prev:i])
			b.WriteString(itVar)
			prev = i + 1
		}
	})
	if prev == 0 {
		return code
	}
	b.WriteString(code[prev:])
	return b.String()
}

// selectors returns the code of an expression, or of a ${for} or ${if} clause, with each selector
// written in Jsonnet. line and col are where the code starts in the source, for errors.
func selectors(code string, line, col int) (string, error) {
	code = withIt(code)
	if !strings.Contains(code, itVar) {
		return code, nil
	}
	// As in keyVars: an object, where self and $ are valid, holding the code as an expression.
	pre, post := "{f: (", "\n)}"
	switch {
	case startsWith(code, "for") && !parses(code):
		pre, post = pre+"[null ", "\n]"+post
	case startsWith(code, "if") && !parses(code):
		post = "\nthen null" + post
	}
	var binds []string
	bind := func(skip string) string {
		var b strings.Builder
		for _, v := range binds {
			if v != skip {
				b.WriteString("local " + v + " = null; ")
			}
		}
		return b.String()
	}
	var n ast.Node
	for {
		var err error
		n, err = jsonnet.SnippetToAST("", bind("")+pre+code+post)
		if m := unknownVar.FindStringSubmatch(fmt.Sprint(err)); m != nil && !slices.Contains(binds, m[1]) {
			binds = append(binds, m[1]) // bound around the expression, or a mistake Jsonnet reports
			continue
		}
		if err != nil {
			return code, nil // Jsonnet reports it, at its line
		}
		break
	}
	if v := stray(n); v != nil {
		return "", fmt.Errorf("line %d: @ is the entry of a list that a selector tests, so it goes in the [...] after the list, like list[@.name == 'x']",
			line+v.Loc().Begin.Line-1)
	}
	head := bind("") + pre
	src := head + code + post
	r := &rewriter{src: src, lines: []int{0}, head: len(head), line: line, col: col}
	for i, ch := range src {
		if ch == '\n' {
			r.lines = append(r.lines, i+1)
		}
	}
	out := r.render(n)
	if !strings.HasPrefix(out, head) || !strings.HasSuffix(out, post) {
		return "", fmt.Errorf("line %d: internal error: rewriting selectors", line)
	}
	out = out[len(head) : len(out)-len(post)]
	// Every @ must have become a selector's.
	if _, err := jsonnet.SnippetToAST("", "local "+kubecfgVar+" = null; "+bind(itVar)+pre+out+post); err != nil && strings.Contains(err.Error(), itVar) {
		return "", fmt.Errorf("line %d: internal error: rewriting selectors: %v", line, err)
	}
	return out, nil
}

// stray returns an @ that no selector holds, or nil. An @ in the brackets of list[...] is the
// selector's, unless brackets nested in them hold it. The code of a selector already rewritten,
// in a YAML block in the expression, binds its own.
func stray(n ast.Node) *ast.Var {
	switch n := n.(type) {
	case *ast.Var:
		if n.Id == itVar {
			return n
		}
	case *ast.Index:
		return stray(n.Target)
	case *ast.Function:
		if len(n.Parameters) == 1 && n.Parameters[0].Name == itVar {
			return nil
		}
	}
	for _, c := range toolutils.Children(n) {
		if v := stray(c); v != nil {
			return v
		}
	}
	return nil
}

// A rewriter writes code back from its AST, with each selector as Jsonnet. What isn't a selector
// is copied as it is, so the code keeps its lines.
type rewriter struct {
	src       string
	lines     []int // offset of the start of each line
	head      int   // offset of the code in src
	line, col int   // source position of the code
}

func (r *rewriter) off(l ast.Location) int { return r.lines[l.Line-1] + l.Column - 1 }

func (r *rewriter) render(n ast.Node) string {
	if ix, ok := n.(*ast.Index); ok && stray(ix.Index) != nil {
		list, cond := ix.Target.Loc(), ix.Index.Loc()
		// The line breaks around the condition, which the brackets around it held.
		before := strings.Repeat("\n", strings.Count(r.src[r.off(list.End):r.off(cond.Begin)], "\n"))
		after := strings.Repeat("\n", strings.Count(r.src[r.off(cond.End):r.off(ix.Loc().End)], "\n"))
		open := r.off(list.End) + strings.LastIndexByte(r.src[r.off(list.End):r.off(cond.Begin)], '[')
		return pick + "(" + r.render(ix.Target) + ", function(" + itVar + ") (" + before + r.render(ix.Index) + after + "), " + "std.thisFile + " + quote(":"+r.where(open)) + ")"
	}
	from, to := r.off(n.Loc().Begin), r.off(n.Loc().End)
	var b strings.Builder
	for _, c := range located(n) {
		cf, ct := r.off(c.Loc().Begin), r.off(c.Loc().End)
		if cf < from || ct > to {
			continue
		}
		b.WriteString(r.src[from:cf])
		b.WriteString(r.render(c))
		from = ct
	}
	b.WriteString(r.src[from:to])
	return b.String()
}

// where returns the source position of offset off in src, as line:column.
func (r *rewriter) where(off int) string {
	code := r.src[r.head:off]
	line, col := position(code, len(code))
	// Each @ before off, on its line, is written itVar.
	col -= strings.Count(code[strings.LastIndexByte(code, '\n')+1:], itVar) * (len(itVar) - 1)
	if line == 1 {
		col += r.col - 1
	}
	return fmt.Sprintf("%d:%d", r.line+line-1, col)
}

// located returns the nodes under n that have a place in the source, the nearest ones, in source
// order. Desugaring adds nodes without one.
func located(n ast.Node) []ast.Node {
	var out []ast.Node
	for _, c := range toolutils.Children(n) {
		if c.Loc().Begin.Line > 0 {
			out = append(out, c)
		} else {
			out = append(out, located(c)...)
		}
	}
	slices.SortStableFunc(out, func(a, b ast.Node) int {
		a1, b1 := a.Loc().Begin, b.Loc().Begin
		if a1.Line != b1.Line {
			return a1.Line - b1.Line
		}
		return a1.Column - b1.Column
	})
	return out
}
