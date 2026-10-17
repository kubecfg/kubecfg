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

// Package jamlet compiles jamlet documents (YAML whose nodes can be ${jsonnet} expressions)
// into plain Jsonnet. See docs/Advanced-Usage/jamlet.md for the language spec.
package jamlet

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/go-jsonnet/formatter"
	"go.yaml.in/yaml/v3"
	k8syaml "sigs.k8s.io/yaml"
)

// Ext is the file extension of jamlet documents.
const Ext = ".jamlet"

// maxCopies bounds the nodes that aliases copy, so that nested aliases ("billion laughs") can't blow up.
const maxCopies = 100_000

var (
	// Same document start marker detection as std.parseYaml.
	docStart = regexp.MustCompile(`(?m)^---(\s|$)`)
	yamlLine = regexp.MustCompile(`line (\d+)`)
)

// Compile translates a jamlet document into equivalent Jsonnet code.
// Code keeps its source line in the output, so Jsonnet errors point at the right line of the .jamlet file.
func Compile(src string) (string, error) {
	if !utf8.ValidString(src) {
		return "", errors.New("not valid UTF-8")
	}
	src = strings.TrimPrefix(src, "\uFEFF")
	// YAML also breaks lines at a lone \r; normalize so that the lexer only deals with \n.
	src = strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(src)
	if i := strings.IndexAny(src, "\u0085\u2028\u2029"); i >= 0 {
		return "", fmt.Errorf("line %d: unsupported Unicode line separator", strings.Count(src[:i], "\n")+1)
	}
	// Like std.parseYaml and kubecfg's YAML reader, which keep the final line break of a block
	// scalar ending the file.
	if src != "" && !strings.HasSuffix(src, "\n") {
		src += "\n"
	}
	c := &compiler{output: &output{line: 1, col: 1}, src: src}
	for r := rune(0xE000); r <= 0xF8FF && c.sentinel == ""; r++ {
		if !strings.ContainsRune(src, r) {
			c.sentinel = string(r)
		}
	}
	if c.sentinel == "" {
		return "", errors.New("source uses every private-use character, cannot pick a placeholder")
	}

	l, err := lex(src, 0, false)
	if err != nil {
		return "", err
	}
	if err := c.unit(l, 0, len(src)); err != nil {
		return "", err
	}
	return c.String(), nil
}

// Lexing.
//
// The lexer knows just enough YAML to find the ${...} expressions that start a node, leaving alone
// comments, quoted and block scalars (except a block scalar holding an expression). Its findings are double-checked against yaml.v3, which parses
// the result: any disagreement is reported as an error rather than silently producing wrong output.

type span struct {
	start, end int      // offsets of "${" and after "}"
	local      bool     // a local declaration, rather than an expression
	field      bool     // a local declaration in a mapping, rather than at the start of a document
	semi       int      // offset of the ";" ending a field local declaration, -1 if none
	doc        int      // index of the document a local declaration precedes
	blocks     []*block // YAML blocks nested in the code
	code       string   // for a quoted expression, its code after YAML unquoting; start is the quote
}

// A block is YAML nested in an expression: "\{" ending its line, then YAML lines, then a line
// indented less than them starting with "}".
type block struct {
	start, end int // offsets of "\{" and "}"
	lx         *lexer
}

type lexer struct {
	src       string
	i, bol    int  // current offset and the offset of the start of its line
	nested    bool // lexing a block, rather than a file
	base      int  // for a block, indentation of its content; -1 until known
	closed    bool // the end of the block has been found, at offset i
	flow      int  // depth of flow collections ([...], {...})
	node      bool // a node can start at i, as opposed to being inside or after one
	nodeCol   int  // column where the last node started
	propCol   int  // column of the anchor or tag starting the next node, -1 if none
	tagged    bool // the next node has a tag, maybe on an earlier line
	parent    int  // column that continuation lines of a block or plain scalar must exceed
	plainOpen bool // the previous line ended inside a plain scalar

	// Documents are counted like yaml.v3 does, to know which one a local declaration precedes.
	doc                    int
	docStarted, docContent bool // the current document has an explicit "---", has content

	spans []span
}

// lex lexes YAML starting at offset i: up to the end of the source or, for a block nested in an
// expression, from the line after its "\{" up to the "}" closing it.
func lex(src string, i int, nested bool) (*lexer, error) {
	l := &lexer{src: src, i: i, nested: nested, base: -1, propCol: -1}
	if nested {
		l.i = lineEnd(src, i) + 1
	}
	for l.i < len(l.src) && !l.closed {
		if err := l.line(); err != nil {
			return nil, err
		}
	}
	if nested && !l.closed {
		return nil, errorAt(src, i-2, "unterminated YAML block: close it with } on a line indented less than its content")
	}
	l.endDoc()
	return l, nil
}

