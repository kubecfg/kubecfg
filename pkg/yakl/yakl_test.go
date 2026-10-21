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
	return evalWith(t, Compile, src)
}

func evalWith(t *testing.T, compile func(string) (string, error), src string) (string, error) {
	t.Helper()
	code, err := compile(src)
	if err != nil {
		return "", err
	}
	out, err := jsonnet.MakeVM().EvaluateAnonymousSnippet("test.yakl", code)
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

// Without expressions, a yakl file must mean exactly what std.parseYaml (and kubectl) make of it.
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
		"bools: [true, False, 'yes', \"no\", !!str on]\n",
		"nums: [0777, 0x1F, 1_000, 1e3, 1.0, -.5, 12345678901234567890]\n",
		"nulls: [~, null, ]\nempty:\n",
		"strs: ['yes', \"no\", 'it''s', \"tab\\there\", !!str 123, !!str 0777]\n",
		"date: 2001-12-14\nts: 2001-12-14t21:59:43.10-05:00\n",
		"1: int key\n'yes': yes key\n1.5: float key\n",
		"lit: |\n  line1\n  line2\nfold: >-\n  a\n  b\nkeep: |+\n  x\n\n",
		"multi: this is\n  a folded\n\n  plain scalar\n",
		"base: &b {x: 1, z: 2}\nlist: [*b, *b]\n",
		"args: [-, http:, x-, a:b, -1, '-']\n",
		"a: &a name\n*a : value\n",
		"a: !!map {b: !!seq [1]}\n",
		"search: ?enabled\nbind: :8080\nflow: {k: http:, ?x: -, w: [?z, a:b]}\n",
		"&key-1 script: |\n  echo ${HOME}\n",
		"&k-1 name: value\ncopy: *k-1\nf: [&d -, &n 1]\ng: *d\n*n : x\n",
		"&k- -: item\ncopy: *k-\n",
		"labels: &default-labels\n  app: web\nselector: {matchLabels: *default-labels}\nself: &self {a: 1}\n",
		"data:\n  run.sh: |\n    echo hello",
		"{b: !<tag:yaml.org,2002:str> 1}\n",
		"html: <a href=\"x\">&amp;</a>\nunicode: héllo ✓ \U0001F600\n",
		"dollar: $foo $ {x} $(cmd) $$\n",
		"weird: a,b [c] {d}\nurl: http://x:80/#frag\n",
		// ${ after the start of a string, and in comments, is literal text, balanced or not.
		"b: \"x${y\"\nc: 'it''s ${'\nd: \"esc \\\" ${\"\ne: ' x ${a} ${b}'\n",
		"# ${unbalanced\na: 1 # ${also {{\n",
		"script: |\n  #!/bin/sh\n  echo ${HOME} ${\n  # ${comment-like content\nnext: 1\n",
		"q: \"multi\n  line ${x}\n  }\"\nr: 'single\n  ${y'\n",
		"text: it's ${ok} # quote in plain\n",
		"image: nginx:${version}\nscript: echo ${HOME}\nlong: it is\n  continued ${x}\n",
		"values: [# ${HOME} is literal here\n  1]\nb: {# ${c\n  d: 1}\ne: \"q\"# ${x\nf: [x, 'y']# ${z\ng: [1,# ${w\n  2]\n",
		"? script\n: |\n  echo ${HOME}\na:\n  ? k\n  :   |\n    x ${y}\n",
		// Continuation lines of a plain scalar need only be indented past its key or item, and
		// may outdent the root; a quote in them is text.
		" 00\n\"\n---",
		"a: &x:0 \"\n---", // yaml.v3 ends an anchor at the first character it can't hold
		"a:\n# comment\n  foo\n '${x}'\nb: [\"\"]\n",
		"- foo\n \"bar\n",
		// Merge keys, as in YAML.
		"a: &a {x: 1, w: 1}\nb: &b {w: 2, z: 2}\nc:\n  <<: [*a, *b]\n  z: 3\nd:\n  <<: *a\ne: {<<: {k: v}, l: w}\n",
	}
	for _, src := range cases {
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
		if desugared, err := evalWith(t, Desugar, src); err != nil || desugared != got {
			t.Errorf("%q: desugared: got %s, %v", src, desugared, err)
		}
	}
}

