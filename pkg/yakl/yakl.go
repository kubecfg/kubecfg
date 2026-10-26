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

// Package yakl compiles yakl documents (YAML whose nodes can be ${jsonnet} expressions)
// into plain Jsonnet. See docs/Advanced-Usage/yakl.md for the language spec.
package yakl

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	jsonnet "github.com/google/go-jsonnet"
	"github.com/google/go-jsonnet/formatter"
	"go.yaml.in/yaml/v3"
	k8syaml "sigs.k8s.io/yaml"
)

// Ext is the file extension of yakl documents.
const Ext = ".yakl"

// maxCopies bounds the nodes that aliases copy, so that nested aliases ("billion laughs") can't blow up.
const maxCopies = 100_000

var (
	// Same document start marker detection as std.parseYaml.
	docStart = regexp.MustCompile(`(?m)^---(\s|$)`)
	// A line, after the first, that starts a document or ends one.
	docMarker = regexp.MustCompile(`\n(---|\.\.\.)(\s|$)`)
	yamlLine  = regexp.MustCompile(`line (\d+)`)
)

// Compile translates a yakl document into equivalent Jsonnet code.
// Code keeps its source line in the output, so Jsonnet errors point at the right line of the .yakl file.
func Compile(src string) (string, error) {
	return compile(src, false)
}

// KubecfgLocal is the code that predefines kubecfg (RFC06): the copy of kubecfg.libsonnet built
// into kubecfg, whatever the search path holds. It ends in a space, not a line break, so code after
// it keeps its line numbers.
const KubecfgLocal = "local kubecfg = import 'internal:///kubecfg.libsonnet'; "

// kubecfgVar is another name for the predefined kubecfg in a yakl file, for code the compiler
// writes: the file's own names can hide kubecfg, but not kubecfgVar. yaklKubecfgLocal is
// KubecfgLocal for a yakl file, declaring both.
const (
	kubecfgVar       = "yakl__kubecfg"
	yaklKubecfgLocal = "local kubecfg = import 'internal:///kubecfg.libsonnet', " + kubecfgVar + " = kubecfg; "
)

// PredefineKubecfg returns Jsonnet code with KubecfgLocal in front if the code reads kubecfg
// without declaring it, and otherwise the code as it is. kubecfg calls it on code that isn't in a
// Jsonnet file, like the code of command-line flags; Compile does the same for a yakl document.
func PredefineKubecfg(code string) string {
	return predefineKubecfg(code, 0, KubecfgLocal)
}

// predefineKubecfg is PredefineKubecfg, with local written at offset at. It's added when the code
// reads kubecfg or kubecfgVar without declaring it.
func predefineKubecfg(code string, at int, local string) string {
	if strings.Contains(code, "kubecfg") {
		if free := freeVars(code); free["kubecfg"] || free[kubecfgVar] {
			return code[:at] + local + code[at:]
		}
	}
	return code
}

// Desugar translates a yakl document into equivalent Jsonnet code meant for people to read:
// formatted, with a field per line, rather than keeping source lines. If the code doesn't parse,
// it returns it unformatted, with the Jsonnet syntax error, whose position ("line:column") is in
// the yakl document.
func Desugar(src string) (string, error) {
	code, err := compile(src, true)
	if err != nil {
		return "", err
	}
	// Readable code drops the parentheses around expressions, which could make a malformed one,
	// like ${1, b: 2}, pass for something else. Compiled code keeps them, and the source lines, so
	// its syntax errors point into the document.
	compiled, err := Compile(src)
	if err != nil {
		return "", err
	}
	if _, err := formatter.Format("", compiled, formatter.DefaultOptions()); err != nil {
		return code, err
	}
	formatted, err := formatter.Format("", code, formatter.DefaultOptions())
	if err != nil {
		return code, err
	}
	return formatted, nil
}

func compile(src string, readable bool) (string, error) {
	c, err := compiled(src, readable, false)
	if err != nil {
		return "", err
	}
	local := yaklKubecfgLocal
	if readable {
		local = strings.ReplaceAll(local, ", ", ",\n") + "\n" // a name per line, as the formatter writes it
	}
	return predefineKubecfg(c.String(), c.localsAt, local), nil
}

// compiled returns the compiler that has compiled src.
func compiled(src string, readable, canonical bool) (*compiler, error) {
	if !utf8.ValidString(src) {
		return nil, errors.New("not valid UTF-8")
	}
	// yaml.v3 skips a byte order mark in its reader and another at the start of a line.
	src = strings.TrimLeft(src, "\uFEFF")
	// YAML also breaks lines at a lone \r; normalize so that the lexer only deals with \n.
	src = strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(src)
	if i := strings.IndexAny(src, "\u0085\u2028\u2029"); i >= 0 {
		return nil, fmt.Errorf("line %d: unsupported Unicode line separator", strings.Count(src[:i], "\n")+1)
	}
	// yaml.v3 rejects these anywhere, but it doesn't see the code of expressions.
	if i := strings.IndexFunc(src, func(r rune) bool {
		return r < 0x20 && r != '\t' && r != '\n' || r >= 0x7F && r <= 0x9F || r == 0xFFFE || r == 0xFFFF
	}); i >= 0 {
		return nil, fmt.Errorf("line %d: control characters are not allowed", strings.Count(src[:i], "\n")+1)
	}
	// Like std.parseYaml and kubecfg's YAML reader, which keep the final line break of a block
	// scalar ending the file.
	if src != "" && !strings.HasSuffix(src, "\n") {
		src += "\n"
	}
	c := &compiler{output: &output{line: 1, col: 1}, src: src, readable: readable, canonical: canonical}
	for r := rune(0xE000); r <= 0xF8FF && c.sentinel == ""; r++ {
		if !strings.ContainsRune(src, r) {
			c.sentinel = string(r)
		}
	}
	if c.sentinel == "" {
		return nil, errors.New("source uses every private-use character, cannot pick a placeholder")
	}

	l, err := lex(src, 0, false)
	if err != nil {
		return nil, err
	}
	if err := c.unit(l, 0, len(src)); err != nil {
		return nil, err
	}
	return c, nil
}

// Lexing.
//
// The lexer knows just enough YAML to find the ${...} expressions that start a node, leaving alone
// comments, quoted and block scalars (except a block scalar holding an expression). Its findings are double-checked against yaml.v3, which parses
// the result: any disagreement is reported as an error rather than silently producing wrong output.

type span struct {
	start, end int      // offsets of "${" and after "}"
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
	parent    int  // column that continuation lines of a block or plain scalar must exceed
	plainOpen bool // the previous line ended inside a plain scalar

	// Documents are counted like yaml.v3 does, to know which ones start with an explicit "---".
	doc                    int
	explicit               []bool // explicit[i]: document i starts with "---"
	docStarted, docContent bool   // the current document has an explicit "---", has content

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
		case rest[0] == '#':
			l.plainOpen = false // a comment line ends one, and changes nothing else
		case l.plainOpen && indent > l.parent && !explicit(rest):
			if err := l.plain(); err != nil {
				return err
			}
		case !l.docContent:
			// The root of a document: the continuation lines of a plain scalar may outdent it.
			l.node, l.parent, l.plainOpen, l.propCol = true, -1, false, -1
		case l.node && indent > l.parent:
			// The value of the key or item left open above, whose column l.parent already is.
			l.plainOpen, l.propCol = false, -1
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
		if c != '!' {
			// Like yaml.v3, which reads what follows the name, like the ":" of `&a: b`, as the next token.
			for l.i++; l.i < len(l.src) && isAnchorChar(l.src[l.i]); l.i++ {
			}
			return nil
		}
		if strings.HasPrefix(rest, "!<") { // a verbatim tag, which may hold flow indicators
			l.i += strings.IndexByte(rest, '>') + 1
		}
		for l.i < len(l.src) && !isBlank(l.src[l.i]) && !(l.flow > 0 && isFlowIndicator(l.src[l.i])) {
			l.i++
		}
	case c == '-' && sep && l.flow == 0:
		l.parent = l.i - l.bol
		l.i++
	case (c == '?' || c == ':') && (sep || l.flow > 0):
		if l.flow == 0 { // an explicit key or value: the mapping's indentation
			l.parent = l.i - l.bol
		}
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
	l.nodeCol, l.node = i-l.bol, false
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

// plain scans a plain scalar up to the end of the line or the first indicator ending it. A ${ after
// its start is literal text.
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
		}
	}
	return nil
}