func (l *lexer) line() error {
	l.bol = l.i
	for l.i < len(l.src) && l.src[l.i] == ' ' {
		l.i++
	}
	indent, rest := l.i-l.bol, l.src[l.i:]
	if l.nested && l.flow == 0 && !blankLine(rest[:lineEnd(rest, 0)]) && rest[0] != '#' {
		switch {
		case l.base < 0 && rest[0] != '}':
			l.base = indent
		case l.base < 0 || indent < l.base:
			if rest[0] != '}' {
				return errorAt(l.src, l.i, "a YAML block in an expression must be closed by } on a line indented less than its content")
			}
			l.closed = true
			return nil
		}
	}
	if l.flow == 0 {
		switch {
		case indent == 0 && marker(rest, "---"):
			l.endDoc()
			l.docStarted = true
			l.i += 3
			l.node, l.parent, l.plainOpen = true, -1, false
		case indent == 0 && marker(rest, "..."):
			l.endDoc()
			l.i += 3
			l.plainOpen = false
		case indent == 0 && strings.HasPrefix(rest, "%") && !l.docStarted && !l.docContent:
			l.i = lineEnd(l.src, l.i) // a directive
		case blankLine(rest[:lineEnd(rest, 0)]):
			// blank lines don't end a multi-line plain scalar
		case l.plainOpen && rest[0] != '#' && indent > l.parent && !explicit(rest):
			if err := l.plain(); err != nil {
				return err
			}
		default:
			l.node, l.parent, l.plainOpen, l.propCol = true, indent-1, false, -1
		}
	}
	return l.tokens()
}

// tokens scans the rest of the line, and the following lines for tokens that span them.
func (l *lexer) tokens() error {
	for l.i < len(l.src) {
		i, node := l.i, l.node
		switch c := l.src[l.i]; {
		case c == '\n':
			l.i++
			return nil
		case c == ' ' || c == '\t' || c == '\r':
			l.i++
		// A # starts a comment after white space, and also where no plain scalar can hold it:
		// where a node starts, or right after a quoted scalar or flow collection.
		case l.commentAt(l.i) || c == '#' && (l.node || strings.IndexByte(`"']}`, l.src[l.i-1]) >= 0):
			l.plainOpen = false
			l.i = lineEnd(l.src, l.i)
		case l.node:
			if err := l.nodeStart(); err != nil {
				return err
			}
		default:
			if err := l.afterNode(); err != nil {
				return err
			}
		}
		if l.i == i && l.node == node {
			return errorAt(l.src, l.i, "internal error: lexer stuck")
		}
	}
	return nil
}

func (l *lexer) nodeStart() error {
	rest := l.src[l.i:]
	if strings.HasPrefix(rest, "${") {
		return l.expr()
	}
	l.docContent = true
	c, sep := rest[0], len(rest) == 1 || isBlank(rest[1])
	switch {
	case c == '\'' || c == '"':
		start := l.i
		l.startNode(start)
		l.i = len(l.src) // unterminated: left for the YAML parser to report
		if end := strEnd(l.src, start+1, c, c == '\''); end >= 0 {
			l.i = end + 1
			l.quoted(start)
		}
	case (c == '|' || c == '>') && l.flow == 0:
		return l.blockScalar()
	case c == '&' || c == '!' || c == '*':
		if c == '!' && sep {
			return errorAt(l.src, l.i, "the non-specific tag ! is not supported, quote the value instead")
		}
		if c == '*' {
			l.startNode(l.i)
		} else if l.propCol < 0 {
			l.propCol = l.i - l.bol
		}
		l.tagged = l.tagged || c == '!'
		if strings.HasPrefix(rest, "!<") { // a verbatim tag, which may hold flow indicators
			l.i += strings.IndexByte(rest, '>') + 1
		}
		for l.i < len(l.src) && !isBlank(l.src[l.i]) && !(l.flow > 0 && isFlowIndicator(l.src[l.i])) {
			l.i++
		}
	case c == '-' && sep && l.flow == 0:
		l.parent, l.tagged = l.i-l.bol, false // a tag before "- " is the sequence's
		l.i++
	case (c == '?' || c == ':') && (sep || l.flow > 0):
		if l.flow == 0 { // an explicit key or value: the mapping's indentation
			l.parent = l.i - l.bol
		}
		l.tagged = false
		l.i++
	case c == '[' || c == '{':
		l.startNode(l.i)
		l.node = true
		l.flow++
		l.i++
	case c == ']' || c == '}':
		l.flow = max(l.flow-1, 0)
		l.node = false
		l.i++
	case c == ',':
		l.i++
	default:
		l.startNode(l.i)
		return l.plain()
	}
	return nil
}

// startNode records that a node starts at offset i, or at the anchor or tag before it.
func (l *lexer) startNode(i int) {
	l.nodeCol, l.node, l.tagged = i-l.bol, false, false
	if l.propCol >= 0 {
		l.nodeCol, l.propCol = l.propCol, -1
	}
}

func (l *lexer) afterNode() error {
	rest := l.src[l.i:]
	switch c := rest[0]; {
	case c == ':' && (l.flow > 0 || len(rest) == 1 || isBlank(rest[1])):
		// the node was a key
		l.parent, l.node = l.nodeCol, true
		l.i++
	case c == ',' && l.flow > 0:
		l.node = true
		l.i++
	case (c == ']' || c == '}') && l.flow > 0:
		l.flow--
		l.i++
	case (c == '[' || c == '{') && l.flow > 0:
		l.node = true // invalid YAML, left for the parser to report
	default:
		// a plain scalar continuing on another line
		return l.plain()
	}
	return nil
}

