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

import (
	"errors"
	"regexp"
	"slices"
	"strings"

	"github.com/google/go-jsonnet/formatter"
	"go.yaml.in/yaml/v3"
)

// Format formats the Jsonnet code of the expressions in a yakl document, as jsonnetfmt does, and
// leaves the YAML around them as it is. A plain expression whose formatted code isn't a valid plain
// YAML scalar any more, like ${ { a: 1 } } with its `: `, gets quoted. Expressions holding YAML
// blocks, ${for} and ${if} clauses, code that would take several lines outside a block
// expression, and a block expression with a string spanning its lines are left as they are. What follows a "---" on its line, like the | in --- |, moves
// to the next line, since kubectl's reader allows only a comment there.
func Format(src string) (string, error) {
	c, err := compiled(src, false, false)
	if err != nil {
		return "", err
	}
	type edit struct {
		start, end int
		text       string
	}
	var edits []edit
	for i, e := range c.exprs {
		n := c.nodes[i]
		if n == nil || len(e.blocks) > 0 || c.clause(n) != "" {
			continue
		}
		start, end := e.from-2, e.to+1 // the scalar: ${...}, with its quotes if quoted
		f, ok := formatCode(e.code, c.src[e.from:e.to])
		if !ok {
			continue
		}
		code := f
		if strings.HasPrefix(f, "{") {
			f = " " + f + " " // ${ {a: 1} }, rather than ${{ a: 1 }}
		}
		s, ok := c.spell(n, "${"+f+"}", start)
		// Indenting the lines of a block changes a string that spans them.
		if ok && n.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
			if g, _ := formatCode("", s[2:len(s)-1]); g != code {
				continue
			}
		}
		if ok {
			edits = append(edits, edit{start, end, s})
		}
	}
	// No expression holds a line starting with ---: YAML ends the document there.
	for _, m := range afterDocStart.FindAllStringSubmatchIndex(c.src, -1) {
		if rest := c.src[m[2]:]; !marker(rest, "---") && !marker(rest, "...") { // which would start a line
			edits = append(edits, edit{m[0] + len("---"), m[2], "\n"})
		}
	}
	slices.SortFunc(edits, func(a, b edit) int { return a.start - b.start })
	var b strings.Builder
	prev := 0
	for _, e := range edits {
		b.WriteString(c.src[prev:e.start])
		b.WriteString(e.text)
		prev = e.end
	}
	b.WriteString(c.src[prev:])
	out := b.String()
	// The YAML means the same, and so does each expression, as formatting it shows. Readable code
	// doesn't depend on where things are in the source, and canonical code leaves out comments.
	want, err := compiled(c.src, true, true)
	if err != nil {
		return "", err
	}
	if got, err := compiled(out, true, true); err != nil || got.String() != want.String() {
		return "", errors.New("internal error: formatting would change the meaning")
	}
	return out, nil
}

// afterDocStart matches a "---" that starts a document with something other than a comment after
// it on its line, which is the submatch.
var afterDocStart = regexp.MustCompile(`(?m)^---[ \t]+([^#\s])`)

// formatCode returns the code of an expression formatted, as jsonnetfmt does: code if the
// expression was quoted, else raw. It returns false if the code doesn't parse on its own.
func formatCode(code, raw string) (string, bool) {
	if code == "" {
		code = raw
	}
	code = withIt(code)
	opts := formatter.DefaultOptions()
	opts.StringStyle = formatter.StringStyleLeave // a string's quotes may be chosen to fit the YAML quotes
	f, err := formatter.Format("", code, opts)
	// A pass removes one level of redundant parentheses, ((0)) giving (0); the next would go on.
	for i := 0; err == nil && i < len(code); i++ {
		g, gerr := formatter.Format("", f, opts)
		if gerr != nil { // the formatter wrote code that doesn't parse: [$ :] as [$:]
			err = gerr
			break
		}
		if g == f {
			break
		}
		f = g
	}
	f = strings.ReplaceAll(strings.TrimSuffix(f, "\n"), itVar, "@")
	// A line comment at the end keeps its line break, or it would take the closing }.
	if end, _, err := exprEnd("${"+f+"}", 2); err != nil || end != len(f)+2 {
		f += "\n"
	}
	return f, err == nil
}

// spell returns how to write expression s in place of scalar n, which starts at offset start, or
// false to leave n as it is.
func (c *compiler) spell(n *yaml.Node, s string, start int) (string, bool) {
	if n.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
		// Lines after the first are indented like it, which keeps them in the block.
		indent := strings.Repeat(" ", start-strings.LastIndexByte(c.src[:start], '\n')-1)
		lines := strings.Split(s, "\n")
		for i := 1; i < len(lines); i++ {
			if lines[i] != "" {
				lines[i] = indent + lines[i]
			}
		}
		return strings.Join(lines, "\n"), true
	}
	if strings.Contains(s, "\n") {
		return "", false
	}
	q := c.src[start]
	if q != '\'' && q != '"' {
		p := c.places[n]
		if lit, err := resolve(s, p.key, p); err == nil && lit == quote(s) {
			return s, true
		}
		q = '\''
		if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
			q = '"'
		}
	}
	if q == '\'' {
		return "'" + strings.ReplaceAll(s, "'", "''") + "'", true
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`, true
}
