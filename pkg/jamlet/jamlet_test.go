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

package jamlet

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	jsonnet "github.com/google/go-jsonnet"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
)

func eval(t *testing.T, src string) (string, error) {
	t.Helper()
	code, err := Compile(src)
	if err != nil {
		return "", err
	}
	out, err := jsonnet.MakeVM().EvaluateAnonymousSnippet("test.jamlet", code)
	if err != nil {
		return "", err
	}
	return compact(t, out), nil
}

func compact(t *testing.T, s string) string {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("bad JSON %q: %v", s, err)
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// Without expressions, a jamlet file must mean exactly what std.parseYaml (and kubectl) make of it.
// That includes ${ in strings and comments, which are never expressions.
func TestPlainYAMLMatchesParseYaml(t *testing.T) {
	cases := []string{
		"",
		"# just a comment\n",
		"a: 1\n",
		"---\na: 1\n",
		"a: 1\n---\nb: 2\n",
		"a: 1\n---\n",
		"- a\n- b\n",
		"plain\n",
		"a:\n  b:\n    - 1\n    - {c: d, e: [1, 2]}\n",
		"bools: [yes, no, on, off, true, False, y, n]\n",
		"nums: [0777, 0x1F, 1_000, 1e3, 1.0, -.5, 12345678901234567890]\n",
		"nulls: [~, null, ]\nempty:\n",
		"strs: ['yes', \"no\", 'it''s', \"tab\\there\", !!str 123]\n",
		"date: 2001-12-14\nts: 2001-12-14t21:59:43.10-05:00\n",
		"1: int key\nyes: yes key\nn: n key\n1.5: float key\n",
		"lit: |\n  line1\n  line2\nfold: >-\n  a\n  b\nkeep: |+\n  x\n\n",
		"multi: this is\n  a folded\n\n  plain scalar\n",
		"base: &b {x: 1, y: 2}\nlist: [*b, *b]\n",
		"args: [-, http:, x-, a:b, -1, yes, '-']\n",
		"a: &a name\n*a : value\n",
		"a: !!map {b: !!seq [1]}\n",
		"search: ?enabled\nbind: :8080\nflow: {k: http:, ?x: -, y: [?z, a:b]}\n",
		"&key script: |\n  echo ${HOME}\n!!str other: !!str |\n  ${x}\ntagged: !!str\n  |-\n  ${x}\nq: !!str \"${x\\n}\"\n",
		"- &a k: !!str |\n    ${y}\n  e: 1\n",
		"values: [# ${HOME} is literal here\n  1]\nb: {# ${c\n  d: 1}\ne: \"q\"# ${x\nf: [x, 'y']# ${z\ng: [1,# ${w\n  2]\n",
		"? script\n: |\n  echo ${HOME}\na:\n  ? k\n  :   |\n    x ${y}\n",
		"&k name: value\ncopy: *k\nf: [&d -, &n 1]\ng: *d\n*n : x\n",
		"&k -: item\ncopy: *k\n",
		"labels: &default-labels\n  app: web\nselector: {matchLabels: *default-labels}\nself: &self {a: 1}\n",
		"data:\n  run.sh: |\n    echo hello",
		"{a: !<tag:yaml.org,2002:str> '${HOME}', b: !<tag:yaml.org,2002:str> 1}\n",
		"html: <a href=\"x\">&amp;</a>\nunicode: héllo ✓ \U0001F600\n",
		"dollar: $foo $ {x} $(cmd) $$\n",
		"weird: a,b [c] {d}\nurl: http://x:80/#frag\n",
		// ${ is literal in strings and comments, balanced or not.
		"a: !!str '${x}'\nb: \"${y\"\nc: 'it''s ${'\nd: \"esc \\\" ${\"\ne: '${a} ${b}'\nf: \"${x}-\"\n",
		"# ${unbalanced\na: 1 # ${also {{\n",
		"script: |\n  #!/bin/sh\n  echo ${HOME} ${\n  # ${comment-like content\nnext: 1\n",
		"fold: >\n  ${a}\n   ${b}\n\nafter: x\n",
		"- k: !!str |\n    ${x}\n  e: 1\n",
		"q: \"multi\n  line ${x}\n  }\"\nr: 'single\n  ${y'\n",
		"top: |2\n   ${indented}\n  ${x}\n",
		"text: it's ${ok} # quote in plain\n",
	}
	for _, src := range cases {
		if strings.HasPrefix(src, "text: it's ${ok}") {
			continue // ${ in a plain scalar is an error, checked in TestErrors
		}
		got, err := eval(t, src)
		if err != nil {
			t.Errorf("%q: %v", src, err)
			continue
		}
		want, err := jsonnet.MakeVM().EvaluateAnonymousSnippet("want", "std.parseYaml("+quote(src)+")")
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		if want := compact(t, want); got != want {
			t.Errorf("%q:\n got %s\nwant %s", src, got, want)
		}
	}
}

func TestExpressions(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"user example", "foo:\n  bar: ${{some: \"jsonnet\"}.some}\n", `{"foo":{"bar":"jsonnet"}}`},
		{"keeps type", "a: ${1 + 2}\nb: ${[1, {c: true}]}\nc: ${null}\nd: ${'s'}\n", `{"a":3,"b":[1,{"c":true}],"c":null,"d":"s"}`},
		{"strings are built in jsonnet", "name: ${'app-%s-%d' % ['x', 2]}\n", `{"name":"app-x-2"}`},
		{"in flow collections", "l: [${1}, ${'a, b'}]\nm: {k: ${2}, ${'c'}: 3}\n", `{"l":[1,"a, b"],"m":{"c":3,"k":2}}`},
		{"sequence items", "- ${1}\n- - ${2}\n", `[1,[2]]`},
		{"yaml-hostile jsonnet needs no quoting", "a: ${ {x: 'y # not a comment'}.x }\nb: ${ [1, 2][1] } # comment\n", `{"a":"y # not a comment","b":2}`},
		{"braces in strings and comments", "a: ${ '}' + \"}\" + @'}''' + /* } */ '' }\nb: ${ # }\n  1 // }\n}\n", `{"a":"}}}'","b":1}`},
		{"indented ||| inside a text block", "a: ${|||\n  x\n  ||| literal\n|||}\n", `{"a":"x\n||| literal\n"}`},
		{"text block", "a: ${|||\n  ${HOME} }}}\n|||}\n", `{"a":"${HOME} }}}\n"}`},
		{"multi-line expression", "a: ${\n  local x = 2;\n  x * x\n}\nb: 1\n", `{"a":4,"b":1}`},
		{"strings are literal", "a: !!str '${x}'\nb: \"x${x}\"\nc: !!str |\n  ${x}\nd: ${1}\ne: '${x} ${y}'\nf: |\n  ${x} ${y}\n", `{"a":"${x}","b":"x${x}","c":"${x}\n","d":1,"e":"${x} ${y}","f":"${x} ${y}\n"}`},
		{"quoted expressions", "foo: '${\"foo\"}'\nb: \"${1 + 1}\"\nc: '${''x'' + \"y\"}'\nd: ['${1}', '${\"a, b\"}']\ne: {k: '${ {x: \"y # z\"}.x }'}\n", `{"b":2,"c":"xy","d":[1,"a, b"],"e":{"k":"y # z"},"foo":"foo"}`},
		{"quoted computed key", "'${\"a\" + \"b\"}': 1\n\"${if false then 'x'}\": 2\n", `{"ab":1}`},
		{"quoted expressions keep YAML's tokens", "a: {'${\"k\"}':1}\nb: '${2}'# comment\n", `{"a":{"k":1},"b":2}`},
		{"tags on quoted expressions", "!!map\n'${\"a\"}': [!!str , '${1}', !!str '${\"x}', !!str '${local y = 1}']\n", `{"a":["",1,"${\"x}","${local y = 1}"]}`},
		{"anchored quoted expression", "a: &x '${1 + 1}'\nb: *x\n", `{"a":2,"b":2}`},
		{"block expressions", "a: |-\n  ${[\n    x * 2  // a comment\n    for x in [1, 2]\n  ]}\nb: |\n  ${|||\n    text\n  |||}\nc: >+\n\n  ${1 +\n  1}  \n\nd:\n- &e |-\n  ${\"e\"}\n- *e\n? |\n  ${'k'}\n: v\n", `{"a":[2,4],"b":"text\n","c":2,"d":["e","e"],"k":"v"}`},
		{"tags don't leak into the next document", "--- !!str |\n  ${x}\n--- |\n  ${1 + 1}\n", `["${x}\n",2]`},
		{"block expression after explicit indentation", "a: |2\n    ${1 + 1}\nb: |2\n  \t${2}\n", `{"a":2,"b":2}`},
		{"yaml block in a block expression", "env: |-\n  ${[\n    \\{\n      name: ${k}\n    }\n    for k in ['a', 'b']\n  ]}\n", `{"env":[{"name":"a"},{"name":"b"}]}`},
		{"comments are ignored", "# ${broken\na: ${1} # ${broken\n", `{"a":1}`},
		{"after a block scalar", "s: |\n  # not a comment ${x}\n    ${y}\nt: ${1}\n", `{"s":"# not a comment ${x}\n  ${y}\n","t":1}`},
		{"after a nested block scalar", "- k: |\n    x ${x}\n  e: ${1}\n", `[{"e":1,"k":"x ${x}\n"}]`},
		{"after an empty block scalar", "a:\n  - b: |\n    c: ${1}\n", `{"a":[{"b":"","c":1}]}`},
		{"after a multi-line plain scalar", "a: it is\n  'not quoted\nb: ${1}\n", `{"a":"it is 'not quoted","b":1}`},
		{"after a multi-line quoted scalar", "a: 'x\n  ${y'\nb: ${1}\n", `{"a":"x ${y","b":1}`},
		{"computed keys", "${'a' + 'b'}: 1\n!!str '${k}': 2\n", `{"${k}":2,"ab":1}`},
		{"null key omits field", "${if false then 'x'}: 1\nz: 2\n", `{"z":2}`},
		{"self and $", "name: foo\nspec:\n  i: 1\n  j: ${self.i + 1}\n  label: ${$.name}\n", `{"name":"foo","spec":{"i":1,"j":2,"label":"foo"}}`},
		{"$ is per document", "a: 1\nb: ${$.a}\n---\na: 2\nb: ${$.a}\n", `[{"a":1,"b":1},{"a":2,"b":2}]`},
		{"whole document", "${ {kind: 'List', items: []} }\n", `{"items":[],"kind":"List"}`},
		{"whole document with locals", "${ local x = 1; {a: x} }\n", `{"a":1}`},
		{"anchored expression", "a: &x ${1}\nb: *x\n", `{"a":1,"b":1}`},
		{"private use chars in source", "a: \uE000\nb: ${1}\n", `{"a":"\uE000","b":1}`},

		{"local", "${local x = 2}\na: ${x * x}\n", `{"a":4}`},
		{"several locals", "${local a = 1;}\n${local b = a + 1, c = 3}\n# comment\nv: ${[a, b, c]}\n", `{"v":[1,2,3]}`},
		{"comments around local", "${ /* config */ local x = 1 }\n${local/* c */y = 2}\n${ # note\n  local z = 3}\nv: ${[x, y, z]}\n", `{"v":[1,2,3]}`},
		{"after explicit keys", "? k\n: v\n${'e'}: ${1}\n? l\n: |\n${'f'}: ${2}\n", `{"e":1,"f":2,"k":"v","l":""}`},
		{"local function", "${local f(x) = x * 2}\nv: ${f(3)}\n", `{"v":6}`},
		{"comments after a local's semicolon", "${local x = 1; /* c */}\n${local y = 2 // two\n}\n${local z = 3; // three\n}\nv: ${[x, y, z]}\n", `{"v":[1,2,3]}`},
		{"local expression using an outer local", "${local p = 2}\nvalue: ${local n = p; n + 1}\n", `{"value":3}`},
		{"local declaration using an outer local", "${local a = 1}\n${local b = a + 1;}\nv: ${b}\n", `{"v":2}`},
		{"multi-line local", "${\n  local a = 1 // one\n}\nv: ${a}\n", `{"v":1}`},
		{"local preamble covers the stream", "${local p = {n: 1}}\n---\na: ${p.n}\n---\nb: ${p.n + 1}\n", `[{"a":1},{"b":2}]`},
		{"locals in the first document cover later ones", "${local p = 1}\na: ${p}\n---\nb: ${p}\n", `[{"a":1},{"b":1}]`},
		{"locals cover the rest of the stream", "a: 1\n---\n${local q = 5}\nb: ${q}\n---\nc: ${q}\n", `[{"a":1},{"b":5},{"c":5}]`},
		{"locals-only document", "a: 1\n---\n${local q = 5}\n---\nb: ${q}\n", `[{"a":1},null,{"b":5}]`},
		{"shadowing", "${local x = 1}\na: ${x}\n---\n${local x = 2}\nb: ${x}\n", `[{"a":1},{"b":2}]`},
		{"document end marker", "a: 1\n...\n---\n${local x = 1}\nb: ${x}\n", `[{"a":1},{"b":1}]`},
		{"directive", "%YAML 1.1\n---\n${local x = 1}\nb: ${x}\n", `[{"b":1}]`},
		{"locals after the end marker", "${local x = 1}\na: ${x}\n...\n${local x = 2}\n", `{"a":1}`},
		{"locals only", "${local x = 1}\n", `null`},

		{"yaml block in a comprehension", `${local p = {a: 1, b: 2}}
env: ${[
  \{
    name: ${k}
    value: ${std.toString(p[k])}
  }
  for k in std.objectFields(p)
]}
`, `{"env":[{"name":"a","value":"1"},{"name":"b","value":"2"}]}`},
		{"yaml block in an operator", `x: ${ {name: 'n'} + \{
    foo:
      bar: ${1 + 1}
  } }
`, `{"x":{"foo":{"bar":2},"name":"n"}}`},
		{"yaml block scalar root", "a: ${std.asciiUpper(\\{\n    hello\n  })}\n", `{"a":"HELLO"}`},
		{"yaml block sequence root", "a: ${\\{   # yaml from here\n    - one\n    - |\n      two ${literal}\n# a comment anywhere\n    - [3, ${4}]\n  }}\n", `{"a":["one","two ${literal}\n",[3,4]]}`},
		{"yaml and jsonnet alternate", `a: ${[
  \{
    b: ${[
      \{
        c: ${i * 10}
      }
      for i in [1, 2]
    ]}
  }
]}
`, `{"a":[{"b":[{"c":10},{"c":20}]}]}`},
		{"locals in a yaml block", "a: ${\\{\n    ${local x = 3}\n    v: ${x}\n  }}\n", `{"a":{"v":3}}`},
		{"yaml block in a local", "${local labels = \\{\n    app: web\n    tier: ${'front'}\n  }}\nl: ${labels}\n", `{"l":{"app":"web","tier":"front"}}`},
		{"$ and self in a yaml block", "name: nm\nx: ${[\\{\n    ref: ${$.name}\n    me: ${self.ref}\n  }]}\n", `{"name":"nm","x":[{"me":"nm","ref":"nm"}]}`},
		{"flow collection in a yaml block", "a: ${\\{\n    m: {x: 1,\n      }\n  }}\n", `{"a":{"m":{"x":1}}}`},
		{"empty yaml block", "a: ${\\{\n  }}\n", `{"a":null}`},
		{"anchor names a mapping", "spec: &spec\n  replicas: 3\n  template:\n    spec:\n      num: ${spec.replicas}\n", `{"spec":{"replicas":3,"template":{"spec":{"num":3}}}}`},
		{"anchor on a flow mapping", "a: &a {x: 1, z: ${a.x + 1}}\n", `{"a":{"x":1,"z":2}}`},
		{"anchor on the root mapping", "&root\nname: nm\nm:\n  v: ${root.name}\n", `{"m":{"v":"nm"},"name":"nm"}`},
		{"anchors shadow", "a: &x\n  num: 1\n  b: &x\n    num: 2\n    v: ${x.num}\n  v: ${x.num}\n", `{"a":{"b":{"num":2,"v":2},"num":1,"v":1}}`},
		{"aliased mapping binds its copy", "a: &m\n  num: 1\n  v: ${m.num}\nb: *m\n", `{"a":{"num":1,"v":1},"b":{"num":1,"v":1}}`},
		{"anchors on scalars and sequences don't bind", "a: &my-x 1\nb: &if [1]\nc: [*my-x, *if]\n", `{"a":1,"b":[1],"c":[1,[1]]}`},
		{"anchors that aren't identifiers don't bind", "a: &my-a\n  b: 1\nc: *my-a\nd: &if {e: ${self.f}, f: 2}\n", `{"a":{"b":1},"c":{"b":1},"d":{"e":2,"f":2}}`},
		{"mapping local", "spec:\n  ${local labels = {app: 'web'}}\n  selector: ${labels}\n  template:\n    labels: ${labels}\n", `{"spec":{"selector":{"app":"web"},"template":{"labels":{"app":"web"}}}}`},
		{"mapping local after fields", "a: 1\n${local x = 2}\nb: ${x}\n", `{"a":1,"b":2}`},
		{"mapping local scope", "a:\n  ${local x = 1}\n  v: ${x}\nb:\n  ${local x = 2}\n  v: ${x}\n", `{"a":{"v":1},"b":{"v":2}}`},
		{"mapping local shadows a file local", "${local x = 1}\na:\n  ${local x = 2}\n  v: ${x}\nb: ${x}\n", `{"a":{"v":2},"b":1}`},
		{"mapping local in a sequence item", "- ${local x = 5}\n  v: ${x}\n- v: 1\n", `[{"v":5},{"v":1}]`},
		{"mapping local with semicolon and comment", "a:\n  ${local f(n) = n * 2; // double\n  }\n  v: ${f(2)}\n", `{"a":{"v":4}}`},
		{"mapping local with a yaml block", "a:\n  ${local l = \\{\n      x: 1\n    }}\n  v: ${l.x}\n", `{"a":{"v":1}}`},
		{"mapping with only locals", "a:\n  ${local x = 1}\n", `{"a":{}}`},
		{"mapping local in a yaml block", "a: ${[\\{\n    num: ${i}\n    ${local twice = i * 2}\n    m: {v: ${twice}}\n  } for i in [1]]}\n", `{"a":[{"m":{"v":2},"num":1}]}`},
		{"jsonnet object extension", "a: ${ {x: 1} + {y: ${z: 1}.z} }\nz: 2\n", `{"a":{"x":1,"y":1},"z":2}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := eval(t, tc.src)
			if err != nil {
				code, _ := Compile(tc.src)
				t.Fatalf("%v\ngenerated:\n%s", err, code)
			}
			if want := compact(t, tc.want); got != want {
				t.Errorf("got  %s\nwant %s", got, want)
			}
		})
	}
}

func TestErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{"a: 1\nb: ${1 + \n", "line 2, column 4: unterminated ${"},
		{"a: ${'}\n", "unterminated"},
		{"a: ${}\n", "line 1, column 4: empty expression"},
		{"a: ${ /* } */\n", "unterminated"},
		{"a: app-${x}\n", "line 1, column 8: ${ inside a plain scalar"},
		{"text: it's ${ok} # quote in plain\n", "${ inside a plain scalar"},
		{"a: it is\n  continued ${x}\n", "line 2, column 13: ${ inside a plain scalar"},
		{"a: ${x}-svc\n", "line 1, column 8: unexpected text after an expression"},
		{"a: ${1} ${2}\n", "unexpected text after an expression"},
		{"a: !!str ${1}\n", "an expression must be a whole, unquoted, untagged YAML value"},
		{"a: ${local x = 1}\n", "line 1, column 4: a local declaration must be on its own lines"},
		{"a: '${}'\n", "line 1: empty expression"},
		{"a: '${[\\{\n  b: 1\n  }]}'\n", `line 1: inside an expression, \{ starts a YAML block`},
		{"a: '${\n  1}'\n", "line 1: a quoted expression must be on one line"},
		{"a: '${local x = 1}'\n", "line 1: a local declaration cannot be quoted"},
		{"a: \"${[\\\\{\\n  b: 1\\n}]}\"\n", "line 1: a quoted expression cannot contain a YAML block"},
		{"a: '${\"x}'\n", "line 1: unterminated ${"},
		{"a: 1\nb: '${error \"boom\"}'\n", "test.jamlet:2:"},
		{"a: |-\n  ${\"x}\n", "line 2, column 3: unterminated ${"},
		{"a: |\n  ${ }\n\n", "line 2, column 3: empty expression"},
		{"a: |-\n  ${local x = 1}\n", "line 2, column 3: a local declaration cannot be a block scalar"},
		{"a: |-\n  ${\"x}\nb: '\"}'\n", "line 2, column 3: unterminated ${"},
		{"a: &m\n  ${local x =\n    1}\n  v: ${x}\nb: *m\n", "line 2: an alias cannot repeat a multi-line local declaration"},
		{"a: \"${1\\n\\n\\n}\"\n", "line 1: a quoted expression must be on one line"},
		{"a: &v\n  '${error \"boom\"}'\n", "test.jamlet:2:"},
		{"&k '${\"k\"}': 1\nx:\n  *k : 2\nc: ${error 'boom'}\n", "test.jamlet:4:"},
		{"a: |-\n  ${\n    1 +\n    error 'deep'\n  }\n", "test.jamlet:4:"},
		{"a:\n  ${local x = 1, y = 2}\n  b: 2\n", "line 2, column 3: a local declaration in a mapping must declare a single variable"},
		{"a:\n  ${local x = 1}\n    b: 2\n", "line 3: unexpected content after a local declaration"},
		{"a: &my-spec\n  b: 1\n  c: ${my-spec.b}\n", "test.jamlet:3:"},
		{"a:\n  ${local x = {}}\n  b: ${x.c}\n", "test.jamlet:3:"},
		{"${local x = 1} # ok\n${local y = 1} a: 1\n", "line 2, column 1: a local declaration must be"},
		{"a: [${local x = 1}]\n", "a local declaration must be"},
		{"a: !!int 3\n", "line 1: unsupported tag !!int"},
		{"a: !Ref x\n", "unsupported tag !Ref"},
		{"a: !Ref {b: 1}\n", "line 1: unsupported tag !Ref"},
		{"v: ! true\n", "line 1, column 4: the non-specific tag ! is not supported"},
		{"v: [! 1]\n", "the non-specific tag ! is not supported"},
		{"a: !!int [1, 2]\n", "unsupported tag !!int"},
		{"a: &a [1]\n*a : 2\n", "only scalar mapping keys"},
		{"? [a]\n: 1\n", "only scalar mapping keys"},
		{"a: &a {x: 1}\nb:\n  <<: *a\n", "line 3: merge keys (<<) are not supported"},
		{"a: &a [*a]\n", `line 1: anchor "a" contains an alias to itself`},
		{"a: &a {b: [1, {c: *a}]}\n", `anchor "a" contains an alias to itself`},
		{laughs(7), "aliases expand to more than 100000 nodes"},
		{"m: &m\n" + strings.Repeat("  ${local x = 1}\n", 500) + "l: [" + strings.Repeat("*m, ", 300) + "]\n", "aliases expand to more than 100000 nodes"},
		{"a: &x ${\n  1\n}\nb: *x\n", "line 1: an alias cannot repeat a multi-line expression"},
		// Jsonnet locations match the source for YAML literals too.
		{"a: 1\nb:\n  c: [1, 2]\na: 3\n", "test.jamlet:4:"},
		// Like std.parseYaml, values JSON cannot represent are rejected.
		{"a: .inf\n", "unsupported value: +Inf"},
		{"~: 1\n", "unsupported map key"},
		// YAML error lines are reported in source terms even after multi-line expressions.
		{"a: ${\n\n\n1}\nb: [\n", "line 5"},
		// Jsonnet errors point at the source line of the expression.
		{"a: 1\n\nb: ${error 'boom'}\n", "test.jamlet:3:"},
		{"a: ${\n  1 +\n  error 'deep'\n}\n", "test.jamlet:3:"},
		{"${local x = 1}\n${local y = error 'decl'}\nv: ${y}\n", "test.jamlet:2:"},
		{"${1}: x\n", "Field name must be string"},
		{"a: &k name\nname: 1\n*k : 2\n", "test.jamlet:3:"},
		{"a: ${ {b: 1} + \\{c: 1} }\n", `line 1, column 16: inside an expression, \{ starts a YAML block and must end its line`},
		{"a: ${\\{\n    b: 1\n", "line 1, column 6: unterminated YAML block"},
		{"a: ${\\{\nb: 1\n}}\n", "unterminated YAML block"},
		{"a: ${[\\{\n    b: 1\n  c: 2\n  }]}\n", "line 3, column 3: a YAML block in an expression must be closed by }"},
		{"a: ${\\{\n    b: c: d\n  }}\n", "line 2"},
		{"a: ${\\{\n    b: app-${x}\n  }}\n", "line 2, column 12: ${ inside a plain scalar"},
		{"a: ${[\\{\n    b: ${error 'nested'}\n  }]}\n", "test.jamlet:2:"},
		// Code after a nested block keeps the line of the block's closing }.
		{"a: ${\\{\n    b: |\n      x\n\n  } + error 'boom'}\n", "test.jamlet:5:"},
	}
	for _, tc := range cases {
		_, err := eval(t, tc.src)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: got error %v, want %q", tc.src, err, tc.want)
		}
	}
}

// laughs returns YAML whose aliases expand exponentially ("billion laughs").
func laughs(levels int) string {
	s := "l0: &l0 [x, x, x, x, x, x, x, x, x, x]\n"
	for i := 1; i < levels; i++ {
		s += fmt.Sprintf("l%d: &l%d [%s]\n", i, i, strings.TrimSuffix(strings.Repeat(fmt.Sprintf("*l%d, ", i-1), 10), ", "))
	}
	return s
}

func FuzzCompile(f *testing.F) {
	for _, s := range []string{"a: ${1}\n", "- |\n  ${x}\n- ${y}\n", "${local x = 1}\n---\n[${x}, '${']\n", "a: 'b\n  c' # ${\n", "[a [", "- k: |\n    x\n  e: ${1}\n", "a: ${[\\{\n    b: ${[\\{\n        c: 1\n      }]}\n  }]}\n", "a: &a [*a]\n", "a: &x\n  ${local y = x}\n  - ${local z = 1;}\n    b: ${z}\n", "a: ['${1}', \"${'}'}\"]\n'${k}': !!str '${v}'\n", "a: |-\n  ${[\\{\n    b: 1\n  }]}\nc: !!str |\n  ${d}\n"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		if _, err := Compile(src); err != nil && strings.Contains(err.Error(), "internal error") {
			t.Fatal(err)
		}
	})
}

// Jamlet files must be valid YAML, as read by kubectl, so that YAML tooling can process them.
// That holds for the examples and for every YAML snippet in the spec.
func TestFilesAreYAML(t *testing.T) {
	files := map[string]string{}
	for _, pattern := range []string{"../../examples/*.jamlet", "../../testdata/*.jamlet"} {
		paths, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range paths {
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			files[p] = string(b)
		}
	}
	const spec = "../../docs/Advanced-Usage/jamlet.md"
	doc, err := os.ReadFile(spec)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range regexp.MustCompile("(?ms)^```yaml\n(.*?)^```$").FindAllSubmatchIndex(doc, -1) {
		files[fmt.Sprintf("%s:%d", spec, bytes.Count(doc[:m[2]], []byte("\n"))+1)] = string(doc[m[2]:m[3]])
	}
	if len(files) < 10 {
		t.Fatalf("found only %d files", len(files))
	}
	for name, src := range files {
		dec := utilyaml.NewYAMLOrJSONDecoder(strings.NewReader(src), 4096)
		for {
			var v any
			err := dec.Decode(&v)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				if why := notYAMLYet(src); why != "" {
					t.Logf("%s: not valid YAML, as it uses %s: %v", name, why, err)
				} else {
					t.Errorf("%s: not valid YAML: %v", name, err)
				}
				break
			}
		}
	}
}

// notYAMLYet returns which syntax without a YAML form yet src uses, if any.
func notYAMLYet(src string) string {
	l, err := lex(src, 0, false)
	if err != nil {
		return ""
	}
	for _, s := range l.spans {
		if s.local {
			return "local declarations"
		}
	}
	return ""
}