// plain scans a plain scalar up to the end of the line or the first indicator ending it.
func (l *lexer) plain() error {
	l.node = false
	for ; l.i < len(l.src); l.i++ {
		rest := l.src[l.i:]
		switch c := rest[0]; {
		case c == '\n':
			l.plainOpen = l.flow == 0
			return nil
		case l.commentAt(l.i),
			c == ':' && (len(rest) == 1 || isBlank(rest[1]) || l.flow > 0 && isFlowIndicator(rest[1])),
			l.flow > 0 && isFlowIndicator(c):
			return nil
		case strings.HasPrefix(rest, "${"):
			return errorAt(l.src, l.i, "${ inside a plain scalar: an expression must be a whole YAML value "+
				"(build strings in Jsonnet, or quote the scalar for a literal ${)")
		}
	}
	return nil
}

// blockScalar skips a literal (|) or folded (>) block scalar: its header line and its content lines,
// which are those indented more than the parent node. Unless tagged, a block scalar whose content
// is a single expression, apart from white space around it, is that expression.
func (l *lexer) blockScalar() error {
	tagged := l.tagged
	l.tagged = false
	indent, first := -1, -1 // first: offset of the content, past the indentation of its first line
	for l.i++; l.i < len(l.src) && strings.IndexByte("+-123456789", l.src[l.i]) >= 0; l.i++ {
		if d := l.src[l.i]; d >= '1' && d <= '9' {
			indent = max(l.parent, 0) + int(d-'0')
		}
	}
	for l.i = lineEnd(l.src, l.i); l.i < len(l.src); {
		bol := l.i + 1
		sp := bol
		for sp < len(l.src) && l.src[sp] == ' ' {
			sp++
		}
		end := lineEnd(l.src, sp)
		if !blankLine(l.src[sp:end]) {
			n := sp - bol
			if indent < 0 && n > l.parent {
				indent = n
			}
			if indent < 0 || n < indent || n == 0 && (marker(l.src[bol:], "---") || marker(l.src[bol:], "...")) {
				break
			}
			if first < 0 {
				// White space after an explicit indentation doesn't matter either.
				first = end - len(strings.TrimLeft(l.src[sp:end], " \t"))
			}
		}
		l.i = end
	}
	l.node = false
	if tagged || first < 0 || !strings.HasPrefix(l.src[first:], "${") {
		return nil
	}
	// The expression can't run past the block, whatever follows it.
	end, blocks, err := exprEnd(l.src[:l.i], first+2)
	switch {
	case err != nil && strings.HasSuffix(strings.TrimRight(l.src[first:l.i], " \t\n"), "}"):
		return err // shaped like an expression, so most likely meant as one
	case err != nil || strings.Trim(l.src[end+1:l.i], " \t\n") != "":
		return nil
	case strings.TrimSpace(l.src[first+2:end]) == "":
		return errorAt(l.src, first, "empty expression")
	case isDecl(withoutBlocks(l.src, first+2, end, blocks)):
		return errorAt(l.src, first, "a local declaration cannot be a block scalar")
	}
	l.spans = append(l.spans, span{start: first, end: end + 1, blocks: blocks})
	return nil
}

func (l *lexer) expr() error {
	start := l.i
	end, blocks, err := exprEnd(l.src, start+2)
	if err != nil {
		return err
	}
	code := l.src[start+2 : end]
	if strings.TrimSpace(code) == "" {
		return errorAt(l.src, start, "empty expression")
	}
	l.i = end + 1
	j := l.i
	for j < len(l.src) && (l.src[j] == ' ' || l.src[j] == '\t' || l.src[j] == '\r') {
		j++
	}
	eol := j == len(l.src) || l.src[j] == '\n' || l.commentAt(j)

	if plain := withoutBlocks(l.src, start+2, end, blocks); isDecl(plain) {
		// Only indentation, or the "- " of sequence items, may precede a declaration on its line.
		before := l.src[l.bol:start]
		if l.flow > 0 || !eol || strings.Trim(before, " -") != "" {
			return errorAt(l.src, start, "a local declaration must be on its own lines")
		}
		s := span{start: start, end: l.i, local: true, doc: l.doc, blocks: blocks, semi: -1}
		// At the start of a document, it scopes over the rest of the file. Elsewhere it is a
		// local of the mapping it is in, which the YAML parser finds by giving it a key.
		if s.field = l.docContent || len(before) != max(l.base, 0); s.field {
			var ok bool
			if s.semi, ok = fieldSemi(l.src, plain, end, blocks); !ok {
				return errorAt(l.src, start, "a local declaration in a mapping must declare a single variable, like ${local x = 1}")
			}
			l.docContent = true
			l.startNode(start)
			l.parent, l.node = l.nodeCol, true
		}
		l.spans = append(l.spans, s)
		return nil
	}
	// Only a key indicator, flow punctuation, a comment or the end of the line may follow.
	if !eol && l.src[j] != ':' && !(l.flow > 0 && strings.IndexByte(",]}", l.src[j]) >= 0) {
		return errorAt(l.src, j, "unexpected text after an expression, which must be a whole YAML value (build strings in Jsonnet)")
	}
	l.docContent = true
	l.startNode(start)
	l.spans = append(l.spans, span{start: start, end: l.i, blocks: blocks})
	return nil
}

func (l *lexer) endDoc() {
	l.tagged = false
	if l.docStarted || l.docContent {
		l.doc++
	}
	l.docStarted, l.docContent = false, false
}