func TestExpressions(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"white space around an expression", "a: \"${1 + 2}\\n\"\nb: ' ${3} '\nc: |\n  ${4}\nd: \" $${x}\"\n", `{"a":3,"b":3,"c":4,"d":" ${x}"}`},
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
		{"strings are literal", "a: $${x}\nb: \"x${x}\"\nd: ${1}\nf: |\n  x ${y}\ng: echo ${HOME}\nh: '$${HOME} ${x}'\ni: |\n  $${y}\n", `{"a":"${x}","b":"x${x}","d":1,"f":"x ${y}\n","g":"echo ${HOME}","h":"${HOME} ${x}","i":"${y}\n"}`},
		{"quoted expressions", "foo: '${\"foo\"}'\nb: \"${1 + 1}\"\nc: '${''x'' + \"y\"}'\nd: ['${1}', '${\"a, b\"}']\ne: {k: '${ {x: \"y # z\"}.x }'}\n", `{"b":2,"c":"xy","d":[1,"a, b"],"e":{"k":"y # z"},"foo":"foo"}`},
		{"multi-line quoted expressions are folded", "a: '${[1,\n  2]}'\nb: \"${\n  3}\"\nc: ${[4,\n  5]}\n", `{"a":[1,2],"b":3,"c":[4,5]}`},
		{"quoted computed key", "'${\"a\" + \"b\"}': 1\n\"${if false then 'x'}\": 2\n", `{"ab":1}`},
		{"quoted expressions keep YAML's tokens", "a: {'${\"k\"}':1}\nb: '${2}'# comment\n", `{"a":{"k":1},"b":2}`},
		{"tags on quoted expressions", "!!map\n'${\"a\"}': [!!str , '${1}', !!str 0777]\n", `{"a":["",1,"0777"]}`},
		{"anchored quoted expression", "a: &x '${1 + 1}'\nb: *x\n", `{"a":2,"b":2}`},
		{"block expressions", "a: |-\n  ${[\n    x * 2  // a comment\n    for x in [1, 2]\n  ]}\nb: |\n  ${|||\n    text\n  |||}\nc: >+\n\n  ${1 +\n  1}  \n\nd:\n- &e |-\n  ${\"e\"}\n- *e\n? |\n  ${'k'}\n: v\n", `{"a":[2,4],"b":"text\n","c":2,"d":["e","e"],"k":"v"}`},
		{"tags don't leak into the next document", "--- !!str |\n  $${x}\n--- |\n  ${1 + 1}\n", `["${x}\n",2]`},
		{"block expression after explicit indentation", "a: |2\n    ${1 + 1}\nb: |2\n  \t${2}\n", `{"a":2,"b":2}`},
		{"yaml block in a block expression", "env: |-\n  ${[\n    \\{\n      name: ${k}\n    }\n    for k in ['a', 'b']\n  ]}\n", `{"env":[{"name":"a"},{"name":"b"}]}`},
		{"comments are ignored", "# ${broken\na: ${1} # ${broken\n", `{"a":1}`},
		{"after a block scalar", "s: |\n  # not a comment ${x}\n    ${y}\nt: ${1}\n", `{"s":"# not a comment ${x}\n  ${y}\n","t":1}`},
		{"after a nested block scalar", "- k: |\n    x ${x}\n  e: ${1}\n", `[{"e":1,"k":"x ${x}\n"}]`},
		{"after an empty block scalar", "a:\n  - b: |\n    c: ${1}\n", `{"a":[{"b":"","c":1}]}`},
		{"after a multi-line plain scalar", "a: it is\n  'not quoted\nb: ${1}\n", `{"a":"it is 'not quoted","b":1}`},
		{"a quote continuing a plain scalar", "a:\n  foo\n \"x\nb: ${1}\n", `{"a":"foo \"x","b":1}`},
		{"after a multi-line quoted scalar", "a: 'x\n  ${y'\nb: ${1}\n", `{"a":"x ${y","b":1}`},
		{"computed keys", "${'a' + 'b'}: 1\n!!str '$${k}': 2\n", `{"${k}":2,"ab":1}`},
		{"null key omits field", "${if false then 'x'}: 1\nz: 2\n", `{"z":2}`},
		{"self and $", "name: foo\nspec:\n  i: 1\n  j: ${self.i + 1}\n  label: ${$.name}\n", `{"name":"foo","spec":{"i":1,"j":2,"label":"foo"}}`},
		{"$ is per document in a list", "a: 1\nb: ${$.a}\n---\na: 2\nb: ${$.a}\n", `[{"a":1,"b":1},{"a":2,"b":2}]`},
		{"whole document", "${ {kind: 'List', items: []} }\n", `{"items":[],"kind":"List"}`},
		{"whole document with locals", "${ local x = 1; {a: x} }\n", `{"a":1}`},
		{"anchored expression", "a: &x ${1}\nb: *x\n", `{"a":1,"b":1}`},
		{"private use chars in source", "a: \uE000\nb: ${1}\n", `{"a":"\uE000","b":1}`},

		// File locals.
		{"file locals", "'&x': 2\n---\na: ${x * x}\n", `{"a":4}`},
		{"file locals use each other in any order", "'&b': ${a + 1}\n'&a': 1\n'&c': 3\n---\nv: ${[a, b, c]}\n", `{"v":[1,2,3]}`},
		{"local function", "'&f': ${function(x) x * 2}\n---\nv: ${f(3)}\n", `{"v":6}`},
		{"local holding a mapping", "'&p':\n  num: 1\n---\na: ${p.num}\n", `{"a":1}`},
		{"locals cover a list", "'&p': {num: 1}\n---\na: ${p.num}\n---\nb: ${p.num + 1}\n", `[{"a":1},{"b":2}]`},
		{"locals anywhere", "a: ${q}\n---\n'&q': 5\n---\nb: ${q}\n", `[{"a":5},{"b":5}]`},
		{"an explicit first document is a list", "---\na: 1\n---\n'&x': 1\n", `[{"a":1}]`},
		{"locals only", "'&x': 1\n", `[]`},
		{"an empty mapping is a document", "{}\n---\n'&x': 1\n", `{}`},
		{"directive", "%YAML 1.1\n---\n'&x': 1\n---\nb: ${x}\n", `{"b":1}`},
		{"document end marker", "a: 1\n...\n---\n'&x': 1\n---\nb: ${x}\n", `[{"a":1},{"b":1}]`},
		{"$ is the document in a single-document file", "'&x': 1\n---\na: ${x}\nb: ${$.a}\n", `{"a":1,"b":1}`},

		// Local keys.
		{"local keys", "spec:\n  '&port': 8080\n  ports: [{containerPort: '${port}'}]\n  args: ['${std.toString(port)}']\n", `{"spec":{"args":["8080"],"ports":[{"containerPort":8080}]}}`},
		{"a local key sees its mapping", "m:\n  '&n': ${self.name}\n  name: web\n  v: ${n}\n", `{"m":{"name":"web","v":"web"}}`},
		{"a local key hides an outer one", "'&x': 1\n---\na:\n  '&x': 2\n  v: ${x}\nb: ${x}\n", `{"a":{"v":2},"b":1}`},
		{"a mapping of local keys is empty", "a:\n  '&x': 1\n", `{"a":{}}`},
		{"a merge copies local keys", "a: &a {'&p': 1, v: '${p}'}\nb:\n  <<: *a\n  '&p': 2\n", `{"a":{"v":1},"b":{"v":2}}`},

		// Anchors.
		{"anchor", "spec:\n  replicas: &r 3\n  template:\n    num: ${r}\n", `{"spec":{"replicas":3,"template":{"num":3}}}`},
		{"anchor after its use", "a: {x: '${y}'}\nb: [&y 1]\n", `{"a":{"x":1},"b":[1]}`},
		{"an anchored mapping's self is the mapping", "a: &m\n  x: 1\n  z: ${self.x + 1}\nb: ${m.z}\n", `{"a":{"x":1,"z":2},"b":2}`},
		{"an anchored value stays in place", "name: outer\nm:\n  name: inner\n  v: &a ['${ { [self.name]: 1 } }']\nw: ${a}\n", `{"m":{"name":"inner","v":[{"inner":1}]},"name":"outer","w":[{"inner":1}]}`},
		{"an alias copies an anchored node", "a: &l {x: 1}\nb: *l\nc: ${l}\n", `{"a":{"x":1},"b":{"x":1},"c":{"x":1}}`},
		{"anchor in a list document", "- &x 1\n- ${x + 1}\n", `[1,2]`},
		{"anchor hides a file local", "'&x': 1\n---\na: &x 2\nb: ${x}\n", `{"a":2,"b":2}`},
		{"a loop outside the anchor's list", "num: &n 2\nl:\n  - ${for i in std.range(1, n)}: ${i}\n", `{"l":[1,2],"num":2}`},
		{"a key outside the anchor's mapping", "m: {a: &n x}\no:\n  ${n}: 1\n", `{"m":{"a":"x"},"o":{"x":1}}`},
		{"a local key hides the anchor from the keys inside", "'&key': foo\nm: {'${key}': 1, value: &key bar}\n", `{"m":{"foo":1,"value":"bar"}}`},
		{"a field name in a key isn't the local", "m:\n  name: &name foo\n  ${'k' + $.l.name + {name: 1}.name}: 1\nl: {name: v}\n", `{"l":{"name":"v"},"m":{"kv1":1,"name":"foo"}}`},
		{"anchor in a sequence item", "- name: &n web\n  v: ${n}\n", `[{"name":"web","v":"web"}]`},
		{"anchors through hidden and quoted keys", "my.cfg::\n  v: &v 1\n0x1F: {a: &k 2}\nw: ${[v, k]}\n", `{"31":{"a":2},"w":[1,2]}`},
		{"an anchor under + is the combined value", "<<+: ${ {labels: {a: '1'}} }\nlabels+: &l {b: '2'}\ncopy: ${l}\n", `{"copy":{"a":"1","b":"2"},"labels":{"a":"1","b":"2"}}`},
		{"items of a list under + count from the end", "<<+: ${ {args: ['x', 'y']} }\nargs+: [&a z]\nv: ${a}\n", `{"args":["x","y","z"],"v":"z"}`},
		{"std is no name for a local", "--- !std\na: &std {x: 1}\nb: *std\nc: ${std.length([1, 2])}\nd:\n  <<+: ${ {} }\n", `{"std":{"a":{"x":1},"b":{"x":1},"c":2,"d":{}}}`},
		{"items of x and x+", "args: [&f a]\nargs+: [b, &l c]\nv: ${[f, l]}\n", `{"args":["a","b","c"],"v":["a","c"]}`},
		{"<<+ sees the document's anchors", "a:\n  <<+: ${l}\n  x: 1\nl: &l {z: 2}\n", `{"a":{"x":1,"z":2},"l":{"z":2}}`},
		{"plain anchors don't bind", "a: &my-a\n  b: 1\nc: *my-a\nd: &if {e: '${self.f}', f: 2}\n", `{"a":{"b":1},"c":{"b":1},"d":{"e":2,"f":2}}`},

		// Comprehensions.
		{"for in a list", "- a\n- ${for x in [1, 2]}:\n    v: ${x}\n- b\n", `["a",{"v":1},{"v":2},"b"]`},
		{"if in a list", "- ${if true}: a\n- ${if false}: b\n- c\n", `["a","c"]`},
		{"only a for", "l:\n  - ${for x in [1, 2]}: ${x * 10}\n", `{"l":[10,20]}`},
		{"for with filters and nested fors", "- ${for x in [1, 2, 3] if x != 2 for y in ['a']}: ${y + x}\n", `["a1","a3"]`},
		{"a list body is one element", "- ${for x in [1, 2]}: [${x}, ${x}]\n", `[[1,1],[2,2]]`},
		{"an if then is a computed key", "${if true then 'k'}: 1\n${if false then 'j'}: 2\n", `{"k":1}`},
		{"loops nest", "- ${for a in ['x', 'y']}:\n    - ${for b in [1, 2]}: ${a + b}\n", `[["x1","x2"],["y1","y2"]]`},
		{"a body's self is the item", "- ${for x in [1]}:\n    a: ${x}\n    b: ${self.a + 1}\n", `[{"a":1,"b":2}]`},
		{"local keys in a body", "- ${for x in [1, 2]}:\n    '&y': ${x * 2}\n    v: ${y}\n", `[{"v":2},{"v":4}]`},
		{"clauses see the locals around them", "'&xs': [1, 2]\n---\nm:\n  '&n': 10\n  l:\n    - ${for x in xs}: ${x + n}\n", `{"m":{"l":[11,12]}}`},
		{"hidden fields in a body", "- ${if true}:\n    h:: 1\n    v: ${self.h}\n", `[{"v":1}]`},
		{"a + list with a loop", "<<+: ${ {args: ['a']} }\nargs+:\n  - ${for x in ['b', 'c']}: ${x}\n", `{"args":["a","b","c"]}`},
		{"an alias copies a loop", "a: [&l-1 {'${for x in [1]}': '${x}'}]\nb: [*l-1]\n", `{"a":[1],"b":[1]}`},
		{"quoted clauses", "l: ['${for x in [1, 2]}': '${x}', '${if true}': 3]\n", `{"l":[1,2,3]}`},
		{"anchors around a loop", "l:\n  - &a 1\n  - ${for x in [2, 3]}: ${x}\n  - &b 4\nv: ${[a, b]}\n", `{"l":[1,2,3,4],"v":[1,4]}`},
		{"an anchored list with a loop", "l: &l\n  - ${for k in ['b']}: ${k}\nv: ${l}\n", `{"l":["b"],"v":["b"]}`},
		{"<<+ adds fields when a condition holds", "a: 1\n<<+: ${if true then {b: 2}}\nm:\n  a: 1\n  <<+: ${if false then {b: 2}}\n", `{"a":1,"b":2,"m":{"a":1}}`},
		{"a loop in a yaml block", "l: ${\\{\n    - ${for x in [1, 2]}: ${x}\n  }}\n", `{"l":[1,2]}`},

		// YAML blocks.
		{"yaml block in a comprehension", `'&p': {a: 1, b: 2}
---
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
		{"anchors in a yaml block", "a: ${\\{\n    v: ${x}\n    w: &x 3\n  }}\n", `{"a":{"v":3,"w":3}}`},
		{"anchor on a yaml block's root", "a: ${\\{\n    &r\n    x: 1\n    z: ${r.x + 1}\n  }}\n", `{"a":{"x":1,"z":2}}`},
		{"yaml block in a local", "'&labels': ${\\{\n    app: web\n    tier: ${'front'}\n  }}\n---\nl: ${labels}\n", `{"l":{"app":"web","tier":"front"}}`},
		{"$ and self in a yaml block", "name: nm\nx: ${[\\{\n    ref: ${$.name}\n    me: ${self.ref}\n  }]}\n", `{"name":"nm","x":[{"me":"nm","ref":"nm"}]}`},
		{"flow collection in a yaml block", "a: ${\\{\n    m: {x: 1,\n      }\n  }}\n", `{"a":{"m":{"x":1}}}`},
		{"empty yaml block", "a: ${\\{\n  }}\n", `{"a":null}`},
		{"local keys in a yaml block", "a: ${\\{\n    '&x': 2\n    b: ${x}\n  }}\n", `{"a":{"b":2}}`},
		{"anchor in a comprehension's yaml block", "a: ${[\\{\n    num: &twice ${i * 2}\n    m: {v: '${twice}'}\n  } for i in [1]]}\n", `{"a":[{"m":{"v":2},"num":2}]}`},
		{"jsonnet object extension", "a: ${ {x: 1} + {y: ${z: 1}.z} }\nz: 2\n", `{"a":{"x":1,"y":1},"z":2}`},

		// Field modifiers, merge keys and <<+.
		{"hidden field", "_config::\n  r: 1\nv: ${self._config.r}\n", `{"v":1}`},
		{"visibility is inherited", "<<+: ${ {h:: 1} }\nh: 2\n", `{}`},
		{"forced visible", "<<+: ${ {h:: 1} }\nh::: 2\n", `{"h":2}`},
		{"<<+ extends an expression", "d:\n  <<+: ${ {spec: {a: 1, b: 2}, h:: 3} }\n  spec+:\n    b: 3\n  shown: ${self.h}\n", `{"d":{"shown":3,"spec":{"a":1,"b":3}}}`},
		{"+ on lists and numbers", "<<+: ${ {args: ['a'], n: 1} }\nargs+: [b]\nn+: 2\n", `{"args":["a","b"],"n":3}`},
		{"+ with nothing inherited", "a+: {x: 1}\n", `{"a":{"x":1}}`},
		{"<<+ null", "a:\n  <<+: ${null}\n  b: 1\n", `{"a":{"b":1}}`},
		{"<<+ list: an earlier source wins", "a:\n  <<+: ['${ {x: 1, w: 1} }', {w: 2, z: 2}]\n", `{"a":{"w":1,"x":1,"z":2}}`},
		{"<<+ alias", "base: &b {x: 1}\nd:\n  <<+: *b\n  w: ${super.x + 1}\n", `{"base":{"x":1},"d":{"w":2,"x":1}}`},
		{"<<+ fields see the mapping", "d:\n  <<+: ${ {v: self.w} }\n  w: 7\n", `{"d":{"v":7,"w":7}}`},
		{"<<+ quoted", "d:\n  <<+: '${ {a: 1} }'\n", `{"d":{"a":1}}`},
		{"merge and <<+", "a: &a {x: 1}\nc:\n  <<: *a\n  <<+: ${ {x: 0, w: 1} }\n", `{"a":{"x":1},"c":{"w":1,"x":1}}`},
		{"merge and +", `containers:
  - &app
    name: app
    image: registry.example.com/app:1.4
    resources: {limits: {memory: 512Mi}}
  - <<: *app
    name: worker
    args: [--worker]
    resources+:
      limits+:
        cpu: "2"
`, `{"containers":[{"image":"registry.example.com/app:1.4","name":"app","resources":{"limits":{"memory":"512Mi"}}},` +
			`{"args":["--worker"],"image":"registry.example.com/app:1.4","name":"worker","resources":{"limits":{"cpu":"2","memory":"512Mi"}}}]}`},
		{"merged expressions that own keys override", "a:\n  <<: [{x: '${1}', w: '${2}'}, {w: '${3}'}]\n  x: 0\n", `{"a":{"w":2,"x":0}}`},
		{"keys name fields as YAML reads them", "a: {0777: p, '0777': q}\nb: {0777: p, 0777+: q, 511+: r}\n", `{"a":{"0777":"q","511":"p"},"b":{"0777":"q","511":"pr"}}`},
		{"merged keys compare as YAML reads them", "a:\n  <<: {0x1F: a}\n  31: b\n", `{"a":{"31":"b"}}`},
		{"merged expression keys compare as text", "a:\n  <<: {'${\"x\"}': 1}\n  ${\"x\"}: 2\n", `{"a":{"x":2}}`},
		{"<<+ of a local key", "a:\n  '&base': ${ {x: 1} }\n  b:\n    <<+: ${base}\n    w: ${super.x + 1}\n", `{"a":{"b":{"w":2,"x":1}}}`},
		{"x:: and x+: make a hidden field", "c:\n  r::\n    m: 1\n  r+: {c: 2}\n  v: ${self.r}\n", `{"c":{"v":{"c":2,"m":1}}}`},
		{"literal key ending in a colon", "${'a:'}: 1\n${'b+'}: 2\n${'&'}: 3\n", `{"&":3,"a:":1,"b+":2}`},

		// Names.
		{"a name makes an object", "'&names': {prefix: p-}\n--- !a\nmetadata:\n  name: ${names.prefix + 'a'}\n---\nmetadata:\n  name: ${$.a.metadata.name + '-copy'}\n",
			`{"a":{"metadata":{"name":"p-a"}},"unnamed":[{"metadata":{"name":"p-a-copy"}}]}`},
		{"hidden name", "--- !config::\nnum: 1\n--- !a\nv: ${$.config.num}\n", `{"a":{"v":1}}`},
		{"forced visible name", "--- !a:::\nnum: 1\n", `{"a":{"num":1}}`},
		{"name tag", "--- !web.conf\nkind: ConfigMap\n--- !b\nv: ${$['web.conf'].kind}\n", `{"b":{"v":"ConfigMap"},"web.conf":{"kind":"ConfigMap"}}`},
		{"name tag with colons", "--- !web.conf::\nkind: ConfigMap\n--- !b\nv: ${$['web.conf'].kind}\n", `{"b":{"v":"ConfigMap"}}`},
		{"name tag on its own line", "!cm\nkind: ConfigMap\n", `{"cm":{"kind":"ConfigMap"}}`},
		{"named list and expression", "--- !l\n- 1\n--- !lib::\n${ {f: 2} }\n--- !v\nx: ${$.lib.f}\n", `{"l":[1],"v":{"x":2}}`},
		{"an anchor on a named document", "--- !web.conf &wc\nkind: ConfigMap\nv: ${wc.kind}\n", `{"web.conf":{"kind":"ConfigMap","v":"ConfigMap"}}`},
		{"a number as a name", "--- !123\na: 1\n", `{"123":{"a":1}}`},
		{"name with dashes", "--- !my-svc\na: 1\n", `{"my-svc":{"a":1}}`},
		{"plain and + documents combine", "--- !web\na: 1\nb: {c: 1}\n--- !web+\nb+: {d: 2}\n", `{"web":{"a":1,"b":{"c":1,"d":2}}}`},
		{"+ document first, hidden plain one", "--- !web+\nb+: {d: 2}\n--- !web::\nb: {c: 1}\n--- !v\nx: ${$.web.b}\n", `{"v":{"x":{"c":1,"d":2}}}`},
		{"+ document alone", "--- !web+\nb: 1\n", `{"web":{"b":1}}`},
		{"hidden + document", "--- !c+::\nx: 1\n--- !v\nw: ${$.c.x}\n", `{"v":{"w":1}}`},
		{"unnamed documents", "--- !a\nx: 1\n---\nw: ${$.a.x}\n---\nz: 3\n", `{"a":{"x":1},"unnamed":[{"w":1},{"z":3}]}`},
		{"file locals see the file", "'&num': ${$.a.x}\n--- !a\nx: 1\n--- !b\nv: ${num}\n", `{"a":{"x":1},"b":{"v":1}}`},
		{"a document with only a name", "--- !a\n", `{"a":null}`},
		{"named document with <<+", "'&base': {x: 1}\n--- !d\n<<+: ${base}\nw: &w 2\nz: ${w}\n", `{"d":{"w":2,"x":1,"z":2}}`},
		{"a document's name in itself", "--- !root\nname: nm\nm:\n  v: ${$.root.name}\n", `{"root":{"m":{"v":"nm"},"name":"nm"}}`},
		{"an anchor on a document's root", "&r\nname: nm\nm:\n  v: ${r.name}\n", `{"m":{"v":"nm"},"name":"nm"}`},
		{"a name isn't a local", "--- !labels\nx: 1\n--- !d\nm: &labels {z: 2}\nv: ${[labels, $.labels]}\n", `{"d":{"m":{"z":2},"v":[{"z":2},{"x":1}]},"labels":{"x":1}}`},
		{"a file local named like a document", "'&x': 1\n--- !x\nv: ${[x, $.x.w]}\nw: 2\n", `{"x":{"v":[1,2],"w":2}}`},
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
			// The readable code means the same.
			if got, err := evalWith(t, Desugar, tc.src); err != nil || got != compact(t, tc.want) {
				code, _ := Desugar(tc.src)
				t.Errorf("desugared: got %s, %v\n%s", got, err, code)
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
		{"a: ${x}-svc\n", "line 1, column 8: unexpected text after an expression"},
		{"a: ${1} ${2}\n", "unexpected text after an expression"},
		{"a: '${v}-alpine'\n", "line 1: a string that starts with ${ must be exactly one expression"},
		{"a: \"${y\"\n", "line 1: a string that starts with ${ must be exactly one expression"},
		{"a: |\n  ${a} ${b}\n", "line 1: a string that starts with ${ must be exactly one expression"},
		{"fold: >\n  ${a}\n   ${b}\n\nafter: x\n", "must be exactly one expression"},
		{"top: |2\n   ${indented}\n  ${x}\n", "must be exactly one expression"},
		{"${0}\n\"\n---", "line 1: a string that starts with ${ must be exactly one expression"}, // a quote continuing the expression's plain scalar
		{"a: ${1}\n  more\n", "line 1: a string that starts with ${ must be exactly one expression"},
		{"a: !!str ${1}\n", "line 1: !!str can't turn an expression into text: write $${"},
		{"a: !!str '${HOME}'\n", "!!str can't turn an expression into text"},
		{"a: !!str |\n  ${x}\n", "!!str can't turn an expression into text"},
		{"{a: !<tag:yaml.org,2002:str> '${HOME}'}\n", "can't turn an expression into text"},
		{"a: yes\n", `line 1: unquoted "yes" is a boolean to kubectl, but a string to kustomize`},
		{"x: 1\non: push\n", `line 2: unquoted "on" is a boolean`},
		{"a: [N]\n", `unquoted "N" is a boolean`},
		{"a: ${local x = 1}\n", "line 1, column 4: ${local ...} declarations are gone"},
		{"${local x = 1}\na: ${x}\n", "line 1, column 1: ${local ...} declarations are gone"},
		{"a: '${local x = 1}'\n", "line 1: ${local ...} declarations are gone"},
		{"a: |-\n  ${local x = 1}\n", "line 2, column 3: ${local ...} declarations are gone"},
		{"a: [${local x = 1}]\n", "declarations are gone"},
		{"a: '${}'\n", "line 1: empty expression"},
		{"a: '${[\\{\n  b: 1\n  }]}'\n", `line 1: inside an expression, \{ starts a YAML block`},
		{"a: \"${[\\\\{\\n  b: 1\\n}]}\"\n", "line 1: a quoted expression cannot contain a YAML block"},
		{"a: '${\"x}'\n", "line 1: unterminated ${"},
		{"a: 1\nb: '${error \"boom\"}'\n", "test.yakl:2:"},
		{"a: |-\n  ${\"x}\n", "line 2, column 3: unterminated ${"},
		{"a: |\n  ${ }\n\n", "line 2, column 3: empty expression"},
		{"a: |-\n  ${\"x}\nb: '\"}'\n", "line 2, column 3: unterminated ${"},
		{"a: &v\n  '${error \"boom\"}'\n", "test.yakl:2:"},
		{"&k-1 '${\"k\"}': 1\nx:\n  *k-1 : 2\nc: ${error 'boom'}\n", "test.yakl:4:"},
		{"a: |-\n  ${\n    1 +\n    error 'deep'\n  }\n", "test.yakl:4:"},
		{"a: &my-spec\n  b: 1\n  c: ${my-spec.b}\n", "test.yakl:3:"},
		{"a:\n  x: &x {}\n  b: ${x.c}\n", "test.yakl:3:"},
		{"a: !!int 3\n", "line 1: unsupported tag !!int"},
		{"a: !Ref x\n", "unsupported tag !Ref"},
		{"a: !Ref {b: 1}\n", "line 1: unsupported tag !Ref"},
		{"v: ! true\n", "line 1, column 4: the non-specific tag ! is not supported"},
		{"v: [! 1]\n", "the non-specific tag ! is not supported"},
		{"a: !!int [1, 2]\n", "unsupported tag !!int"},
		{"a: &a [1]\n*a : 2\n", "only scalar mapping keys"},
		{"? [a]\n: 1\n", "only scalar mapping keys"},
		{"a: &a [*a]\n", `line 1: anchor "a" contains an alias to itself`},
		{"a: &a {b: [1, {c: *a}]}\n", `anchor "a" contains an alias to itself`},
		{"a: &a {b: {<<: *a}}\n", `anchor "a" contains an alias to itself`},
		{laughs(7), "aliases expand to more than 100000 nodes"},
		{"m: &m\n" + strings.Repeat("  - 1\n", 1000) + "l: [" + strings.Repeat("*m, ", 300) + "]\n", "aliases expand to more than 100000 nodes"},
		{"a: &x ${\n  1\n}\nb: *x\n", "line 1: an alias cannot repeat a multi-line expression"},
		// Jsonnet locations match the source for YAML literals too.
		{"a: 1\nb:\n  c: [1, 2]\na: 3\n", `line 4: duplicate key "a"`},
		// Like std.parseYaml, values JSON cannot represent are rejected.
		{"a: .inf\n", "unsupported value: +Inf"},
		{"~: 1\n", "unsupported map key"},
		// YAML error lines are reported in source terms even after multi-line expressions.
		{"a: ${\n\n\n1}\nb: [\n", "line 5"},
		// Jsonnet errors point at the source line of the expression.
		{"a: 1\n\nb: ${error 'boom'}\n", "test.yakl:3:"},
		{"a: ${\n  1 +\n  error 'deep'\n}\n", "test.yakl:3:"},
		{"'&x': 1\n'&z': ${error 'decl'}\n---\nv: ${z}\n", "test.yakl:2:"},
		{"'&x': 1\n--- !a\nv: ${x}\nw: ${error 'boom'}\n", "test.yakl:4:"},
		{"&a\nx: 1\n---\nv: ${a.x}\n", "test.yakl:4:"},
		{"a:\n  b: &l\n    c: 1\n  d: ${l.e}\n", "test.yakl:4:"},
		{"${1}: x\n", "Field name must be string"},
		{"a: &k name\nname: 1\n*k : 2\n", `line 3: duplicate key "name"`},
		{"a: ${ {b: 1} + \\{c: 1} }\n", `line 1, column 16: inside an expression, \{ starts a YAML block and must end its line`},
		{"a: ${\\{\n    b: 1\n", "line 1, column 6: unterminated YAML block"},
		{"a: ${\\{\nb: 1\n}}\n", "unterminated YAML block"},
		{"a: ${[\\{\n    b: 1\n  c: 2\n  }]}\n", "line 3, column 3: a YAML block in an expression must be closed by }"},
		{"a: ${\\{\n    b: c: d\n  }}\n", "line 2"},
		{"a: ${[\\{\n    b: ${error 'nested'}\n  }]}\n", "test.yakl:2:"},
		{"a: ${\\{\n    '&': x\n    b: 1\n  }}\n", `line 2: the key "&" doesn't name a document: a tag does, on a line of its own where the document starts, as in !web`},
		// Code after a nested block keeps the line of the block's closing }.
		{"a: ${\\{\n    b: |\n      x\n\n  } + error 'boom'}\n", "test.yakl:5:"},

		// Field modifiers, merge keys and <<+.
		{"x: 1\nx:: 2\n", `line 2: the keys "x" and "x:" give the field "x" twice`},
		{"x+: 1\nx+: 2\n", `duplicate key "x+"`},
		{"x+: 1\n'x+:': 2\n", `the keys "x+" and "x+:" give the field "x" twice`},
		{"x: 1\nx+:: 2\n", `line 2: "x+:" next to "x" must be "x+"`},
		{"x:::: 1\n", `line 1: the key "x:::" ends in more than two colons`},
		{"a: 1\n<<: {b: 1}\n", "line 2: the merge key (<<) must be the first key"},
		{"<<: {b: 1}\n<<: {c: 1}\n", "must be the first key of a mapping, and appear once"},
		{"<<: 1\n", "line 1: a merge key (<<) copies mappings"},
		{"<<+: 1\n", "line 1: <<+ extends an expression, a mapping, or a list of them"},
		{"a:\n  <<+: ${[1]}\n", "<<+ needs an object or null, not array"},
		{"a:\n  <<+: ${'s'}\n  b: 1\n", "<<+ needs an object or null, not string"},
		{"a:\n  <<+: ${base}\n  '&base': {x: 1}\n", "Unknown variable: base"},

		// Names and locals.
		{"--- !a\nx: 1\n--- !a\ny: 2\n", `line 3: the name "a" is already given on line 1`},
		{"--- !a+\n--- !a+\n", `the name "a" is already given`},
		{"--- !a\n--- !a+::\n", `the name "a" is already given`},
		{"name: &name foo\n${name}: 1\n", "line 2: ${name} uses the local name, which reads line 1 from what ${name} builds"},
		{"- &count 2\n- ${for i in std.range(1, count)}: ${i}\n", "line 2: ${for i in std.range(1, count)} uses the local count, which reads line 1"},
		{"l:\n  - ${if n > 1}: 1\n  - [&n 2]\n", "line 2: ${if n > 1} uses the local n, which reads line 3"},
		{"m: {a: &n x}\n${n}: 1\n", "line 2: ${n} uses the local n, which reads line 1"},
		{"args:\n  - ${for x in [1]}: ${x}\n  - &f a\nargs+: [b]\n", "line 3: &f in a list item whose index depends on ${for} or ${if} items"},
		{"x: &a {args+: [b]}\ny:\n  <<: *a\n  args:\n    - ${for x in [1]}: ${x}\n    - &f a\n", "line 6: &f in a list item whose index depends on ${for} or ${if} items"},
		{"{'${': 1, value: &value 2}\n", "line 1: a string that starts with ${ must be exactly one expression"},
		{"x: {args: [{'${if true}': a}, &l b]}\nx+: {args+: [c]}\n", "line 1: &l in a list item whose index depends on ${for} or ${if} items"},
		{"'&std': 1\n", `"std" must be a Jsonnet variable name, not a keyword or std`},
		{"a: ' ${a} ${b}'\n", "line 1: a string that starts with ${ must be exactly one expression"},
		{"a: &x 1\nb: &x 2\n", `line 2: the anchor &x is also on line 1: a local can't have two values`},
		{"a:\n  '&x': 1\n  '&x': 2\n", `line 3: duplicate key "&x"`},
		{"'&': a\nb: 1\n", `line 1: the key "&" doesn't name a document: a tag does, on a line of its own where the document starts, as in !web`},
		{"'&+': a\n", `line 1: the key "&+" doesn't name a document`},
		{"a:\n  '&': x\n", `line 2: the key "&" doesn't name a document`},
		{"--- !+\na: 1\n", "line 1: the tag !+ names no field"},
		{"--- !a::::\n", "the tag !a:::: ends in more than three colons"},
		{"a: !web {b: 1}\n", "line 1: unsupported tag !web: a tag like it names a document, on the document's root only"},
		{"'&web.conf':\nkind: ConfigMap\n", `line 1: the key "&web.conf" declares a local, so "web.conf" must be a Jsonnet variable name, not a keyword or std; to name a document, tag it: !web.conf`},
		{"'&web+':\nb: 1\n", `"web+" must be a Jsonnet variable name, not a keyword or std; to name a document, tag it: !web+`},
		{"'&web':\nkind: ConfigMap\n", `line 1: the local key "&web" has no value; to name the document, tag it: !web`},
		{"'&if': 1\n", `"if" must be a Jsonnet variable name, not a keyword`},
		{"'&-x': 1\n", `"-x" must be a Jsonnet variable name, not a keyword`},
		{"--- !unnamed\nx: 1\n", `the name "unnamed" is reserved`},
		{"--- !unnamed+\n", `the name "unnamed" is reserved`},
		{"'&x': {a: &y 1}\n", "line 1: &y in a document of local keys: the node has no place of its own in the output"},
		{"&f\n'&x': 1\n", "line 1: &f in a document of local keys: the node has no place of its own in the output, so its anchor can't declare a local"},
		{"m:\n  '&x': {a: &y 1}\n", "line 2: &y in the value of a local key"},
		{"<<: &b {x: 1}\n", "line 1: &b in a merge source (<<)"},
		{"a: &a {x: 1}\nb:\n  <<: [{z: &c 1}, *a]\n", "line 3: &c in a merge source (<<)"},
		{"a:\n  <<+: &b '${ {} }'\n", "line 2: &b in a source of <<+"},
		{"${'k'}: {a: &b 1}\n", "line 1: &b under a key expression"},
		{"&web apiVersion: v1\n", `line 1: &web anchors the key "apiVersion", which has no place of its own in the output; an anchor for the whole document goes on its own line`},
		{"a: {&k b: 1}\n", `&k anchors the key "b"`},
		{"&web: value\n", "line 1: &web: anchors an empty key: write '&web': for a local key, or tag the document to name it: !web"},
		{"a:\n  &port: 8080\n", "line 2: &port: anchors an empty key"},
		{"&web.conf:\nx: 1\n", "did not find expected alphabetic or numeric character"},
		{"&config- &config\nx: 1\n", "did not find expected <document start>"},
		{"'&x': ${$.a}\n---\nb: 1\n", "line 1: $ in a file local, in a file without names"},
		{"'&x': {w: '${[$]}'}\n---\nb: 1\n", "$ in a file local"},
		{"a: ${\\{\n    !Ref\n    b: 1\n  }}\n", "line 2: unsupported tag !Ref"},
		{"&k\n'${x}': 1\n", "test.yakl:2:"},
		{"a: '${0)(}'\n", "line 1: unmatched ) in an expression"},

		// Comprehensions.
		{"- ${for x in [1]}:\n  v: 1\n", "line 1: in a list, ${for x in [1]} must be the item's only key, with the body indented under it"},
		{"- a: 1\n  ${if true}: {b: 2}\n", "line 2: in a list, ${if true} must be the item's only key"},
		{"- ${for x in [1]}:\n", "line 1: ${for x in [1]} has no body: indent the body under the clause"},
		{"${for x in [1]}: ${x}\n", `line 1: ${for x in [1]} can only be a list item, as in "- ${for x in [1]}:"; to add fields to a mapping, use <<+: ${...}`},
		{"m:\n  a: 1\n  ${if true}:\n    a: 2\n", "line 3: ${if true} can only be a list item"},
		{"- ${if true}:\n- b\n", "line 1: ${if true} has no body"},
		{"${for x in [1]}+: {a: 1}\n", "unexpected text after an expression"},
		{"- ${for x in [1]}:\n    a: &a ${x}\n", "line 2: &a in the body of a ${for} or ${if}: the body is repeated or left out, so its anchor can't declare a local"},
		{"- ${if true}: 1\n- &a 2\n- ${if true}: 3\n", "line 2: &a in a list item whose index depends on ${for} or ${if} items, so its anchor can't declare a local"},
		{"<<+: ${ {l: []} }\nl+:\n  - [&a 1]\n  - ${if true}: 3\n", "line 3: &a in a list item whose index depends on ${for} or ${if} items"},
		{"- &a\n  ${if true}: 1\n", "line 1: &a in the body of a ${for} or ${if}"},
		{"- ${for x in xs}:\n    a: 1\n", "Unknown variable: xs"},
		{"- ${for x in [1]}:\n    a: 1\n    b: ${y}\n", "test.yakl:3:"},
		{"- ${for x in [1] if}: 1\n", "test.yakl:1:"},
	}
	for _, tc := range cases {
		_, err := eval(t, tc.src)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: got error %v, want %q", tc.src, err, tc.want)
		}
	}
}