// blockScalar skips a literal (|) or folded (>) block scalar: its header line and its content lines,
// which are those indented more than the parent node. A block scalar whose content is a single
// expression, apart from white space around it, is that expression.
func (l *lexer) blockScalar() error {
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
	if first < 0 || !strings.HasPrefix(l.src[first:], "${") {
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
		return errorAt(l.src, first, "%s", errDecl)
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
	// YAML tools would end the document there, and see the expression's code as YAML.
	if m := docMarker.FindStringSubmatchIndex(code); m != nil {
		return errorAt(l.src, start+2+m[2], "a line starting with %s in an expression ends the YAML document; indent it", code[m[2]:m[3]])
	}
	l.i = end + 1
	j := l.i
	for j < len(l.src) && (l.src[j] == ' ' || l.src[j] == '\t' || l.src[j] == '\r') {
		j++
	}
	eol := j == len(l.src) || l.src[j] == '\n' || l.commentAt(j)

	if isDecl(withoutBlocks(l.src, start+2, end, blocks)) {
		return errorAt(l.src, start, "%s", errDecl)
	}
	// Only a key indicator, flow punctuation, a comment or the end of the line may follow.
	if !eol && l.src[j] != ':' && !(l.flow > 0 && strings.IndexByte(",]}", l.src[j]) >= 0) {
		return errorAt(l.src, j, "unexpected text after an expression, which must be a whole YAML value (build strings in Jsonnet)")
	}
	l.docContent = true
	l.startNode(start)
	// To YAML, it is a plain scalar, which the following lines may continue.
	l.plainOpen = l.flow == 0 && (j == len(l.src) || l.src[j] == '\n')
	l.spans = append(l.spans, span{start: start, end: l.i, blocks: blocks})
	return nil
}

// quoted turns a quoted scalar ending at l.i into an expression if its whole value is one, like
// '${1 + 1}'. The code is the value after YAML unquoting, which folds its lines. The compiler
// reports the rest: malformed expressions, and other strings starting with ${.
func (l *lexer) quoted(start int) {
	raw := l.src[start:l.i]
	if !strings.Contains(raw, "${") && !strings.Contains(raw, `\`) {
		return
	}
	var v string
	if yaml.Unmarshal([]byte(raw), &v) != nil {
		return // left for the YAML parser to report
	}
	if code, ok, _ := quotedExpr(strings.Trim(v, " \t\n")); ok {
		l.spans = append(l.spans, span{start: start, end: l.i, code: code})
	}
}

func (l *lexer) endDoc() {
	if l.docStarted || l.docContent {
		l.doc++
		l.explicit = append(l.explicit, l.docStarted)
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

// isAnchorChar reports whether c can be in an anchor name: yaml.v3, kubectl's YAML parser and
// libyaml allow exactly these.
func isAnchorChar(c byte) bool {
	return c == '-' || c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isFlowIndicator(c byte) bool { return strings.IndexByte(",[]{}", c) >= 0 }

const errDecl = "${local ...} declarations are gone: declare a local with a key like '&x': value, in the mapping " +
	"that uses it, or in a document of such keys for the whole file"

// isDecl reports whether code is a local declaration without a body, like `local x = 1`.
func isDecl(code string) bool {
	return startsWith(code, "local") && !parses(code)
}

// startsWith reports whether the first token of Jsonnet code is the keyword kw.
func startsWith(code, kw string) bool {
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
	// The keyword, not an identifier starting with it.
	n := len(kw)
	return strings.HasPrefix(t, kw) && !(len(t) > n && (t[n] == '_' || unicode.IsLetter(rune(t[n])) || unicode.IsDigit(rune(t[n]))))
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
	depth, parens := 0, 0
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
		case rest[0] == '(':
			parens++
		case rest[0] == ')':
			// The expression is compiled in parentheses, which it must not close.
			if parens == 0 {
				return 0, nil, errorAt(s, i, "unmatched ) in an expression")
			}
			parens--
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
	code      string   // for a quoted expression, its code after YAML unquoting
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

	exprs     []expr
	nodes     []*yaml.Node                   // the scalar that each expression is, once written
	dropped   map[int]bool                   // expressions left out on purpose: in merged entries that own keys override
	merged    map[*yaml.Node][]entry         // the entries of each mapping, see entries
	vars      map[*yaml.Node]map[string]bool // the variables that each key expression reads, see keyVars
	lines     []int                          // lines[n] is the source line of line n of the placeholder-substituted YAML
	aliases   []*yaml.Node                   // aliases being expanded
	places    map[*yaml.Node]place           // where each node's text is, to resolve scalars in the same syntax
	copies    int                            // nodes copied by aliases
	noDollar  bool                           // compiling a file local where there is no file object: $ is an error
	localsAt  int                            // where the file's code starts, after its first comments: see KubecfgLocal
	readable  bool                           // see Desugar
	canonical bool                           // writes expressions as Format formats them, and no YAML comments, to compare meanings
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
	c.dropped = map[int]bool{}
	c.merged = map[*yaml.Node][]entry{}
	c.vars = map[*yaml.Node]map[string]bool{}
	for _, d := range docs {
		c.place(d, false)
	}

	// A nested unit is a value inside Jsonnet code, like a parenthesized expression.
	paren := l.nested || !c.readable
	if paren {
		c.write("(")
	}
	var err error
	if l.nested {
		err = c.block(docs)
	} else {
		err = c.file(docs, l.explicit, docStart.MatchString(text))
	}
	if err != nil {
		return err
	}
	if paren {
		c.write(")")
	}
	for i, n := range c.nodes {
		if n == nil && !c.dropped[i] {
			return fmt.Errorf("line %d: internal error: the YAML parser dropped this expression", c.exprs[i].line)
		}
	}
	return nil
}

// build replaces expressions with placeholders, leaving plain YAML.
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
		e := expr{from: s.start + 2, to: s.end - 1, blocks: s.blocks, line: line, col: utf8.RuneCountInString(src[bol:s.start]) + 3, code: s.code}
		if s.code != "" {
			e.col++ // past the quote
		}
		q := map[bool]string{true: `"`}[s.code != ""] // a quoted expression stays quoted, for YAML's tokens
		fmt.Fprintf(&b, "%s%s%d%s%s", q, c.sentinel, len(c.exprs), c.sentinel, q)
		c.exprs = append(c.exprs, e)
		c.nodes = append(c.nodes, nil)
		advance(s.end, false)
	}
	advance(to, true)
	return b.String()
}

// Documents and names.

// A doc is a document of the file, and what its name key makes of it.
type doc struct {
	root   *yaml.Node // nil if empty
	at     *yaml.Node // the node to report errors at
	kind   docKind
	name   string // the field name
	plus   bool   // the field extends the inherited one: name+:
	colons int    // 1 to 3, as after a Jsonnet field name

	head, foot []string // comments before and after the document, the YAML document's first
}

type docKind int

const (
	unnamedDoc    docKind = iota
	fileLocalsDoc         // only local keys ('&x': ...), each a file local
	fieldDoc              // with a name key (^^: web): a field of the file
)

// unnamed is the field holding the documents without a name, in a file with names.
const unnamed = "unnamed"

// nameKey matches a document's name key: ^^, and the + and colons of the field it gives.
var nameKey = regexp.MustCompile(`^\^\^\+?:{0,2}$`)

// classify finds the kind and name of a document.
func (c *compiler) classify(d *yaml.Node) (doc, error) {
	r := doc{at: d, head: []string{d.HeadComment}, foot: []string{d.FootComment}}
	if len(d.Content) == 0 {
		return r, nil
	}
	root := d.Content[0]
	r.root, r.at = root, root
	r.head = append(r.head, root.HeadComment, root.LineComment)
	r.foot = append(r.foot, root.FootComment)
	if t := root.Tag; root.Style&yaml.TaggedStyle != 0 && strings.HasPrefix(t, "!") && !strings.HasPrefix(t, "!!") {
		return r, c.errorf(root, "unsupported tag %s; to name a document, write %s among its top-level keys", t, writeNameKey(t[1:]))
	}
	if err := c.checkTag(root); err != nil { // a root mapping isn't written by value
		return r, err
	}
	if root.Kind != yaml.MappingNode {
		return r, nil
	}
	var key, value *yaml.Node // the name key
	locals := 0
	rest := root.Content[:0:0]
	for i := 0; i < len(root.Content); i += 2 {
		k, v := root.Content[i], root.Content[i+1]
		switch {
		case k.Kind != yaml.ScalarNode:
		case nameKey.MatchString(k.Value):
			if key != nil {
				return r, c.errorf(k, "a document has one name key, but this one has %q on line %d too", key.Value, c.srcLine(key))
			}
			key, value = k, v
			continue
		case strings.HasPrefix(k.Value, "&"):
			locals++
		}
		rest = append(rest, k, v)
	}
	switch {
	case key != nil:
		// The name key names the document; it isn't one of its fields.
		root.Content, r.at = rest, key
		r.head = append(r.head, key.HeadComment, key.LineComment, value.LineComment)
		r.foot = append(r.foot, key.FootComment, value.FootComment)
		return r, c.name(&r, key, value)
	case locals > 0 && 2*locals == len(root.Content):
		r.kind = fileLocalsDoc
	}
	return r, nil
}

// extension returns the <<+ entry of document d if <<+ and local keys are all d holds. In a file
// of several documents, the first such document says what the file extends, as <<+ does for a
// mapping, and its local keys are file locals.
func extension(d *yaml.Node) *entry {
	if len(d.Content) == 0 {
		return nil
	}
	root := d.Content[0]
	if root.Kind != yaml.MappingNode || root.Style&yaml.TaggedStyle != 0 {
		return nil
	}
	var ext *entry
	for i := 0; i < len(root.Content); i += 2 {
		switch k := root.Content[i]; {
		case k.Kind == yaml.ScalarNode && k.Value == "<<+":
			ext = &entry{k: k, v: root.Content[i+1]}
		case k.Kind != yaml.ScalarNode || !strings.HasPrefix(k.Value, "&"):
			return nil
		}
	}
	return ext
}

// name reads a document's name key k, like ^^: web. The value is the field's name, as text, so
// that the file's shape is known without evaluating it; the key adds the field's + and colons, as
// any key does: ^^:: config is hidden.
func (c *compiler) name(d *doc, k, v *yaml.Node) error {
	if v.Kind == yaml.ScalarNode && (strings.Contains(v.Value, c.sentinel) || strings.HasPrefix(v.Value, "${")) {
		return c.errorf(v, "the key %q takes the document's name as text, not as an expression: the file's shape is known before evaluating it", k.Value)
	}
	var name string
	if v.Kind == yaml.ScalarNode && !yaml11Bool(v) {
		if lit, err := literal(v, false, c.places[v]); err == nil {
			_ = json.Unmarshal([]byte(lit), &name) // not a string: no name
		}
	}
	trimmed := strings.TrimRight(name, "+:")
	switch {
	case name == "":
		return c.errorf(v, "the key %q takes the document's name, as a string (quote a number: '123')", k.Value)
	case trimmed != name:
		return c.errorf(v, "the name %q can't end in + or :; they go on the key, as in %s", name, writeNameKey(name))
	case name == unnamed:
		return c.errorf(v, "the name %q is reserved for the documents without a name", unnamed)
	}
	d.kind, d.name = fieldDoc, name
	d.plus = strings.Contains(k.Value, "+")
	d.colons = 1 + strings.Count(k.Value, ":")
	return nil
}

// writeNameKey returns the name key, as written in YAML, that gives the field f written as in
// Jsonnet: config+:: gives ^^+:: config.
func writeNameKey(f string) string {
	name := strings.TrimRight(f, ":")
	colons := max(len(f)-len(name), 1)
	name, plus := strings.CutSuffix(name, "+")
	return "^^" + map[bool]string{true: "+"}[plus] + strings.Repeat(":", colons) + " " + name
}

// localKey checks a local key, '&name': value.
func (c *compiler) localKey(k, v *yaml.Node) error {
	name := k.Value[1:]
	switch {
	case !isIdentifier(name):
		msg := fmt.Sprintf("the key %q declares a local, so %q must be a Jsonnet variable name, not a keyword or std", k.Value, name)
		if trimmed := strings.TrimRight(name, "+:"); !strings.HasPrefix(name, "-") && trimmed != "" {
			msg += fmt.Sprintf("; to name a document, write ^^%s: %s among its top-level keys", name[len(trimmed):], trimmed)
		}
		return c.errorf(k, "%s", msg)
	case v.Kind == yaml.ScalarNode && v.Tag == "!!null" && v.Value == "" && v.Style == 0 && v.Anchor == "":
		return c.errorf(k, "the local key %q has no value; to name the document, write ^^: %s", k.Value, name)
	}
	return nil
}

// file writes the documents of a file. Without names, they are the document, or a list of them,
// like std.parseYaml, after file locals. With names, they are fields of an object.
func (c *compiler) file(yamlDocs []*yaml.Node, explicit []bool, docStart bool) error {
	docs := make([]doc, len(yamlDocs))
	var fileLocals, objectFile bool
	fields := map[string][]int{}
	locals := map[string]*yaml.Node{} // file locals
	addLocal := func(name string, at *yaml.Node) error {
		if prev := locals[name]; prev != nil {
			return c.errorf(at, "the local %q is also declared on line %d", name, c.srcLine(prev))
		}
		locals[name] = at
		return nil
	}
	var ext *entry // what the file extends
	for i, d := range yamlDocs {
		if e := extension(d); e != nil && len(yamlDocs) > 1 {
			if i > 0 {
				return c.errorf(e.k, "<<+ in a document of its own extends the file, so the document must "+
					"be the first; for a document that is an expression's object, write the expression")
			}
			if err := c.anchored(e.v, "", "in a source of <<+"+noPlace, false, false, nil, nil); err != nil {
				return err
			}
			ext, objectFile = e, true
			root := d.Content[0]
			var rest []*yaml.Node // the local keys
			for j := 0; j < len(root.Content); j += 2 {
				if root.Content[j] != e.k {
					rest = append(rest, root.Content[j], root.Content[j+1])
				}
			}
			root.Content = rest
		}
		var err error
		if docs[i], err = c.classify(d); err != nil {
			return err
		}
		if i == 0 && ext != nil {
			docs[i].kind = fileLocalsDoc
		}
		switch d := docs[i]; d.kind {
		case fileLocalsDoc:
			fileLocals = true
			if isIdentifier(d.root.Anchor) {
				return c.errorf(d.root, "&%s in a document of local keys%s, so its anchor can't declare a local", d.root.Anchor, noPlace)
			}
			for j := 0; j < len(d.root.Content); j += 2 {
				k, v := d.root.Content[j], d.root.Content[j+1]
				if err := c.localKey(k, v); err != nil {
					return err
				}
				if err := c.anchored(v, "", "in a document of local keys"+noPlace, false, false, nil, nil); err != nil {
					return err
				}
				if err := addLocal(k.Value[1:], k); err != nil {
					return err
				}
			}
		case fieldDoc:
			objectFile = true
			fields[d.name] = append(fields[d.name], i)
		}
	}
	// One name may be given by two documents: a plain one and one extending it (^^+: name).
	for name, idx := range fields {
		if len(idx) == 1 {
			continue
		}
		a, b := docs[idx[0]], docs[idx[1]]
		if len(idx) > 2 || a.plus == b.plus || a.plus && a.colons != 1 || b.plus && b.colons != 1 {
			last := docs[idx[len(idx)-1]]
			return c.errorf(last.at, "the name %q is already given on line %d: a name can be given twice only "+
				"by a key without + and a plain ^^+, as in ^^: %[1]s and ^^+: %[1]s", name, c.srcLine(docs[idx[0]].at))
		}
	}

	// The comments at the top of the file stay there.
	if len(docs) > 0 {
		if c.comment(docs[0].head[0]) {
			c.write("\n") // YAML gives the document a comment that a blank line separates from it
		}
		docs[0].head[0] = ""
	}
	c.localsAt = c.Len()
	if objectFile {
		return c.objectFile(docs, fields, ext)
	}

	var body []int // the documents that aren't file locals
	for i, d := range docs {
		if d.kind == unnamedDoc {
			body = append(body, i)
		}
	}
	list := docStart || len(docs) > 1
	if fileLocals {
		// A "---" that only separates file locals from the document doesn't make a list.
		list = len(body) > 1 || len(body) == 1 && body[0] == 0 && explicit[0]
	}
	if fileLocals {
		c.noDollar = true
		n := 0
		for _, d := range docs {
			if d.kind == fileLocalsDoc {
				n += len(d.root.Content) / 2
			}
		}
		i := 0
		for _, d := range docs {
			if d.kind != fileLocalsDoc {
				continue
			}
			c.comment(d.head...)
			for j := 0; j < len(d.root.Content); j += 2 {
				k, v := d.root.Content[j], d.root.Content[j+1]
				c.comment(k.HeadComment, k.LineComment)
				c.align(k)
				if i == 0 {
					c.write("local ")
				}
				c.write(k.Value[1:] + " = ")
				if err := c.value(v); err != nil {
					return err
				}
				if i++; i < n {
					c.end(nil, ", ", v.LineComment)
				} else {
					c.end(nil, "; ", v.LineComment)
				}
				c.comment(k.FootComment, v.FootComment)
			}
			c.comment(d.foot...)
		}
		c.noDollar = false
	}
	switch {
	case list || len(body) == 0 && fileLocals:
		c.write("[")
		c.br(nil)
		for _, i := range body {
			c.comment(docs[i].head...)
			if err := c.document(docs[i]); err != nil {
				return err
			}
			c.end(nil, ", ")
			c.comment(docs[i].foot...)
		}
		c.write("]")
		return nil
	case len(body) == 0:
		c.write("null")
		return nil
	}
	c.comment(docs[body[0]].head...)
	if err := c.document(docs[body[0]]); err != nil {
		return err
	}
	c.br(nil)
	c.comment(docs[body[0]].foot...)
	return nil
}

// objectFile writes a file with names, or one that extends something (ext), as an object: a field
// per name, object locals for file locals, and the documents without a name in the field unnamed.
// Everything stays in file order, so that the code keeps its source lines.
func (c *compiler) objectFile(docs []doc, fields map[string][]int, ext *entry) error {
	if ext != nil {
		c.comment(ext.k.HeadComment, ext.k.LineComment, ext.v.LineComment, ext.k.FootComment, ext.v.FootComment)
		c.write("(")
		if err := c.extends(*ext); err != nil {
			return err
		}
	}
	c.write("{")
	c.br(nil)
	if ext != nil {
		// Extended with documents, a single object would hide them as fields of its own.
		c.write("assert !(std.objectHas(self, 'apiVersion') && std.objectHas(self, 'kind')) : " +
			"'<<+ at the top of a file extends a file, not a single object; to extend one object, " +
			"give its document <<+ and the fields', ")
		c.br(nil)
	}
	var unnamedDocs []string
	pending := map[string]string{} // the local holding the first of two documents giving a name
	for i, d := range docs {
		switch d.kind {
		case fileLocalsDoc:
			c.comment(d.head...)
			for j := 0; j < len(d.root.Content); j += 2 {
				k, v := d.root.Content[j], d.root.Content[j+1]
				c.comment(k.HeadComment, k.LineComment)
				c.align(k)
				c.write("local " + k.Value[1:] + " = ")
				if err := c.value(v); err != nil {
					return err
				}
				c.end(nil, ", ", v.LineComment)
				c.comment(k.FootComment, v.FootComment)
			}
			c.comment(d.foot...)
			continue
		}
		c.comment(d.head...)
		switch d.kind {
		case unnamedDoc:
			tmp := fmt.Sprintf("yakl__unnamed%d", i)
			unnamedDocs = append(unnamedDocs, tmp)
			c.write("local " + tmp + " = ")
		case fieldDoc:
			c.align(d.at)
			idx := fields[d.name]
			switch {
			case len(idx) == 2 && idx[0] == i:
				pending[d.name] = fmt.Sprintf("yakl__doc%d", i)
				c.write("local " + pending[d.name] + " = ")
			case len(idx) == 2:
				// The field takes the visibility of the plain document, and extends it with the other.
				first := docs[idx[0]]
				colons := first.colons
				if first.plus {
					colons = d.colons
				}
				c.write(quote(d.name) + strings.Repeat(":", colons) + " ")
				if d.plus {
					c.write(pending[d.name] + " + ")
				}
				c.write("(")
				if err := c.document(d); err != nil {
					return err
				}
				c.write(")")
				if !d.plus {
					c.write(" + " + pending[d.name])
				}
				c.end(nil, ", ")
				c.comment(d.foot...)
				continue
			default:
				c.write(quote(d.name) + map[bool]string{true: "+"}[d.plus] + strings.Repeat(":", d.colons) + " ")
			}
		}
		if err := c.document(d); err != nil {
			return err
		}
		c.end(nil, ", ")
		c.comment(d.foot...)
	}
	if len(unnamedDocs) > 0 {
		// Appended, so that the documents without a name in an overlay add to the base's.
		c.write(unnamed + "+: [" + strings.Join(unnamedDocs, ", ") + "], ")
		c.br(nil)
	}
	c.write("}")
	if ext != nil {
		c.write(")")
	}
	return nil
}

// access returns the code that reads the field name of the object that code o gives.
func access(o, name string) string {
	if isIdentifier(name) {
		return o + "." + name
	}
	return o + "[" + quote(name) + "]"
}

// block writes the document of a YAML block nested in an expression.
func (c *compiler) block(docs []*yaml.Node) error {
	if len(docs) == 0 || len(docs[0].Content) == 0 {
		c.write("null")
		return nil
	}
	root := docs[0].Content[0]
	if err := c.checkTag(root); err != nil {
		return err
	}
	return c.document(doc{root: root, at: root})
}

// rootVar holds the root of a document whose nodes' anchors declare locals.
const rootVar = "yakl__root"

// Why an anchor can't declare a local where it is, for most places: see anchored.
const (
	noPlace = ": the node has no place of its own in the output"
	inBody  = "in the body of a ${for} or ${if}: the body is repeated or left out"
)

// An anchor is a node whose anchor declares a local of its document.
type anchor struct {
	n    *yaml.Node
	path string // the code that reads the node from the document's root, after rootVar
}

// document writes a document. The anchors of its nodes, its root's included, declare locals:
// the document is written as `local yakl__root = <document>, name = yakl__root.<path>;
// yakl__root`, so that every node stays in place, and a local is the node as the document
// writes it, before any overlay.
func (c *compiler) document(d doc) error {
	if d.root == nil {
		c.write("null")
		return nil
	}
	var as []anchor
	if err := c.anchored(d.root, "", "", false, false, nil, &as); err != nil {
		return err
	}
	if len(as) == 0 {
		return c.value(d.root)
	}
	c.write("local " + rootVar + " = ")
	if err := c.value(d.root); err != nil {
		return err
	}
	for _, a := range as {
		c.write(", " + a.n.Anchor + " = " + rootVar + a.path)
	}
	c.write("; " + rootVar)
	return nil
}

// anchored collects node n, at path from the document's root, and the nodes under it whose
// anchors declare locals: those that are Jsonnet variable names. Others are plain YAML anchors.
// Where nodes have no place of their own in the output, such anchors are errors, and where says
// why. before says that a + key adds n to a list before it, after that a + key may add to n: items
// after a list's, or to the fields of a mapping. holders are the mappings and lists that hold n,
// outermost first.
func (c *compiler) anchored(n *yaml.Node, path, where string, before, after bool, holders []*yaml.Node, out *[]anchor) error {
	if n.Kind == yaml.MappingNode || n.Kind == yaml.SequenceNode {
		holders = append(holders, n)
	}
	if isIdentifier(n.Anchor) {
		if where != "" {
			return c.errorf(n, "&%s %s, so its anchor can't declare a local", n.Anchor, where)
		}
		for _, a := range *out {
			if a.n.Anchor == n.Anchor {
				return c.errorf(n, "the anchor &%s is also on line %d: a local can't have two values", n.Anchor, c.srcLine(a.n))
			}
		}
		// Jsonnet needs the keys of a mapping, and the ${for} and ${if} clauses of a list, to build
		// it, and the local reads the mapping or list. A local key of a mapping hides the local from
		// the keys inside it.
	holders:
		for _, m := range holders {
			var keys []*yaml.Node
			var es []entry
			if m.Kind == yaml.SequenceNode {
				for _, item := range m.Content {
					if k, _, _ := c.clauseItem(item); k != nil {
						keys = append(keys, k)
					}
				}
			} else {
				es, _ = c.entries(m, map[*yaml.Node]bool{}) // mapping reports errors
				for _, e := range es {
					keys = append(keys, e.k)
				}
			}
			for _, k := range keys {
				if c.keyVars(k)[n.Anchor] {
					return c.errorf(k, "%s uses the local %s, which reads line %d from what %[1]s builds; "+
						"to use the value in both places, declare it with a local key, as '&%[2]s': value", c.keyText(k), n.Anchor, c.srcLine(n))
				}
			}
			for _, e := range es {
				if c.keyText(e.k) == "&"+n.Anchor {
					break holders
				}
			}
		}
		*out = append(*out, anchor{n, path})
	}
	return c.anchors(n, path, where, before, after, holders, out)
}

var unknownVar = regexp.MustCompile(`Unknown variable: ([A-Za-z_][A-Za-z0-9_]*)`)

// keyVars returns the variables that key k reads from around it, if it is an expression or a
// ${for} or ${if} clause, as opposed to fields and variables that it binds itself. It returns none
// if the code doesn't parse on its own.
func (c *compiler) keyVars(k *yaml.Node) map[string]bool {
	if k.Kind == yaml.AliasNode {
		k = k.Alias
	}
	i, ok := c.placeholder(k.Value)
	if !ok {
		return nil
	}
	if vars, ok := c.vars[k]; ok {
		return vars
	}
	e := c.exprs[i]
	code := e.code
	if code == "" {
		code = withoutBlocks(c.src, e.from, e.to, e.blocks)
	}
	switch c.clause(k) {
	case "for":
		code = "[null " + code + "\n]"
	case "if":
		code = code + "\nthen null"
	}
	vars := freeVars(code)
	c.vars[k] = vars
	return vars
}

// freeVars returns the variables that the Jsonnet expression code reads without declaring them.
// It returns none if the code doesn't parse.
func freeVars(code string) map[string]bool {
	vars, bind := map[string]bool{}, ""
	for {
		// Jsonnet reports the first unbound variable, so bind each in turn. The code is in an
		// object, where self and $ are valid, so that they don't stop the check.
		_, err := jsonnet.SnippetToAST("", bind+"{f: ("+code+"\n)}")
		m := unknownVar.FindStringSubmatch(fmt.Sprint(err))
		if m == nil || vars[m[1]] {
			return vars
		}
		vars[m[1]] = true
		bind += "local " + m[1] + " = null; "
	}
}

// anchors collects the nodes under n whose anchors declare locals, like anchored, but not n.
func (c *compiler) anchors(n *yaml.Node, path, where string, before, after bool, holders []*yaml.Node, out *[]anchor) error {
	switch n.Kind {
	case yaml.SequenceNode:
		// Past a ${for} or ${if} item, or the items of a list that this one extends, items are
		// counted from the end, which a ${for} or ${if} item after them, or the items of a list
		// that extends this one, makes unknown too.
		clauses, seen := 0, 0
		for _, m := range n.Content {
			if k, _, _ := c.clauseItem(m); k != nil {
				clauses++
			}
		}
		for i, m := range n.Content {
			if k, _, _ := c.clauseItem(m); k != nil {
				seen++
				if err := c.anchored(m, "", cmp.Or(where, inBody), false, false, holders, out); err != nil {
					return err
				}
				continue
			}
			item, w := fmt.Sprintf("%s[%d]", path, i), where
			if before || seen > 0 {
				item = fmt.Sprintf("%s[std.length(%s%s) - %d]", path, rootVar, path, len(n.Content)-i)
				if (after || seen < clauses) && w == "" {
					w = "in a list item whose index depends on ${for} or ${if} items"
					if clauses == 0 {
						w = "in a list item whose index depends on what + keys add before and after it"
					}
				}
			}
			if err := c.anchored(m, item, w, false, false, holders, out); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		// The fields of + keys, merged ones too, which extend those of keys without + in the mapping.
		extended := map[string]bool{}
		es, _ := c.entries(n, map[*yaml.Node]bool{}) // mapping reports errors
		for _, e := range es {
			text := c.keyText(e.k)
			if strings.HasSuffix(strings.TrimRight(text, ":"), "+") && text != "<<+" && !strings.HasPrefix(text, "&") && !strings.HasPrefix(text, "${") {
				extended[c.fieldName(e.k)] = true
			}
		}
		for i := 0; i < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if k.Anchor != "" && k.Kind == yaml.ScalarNode && k.Tag == "!!null" && k.Value == "" && k.Style == 0 {
				return c.errorf(k, "&%s: anchors an empty key: write '&%[1]s': for a local key, or ^^: %[1]s to name the document", k.Anchor)
			}
			if isIdentifier(k.Anchor) {
				return c.errorf(k, "&%s anchors the key %q, which has no place of its own in the output; an anchor for the whole document goes on its own line", k.Anchor, c.keyText(k))
			}
			w, p, plus, ext := where, "", false, false
			switch text := c.keyText(k); {
			case k.Tag == "!!merge":
				w = "in a merge source (<<)" + noPlace
			case text == "<<+":
				w = "in a source of <<+" + noPlace
			case strings.HasPrefix(text, "&"):
				w = "in the value of a local key" + noPlace
			case c.clause(k) != "":
				w = inBody
			case strings.HasPrefix(text, "${"):
				w = "under a key expression" + noPlace
			default:
				_, plus = strings.CutSuffix(strings.TrimRight(text, ":"), "+")
				p = access(path, c.fieldName(k))
				ext = after || (!plus && extended[c.fieldName(k)])
			}
			if err := c.anchored(v, p, w, plus, ext, holders, out); err != nil {
				return err
			}
		}
	}
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

// srcLine returns the source line of a node.
func (c *compiler) srcLine(n *yaml.Node) int {
	if n.Line < len(c.lines) {
		return c.lines[n.Line]
	}
	return n.Line
}

func (c *compiler) errorf(n *yaml.Node, format string, args ...any) error {
	return fmt.Errorf("line %d: %s", c.srcLine(n), fmt.Sprintf(format, args...))
}

// code writes an expression, first padding the output to its source position unless the output is
// already past it. YAML blocks in the code are compiled in place. A bare expression, like the
// clause of a comprehension, isn't parenthesized.
func (c *compiler) code(e expr, bare bool) error {
	if err := c.checkScope(e); err != nil {
		return err
	}
	c.padTo(e.line, e.col-1) // room for the opening parenthesis
	// Readable code puts expressions only where a whole expression goes: a field, an item, a key,
	// a local or a function argument. The operands of + are parenthesized where they're written.
	open, close := "(", ")"
	if c.readable || bare {
		open, close = "", ""
	}
	c.write(open)
	if c.canonical && len(e.blocks) == 0 {
		if f, ok := formatCode(e.code, c.src[e.from:e.to]); ok {
			c.write(f)
			c.write(close)
			return nil
		}
	}
	if e.code != "" {
		c.write(e.code)
		c.write(close)
		return nil
	}
	from := e.from
	for _, b := range e.blocks {
		c.write(c.src[from:b.start])
		nested := &compiler{output: c.output, src: c.src, sentinel: c.sentinel, noDollar: c.noDollar, readable: c.readable, canonical: c.canonical}
		if err := nested.unit(b.lx, b.start+2, b.end); err != nil {
			return err
		}
		from = b.end + 1
		c.padTo(position(c.src, from))
	}
	c.write(c.src[from:e.to])
	c.write(close)
	return nil
}

// checkScope reports $ where it would mean something else than the file.
func (c *compiler) checkScope(e expr) error {
	if !c.noDollar {
		return nil
	}
	code := e.code
	if code == "" {
		code = withoutBlocks(c.src, e.from, e.to, e.blocks)
	}
	var err error
	words(code, func(w string) {
		if err == nil && w == "$" {
			err = fmt.Errorf("line %d: $ in a file local, in a file without names: there is no file object, "+
				"so Jsonnet would read $ as the local's own outermost object", e.line)
		}
	})
	return err
}

// words calls f with each identifier and $ in Jsonnet code. Like exprEnd, it skips strings and
// comments.
func words(code string, f func(w string)) {
	isWord := func(c byte) bool {
		return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
	}
	for i := 0; i < len(code); i++ {
		rest := code[i:]
		n := 0
		switch c := rest[0]; {
		case c == '$':
			f("$")
		case c == '"' || c == '\'':
			n = strEnd(rest, 1, c, false)
		case strings.HasPrefix(rest, `@"`) || strings.HasPrefix(rest, `@'`):
			n = strEnd(rest, 2, rest[1], true)
		case strings.HasPrefix(rest, "|||"):
			n = textBlockEnd(rest)
		case c == '#' || strings.HasPrefix(rest, "//"):
			n = strings.IndexByte(rest, '\n')
		case strings.HasPrefix(rest, "/*"):
			if n = strings.Index(rest[2:], "*/"); n >= 0 {
				n += 3
			}
		case isWord(c):
			for n < len(rest) && isWord(rest[n]) {
				n++
			}
			f(rest[:n])
			n--
		}
		if n < 0 {
			return
		}
		i += n
	}
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
// match the .yakl file.
func (c *compiler) align(n *yaml.Node) {
	if n.Line < len(c.lines) {
		c.padTo(c.lines[n.Line], 0)
	}
}

// padTo moves the output to a source position, unless it is already past it.
func (c *compiler) padTo(line, col int) {
	if c.readable {
		return
	}
	if c.line < line {
		c.write(strings.Repeat("\n", line-c.line))
	}
	if c.line == line && c.col < col {
		c.write(strings.Repeat(" ", col-c.col))
	}
}

// br breaks the line after the opening bracket or an item of collection n (nil for the file), in
// readable code, unless the YAML collection is in flow style.
func (c *compiler) br(n *yaml.Node) {
	if c.readable && (n == nil || n.Style&yaml.FlowStyle == 0) {
		c.write("\n")
	}
}

// comment writes YAML comments as Jsonnet comments, each ending its line, in readable code.
// Copies made by aliases have none. It reports whether it wrote any.
func (c *compiler) comment(texts ...string) bool {
	// yaml.v3 places comments by the lines around them, which Format changes.
	if !c.readable || c.canonical || len(c.aliases) > 0 {
		return false
	}
	wrote := false
	for _, t := range texts {
		if t == "" {
			continue
		}
		for _, l := range strings.Split(t, "\n") {
			if l = strings.TrimSpace(l); l == "" {
				c.write("\n")
			} else {
				c.write("//" + strings.TrimPrefix(l, "#") + "\n")
			}
		}
		wrote = true
	}
	return wrote
}

// end ends an item, a field or a local of collection n (nil for the file) with sep, followed by
// line comments, or else by a line break as br writes it.
func (c *compiler) end(n *yaml.Node, sep string, lineComments ...string) {
	c.write(sep)
	if !c.comment(lineComments...) {
		c.br(n)
	}
}

// tags are the explicit tags allowed on each kind of node; they don't change its meaning.
var tags = map[yaml.Kind]string{yaml.ScalarNode: "!!str", yaml.MappingNode: "!!map", yaml.SequenceNode: "!!seq"}

// checkTag reports a tag other than the one a node has anyway, which would change its meaning.
func (c *compiler) checkTag(n *yaml.Node) error {
	if n.Style&yaml.TaggedStyle != 0 && n.Tag != tags[n.Kind] {
		return c.errorf(n, "unsupported tag %s", n.Tag)
	}
	return nil
}

// copied counts a node copied by an alias, bounding them.
func (c *compiler) copied() error {
	if len(c.aliases) > 0 {
		if c.copies++; c.copies > maxCopies {
			return c.errorf(c.aliases[0], "aliases expand to more than %d nodes", maxCopies)
		}
	}
	return nil
}

// value writes a node's value.
func (c *compiler) value(n *yaml.Node) error {
	c.align(n)
	if err := c.checkTag(n); err != nil {
		return err
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
		return c.value(n.Content[0])
	case yaml.AliasNode:
		// An alias is a copy of the anchored node; its expressions are evaluated at the alias position.
		return c.alias(n, func() error { return c.value(n.Alias) })
	case yaml.SequenceNode:
		return c.sequence(n)
	case yaml.MappingNode:
		return c.mapping(n)
	case yaml.ScalarNode:
		return c.scalar(n, false)
	}
	return c.errorf(n, "unsupported YAML node")
}

// alias runs f, which writes a copy of the node that alias a names.
func (c *compiler) alias(a *yaml.Node, f func() error) error {
	if a == nil {
		return f()
	}
	for _, b := range c.aliases {
		if b.Alias == a.Alias {
			return c.errorf(a, "anchor %q contains an alias to itself", a.Value)
		}
	}
	c.aliases = append(c.aliases, a)
	defer func() { c.aliases = c.aliases[:len(c.aliases)-1] }()
	return f()
}

// Mappings.

// An entry is a key and a value of a mapping, maybe copied in by a merge key.
type entry struct {
	k, v  *yaml.Node
	alias *yaml.Node // the alias that a merge key copied the entry through, if any
}

// A field is an entry whose key is a field name, maybe with modifiers (a literal key), or an
// expression (a computed key).
type field struct {
	entry
	name     string // for a literal key, the field name without modifiers
	computed bool
	local    bool // a local key, '&name': its name is the local's
	plus     bool
	colons   int
	text     string // the key's text
}

// entries returns the entries of mapping n, with those copied in by a merge key (<<) as in YAML:
// own keys win over copied ones, and an earlier source wins over a later one. Aliases can make a
// mapping a merge source many times over, so each mapping's entries are found once.
func (c *compiler) entries(n *yaml.Node, seen map[*yaml.Node]bool) ([]entry, error) {
	if es, ok := c.merged[n]; ok {
		return es, nil
	}
	if seen[n] {
		return nil, c.errorf(n, "a merge key (<<) copies a mapping into itself")
	}
	seen[n] = true
	defer delete(seen, n)
	var own, merged []entry
	keys := map[string]bool{} // of merged
	drop := func(e entry) {
		c.drop(e.k)
		c.drop(e.v)
	}
	for i := 0; i < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if k.Tag != "!!merge" {
			own = append(own, entry{k: k, v: v})
			continue
		}
		// kubectl and kustomize disagree on the other cases.
		if i != 0 {
			return nil, c.errorf(k, "the merge key (<<) must be the first key of a mapping, and appear once")
		}
		sources := []*yaml.Node{v}
		if v.Kind == yaml.SequenceNode {
			sources = v.Content
		}
		for _, s := range sources {
			m, alias := s, (*yaml.Node)(nil)
			if s.Kind == yaml.AliasNode {
				m, alias = s.Alias, s
			}
			if m.Kind != yaml.MappingNode {
				return nil, c.errorf(s, "a merge key (<<) copies mappings, or a list of them; to extend an object, use <<+")
			}
			es, err := c.entries(m, seen)
			if err != nil {
				return nil, err
			}
			for _, e := range es {
				if e.alias == nil {
					e.alias = alias
				}
				if k := c.keyName(e.k); keys[k] {
					drop(e)
				} else {
					keys[k] = true
					merged = append(merged, e)
				}
			}
		}
	}
	var all []entry
	if len(merged) > 0 {
		owned := map[string]bool{}
		for _, e := range own {
			owned[c.keyName(e.k)] = true
		}
		for _, e := range merged {
			if owned[c.keyName(e.k)] {
				drop(e)
			} else {
				all = append(all, e)
			}
		}
	}
	all = append(all, own...)
	c.merged[n] = all
	return all, nil
}

// keyName returns the field name that key k gives to YAML tools, to compare keys: resolved like
// kubectl does, as 0x1F gives 31, and for an expression, its code as Format formats it, which
// formatting some keys and not others can't change.
func (c *compiler) keyName(k *yaml.Node) string {
	text := c.keyText(k)
	if k.Kind == yaml.AliasNode {
		k = k.Alias
	}
	if i, ok := c.placeholder(k.Value); ok {
		e := c.exprs[i]
		if f, ok := formatCode(e.code, c.src[e.from:e.to]); ok && len(e.blocks) == 0 {
			text = "${" + f + "}"
		}
	} else if lit, err := literal(k, true, c.places[k]); err == nil {
		_ = json.Unmarshal([]byte(lit), &text)
	}
	return text
}

// keyText returns the text of a key as YAML tools see it: through an alias, and with the code of
// an expression rather than its placeholder, which differs for each occurrence.
func (c *compiler) keyText(k *yaml.Node) string {
	if k.Kind == yaml.AliasNode {
		k = k.Alias
	}
	if i, ok := c.placeholder(k.Value); ok {
		e := c.exprs[i]
		if e.code == "" {
			e.code = c.src[e.from:e.to]
		}
		return "${" + e.code + "}"
	}
	return k.Value
}

// fieldName returns the name of the field that literal key k gives, without modifiers. A key
// without them is resolved like kubectl does, as 0x1F gives 31; a key with them gives a string.
func (c *compiler) fieldName(k *yaml.Node) string {
	text := c.keyText(k)
	name, _ := strings.CutSuffix(strings.TrimRight(text, ":"), "+")
	if name == text {
		name = c.keyName(k)
	}
	return unescape(name)
}

// placeholder returns the index of the expression that the text of a scalar is the placeholder of.
func (c *compiler) placeholder(v string) (int, bool) {
	parts := strings.Split(strings.Trim(v, " \t\n"), c.sentinel)
	if len(parts) != 3 || parts[0] != "" || parts[2] != "" {
		return 0, false
	}
	i, err := strconv.Atoi(parts[1])
	return i, err == nil && i < len(c.exprs)
}

// drop records that the expressions under n are left out, as in an entry that a merge key copies
// but an own key overrides.
func (c *compiler) drop(n *yaml.Node) {
	if i, ok := c.placeholder(n.Value); ok {
		c.dropped[i] = true
	}
	for _, m := range n.Content {
		c.drop(m)
	}
}

// mapping writes a mapping as an object, with its local keys as object locals. It extends the
// sources of <<+, if any.
func (c *compiler) mapping(n *yaml.Node) error {
	es, err := c.entries(n, map[*yaml.Node]bool{})
	if err != nil {
		return err
	}
	var ext *entry
	var fields []field
	byName := map[string][]int{}
	locals := map[string]bool{}
	for _, e := range es {
		k := e.k
		if k.Kind == yaml.AliasNode {
			k = k.Alias
		}
		if k.Kind != yaml.ScalarNode {
			return c.errorf(k, "only scalar mapping keys are supported")
		}
		if k.Value == "<<+" {
			if ext != nil {
				return c.errorf(k, "<<+ appears twice; give it a list to extend several objects")
			}
			ext = &e
			continue
		}
		f := field{entry: e, text: k.Value, colons: 1}
		f.computed = strings.Contains(k.Value, c.sentinel) || strings.HasPrefix(strings.TrimLeft(k.Value, " \t\n"), "${")
		switch {
		case f.computed:
			if c.clause(k) != "" {
				return c.errorf(k, "%s can only be a list item, as in \"- %[1]s:\"; to add fields to a mapping, use <<+: ${...}", c.keyText(k))
			}
		case nameKey.MatchString(k.Value):
			return c.errorf(k, "the key %q names a document, so it goes among the top-level keys of a document; "+
				"for a literal key, write ${%s}", k.Value, quote(k.Value))
		case strings.HasPrefix(k.Value, "&"):
			if err := c.localKey(k, e.v); err != nil {
				return err
			}
			f.local, f.name = true, k.Value[1:]
			if locals[f.name] {
				return c.errorf(k, "duplicate key %q", k.Value)
			}
			locals[f.name] = true
		default:
			f.name = strings.TrimRight(k.Value, ":")
			if colons := len(k.Value) - len(f.name); colons > 2 {
				return c.errorf(k, "the key %q ends in more than two colons; for a literal key, write ${%s}", k.Value, quote(k.Value))
			} else {
				f.colons += colons
			}
			f.name, f.plus = strings.CutSuffix(f.name, "+")
			byName[c.fieldName(k)] = append(byName[c.fieldName(k)], len(fields))
		}
		fields = append(fields, f)
	}
	// One name, at most two keys: one without + and one with a plain +.
	combined := map[int]int{} // the index of a key without + to the index of the + key it combines with
	skip := map[int]bool{}
	for name, idx := range byName {
		if len(idx) == 1 {
			continue
		}
		a, b := fields[idx[0]], fields[idx[1]]
		if last := fields[idx[len(idx)-1]]; len(idx) > 2 || a.plus == b.plus {
			if a.text == last.text {
				return c.errorf(last.k, "duplicate key %q", a.text)
			}
			return c.errorf(last.k, "the keys %q and %q give the field %q twice", a.text, last.text, name)
		}
		if a.plus {
			a, b = b, a
			idx[0], idx[1] = idx[1], idx[0]
		}
		if b.colons != 1 {
			return c.errorf(b.k, "%q next to %q must be %q: the field takes the visibility of %q", b.text, a.text, name+"+", a.text)
		}
		combined[idx[0]] = idx[1]
		skip[idx[1]] = true
	}

	if ext != nil {
		if ext.alias == nil {
			c.comment(ext.k.HeadComment, ext.k.LineComment, ext.v.LineComment, ext.k.FootComment, ext.v.FootComment)
		}
		c.write("(")
		if err := c.extends(*ext); err != nil {
			return err
		}
	}
	c.write("{")
	c.br(n)
	if len(n.Content) > 0 && n.Content[0].Tag == "!!merge" { // a merge key, always the first
		k, v := n.Content[0], n.Content[1]
		c.comment(k.HeadComment, k.LineComment, v.LineComment, k.FootComment, v.FootComment)
	}
	for i, f := range fields {
		if skip[i] {
			continue
		}
		j, ok := combined[i]
		fs := []field{f}
		if ok {
			fs = append(fs, fields[j])
		}
		var head, line, foot []string
		for _, f := range fs {
			if f.alias == nil { // a copy made by an alias has no comments
				head = append(head, f.k.HeadComment, f.k.LineComment, f.v.HeadComment)
				line = append(line, f.v.LineComment)
				foot = append(foot, f.k.FootComment, f.v.FootComment)
			}
		}
		c.comment(head...)
		if err := c.alias(f.alias, func() error { return c.key(f) }); err != nil {
			return err
		}
		if ok {
			c.write("(")
		}
		if err := c.alias(f.alias, func() error { return c.value(f.v) }); err != nil {
			return err
		}
		if ok {
			c.write(") + (")
			if err := c.alias(fields[j].alias, func() error { return c.value(fields[j].v) }); err != nil {
				return err
			}
			c.write(")")
		}
		c.end(n, ", ", line...)
		c.comment(foot...)
	}
	c.write("}")
	if ext != nil {
		c.write(")")
	}
	return nil
}

// key writes a field's name and modifiers, or the start of a local's declaration.
func (c *compiler) key(f field) error {
	if f.k.Kind == yaml.AliasNode {
		c.align(f.k) // the field is where the alias is
	}
	k := f.k
	if k.Kind == yaml.AliasNode {
		k = k.Alias
	}
	if f.local {
		c.align(k)
		c.write("local " + f.name + " = ")
		return nil
	}
	if f.computed || f.name == f.text {
		if err := c.scalar(k, true); err != nil {
			return err
		}
	} else {
		c.align(k)
		c.write(quote(unescape(f.name)))
	}
	if f.plus {
		c.write("+")
	}
	c.write(strings.Repeat(":", f.colons) + " ")
	return nil
}

// extends writes the sources of <<+ followed by +, so that the mapping extends them: S + {...}.
// A later source is extended by an earlier one, as an earlier one wins in a YAML merge.
func (c *compiler) extends(e entry) error {
	sources := []*yaml.Node{e.v}
	if e.v.Kind == yaml.SequenceNode {
		sources = e.v.Content
	}
	for i := len(sources) - 1; i >= 0; i-- {
		s := sources[i]
		m := s
		if m.Kind == yaml.AliasNode {
			m = m.Alias
		}
		err := c.alias(e.alias, func() error {
			if m.Kind == yaml.MappingNode {
				return c.value(s)
			}
			if m.Kind != yaml.ScalarNode || !strings.Contains(m.Value, c.sentinel) {
				return c.errorf(s, "<<+ extends an expression, a mapping, or a list of them")
			}
			// + would silently join strings and arrays, or add numbers.
			c.write("(function(s) if s == null then {} else if std.isObject(s) then s " +
				"else error '<<+ needs an object or null, not ' + std.type(s))(")
			if err := c.value(s); err != nil {
				return err
			}
			c.write(")")
			return nil
		})
		if err != nil {
			return err
		}
		c.write(" + ")
	}
	return nil
}

// Comprehensions.

// clause returns "for" or "if" if key k is a comprehension clause, like ${for x in xs} or
// ${if cond}, and "" otherwise. An expression that is whole on its own, like ${if c then 'k'}, is
// a computed key instead.
func (c *compiler) clause(k *yaml.Node) string {
	if k.Kind == yaml.AliasNode {
		k = k.Alias
	}
	i, ok := c.placeholder(k.Value)
	if k.Kind != yaml.ScalarNode || !ok {
		return ""
	}
	e := c.exprs[i]
	code := e.code
	if code == "" {
		code = withoutBlocks(c.src, e.from, e.to, e.blocks)
	}
	for _, kw := range []string{"for", "if"} {
		if startsWith(code, kw) && !parses(code) {
			return kw
		}
	}
	return ""
}

// clauseItem returns the clause key and the body of a sequence item that is a ${for} or ${if}: a
// mapping whose only key is the clause. It returns a nil key for other items.
func (c *compiler) clauseItem(item *yaml.Node) (k, v *yaml.Node, err error) {
	m := item
	if m.Kind == yaml.AliasNode {
		m = m.Alias
	}
	if m.Kind != yaml.MappingNode {
		return nil, nil, nil
	}
	for i := 0; i < len(m.Content); i += 2 {
		if c.clause(m.Content[i]) == "" {
			continue
		}
		if len(m.Content) > 2 {
			return nil, nil, c.errorf(m.Content[i], "in a list, %s must be the item's only key, with the body indented under it", c.keyText(m.Content[i]))
		}
		return m.Content[i], m.Content[i+1], c.checkTag(m)
	}
	return nil, nil, nil
}

// sequence writes a sequence as an array. Its ${for} and ${if} items are comprehensions, joined
// with + to the arrays of the items between them.
func (c *compiler) sequence(n *yaml.Node) error {
	open, sep := false, ""
	for _, item := range n.Content {
		k, v, err := c.clauseItem(item)
		if err != nil {
			return err
		}
		if k == nil {
			if !open {
				c.write(sep + "[")
				c.br(n)
				open, sep = true, " + "
			}
			c.comment(item.HeadComment)
			if err := c.value(item); err != nil {
				return err
			}
			c.end(n, ", ", item.LineComment)
			c.comment(item.FootComment)
			continue
		}
		if open {
			c.write("]")
			open = false
		}
		c.write(sep)
		sep = " + "
		var alias *yaml.Node
		if item.Kind == yaml.AliasNode {
			alias = item
		}
		c.comment(item.HeadComment)
		if err := c.alias(alias, func() error {
			c.comment(k.HeadComment, k.LineComment)
			if err := c.comprehension(k, v); err != nil {
				return err
			}
			c.comment(v.LineComment, k.FootComment, v.FootComment)
			return nil
		}); err != nil {
			return err
		}
		c.comment(item.FootComment)
	}
	if sep == "" {
		c.write("[")
		open = true
	}
	if open {
		c.write("]")
	}
	return nil
}

// comprehension writes the array that clause key k gives with body v: [v for ...] for a ${for},
// and (if ... then [v] else []) for an ${if}.
func (c *compiler) comprehension(k, v *yaml.Node) error {
	if k.Kind == yaml.AliasNode {
		k = k.Alias
	}
	m := v
	if m.Kind == yaml.AliasNode {
		m = m.Alias
	}
	if m.Kind == yaml.ScalarNode && m.Tag == "!!null" && m.Value == "" {
		return c.errorf(k, "%s has no body: indent the body under the clause", c.keyText(k))
	}
	kind := c.clause(k)
	e, _, err := c.expr(k)
	if err != nil {
		return err
	}
	c.align(k)
	if kind == "if" {
		c.write("(")
		if err := c.code(e, true); err != nil {
			return err
		}
		c.write(" then [")
		if err := c.value(v); err != nil {
			return err
		}
		c.write("] else [])")
		return nil
	}
	c.write("[")
	if err := c.value(v); err != nil {
		return err
	}
	c.write(" ")
	if err := c.code(e, true); err != nil {
		return err
	}
	c.write("]")
	return nil
}

// keywords are the Jsonnet keywords, which can't name variables.
var keywords = map[string]bool{
	"assert": true, "else": true, "error": true, "false": true, "for": true, "function": true,
	"if": true, "import": true, "importbin": true, "importstr": true, "in": true, "local": true,
	"null": true, "self": true, "super": true, "tailstrict": true, "then": true, "true": true,
}

var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// isIdentifier reports whether s can name a local. Not std, which generated code calls.
func isIdentifier(s string) bool { return identifier.MatchString(s) && !keywords[s] && s != "std" }

// Scalars.

const errNotWhole = "a string that starts with ${ must be exactly one expression: " +
	"build the string in Jsonnet, or start it with $${ for a literal ${"

func (c *compiler) scalar(n *yaml.Node, key bool) error {
	c.align(n)
	if err := c.checkTag(n); err != nil {
		return err
	}
	tagged := n.Style&yaml.TaggedStyle != 0
	e, ok, err := c.expr(n)
	switch {
	case err != nil:
		return err
	case ok && tagged:
		return c.errorf(n, "%s can't turn an expression into text: write $${ for a literal ${", n.Tag)
	case ok && key:
		// A computed field name; null omits the field.
		c.write("[")
		if err := c.code(e, false); err != nil {
			return err
		}
		c.write("]")
		return nil
	case ok:
		return c.code(e, false)
	case unescape(n.Value) != n.Value:
		c.write(quote(unescape(n.Value)))
		return nil
	case yaml11Bool(n):
		return c.errorf(n, "unquoted %q is a boolean to kubectl, but a string to kustomize: write true or false, or quote it", n.Value)
	}
	lit, err := literal(n, key, c.places[n])
	if err != nil {
		return c.errorf(n, "%v", err)
	}
	c.write(lit)
	return nil
}

// expr returns the expression that scalar n is, if any: a placeholder, for a string whose whole
// value, apart from white space around it, is ${...}. Other strings starting with ${ are errors.
func (c *compiler) expr(n *yaml.Node) (expr, bool, error) {
	parts := strings.Split(n.Value, c.sentinel)
	if len(parts) == 1 {
		v := strings.Trim(n.Value, " \t\n")
		if !strings.HasPrefix(v, "${") {
			return expr{}, false, nil
		}
		if n.Style&(yaml.SingleQuotedStyle|yaml.DoubleQuotedStyle) != 0 {
			// The lexer made the well-formed ones placeholders.
			if _, _, err := quotedExpr(v); err != nil {
				return expr{}, false, c.errorf(n, "%v", err)
			}
		}
		return expr{}, false, c.errorf(n, "%s", errNotWhole)
	}
	// White space (which the lexer checked) may surround the expression.
	i, ok := c.placeholder(n.Value)
	if !ok {
		return expr{}, false, c.errorf(n, "%s", errNotWhole)
	}
	// Re-emitting a multi-line expression would push the output past the source lines that follow,
	// so later errors would report wrong lines.
	if c.nodes[i] != nil && c.exprs[i].code == "" && strings.Contains(c.src[c.exprs[i].from:c.exprs[i].to], "\n") {
		return expr{}, false, c.errorf(n, "an alias cannot repeat a multi-line expression; use the anchor's local instead, as an expression like ${name}")
	}
	c.nodes[i] = n
	return c.exprs[i], true, nil
}

// quotedExpr returns the code of a quoted scalar whose whole value is an expression, like '${1 + 1}'.
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
	case isDecl(v[2:end]):
		return "", false, errors.New(errDecl)
	}
	return v[2:end], true, nil
}

// unescape turns a $${ starting s, after any white space, into the literal ${ that it stands for.
func unescape(s string) string {
	t := strings.TrimLeft(s, " \t\n")
	if !strings.HasPrefix(t, "$${") {
		return s
	}
	return s[:len(s)-len(t)] + t[1:]
}

// yaml11Bools are the YAML 1.1 booleans that YAML 1.2 reads as strings.
var yaml11Bools = map[string]bool{"y": true, "n": true, "yes": true, "no": true, "on": true, "off": true}

// yaml11Bool reports whether n is an unquoted YAML 1.1 boolean, which kubectl and kustomize read differently.
func yaml11Bool(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && n.Style == 0 && yaml11Bools[strings.ToLower(n.Value)]
}

// literal returns the JSON encoding of a scalar without expressions.
// Plain scalars are resolved with the YAML 1.1 rules that kubectl and std.parseYaml use
// (`0777` is octal, keys are stringified), by letting sigs.k8s.io/yaml parse them
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