func (l *lexer) commentAt(i int) bool {
	return i < len(l.src) && l.src[i] == '#' && (i == 0 || isBlank(l.src[i-1]))
}

func errorAt(src string, off int, format string, args ...any) error {
	line, col := position(src, off)
	return fmt.Errorf("line %d, column %d: %s", line, col, fmt.Sprintf(format, args...))
}

// position returns the line and column of offset off.
func position(src string, off int) (line, col int) {
	bol := strings.LastIndexByte(src[:off], '\n') + 1
	return strings.Count(src[:off], "\n") + 1, utf8.RuneCountInString(src[bol:off]) + 1
}

// explicit reports whether s starts with the indicator of an explicit mapping key or value.
func explicit(s string) bool {
	return marker(s, "?") || marker(s, ":")
}

func marker(s, m string) bool {
	return strings.HasPrefix(s, m) && (len(s) == len(m) || isBlank(s[len(m)]))
}

func lineEnd(s string, i int) int {
	if n := strings.IndexByte(s[i:], '\n'); n >= 0 {
		return i + n
	}
	return len(s)
}

// blankLine reports whether s holds only YAML white space (unlike strings.TrimSpace, not U+00A0).
func blankLine(s string) bool { return strings.Trim(s, " \t\r") == "" }

func isBlank(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

func isFlowIndicator(c byte) bool { return strings.IndexByte(",[]{}", c) >= 0 }

// isDecl reports whether code is a local declaration without a body, like `local x = 1`.
func isDecl(code string) bool {
	t := code
	for { // skip white space and comments to the first token
		t = strings.TrimLeft(t, " \t\n")
		switch {
		case strings.HasPrefix(t, "#") || strings.HasPrefix(t, "//"):
			t = t[lineEnd(t, 0):]
			continue
		case strings.HasPrefix(t, "/*"):
			if end := strings.Index(t, "*/"); end >= 0 {
				t = t[end+2:]
				continue
			}
		}
		break
	}
	// The keyword "local", not an identifier starting with it.
	if !strings.HasPrefix(t, "local") || len(t) > 5 && (t[5] == '_' || unicode.IsLetter(rune(t[5])) || unicode.IsDigit(rune(t[5]))) {
		return false
	}
	return !parses(code)
}

// fieldSemi checks that a declaration (src up to offset end, plain without YAML blocks) is valid
// as a Jsonnet object local, which declares a single variable and has no ";". It returns the offset
// of a trailing ";" to drop, or -1.
func fieldSemi(src, plain string, end int, blocks []*block) (int, bool) {
	if parses("{" + plain + "\n}") {
		return -1, true
	}
	from := end - len(plain) // the ";" is after any YAML block, where src and plain match
	if len(blocks) > 0 {
		from = blocks[len(blocks)-1].end + 1
	}
	for i := end - 1; i >= from; i-- {
		if p := len(plain) - (end - i); src[i] == ';' && parses("{"+plain[:p]+plain[p+1:]+"\n}") {
			return i, true
		}
	}
	return -1, false
}

// parses reports whether code is syntactically valid Jsonnet. Unlike jsonnet.SnippetToAST, the
// formatter doesn't check variables, which may be bound by the code around an expression.
func parses(code string) bool {
	_, err := formatter.Format("", code, formatter.DefaultOptions())
	return err == nil
}

// withoutBlocks returns src[from:to] with the YAML blocks replaced by null, so that it is plain Jsonnet.
func withoutBlocks(src string, from, to int, blocks []*block) string {
	var b strings.Builder
	for _, blk := range blocks {
		b.WriteString(src[from:blk.start])
		b.WriteString("null")
		from = blk.end + 1
	}
	b.WriteString(src[from:to])
	return b.String()
}

// exprEnd returns the offset of the '}' closing the expression that starts at offset i, and the
// YAML blocks nested in it. It knows just enough Jsonnet lexical syntax (strings, comments, text
// blocks) to ignore braces inside them.
func exprEnd(s string, i int) (int, []*block, error) {
	start := i - 2
	var blocks []*block
	depth := 0
	for ; i < len(s); i++ {
		rest := s[i:]
		n := 0 // offset of the last byte of a token starting at i, -1 if unterminated
		switch {
		case strings.HasPrefix(rest, `\{`):
			j := i + 2
			for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
				j++
			}
			if j < len(s) && s[j] != '\n' && s[j] != '#' {
				return 0, nil, errorAt(s, i, `inside an expression, \{ starts a YAML block and must end its line`)
			}
			l, err := lex(s, i+2, true)
			if err != nil {
				return 0, nil, err
			}
			blocks = append(blocks, &block{start: i, end: l.i, lx: l})
			n = l.i - i
		case rest[0] == '{':
			depth++
		case rest[0] == '}':
			if depth == 0 {
				return i, blocks, nil
			}
			depth--
		case rest[0] == '"' || rest[0] == '\'':
			n = strEnd(rest, 1, rest[0], false)
		case strings.HasPrefix(rest, `@"`) || strings.HasPrefix(rest, `@'`):
			n = strEnd(rest, 2, rest[1], true)
		case strings.HasPrefix(rest, "|||"):
			n = textBlockEnd(rest)
		case rest[0] == '#' || strings.HasPrefix(rest, "//"):
			n = strings.IndexByte(rest, '\n')
		case strings.HasPrefix(rest, "/*"):
			n = strings.Index(rest[2:], "*/")
			if n >= 0 {
				n += 3
			}
		}
		if n < 0 {
			break
		}
		i += n
	}
	return 0, nil, errorAt(s, start, "unterminated ${")
}