// Readable code drops parentheses, which must not make a malformed expression pass for fields.
func TestDesugarSyntaxError(t *testing.T) {
	if code, err := Desugar("a: '${1, b: 2}'\n"); err == nil {
		t.Errorf("no error, code:\n%s", code)
	}
}

// Each mapping merges the one before ten times, so expanding every merge would take 10^30 steps.
func TestMergeChain(t *testing.T) {
	src := "a0: &a0 {x: 1}\n"
	for i := 1; i <= 30; i++ {
		src += fmt.Sprintf("a%d: &a%d {<<: [%s]}\n", i, i, strings.TrimSuffix(strings.Repeat(fmt.Sprintf("*a%d, ", i-1), 10), ", "))
	}
	if _, err := Compile(src); err != nil {
		t.Fatal(err)
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
	for _, s := range []string{"a: ${1}\n", "- |\n  ${x}\n- ${y}\n", "'&x': 1\n---\n[${x}, '${']\n", "a: 'b\n  c' # ${\n", "[a [", "- k: |\n    x\n  e: ${1}\n", "a: ${[\\{\n    b: ${[\\{\n        c: 1\n      }]}\n  }]}\n", "a: &a [*a]\n", "--- !a\nb: &z ${1}\n--- !c+\n<<+: [${a}]\nd+: 1\ne: {'&l': '${z}'}\n", "a: ['${1}', \"${'}'}\"]\n'${k}': !!str '${v}'\n", "a: |-\n  ${[\\{\n    b: 1\n  }]}\nc: $${d}\n", "- ${for x in [1]}:\n    a: &a ${x}\n- &b 2\nm:\n  c: 1\n  <<+: ${if true then {d: 2}}\n", "# a\n\n--- !b # c\n# d\nx: 1 # e\n<<: {f: 1} # g\n# h\nl:\n  # i\n  - ${for y in [1]}: 2 # j\n  # k\n"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		code, err := Compile(src)
		if err != nil && strings.Contains(err.Error(), "internal error") {
			t.Fatal(err)
		}
		// Readable code drops parentheses, so it must still parse wherever the compiled code does.
		if err == nil && parses(code) {
			if _, err := Desugar(src); err != nil {
				t.Fatalf("%v\ncompiled:\n%s", err, code)
			}
		}
		// Formatting checks that it keeps the meaning.
		if _, err := Format(src); err != nil && strings.Contains(err.Error(), "internal error") {
			t.Fatal(err)
		}
	})
}