// textBlockEnd returns the offset of the last "|" closing the Jsonnet text block that s starts with,
// or -1. As in Jsonnet, the block holds the lines starting with the indentation of its first line.
func textBlockEnd(s string) int {
	i := strings.IndexByte(s, '\n') + 1
	for i > 0 && i < len(s) && s[i] == '\n' {
		i++
	}
	indent := s[i : len(s)-len(strings.TrimLeft(s[i:], " \t"))]
	for i > 0 && indent != "" && i < len(s) {
		switch {
		case s[i] == '\n':
			i++
		case strings.HasPrefix(s[i:], indent):
			i = lineEnd(s, i) + 1
		default:
			j := len(s) - len(strings.TrimLeft(s[i:], " \t"))
			if strings.HasPrefix(s[j:], "|||") {
				return j + 2
			}
			return -1
		}
	}
	return -1
}

// strEnd returns the offset of the quote q closing a string whose body starts at offset start, or -1.
// Verbatim strings (Jsonnet @"...", YAML '...') escape quotes by doubling them instead of with backslashes.
func strEnd(s string, start int, q byte, verbatim bool) int {
	for j := start; j < len(s); j++ {
		switch {
		case !verbatim && s[j] == '\\':
			j++
		case s[j] == q && verbatim && j+1 < len(s) && s[j+1] == q:
			j++
		case s[j] == q:
			return j
		}
	}
	return -1
}

// Code generation.

type expr struct {
	from, to  int      // offsets of the code in the source
	blocks    []*block // YAML blocks nested in the code
	line, col int      // source position of the code
	doc       int      // for local declarations, see span.doc
	field     bool     // a local declaration in a mapping
	semi      int      // see span.semi
	code      string   // see span.code
}

type output struct {
	strings.Builder
	line, col int // position of the next character written
}

func (o *output) write(s string) {
	o.WriteString(s)
	if nl := strings.LastIndexByte(s, '\n'); nl >= 0 {
		o.line += strings.Count(s, "\n")
		o.col = 1 + utf8.RuneCountInString(s[nl+1:])
	} else {
		o.col += utf8.RuneCountInString(s)
	}
}

// A compiler compiles a unit of YAML: the file, or a block nested in an expression.
// Nested units share the output with the unit containing them.
type compiler struct {
	*output
	src, sentinel string // sentinel delimits placeholders; a character absent from the source

	exprs   []expr
	used    []bool
	locals  []expr
	lines   []int                // lines[n] is the source line of line n of the placeholder-substituted YAML
	aliases []*yaml.Node         // aliases being expanded
	places  map[*yaml.Node]place // where each node's text is, to resolve scalars in the same syntax
	copies  int                  // nodes copied by aliases
}

// unit compiles the YAML in src[from:to], as lexed by l.
func (c *compiler) unit(l *lexer, from, to int) error {
	text := c.build(from, to, l.spans)
	var docs []*yaml.Node
	dec := yaml.NewDecoder(strings.NewReader(text))
	for {
		var n yaml.Node
		err := dec.Decode(&n)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return c.fixLines(err)
		}
		docs = append(docs, &n)
	}
	if len(docs) != l.doc {
		return fmt.Errorf("internal error: found %d YAML documents, the YAML parser found %d", l.doc, len(docs))
	}
	c.places = map[*yaml.Node]place{}
	for _, d := range docs {
		c.place(d, false)
	}

	c.write("(")
	if err := c.stream(docs, !l.nested && (docStart.MatchString(text) || len(docs) > 1)); err != nil {
		return err
	}
	c.write(")")
	for i, used := range c.used {
		if !used {
			return fmt.Errorf("line %d: internal error: the YAML parser dropped this expression", c.exprs[i].line)
		}
	}
	return nil
}

// build replaces expressions with placeholders and drops local declarations, leaving plain YAML.
func (c *compiler) build(from, to int, spans []span) string {
	var b strings.Builder
	src := c.src
	line, bol, prev := strings.Count(src[:from], "\n")+1, strings.LastIndexByte(src[:from], '\n')+1, from
	c.lines = []int{0, line}
	advance := func(end int, keep bool) { // moves past src[prev:end], copying it if keep
		for k := prev; k < end; k++ {
			if src[k] == '\n' {
				line++
				bol = k + 1
				if keep {
					c.lines = append(c.lines, line)
				}
			}
		}
		if keep {
			b.WriteString(src[prev:end])
		}
		prev = end
	}
	for _, s := range spans {
		advance(s.start, true)
		e := expr{from: s.start + 2, to: s.end - 1, blocks: s.blocks, line: line, col: utf8.RuneCountInString(src[bol:s.start]) + 3, doc: s.doc, field: s.field, semi: s.semi, code: s.code}
		if s.code != "" {
			e.col++ // past the quote
		}
		if s.local && !s.field {
			c.locals = append(c.locals, e)
		} else {
			q := map[bool]string{true: `"`}[s.code != ""] // a quoted expression stays quoted, for YAML's tokens
			fmt.Fprintf(&b, "%s%s%d%s%s", q, c.sentinel, len(c.exprs), c.sentinel, q)
			if s.field {
				b.WriteString(":") // the key of a placeholder field, which the mapping turns into a local
			}
			c.exprs = append(c.exprs, e)
			c.used = append(c.used, false)
		}
		advance(s.end, false)
	}
	advance(to, true)
	return b.String()
}

// stream writes the documents. A local declaration scopes over the rest of the file, as in
// `local x = 1; rest`, so declarations met mid-stream open a nested `] + (local ...; [` scope.
func (c *compiler) stream(docs []*yaml.Node, isStream bool) error {
	if !isStream {
		for _, e := range c.locals {
			if e.doc > 0 { // after the document's end marker
				continue
			}
			if err := c.code(e, true); err != nil {
				return err
			}
		}
		if len(docs) == 0 {
			c.write("null")
			return nil
		}
		return c.node(docs[0])
	}
	open, scopes := false, 0
	for i := 0; i <= len(docs); i++ {
		for _, e := range c.locals {
			if e.doc != i {
				continue
			}
			if open {
				c.write("] + (")
				open = false
				scopes++
			}
			if err := c.code(e, true); err != nil {
				return err
			}
		}
		if i == len(docs) {
			continue
		}
		if !open {
			c.write("[")
			open = true
		}
		if err := c.node(docs[i]); err != nil {
			return err
		}
		c.write(", ")
	}
	if !open {
		c.write("[")
	}
	c.write("]" + strings.Repeat(")", scopes))
	return nil
}

// fixLines maps line numbers in YAML errors back to the source; they differ after multi-line expressions.
func (c *compiler) fixLines(err error) error {
	return errors.New(yamlLine.ReplaceAllStringFunc(err.Error(), func(m string) string {
		if n, _ := strconv.Atoi(m[len("line "):]); n < len(c.lines) {
			return "line " + strconv.Itoa(c.lines[n])
		}
		return m
	}))
}

func (c *compiler) errorf(n *yaml.Node, format string, args ...any) error {
	line := n.Line
	if line < len(c.lines) {
		line = c.lines[line]
	}
	return fmt.Errorf("line %d: %s", line, fmt.Sprintf(format, args...))
}

// code writes an expression, or a local declaration, first padding the output to its source
// position unless the output is already past it. YAML blocks in the code are compiled in place.
func (c *compiler) code(e expr, decl bool) error {
	c.padTo(e.line, e.col-1) // room for the opening parenthesis
	if !decl {
		c.write("(")
	}
	from := e.from
	for _, b := range e.blocks {
		c.write(c.src[from:b.start])
		nested := &compiler{output: c.output, src: c.src, sentinel: c.sentinel}
		if err := nested.unit(b.lx, b.start+2, b.end); err != nil {
			return err
		}
		from = b.end + 1
		c.padTo(position(c.src, from))
	}
	if e.field && e.semi >= 0 {
		c.write(c.src[from:e.semi])
		from = e.semi + 1
	}
	if e.code != "" {
		c.write(e.code)
	} else {
		c.write(c.src[from:e.to])
	}
	if !decl {
		c.write(")")
		return nil
	}
	if e.field {
		c.write(", ")
		return nil
	}
	// A declaration needs a ";", unless its last token (maybe followed by comments) is one already.
	if !parses(withoutBlocks(c.src, e.from, e.to, e.blocks) + "\nnull") {
		c.write(";")
	}
	c.write(" ")
	return nil
}

// A place is where the text of a node is: in a flow collection or not, as a sequence item or not.
type place struct{ flow, item, key bool }

// place records the places of the nodes under n.
func (c *compiler) place(n *yaml.Node, flow bool) {
	flow = flow || n.Style&yaml.FlowStyle != 0
	for i, m := range n.Content {
		c.places[m] = place{flow: flow, item: n.Kind == yaml.SequenceNode, key: n.Kind == yaml.MappingNode && i%2 == 0}
		c.place(m, flow)
	}
}

// align moves the output down to the source line of n, so that Jsonnet locations (errors, traceback)
// match the .jamlet file.
func (c *compiler) align(n *yaml.Node) {
	if n.Line < len(c.lines) {
		c.padTo(c.lines[n.Line], 0)
	}
}

// padTo moves the output to a source position, unless it is already past it.
func (c *compiler) padTo(line, col int) {
	if c.line < line {
		c.write(strings.Repeat("\n", line-c.line))
	}
	if c.line == line && c.col < col {
		c.write(strings.Repeat(" ", col-c.col))
	}
}

// tags are the explicit tags allowed on each kind of node; they don't change its meaning.
var tags = map[yaml.Kind]string{yaml.ScalarNode: "!!str", yaml.MappingNode: "!!map", yaml.SequenceNode: "!!seq"}

// copied counts a node copied by an alias, bounding them.
func (c *compiler) copied() error {
	if len(c.aliases) > 0 {
		if c.copies++; c.copies > maxCopies {
			return c.errorf(c.aliases[0], "aliases expand to more than %d nodes", maxCopies)
		}
	}
	return nil
}