// yaklFiles returns the examples, and every YAML snippet in the spec, by name.
func yaklFiles(t *testing.T) map[string]string {
	t.Helper()
	files := map[string]string{}
	for _, pattern := range []string{"../../examples/*.yakl", "../../examples/*/*.yakl", "../../testdata/*.yakl", "testdata/*/*.yakl"} {
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
	const spec = "../../docs/Advanced-Usage/yakl.md"
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
	return files
}

// notYAML returns why src isn't valid YAML, as read by kubectl, if it isn't.
func notYAML(src string) error {
	dec := utilyaml.NewYAMLOrJSONDecoder(strings.NewReader(src), 4096)
	for {
		var v any
		if err := dec.Decode(&v); errors.Is(err, io.EOF) {
			return nil
		} else if err != nil {
			return err
		}
	}
}

// YAKL files must be valid YAML, as read by kubectl, so that YAML tooling can process them.
// That holds for the examples and for every YAML snippet in the spec.
func TestFilesAreYAML(t *testing.T) {
	for name, src := range yaklFiles(t) {
		if err := notYAML(src); err != nil {
			t.Errorf("%s: not valid YAML: %v", name, err)
		}
	}
}

func TestFormat(t *testing.T) {
	cases := []struct{ src, want string }{
		{"a: ${1+2} # c\n", "a: ${1 + 2} # c\n"},
		{"${'k'+'x'}: ${std.length( [1] )}\n", "${'k' + 'x'}: ${std.length([1])}\n"},
		// Quoted where a plain scalar can't hold the code, as before otherwise.
		{"a: ${ {b:1} }\n", "a: '${ { b: 1 } }'\n"},
		{"a: ${ {b:'x'} }\n", "a: \"${ { b: 'x' } }\"\n"},
		{"a: ${ {b:'it'+\"'s\"} }\n", "a: '${ { b: ''it'' + \"''s\" } }'\n"},
		{"a: [${1+1}, ${2}]\n", "a: ['${1 + 1}', '${2}']\n"},
		{"a: '${ {b:\"c\"} }'\nd: \"${ {e:'f'} }\"\n", "a: '${ { b: \"c\" } }'\nd: \"${ { e: 'f' } }\"\n"},
		// Block expressions keep their lines in the block.
		{"a: |-\n  ${[\n  {b:1}, # c\n      2]}\nd: 1\n", "a: |-\n  ${[\n    { b: 1 },  // c\n    2,\n  ]}\nd: 1\n"},
		{"a: |-\n  ${1+2 # c\n  }\n", "a: |-\n  ${1 + 2  // c\n  }\n"},
		// What follows --- goes on the next line, which is all that kubectl's reader allows.
		{"--- !web\na: ${1+2}\n--- !b # c\nx: 1\n---  # d\n--- |\n  ${3}\n--- ${ {a:1} }\n",
			"---\n!web\na: ${1 + 2}\n---\n!b # c\nx: 1\n---  # d\n---\n|\n  ${3}\n---\n'${ { a: 1 } }'\n"},
		{"a: |\n  --- !x\n", "a: |\n  --- !x\n"},
		// Left as they are: clauses, YAML blocks, ${ in strings and comments.
		{"l:\n  - ${for x in  [1,2]}: ${x*2}\n", "l:\n  - ${for x in  [1,2]}: ${x * 2}\n"},
		{"a: ${ [\\{\n    b:  1\n  }] }\n", "a: ${ [\\{\n    b:  1\n  }] }\n"},
		{"# ${1+1}\na: echo ${b+1}\n", "# ${1+1}\na: echo ${b+1}\n"},
	}
	for _, tc := range cases {
		got, err := Format(tc.src)
		if err != nil || got != tc.want {
			t.Errorf("%q: got %q, %v, want %q", tc.src, got, err, tc.want)
		}
		if again, err := Format(got); err != nil || again != got {
			t.Errorf("%q: formatted again: %q, %v", got, again, err)
		}
	}
	// The examples are formatted, and formatting keeps every snippet of the spec valid YAML.
	for name, src := range yaklFiles(t) {
		got, err := Format(src)
		if err != nil {
			continue // a snippet that shows an error
		}
		if err := notYAML(got); err != nil {
			t.Errorf("%s: formatted, not valid YAML: %v\n%s", name, err, got)
		}
		if got != src && !strings.Contains(name, ".md:") {
			t.Errorf("%s: not formatted, yaklfmt gives:\n%s", name, got)
		}
	}
}

// Readable code keeps the YAML comments, next to the fields and items they're on.
func TestDesugarComments(t *testing.T) {
	const src = `# A web app.

# The settings.
--- !config::
replicas: 2 # the default
# The deployment.
--- !deployment
spec:
  # Scaled by config.
  replicas: ${$.config.replicas}
  args:
    # Verbose first.
    - --verbose
    # One per port.
    - ${for p in [80]}: ${'--port=' + p}
`
	const want = `// A web app.

{
  // The settings.
  config:: {
    replicas: 2,  // the default
  },
  // The deployment.
  deployment: {
    spec: {
      // Scaled by config.
      replicas: $.config.replicas,
      args: [
              // Verbose first.
              '--verbose',
            ] +  // One per port.
            ['--port=' + p for p in [80]],
    },
  },
}
`
	got, err := Desugar(src)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// A document read through $ is the field, so it follows overlays; a nested anchor's local is the
// node as the document writes it, so it doesn't.
func TestBinding(t *testing.T) {
	const src = "--- !config::\nreplicas: 2\n--- !deployment\nmetadata:\n  labels: &labels {app: web}\n" +
		"spec:\n  replicas: ${$.config.replicas}\n  selector: {matchLabels: '${labels}'}\n"
	const overlay = "{config+: {replicas: 5}, deployment+: {metadata+: {labels+: {canary: 'true'}}}}"
	const want = `{"deployment":{"metadata":{"labels":{"app":"web","canary":"true"}},"spec":{"replicas":5,"selector":{"matchLabels":{"app":"web"}}}}}`
	for _, compile := range []func(string) (string, error){Compile, Desugar} {
		code, err := compile(src)
		if err != nil {
			t.Fatal(err)
		}
		got, err := jsonnet.MakeVM().EvaluateAnonymousSnippet("test.yakl", "("+code+") + "+overlay)
		if err != nil {
			t.Fatal(err)
		}
		if got := compact(t, got); got != want {
			t.Errorf("got  %s\nwant %s", got, want)
		}
	}
}

// The worked example of the spec: a stream with names, and an overlay applied as `base + overlay`.
func TestOverlay(t *testing.T) {
	for _, compile := range []func(string) (string, error){Compile, Desugar} {
		testOverlay(t, compile)
	}
}

func testOverlay(t *testing.T, compile func(string) (string, error)) {
	files := map[string]string{}
	for _, name := range []string{"lib.libsonnet", "web.yakl", "prod.yakl"} {
		b, err := os.ReadFile(filepath.Join("testdata", "overlay", name))
		if err != nil {
			t.Fatal(err)
		}
		files[name] = string(b)
		if strings.HasSuffix(name, Ext) {
			if files[name], err = compile(string(b)); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
	}
	vm := jsonnet.MakeVM()
	data := map[string]jsonnet.Contents{}
	for name, code := range files {
		data[name] = jsonnet.MakeContents(code)
	}
	vm.Importer(&jsonnet.MemoryImporter{Data: data})
	get := func(code, query string) string {
		t.Helper()
		out, err := vm.EvaluateAnonymousSnippet("test", "local f = "+code+"; "+query)
		if err != nil {
			t.Fatal(err)
		}
		return compact(t, out)
	}
	for _, tc := range []struct{ query, base, prod string }{
		{"std.objectFields(f)", `["deployment","service","unnamed","web.conf"]`, `["deployment","pdb","service","unnamed","web.conf"]`},
		{"f.deployment.spec.replicas", `2`, `5`},
		{"f.deployment.spec.template.spec.containers[0]",
			`{"image":"nginx:1.27","name":"web","ports":[{"containerPort":8080}],"readinessProbe":{"httpGet":{"path":"/healthz","port":8080}},"volumeMounts":[{"mountPath":"/etc/nginx/conf.d","name":"conf"}]}`,
			`{"image":"nginx:1.29","name":"web","ports":[{"containerPort":8080}],"readinessProbe":{"httpGet":{"path":"/healthz","port":8080}},"volumeMounts":[{"mountPath":"/etc/nginx/conf.d","name":"conf"}]}`},
		{"f.deployment.spec.template.spec.volumes[0].configMap.name", `"web-conf"`, `"web-conf"`},
		{"f.service.spec", `{"ports":[{"name":"web","port":80,"targetPort":8080}],"selector":{"app":"web"}}`, `{"ports":[{"name":"web","port":80,"targetPort":8080}],"selector":{"app":"web"},"type":"LoadBalancer"}`},
		{"[o.kind + '/' + o.metadata.name for o in f.unnamed]", `["NetworkPolicy/web-ingress"]`, `["NetworkPolicy/web-ingress","ServiceAccount/web"]`},
		{"f['web.conf'].data", `{"default.conf":"server { listen 8080; }\n"}`, `{"default.conf":"server { listen 8080; gzip on; }\n"}`},
		{"if std.objectHas(f, 'pdb') then f.pdb.spec", `null`, `{"minAvailable":2,"selector":{"matchLabels":{"app":"web"}}}`},
		{"std.objectHasAll(f, 'config') && !std.objectHas(f, 'config')", `true`, `true`},
	} {
		if got := get("import 'web.yakl'", tc.query); got != tc.base {
			t.Errorf("base %s:\n got %s\nwant %s", tc.query, got, tc.base)
		}
		if got := get("(import 'web.yakl') + (import 'prod.yakl')", tc.query); got != tc.prod {
			t.Errorf("with overlay %s:\n got %s\nwant %s", tc.query, got, tc.prod)
		}
	}
}