func (c *compiler) node(n *yaml.Node) error {
	c.align(n)
	if n.Style&yaml.TaggedStyle != 0 && n.Tag != tags[n.Kind] {
		return c.errorf(n, "unsupported tag %s", n.Tag)
	}
	if err := c.copied(); err != nil {
		return err
	}
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			c.write("null")
			return nil
		}
		return c.node(n.Content[0])
	case yaml.AliasNode:
		// An alias is a copy of the anchored node; its expressions are evaluated at the alias position.
		for _, a := range c.aliases {
			if a.Alias == n.Alias {
				return c.errorf(n, "anchor %q contains an alias to itself", n.Value)
			}
		}
		c.aliases = append(c.aliases, n)
		err := c.node(n.Alias)
		c.aliases = c.aliases[:len(c.aliases)-1]
		return err
	case yaml.SequenceNode:
		c.write("[")
		for _, e := range n.Content {
			if err := c.node(e); err != nil {
				return err
			}
			c.write(", ")
		}
		c.write("]")
		return nil
	case yaml.MappingNode:
		return c.mapping(n)
	case yaml.ScalarNode:
		return c.scalar(n, false)
	}
	return c.errorf(n, "unsupported YAML node")
}

func (c *compiler) mapping(n *yaml.Node) error {
	c.write("{")
	// An anchor names the mapping in the Jsonnet code inside it. Other anchors, like
	// &default-labels, are just YAML anchors.
	if isIdentifier(n.Anchor) {
		c.write("local " + n.Anchor + " = self, ")
	}
	for i := 0; i < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if k.Kind == yaml.AliasNode {
			c.align(k) // the field is where the alias is
			k = k.Alias
		}
		if e, ok := c.fieldLocal(k); ok {
			if err := c.copied(); err != nil {
				return err
			}
			if len(c.aliases) > 0 && strings.Contains(c.src[e.from:e.to], "\n") {
				return c.errorf(k, "an alias cannot repeat a multi-line local declaration")
			}
			if v.Kind != yaml.ScalarNode || v.Tag != "!!null" || v.Value != "" || v.Anchor != "" {
				return c.errorf(v, "unexpected content after a local declaration, at deeper indentation")
			}
			if err := c.code(e, true); err != nil {
				return err
			}
			continue
		}
		if k.Tag == "!!merge" {
			return c.errorf(k, "merge keys (<<) are not supported")
		}
		if k.Kind != yaml.ScalarNode {
			return c.errorf(k, "only scalar mapping keys are supported")
		}
		if err := c.scalar(k, true); err != nil {
			return err
		}
		c.write(": ")
		if err := c.node(v); err != nil {
			return err
		}
		c.write(", ")
	}
	c.write("}")
	return nil
}

// fieldLocal returns the local declaration that a placeholder key stands for, if any.
func (c *compiler) fieldLocal(k *yaml.Node) (expr, bool) {
	parts := strings.Split(k.Value, c.sentinel)
	if k.Kind != yaml.ScalarNode || k.Style != 0 || len(parts) != 3 || parts[0] != "" || parts[2] != "" {
		return expr{}, false
	}
	i, err := strconv.Atoi(parts[1])
	if err != nil || i >= len(c.exprs) || !c.exprs[i].field {
		return expr{}, false
	}
	c.used[i] = true
	return c.exprs[i], true
}

// keywords are the Jsonnet keywords, which can't name variables.
var keywords = map[string]bool{
	"assert": true, "else": true, "error": true, "false": true, "for": true, "function": true,
	"if": true, "import": true, "importbin": true, "importstr": true, "in": true, "local": true,
	"null": true, "self": true, "super": true, "tailstrict": true, "then": true, "true": true,
}

var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func isIdentifier(s string) bool { return identifier.MatchString(s) && !keywords[s] }

func (c *compiler) scalar(n *yaml.Node, key bool) error {
	c.align(n)
	parts := strings.Split(n.Value, c.sentinel)
	if len(parts) == 1 {
		switch {
		case n.Style == 0 && strings.Contains(n.Value, "${"):
			return c.errorf(n, "${ inside a plain scalar: an expression must be a whole YAML value")
		case n.Style&yaml.TaggedStyle != 0 && n.Tag != tags[n.Kind]:
			return c.errorf(n, "unsupported tag %s", n.Tag)
		case n.Style&yaml.TaggedStyle == 0 && n.Style&(yaml.SingleQuotedStyle|yaml.DoubleQuotedStyle) != 0:
			// The lexer turns well-formed quoted expressions on one line into placeholders.
			if _, ok, err := quotedExpr(n.Value); err != nil {
				return c.errorf(n, "%v", err)
			} else if ok {
				return c.errorf(n, "%s", errMultiLineQuoted)
			}
		}
		lit, err := literal(n, key, c.places[n])
		if err != nil {
			return c.errorf(n, "%v", err)
		}
		c.write(lit)
		return nil
	}
	// In a block scalar, white space (which the lexer checked) may surround the expression.
	if len(parts) != 3 || strings.Trim(parts[0], " \t\n") != "" || strings.Trim(parts[2], " \t\n") != "" {
		return c.errorf(n, "an expression must be a whole, unquoted, untagged YAML value")
	}
	i, err := strconv.Atoi(parts[1])
	if err != nil || i >= len(c.exprs) {
		return c.errorf(n, "corrupted placeholder %q", parts[1])
	}
	code := c.exprs[i].code
	switch quoted := n.Style&^yaml.TaggedStyle == yaml.DoubleQuotedStyle && code != ""; {
	case quoted && n.Style&yaml.TaggedStyle != 0:
		if n.Tag != tags[n.Kind] {
			return c.errorf(n, "unsupported tag %s", n.Tag)
		}
		c.used[i] = true
		c.write(quote("${" + code + "}")) // !!str keeps the string
		return nil
	case quoted && isDecl(code):
		return c.errorf(n, "a local declaration cannot be quoted")
	case !quoted && (code != "" || n.Style != 0 && n.Style != yaml.LiteralStyle && n.Style != yaml.FoldedStyle):
		return c.errorf(n, "an expression must be a whole, unquoted, untagged YAML value")
	}
	// Re-emitting a multi-line expression would push the output past the source lines that follow,
	// so later errors would report wrong lines.
	if c.used[i] && strings.Contains(c.src[c.exprs[i].from:c.exprs[i].to], "\n") {
		return c.errorf(n, "an alias cannot repeat a multi-line expression; bind it with a local declaration instead")
	}
	c.used[i] = true
	if !key {
		return c.code(c.exprs[i], false)
	}
	// A computed field name; null omits the field.
	c.write("[")
	if err := c.code(c.exprs[i], false); err != nil {
		return err
	}
	c.write("]")
	return nil
}

// quoted turns a quoted scalar ending at l.i into an expression if its whole value is one, like
// '${1 + 1}', on one line. The code is the value after YAML unquoting. The compiler reports the
// rest: malformed expressions, and those spanning lines, whose code would lose its source lines.
// It also knows which node a tag is on, so it handles !!str.
func (l *lexer) quoted(start int) {
	raw := l.src[start:l.i]
	if !strings.Contains(raw, "${") && !strings.Contains(raw, `\`) || strings.Contains(raw, "\n") {
		return
	}
	var v string
	if yaml.Unmarshal([]byte(raw), &v) != nil {
		return // left for the YAML parser to report
	}
	if code, ok, _ := quotedExpr(v); ok && !strings.Contains(code, "\n") {
		l.spans = append(l.spans, span{start: start, end: l.i, code: code})
	}
}

const errMultiLineQuoted = "a quoted expression must be on one line; use a block expression for multi-line code"

// quotedExpr returns the code of a quoted scalar whose whole value is an expression, like '${1 + 1}'.
// Other values, like 'echo ${HOME}' or an unterminated '${', are strings.
func quotedExpr(v string) (string, bool, error) {
	if !strings.HasPrefix(v, "${") {
		return "", false, nil
	}
	end, blocks, err := exprEnd(v, 2)
	switch {
	case err != nil && strings.HasSuffix(v, "}"):
		// Shaped like an expression, so most likely meant as one. Positions in err are within v.
		msg := err.Error()
		return "", false, errors.New(msg[strings.Index(msg, ": ")+2:])
	case err != nil || end != len(v)-1:
		return "", false, nil
	case len(blocks) > 0:
		return "", false, errors.New(`a quoted expression cannot contain a YAML block (\{)`)
	case strings.TrimSpace(v[2:end]) == "":
		return "", false, errors.New("empty expression")
	}
	return v[2:end], true, nil
}

// literal returns the JSON encoding of a scalar without expressions.
// Plain scalars are resolved with the YAML 1.1 rules that kubectl and std.parseYaml use
// (`yes` is true, `0777` is octal, keys are stringified), by letting sigs.k8s.io/yaml parse them
// again, in the same syntax as at its place p, since that changes what is valid: `-` is a value in
// `[-]` but not in `v: -`, `?x` is a string in `v: ?x` but a key in `[?x]`. Through aliases, a
// scalar can be used as a key or a value whatever its place.
func literal(n *yaml.Node, key bool, p place) (string, error) {
	// Multi-line plain scalars are always strings and cannot be re-parsed on one line.
	if n.Style != 0 || strings.Contains(n.Value, "\n") {
		return quote(n.Value), nil
	}
	lit, err := resolve(n.Value, key, p)
	if err != nil && !key && p.key {
		// An alias uses a key as a value, but its text may only be valid as a key (like `-`):
		// then it is the key's string.
		return resolve(n.Value, true, p)
	}
	return lit, err
}

// resolve returns the JSON encoding of plain scalar text, parsed as a key or a value at place p.
func resolve(text string, key bool, p place) (string, error) {
	var context string
	switch {
	case key && p.flow:
		context = "{%s: 0}"
	case key:
		context = "%s: 0"
	case p.item && p.flow:
		context = "[%s]"
	case p.item:
		context = "- %s"
	case p.flow:
		context = "{v: %s}"
	default:
		context = "v: %s"
	}
	j, err := k8syaml.YAMLToJSON([]byte(fmt.Sprintf(context, text)))
	if err != nil {
		return "", err
	}
	var seq []json.RawMessage
	if json.Unmarshal(j, &seq) == nil && len(seq) == 1 {
		return string(seq[0]), nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(j, &m); err != nil || len(m) != 1 {
		return "", fmt.Errorf("cannot resolve scalar %q", text)
	}
	for k, v := range m {
		if key {
			return quote(k), nil
		}
		return string(v), nil
	}
	return "", nil
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
