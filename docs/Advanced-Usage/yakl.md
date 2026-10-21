# YAKL: YAML with Jsonnet expressions

!!! warning "Prototype"
    YAKL is an experimental format, enabled with `--alpha` (or `KUBECFG_ALPHA=true`). The
    syntax and semantics below may change.

Jsonnet is a great language, but many people would rather write YAML. Tools like
[kro](https://kro.run) keep YAML as the outer language and embed `${...}` expressions
in it. YAKL (yet another konfig language) does the same, but the language inside `${...}`
is Jsonnet instead of CEL. Files use the `.yakl` extension.

```yaml
foo:
  bar: '${{some: "jsonnet"}.some}'
```

(the quotes are YAML's, needed because of the `: `; they don't make it a string) means the
same as the Jsonnet

```jsonnet
{
  foo: {
    bar: {some: "jsonnet"}.some,
  },
}
```

and evaluates to `{"foo": {"bar": "jsonnet"}}`.

The model is: **a yakl file is YAML in which any node can be written as a Jsonnet
expression.** YAKL is not a text template language. Expressions produce *values* that
are placed in the document tree, so there is no indentation or quoting to get right in the
output (no `nindent`, no `toYaml`), and YAML strings are never interpolated.

On top of that, yakl gives YAML the Jsonnet features that make files reusable, each spelled
in plain YAML:

| You write | Jsonnet meaning |
| --- | --- |
| `${expr}` as a whole string, in any YAML style | the expression's value |
| `$${...}` at the start of a string | the literal text `${...}` |
| `- ${for x in xs}:`, with a body under it | the body once per `x`, as list items |
| `- ${if cond}:`, with a body under it | the body as a list item, if `cond` holds |
| `spec+:` | `spec+:`, extend the inherited field |
| `name::` and `name:::` | hidden, and forced visible |
| `<<+: S` | `S + {the mapping's own fields}` |
| `<<: *base` | YAML merge: copy fields, own keys win |
| `&labels` on a node | `local labels = <the node>` for the whole document; the node stays where it is |
| `'&': web` at the top of a document | field `web:` of the file, and a local `web` that is that field |
| `'&:': config`, `'&+': service` | hidden `config::`, and `service+:` that extends `service` |
| `'&port': 8080` in a mapping | `local port = 8080` for that mapping; not output |
| a document made only of `'&name':` keys | locals of the whole file |
| documents without a name, next to named ones | `unnamed+: [...]` |
| a `.yakl` overlay | `base + overlay`, as written |

Four ideas carry the design:

- **`&name` always declares a local.** On a node, the node stays where it is and also gets a
  name. As a quoted key, `'&port': 8080`, the value is only a local.
- **The key `'&'` names a document.** The document becomes the field with that name in the
  file, and the name is also a local. Like any key, it takes `+` and colons: `'&+': service` extends `service`, and
  `'&:': config` hides `config`.
- **A stream is a list until a document gets a name.** From then on the file is one object, as
  a Jsonnet file usually is.
- **Meaning lives in data and anchors,** never in quoting, key order or custom tags. Tools like
  kubectl, kustomize and prettier rewrite quoting, reorder keys and drop tags, so none of those
  can carry meaning. No editor needs configuring.

## YAML compatibility

**A yakl file is valid YAML**, as read by `kubectl` (`sigs.k8s.io/yaml`), go-yaml v3 (under
kustomize), PyYAML and yamllint, eemeli/yaml (prettier and the VS Code YAML server),
tree-sitter and VS Code's highlighter, so editors, linters and other YAML tools can work with
it. To them, every expression is a single
scalar node, anchors are anchors, and local keys, name keys and key modifiers like `spec+` are
just keys.

Most expressions are valid plain scalars as they are (`replicas: ${params.replicas}`). Those
containing `: ` or ` #`, and those inside flow collections (`[...]`, `{...}`), are written as
[quoted expressions](#quoted-expressions). Those spanning lines are best written as
[block expressions](#block-expressions).

The compiler doesn't enforce all of it: it still accepts unquoted expressions that a YAML
parser would reject or split, like `a: ${ {b: 1}.b }`.

The examples and the snippets in this document are checked with kubectl's YAML parser.

### What round trips keep

kubectl and kustomize parse a file and write new YAML. Everything spelled as data survives
that: expressions, `$${`, key modifiers, `<<+`, local keys (`'&port':`) and field names given
with `'&':`. Anchors don't: both tools expand them. After a round trip, a `${name}` that used an
anchor's local fails with "Unknown variable". kustomize
also needs every document to be a Kubernetes resource, so a file with a document of local keys,
or a hidden settings document, can't go through kustomize at all. A file meant for kustomize
can use local keys instead of anchors. See
[What tools do with it](#what-tools-do-with-it).

## Implementation model

A yakl file is compiled to Jsonnet and then evaluated like any other Jsonnet file:

1. **Lex.** A YAML-aware lexer finds every `${` that starts a YAML node and reads the
   Jsonnet expression up to the matching `}`. It skips comments, quoted scalars and block
   scalars, except those whose whole value is an expression. YAML blocks nested inside
   expressions are lexed the same way, recursively. Expressions are replaced by opaque
   placeholders.
2. **Parse.** The result, now plain YAML, is parsed as a YAML stream. The parser also checks
   the lexer: every placeholder must come back as a whole scalar, and the documents must
   match. Any disagreement is an error, never silently wrong output.
3. **Generate.** The YAML tree is translated to Jsonnet: mappings become objects, sequences
   become arrays, scalars become literals, placeholders become the original expressions.
   Top-level `'&'` keys become fields of the file, anchors and `'&x':` keys become locals, key modifiers become Jsonnet's `+:`, `::` and
   `:::`, and `${for}` and `${if}` list items become comprehensions. YAML blocks inside expressions
   are compiled in place.

The generated code keeps YAML nodes and expressions on their original source lines, so
Jsonnet errors and `kubecfg traceback` / `eval --trace` point at the right line of the
`.yakl` file. The exceptions are listed in [Corner cases](#corner-cases).

### Seeing the generated Jsonnet

`kubecfg --alpha eval -o jsonnet app.yakl` prints the Jsonnet a file compiles to, formatted
to be read, with a field per line and YAML comments as `//` comments. It doesn't evaluate the
file, so it works when evaluating fails, and needs neither a cluster nor the file's imports. It
is the quickest way to see what names, locals and key modifiers mean. For this document:

```yaml
'&k': ${import 'k.libsonnet'}
---
'&:': config
replicas: 2 # overlays change it
---
'&': deployment
kind: Deployment
metadata:
  labels: &labels {app: web}
spec:
  replicas: ${config.replicas}
  selector: {matchLabels: '${labels}'}
```

it prints:

```jsonnet
{
  local k = import 'k.libsonnet',
  local config = self.config,
  config:: {
    replicas: 2,  // overlays change it
  },
  local deployment = self.deployment,
  deployment: local yakl__root = {
    kind: 'Deployment',
    metadata: {
      labels: { app: 'web' },
    },
    spec: {
      replicas: config.replicas,
      selector: { matchLabels: labels },
    },
  }, labels = yakl__root.metadata.labels; yakl__root,
}
```

It's the same code kubecfg evaluates, laid out differently: kubecfg keeps each node on its
source line instead. Internal names, like `yakl__root` for a document whose nodes have
anchors and `yakl__unnamed3` for a document without a name, and the check that `<<+` extends
an object, show up as they are. A comment stays next to the field or item it's written on; one
above a document's `'&'` key goes above the document's field. Copies made by aliases have no
comments. Like other commands, `eval` reads URLs and `/dev/stdin` too. If the code doesn't
parse, because of an expression's syntax, it prints the code unformatted, and the error with its
line in the yakl file.

## Expressions

`${` at the start of a YAML node begins an expression, which runs to the matching `}`. The
body is any Jsonnet expression. Braces are matched with awareness of Jsonnet lexical
syntax, so braces inside these do not count:

- string literals: `"..."`, `'...'` (with backslash escapes)
- verbatim strings: `@"..."`, `@'...'` (quotes escaped by doubling)
- text blocks: `|||` ... `|||`
- comments: `// ...`, `# ...`, `/* ... */`

An expression is always a whole node and produces a value. Its YAML spelling doesn't matter,
because tools rewrite spelling:

```yaml
replicas: ${params.replicas}            # plain
labels: '${ {app: "web"} }'             # quoted, because of the `: `
ports: |-                               # block, for several lines
  ${[{containerPort: p} for p in [80, 443]]}
literal: $${HOME}                       # the string ${HOME}
script: echo ${HOME}                    # literal text: ${ isn't at the start
image: ${'nginx:' + version}            # build strings in Jsonnet
```

- **A string whose whole value is `${...}` is an expression,** in any YAML style: plain,
  single-quoted, double-quoted, literal or folded block. White space at the start or end of a
  string doesn't count, here and below, so a tool that rewrites a block as `"${...}\n"` keeps
  it an expression.
- **`$${` at the start of a string is a literal `${`.** `$${HOME}` is the string `${HOME}`.
- **`${` after other text is literal text,** in any style: `echo ${HOME}`,
  `nginx:${version}`. A tool may unquote a literal string, so it has to stay literal either way.
- **A string that starts with `${` must be exactly one expression,** in any style.
  `${version}-alpine` and `'${a} ${b}'` are errors. Start such text with `$${`, or build the
  string in Jsonnet.

### Quoted expressions

A quoted scalar whose whole value is `${...}` is that expression: `'${1 + 1}'` and
`"${1 + 1}"` mean exactly `${1 + 1}`, the number 2. This is not string interpolation; it is
YAML quoting around Jsonnet that is not a valid plain scalar:

```yaml
a: '${ {x: "y # not a YAML comment"}.x }'   # `: ` and ` #` inside the expression
b: ['${1}', '${"a, b"}']                    # inside a flow sequence
```

YAML unquotes the scalar first, and its value is the Jsonnet code. So inside `'...'`, write a
`'` as `''` (or use `"` for Jsonnet strings), and inside `"..."` YAML's backslash escapes
apply. A quoted expression may span lines, but YAML folds its line breaks into spaces, and
yakl compiles the folded text: a `//` or `#` comment would swallow the lines after it, and
`|||` text blocks break. Code with comments, text blocks or [YAML blocks](#yaml-inside-expressions)
needs a block expression.

### Block expressions

A block scalar (`|` or `>`) whose content is a single `${...}` is that expression too. Its
lines are the Jsonnet code as written, so it can hold comments, text blocks and YAML blocks,
and Jsonnet errors point at the right line. This is how to write a multi-line expression:

```yaml
ports: |-
  ${[
    {containerPort: p}  // one entry per port
    for p in [80, 443]
  ]}
```

White space around the expression doesn't matter, and neither does the block's style:
chomping (`|-`, `|+`) has nothing to chomp, and `>` doesn't fold the code. The code must
stay indented as the block's content, since a line indented less ends the block.

### Where an expression can be

An expression can be a mapping value, a mapping key, a sequence item, an item of a flow
collection, or the entire document:

```yaml
replicas: ${params.replicas}
${name}: value
items:
- ${item}
flow: ['${a}', '${b}']
```

After the closing `}` of an unquoted expression, only these may follow on the line: a `:`
(the expression is a key), flow punctuation (`,`, `]`, `}`), a comment, or nothing. An
expression can carry an anchor (`&a ${x}`) but no tag.

### Strings and comments are never interpolated

Except for a scalar that is a whole expression, `${` in a string or a comment is plain text.
So shell variables need no escaping:

```yaml
command: [sh, -c, 'echo ${HOME}']   # the string "echo ${HOME}"
script: |
  #!/bin/sh
  echo "${GREETING:-hello}"
# ${ in a comment is just a comment
```

To put computed values into a multi-line string, build it in Jsonnet, e.g. with a text block:

```yaml
config.ini: |-
  ${|||
    [server]
    port = %d
  ||| % params.port}
```

A scalar that starts with `${` and ends with `}` but isn't a well-formed expression, like
`'${"x}'`, reports the Jsonnet error (here an unterminated string), as it was most likely
meant as an expression. A literal string that starts with `$${` can't be written directly,
because yakl drops one `$`: use an expression, `${'$${X}'}`.

### YAML inside expressions

Inside an expression, `\{` followed by a line break opens a YAML block. So `${` always means
"Jsonnet here" and `\{` always means "YAML here", and the two languages nest in each other to
any depth. As with Jsonnet `|||` text blocks, the first content line sets the block's
indentation. The block ends at the first line indented less than that, which must start with
`}`. How deep the block sits in the surrounding code doesn't matter.

```yaml
env: |-
  ${[
    \{
      name: ${k}
      value: ${params.env[k]}
    }
    for k in std.objectFields(params.env)
  ]}
```

The block compiles to a Jsonnet value in place, so expressions in it see the surrounding
Jsonnet scope, like the comprehension variable `k` above. For a loop like this one, a
[`${for}` item](#loops-and-conditions) is shorter and keeps the body YAML to every tool; blocks
cover what it doesn't. A block is one value, not a file
of documents: its anchors and its `'&x':` keys declare locals as in a document, and a `'&'`
key is an error.

Rules:

- `\{` must end its line; only a YAML comment may follow it. A backslash means nothing in
  Jsonnet outside string literals, so `\{` can't clash with Jsonnet code. Inside strings it is
  just text.
- The block's content must be indented more than its closing `}`. After the `}`, the
  Jsonnet code continues on the same line: `} for k in ...`, `} + {...}`, `},`.
- Blank and comment lines don't count for indentation.
- A block holds a single YAML document. An empty block is `null`.

## Loops and conditions

A list item can be a Jsonnet comprehension clause, `${for ...}` or `${if ...}`, with a body
indented under it. The loop is YAML; only the clause is Jsonnet.

```yaml
'&params': ${import 'params.libsonnet'}
'&port': 8080
---
'&': deployment
'&envs': ${std.objectKeysValues(params.env)}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  labels:
    app: web
    <<+: ${params.extraLabels}
spec:
  template:
    spec:
      containers:
        - name: web
          env:
            - ${for e in envs}:
                name: ${e.key}
                value: ${e.value}
            - ${if params.debug}:
                name: DEBUG
                value: "1"
            - name: PORT
              value: ${std.toString(port)}
          args:
            - --verbose
            - ${for p in params.ports if p != 9090}: ${'--port=' + p}
```

| You write | Meaning |
| --- | --- |
| `- ${for e in envs}:` with a body | the body once per `e`, as items of the list |
| `- ${if params.debug}:` with a body | the body as an item, if the condition holds; otherwise nothing |

With `params.env` = `{LOG: 'info', MODE: 'prod'}`, `debug: true`, `ports: [8080, 9090, 9100]`
and two extra labels, `env` is `LOG`, `MODE`, `DEBUG`, `PORT`, in that order; `args` is
`--verbose`, `--port=8080`, `--port=9100`; and `labels` is `app` plus the two extra labels.

It means:

```jsonnet
deployment: {
  local envs = std.objectKeysValues(params.env),
  apiVersion: 'apps/v1',
  kind: 'Deployment',
  metadata: {
    name: 'web',
    labels: params.extraLabels + { app: 'web' },
  },
  spec: { template: { spec: { containers: [{
    name: 'web',
    env: [{ name: e.key, value: e.value } for e in envs]
      + (if params.debug then [{ name: 'DEBUG', value: '1' }] else [])
      + [{ name: 'PORT', value: std.toString(port) }],
    args: ['--verbose'] + ['--port=' + p for p in params.ports if p != 9090],
  }] } } },
},
```

A loop becomes an array comprehension, a condition becomes `if ... then [body] else []`, and
the pieces of the list are joined with `+`.

- **The clause is Jsonnet's own comprehension clause:** one `for`, then any number of further
  `for`s and `if` filters, as in `${for p in params.ports if p != 9090}`. On its own,
  `${if cond}` is a condition.
- **The body goes under the clause.** It is any YAML node, with expressions anywhere in it,
  and local keys, declared once per iteration. The loop variable is in scope in the body, and
  hides an outer name, as in Jsonnet. `self` in a body is the item.
- **The clause is the item's only key.** A body written at the clause's own indentation would
  put its keys next to the clause, so that's an error, and so is an empty body; both errors say
  to indent the body.
- **Plain items stay where they are,** so a loop, a condition and a fixed item sit side by side,
  in order, with no `+` between them.
- **A mapping key can't be a clause.** A mapping gets fields from somewhere else through
  [`<<+`](#-extend-an-object) only, as `labels` does above: `<<+: ${params.extraLabels}`, where
  the mapping's own `app: web` wins over an `app` in `params.extraLabels`. A condition is
  `<<+: ${if params.debug then {debugPort: 9229}}`, since `null` extends nothing, and several
  sources are a list: `<<+: [${a}, ${b}]`. A `${for}` or `${if}` key in a mapping is an error
  that says so.
- **No anchors inside a body,** since a body is repeated or left out, so there's no single node
  for an anchor to name. An anchor on a plain item next to loops needs a fixed index: the item
  comes before every loop of its list, or after every loop.
- **A long collection goes into a local first,** as `envs` does above, because a YAML key must
  fit on one line.
- **No modifiers on a clause key.** `${for ...}+:` is an error, since a string that starts with
  `${` is one whole expression. Literal keys inside the body keep theirs (`spec+:`).
- **One element per iteration.** For several, add a `for` to the clause
  (`${for a in as for x in [a, a + 1]}`), or write two loops. Loops nest: a body can hold more
  loops and conditions.
- **`${if debug then 'debugPort'}: 9229` is still a computed key.** `for ...`, and `if` without
  `then`, aren't Jsonnet expressions on their own, so a key is a clause only when it isn't one.
- **Overlays just work:** `env+:` with loop items appends them like any other items.
- **The escape stays.** A block expression with `\{` blocks still covers anything this form
  doesn't.

Changing every item of a list is a loop whose body extends the item. In an overlay:

```yaml
'&+': redis
statefulset+:
  spec+:
    volumeClaimTemplates:
      - ${for t in super.volumeClaimTemplates}:
          <<+: ${t}
          spec+:
            resources+:
              requests+: {storage: 20Gi}
```

## Extending and hiding fields

Jsonnet writes how a field combines with an inherited one right after its name: `spec+:`,
`_config::`. YAKL keys are written the same way.

| YAML key | Key text after parsing | Jsonnet |
| --- | --- | --- |
| `spec:` | `spec` | `spec:`, visible unless the base hides it |
| `spec+:` | `spec+` | `spec+:`, extend the inherited field |
| `_config::` | `_config:` | `_config::`, hidden |
| `_config+::` | `_config+:` | `_config+::` |
| `name:::` | `name::` | `name:::`, forced visible |

- **The meaning comes from the key's text.** kustomize writes `_config::` back as
  `'_config:':`. That's the same text, so it means the same.
- **`+` works on any value,** as Jsonnet's `+:` does. Objects are extended, lists appended
  (`args+: [--verbose]`), numbers added (`replicas+: 1`). With nothing inherited, the value is
  used as it is.
- **One name, at most two keys in a mapping:** one without `+` (`x:`, `x::` or `x:::`) and
  one with it. Together they make one field, `x + x+`, which lets merge keys and extension
  work together (example below). The `+` key must then be plain `x+:`, and the field takes the
  other key's visibility: `x::` with `x+:` gives a hidden `x + x+`. Anything else is an
  error: `x:` with `x::`, `x:` with `x+::`, or two `+` keys.
- **A literal key ending in `+` or `:`** is written as a key expression: `${'name:'}: value`.
  Kubernetes field names, label and annotation keys, and ConfigMap keys can't end in `+` or `:`.

### `<<+`: extend an object

`<<+: S` makes the mapping `S + {its own fields}`.

```yaml
deployment:
  <<+: ${k.deployment('web', image='nginx:1.27')}
  spec+:
    replicas: 3
```

That's `deployment: k.deployment('web', image='nginx:1.27') + {spec+: {replicas: 3}}`.

- **`S` is an expression or a mapping.** An alias to a mapping is fine; tools expand it into
  the same mapping.
- **An expression must give an object, or `null`, which extends nothing.** Anything else is an
  error, including a list, so `+` can't silently join strings or arrays.
- **Several sources are a YAML sequence, in `<<` order.** Each item is an expression or a
  mapping, and an earlier source wins over a later one, as in a YAML merge.
  `<<+: [A, B]` means `B + A + {own fields}`.
- **`S` is evaluated outside the mapping,** as `S` in `S + {...}` is. It sees the document's
  anchors and the file's locals, but not the local keys (`'&x':`) of the mapping it extends.
  Inside the mapping, `self` and `super` work as in Jsonnet, so fields of `S` that use `self`
  see the mapping's own fields.
- **With `<<` in the same mapping,** `<<` first copies fields in, and `<<+` is the parent of
  the result.
- **To YAML tools, `<<+` is an ordinary key,** not a merge key, so kubectl and kustomize keep
  it. (`<<: ${...}` would not work: kubectl and kustomize reject a merge key whose value isn't
  a mapping.)

### Merge keys keep their YAML meaning

- **`<<` copies fields,** as in YAML. Sources are aliases to mappings, inline mappings, or a
  list of those. Own keys win, and an earlier source wins over a later one.
- **`<<` must be the very first key, and appear once,** before any local key too. With an own
  key before `<<`, kubectl keeps
  the merged value and kustomize the own one. Two `<<` keys are accepted by kubectl and
  rejected by kustomize.
- **Anchors can't be used across documents,** as in YAML.

Two containers sharing defaults:

```yaml
containers:
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
```

The merge copies `resources` into the worker, and `resources+` extends that copy. The worker
gets `limits: {memory: 512Mi, cpu: "2"}`, and so does the file kubectl or kustomize writes back.
The anchor `&app` is also a local, so `${app.image}` works anywhere in the document.

## The file: a list until something has a name

A YAML file can hold several documents separated by `---`, and Kubernetes users put a
Deployment, a Service and a ConfigMap in one file all the time. A Jsonnet file is usually one
object with a field per resource, so resources can use each other (`$.deployment.spec`) and
overlays can change them (`file + {deployment+: {...}}`). YAKL lets a stream become that
object one document at a time.

1. **No names: a document, or a list.**
   [Documents of local keys](#file-locals-a-document-of-local-keys) are set aside first.
   What's left follows `std.parseYaml`'s rules: one document gives that document; a `---` or
   several documents give a list; an empty file gives `null`. A `---` that only separates
   documents of local keys from the rest doesn't count, so a file of imports plus one document
   still gives that document, and adding imports never changes the shape. A file with only
   documents of local keys gives an empty list.
2. **At least one name: an object.** Each named document is a field. Documents without a name
   go, in file order, into a field called `unnamed`. It compiles as `unnamed+:`, so an
   overlay's unnamed documents are appended rather than swapped in. When there are none, the
   field is left out. `unnamed` is reserved as a name.
3. **`$` follows Jsonnet: it's the outermost object.** In an object file, `$` is the file, in
   every document, named or not. In a file without names, each document is its own `$`.
   Document names are usually shorter: `config` instead of `$.config`.
4. **Order doesn't matter.** Names and file locals belong to one object, so a document can use
   one defined further down.

yakl takes the `'&'` key out of the document. Other YAML tools keep it, so to them a named
document is an object with one more key. kubecfg finds the same Kubernetes objects in an object
as in a list.

### Step by step from plain YAML

Plain Kubernetes YAML compiles to a list:

```yaml
apiVersion: v1
kind: ConfigMap
metadata: {name: a}
---
apiVersion: v1
kind: ConfigMap
metadata: {name: b}
```

An import and expressions keep it a list:

```yaml
'&names': ${import 'names.libsonnet'}
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: ${names.prefix + 'a'}
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: ${names.prefix + 'b'}
```

```jsonnet
local names = import 'names.libsonnet';
[{..., metadata: {name: names.prefix + 'a'}},
 {..., metadata: {name: names.prefix + 'b'}}]
```

Naming one document makes the file an object, and the other document goes to `unnamed`:

```yaml
'&names': ${import 'names.libsonnet'}
---
'&': a
apiVersion: v1
kind: ConfigMap
metadata:
  name: ${names.prefix + 'a'}
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: ${a.metadata.name + '-copy'}
```

```jsonnet
{
  local names = import 'names.libsonnet',
  local a = self.a,
  a: {..., metadata: {name: names.prefix + 'a'}},
  unnamed+: [{..., metadata: {name: a.metadata.name + '-copy'}}],
}
```

Naming every document, moving settings into a hidden field and adding an overlay is the
[worked example](#worked-example).

Naming the first document is the only step that changes the file's shape, and it fails loudly
where it matters: `kubecfg` finds the same objects; importers of the file get an object instead
of a list, so code that indexed it (`[0]`) fails; and `$` now means the file, so an expression
that used `$` for its own document fails with "field does not exist".

A document that should have had a name but lacks its `'&'` key lands in `unnamed` and is still
deployed, which is what a YAML user expects. Anything that uses the missing name fails:
`${service}` gives "Unknown variable", `$.service` gives "field does not exist", and an overlay
aimed at it leaves a fragment without `kind`, which kubecfg rejects ("Looking for kubernetes
object at … but instead found …").

## Names

A key that is exactly `'&'`, among the top-level keys of a document, names the document. Its
value is the name, and the document becomes the field with that name in the file.

```yaml
'&:': config
name: web
replicas: 2
---
'&': deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${config.name}
spec:
  replicas: ${config.replicas}
```

```jsonnet
{
  local config = self.config,
  config:: { name: 'web', replicas: 2 },
  local deployment = self.deployment,
  deployment: { /* ... */ spec: { replicas: config.replicas } },
}
```

- **The name is also a local, and the local is the field.** They're one thing with one name, so
  an overlay that changes `config` changes what `${config.replicas}` reads. `$.config` gives the
  same value. That's the difference from an anchor, whose local is the node as written.
- **Only a Jsonnet variable name gives a local.** `'&': web.conf` gives the field
  `'web.conf':`, and expressions read it as `$['web.conf']`. Neither does `std`, which would
  hide Jsonnet's standard library.
- **Scope: the whole file,** in any order.
- **The key says how the field combines,** with the `+` and colons a mapping key has after its
  name. `'&:'` is to `'&'` what `config::` is to `config:`.

| Key | Field |
| --- | --- |
| `'&': web` | `web:`, and a local `web` |
| `'&:': config` | `config::`, hidden |
| `'&::': config` | `config:::`, forced visible |
| `'&+': service` | `service+:`, extends `service` |
| `'&+:': config` | `config+::` |
| `'&': my-svc` | `'my-svc':`, and no local: `my-svc` isn't a variable name |

- **Hiding a field doesn't rename it.** Add or remove the colon, and `config` and `$.config` keep
  working.
- **Overlays keep the base's visibility.** In Jsonnet, `:` and `+:` keep the visibility the
  base gave a field: `{config:: {r: 1}} + {config+: {r: 2}}` outputs `{}`. An overlay writes
  `'&+': config`, without the colon, and `'&+::'` makes a hidden base field visible.
- **The value is text, never an expression,** so the file's shape is known without evaluating
  it. It can't be empty, and it can't end in `+` or `:`, which belong on the key: `'&': web+`
  is an error that suggests `'&+': web`. A number must be quoted: `'&': '123'`.
- **The key's grammar:** `&`, an optional `+`, then up to two colons. Other keys starting with
  `&` are [local keys](#locals-that-arent-output).
- **Only a mapping can have a name,** since the name is one of its keys. An object that an
  expression gives goes in `<<+`: `'&': redis` with `<<+: ${...}`. A string or a list goes in a
  field of a hidden document.
- **Anywhere among the top-level keys of the document,** with the first line as the convention.
  A name key anywhere else is an error; literal data uses `${'&'}`.
- **It survives round trips,** because it's a key. kustomize keeps it and moves it to the first
  line, since `&` sorts before letters. prettier writes it as `"&"`, which means the same.

## Anchors are locals

An anchor names a node, as in YAML, and that name is also a Jsonnet local: `&name` declares a
local wherever it appears.

```yaml
'&': deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: frontend
  labels: &labels {app: guestbook, tier: frontend}
spec:
  selector:
    matchLabels: ${labels}
  template:
    metadata:
      labels: ${labels}
```

```jsonnet
deployment:
  local yakl__root = {
    apiVersion: 'apps/v1',
    kind: 'Deployment',
    metadata: { name: 'frontend', labels: { app: 'guestbook', tier: 'frontend' } },
    spec: { selector: { matchLabels: labels }, template: { metadata: { labels: labels } } },
  }, labels = yakl__root.metadata.labels;
  yakl__root,
```

The node stays where it's written, and the local reads it there.

- **Scope: the whole document,** above or below the anchor. Another document can't use it:
  Jsonnet reports "Unknown variable". Something several documents share belongs in a file local
  or a field.
- **Value: the node as the document writes it,** after the document's own `<<+` and `x+` keys
  are applied, but before any overlay. Overlays change fields, never locals: an overlay that
  adds a label to `deployment.metadata.labels` changes only that field. The selector and the
  template keep the local's value, which is what a Deployment's immutable selector needs.
- **Under a `+` key, that's the combined value.** `labels+: &labels {...}` holds the inherited
  labels too. An item of a list under `args+:` is counted from the end of the combined list,
  so it's still the item written there.
- **Aliases still copy.** `*labels` gives the same value as `${labels}`, unless the node uses
  `self` or `$`, which a copy reads where the alias is.
- **Names:** a Jsonnet variable name, not a keyword or `std`. Other anchors, like
  `&default-labels`, `&if` or `&std`, stay plain YAML anchors. The same name twice in one document is an error: YAML lets a
  later anchor replace an earlier one, but a local can't have two values. An anchor may hide a
  file local or a document name with the same name, as Jsonnet locals do.
- **Not in a key of a mapping that holds the node.** Jsonnet needs a mapping's keys to build it,
  and the local reads the node from the mapping, so `${name}: 1` next to `name: &name web` is an
  error. Declare the value with a local key, `'&name': web`, and use `${name}` in both places.
- **Only on nodes that have a place in the output.** An anchor on a key, on the value of `<<`
  or `<<+`, inside a merge source, or under a key expression is an error. A common slip:
  `&web apiVersion: v1` on one line anchors the key `apiVersion`; an anchor for the whole
  document goes on its own line.
- **Expressions in the node are compiled in place,** so `self`, `$` and the local keys around
  them mean what they mean anywhere.
- **On a document's root,** the local is the whole document as written, for that document only.
  Other documents reach it by its name.
- **Inside a `\{` block,** anchors declare locals of the block's value the same way.

## Locals that aren't output

An anchor keeps its node in the output. For a value that only expressions use, write the anchor
as a quoted key: `'&name': value`. It reads as "an anchor named `name`, holding this value", and
it declares a local of the mapping it's in.

```yaml
'&': deployment
'&labels': {app: guestbook, tier: frontend}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: frontend
  labels: ${labels}
spec:
  selector:
    matchLabels: ${labels}
  template:
    metadata:
      '&port': 8080
      labels: ${labels}
      annotations:
        port: ${std.toString(port)}
```

```jsonnet
deployment: {
  local labels = { app: 'guestbook', tier: 'frontend' },
  apiVersion: 'apps/v1',
  kind: 'Deployment',
  metadata: { name: 'frontend', labels: labels },
  spec: {
    selector: { matchLabels: labels },
    template: { metadata: { local port = 8080, labels: labels, annotations: { port: std.toString(port) } } },
  },
},
```

- **Scope: the mapping that holds the key, and everything inside it,** in any order, as a
  Jsonnet object local. `port` above exists only in the template's `metadata`. A local used in
  both `metadata` and `spec` goes at the top of the document.
- **The value is YAML data or an expression.** It stays an object local where it's written, so
  it can use `self`, `$` and other locals.
- **Names:** a Jsonnet variable name, not a keyword or `std`, at most once per mapping. A local may hide
  an outer one with the same name.
- **The quotes are required.** Unquoted, `&labels: {...}` is a YAML anchor on an empty key,
  which kubectl rejects. yakl sees that shape and says to quote it.
- **It needs a value.** `'&x':` with nothing after it is an error, since it's most likely meant
  as a name. Write `null` for a local holding null.
- **No anchors inside the value,** since the value isn't in the output.
- **It's a key like any other to YAML tools,** so it survives round trips: kustomize keeps it
  and moves it to the top of its mapping. A merge (`<<`) copies it too, and an own local key
  wins over a copied one.

### File locals: a document of local keys

A document made only of local keys, with no `'&'` key, holds locals of the whole file. That's where
imports go:

```yaml
'&k': ${import 'k.libsonnet'}
'&names': ${import 'names.libsonnet'}
'&image': nginx:1.27
'&webName': ${function(suffix) 'web-' + suffix}
---
'&': deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${webName('api')}
```

```jsonnet
{
  local k = import 'k.libsonnet',
  local names = import 'names.libsonnet',
  local image = 'nginx:1.27',
  local webName = function(suffix) 'web-' + suffix,
  local deployment = self.deployment,
  deployment: { /* ... */ },
}
```

In a file without names, the same locals become `local k = ..., names = ...;` in front of the
document or list.

- **What counts:** a document whose root is a non-empty mapping in which every key is a local
  key. A `'&'` key makes it a named document instead. An anchor on it is an error, since the
  document isn't output.
- **Not output, and not a list element.** Adding one never changes the file's shape.
- **Not tied to position.** It can go anywhere, and there can be several. The convention is to
  put it first, like Jsonnet imports, but nothing depends on it.
- **Seen by every document, in any order.** File locals can use each other in any order too.
- **One name space per file.** File locals and the locals of document names must all differ.
- **Private.** Importers and overlays can't see file locals. A hidden field (`'&:': config`) is the
  overlay-friendly alternative: Jsonnet's `local x = ...` versus `x:: ...`.
- **`$` inside a file local** is the file in an object file. A file without names has no file
  object: there Jsonnet would read `$` as the local's own outermost object, so yakl reports it
  instead. `self` outside an object gives Jsonnet's own "Can't use self outside of an object".

### Every way to declare a local

| You want | Write | Scope | In the output |
| --- | --- | --- | --- |
| a name for a node that stays in the output | `&labels` on the node | its document | yes, where it's written |
| a value used only by expressions | `'&port': 8080` in a mapping | that mapping | no |
| imports and helpers for the whole file | a document of `'&k':` keys | the file | no |
| a document with a name | `'&': web` in the document | the file | yes, as the field `web` |
| settings an overlay can change | `'&:': config` in a document | the file | no, it's a hidden field |

## Overlays

An overlay is a yakl file written with the same rules, and kubecfg combines it with the
base as `base + overlay` (`--overlay-code-file prod.yakl`). Because the overlay spells its
own `+:` and `::`, it says exactly what it does to each field.

A single-document overlay uses key modifiers:

```yaml
spec+:
  replicas: 5
  template+:
    metadata+:
      labels+:
        canary: "true"
```

An overlay for a stream is a stream too. Each document says what it does to one named
document:

| Overlay document | Jsonnet | Effect |
| --- | --- | --- |
| `'&+': service` with some fields | `service+: {...}` | extends `service` |
| `'&+': config` with some fields | `config+: {...}` | extends hidden `config`, which stays hidden |
| `'&+': web.conf` with some fields | `'web.conf'+: {...}` | extends a field whose name has a dot |
| `'&': pdb` with a whole object | `pdb: {...}` | adds a document, or replaces one with that name |
| a document without a name, in an overlay that has names | `unnamed+: [...]` | appended after the base's unnamed documents |

- **The overlay is a separate file.** The base's locals aren't in scope there, so the overlay
  reaches the base through `$`: `$.config.name`. `$` in the overlay is the combined file, so it
  sees the base's fields and the overlay's changes.
- **The base sees the overlay's changes too,** through document names and `$`: a
  `${config.replicas}` in the base reads the overlaid value.
- **`unnamed+:` keeps both lists.** With a plain `unnamed:`, an overlay's ServiceAccount would
  replace the base's NetworkPolicy, which would silently disappear.
- **An overlay without names follows the same file rules.** One document is merged into the
  base's root, which is the single-document overlay above. Several documents make a list, and
  an object plus a list is an error. To add documents to a named base, the overlay needs at
  least one name.
- **A whole object as an unnamed single-document overlay is a mistake:** merged into a named
  base's root, its `apiVersion` and `kind` would turn the whole file into that one object.
  kubecfg reports an overlay with `apiVersion` and `kind` but no name when the base has no
  `kind` of its own.

## Worked example

A base file and a prod overlay, both yakl streams. They are also checked by the tests, in
`pkg/yakl/testdata/overlay`.

`lib.libsonnet`:

```jsonnet
{
  conf(name):: name + '-conf',
  probes(port):: { readinessProbe: { httpGet: { path: '/healthz', port: port } } },
}
```

`web.yakl`:

```yaml
'&lib': ${import 'lib.libsonnet'}
'&port': 8080
---
'&:': config
name: web
image: nginx:1.27
replicas: 2
---
'&': deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${config.name}
  labels: &labels
    app: ${config.name}
spec:
  replicas: ${config.replicas}
  selector:
    matchLabels: ${labels}
  template:
    metadata:
      labels: ${labels}
    spec:
      containers:
        - <<+: ${lib.probes(port)}
          name: web
          image: ${config.image}
          ports:
            - containerPort: ${port}
          volumeMounts:
            - name: conf
              mountPath: /etc/nginx/conf.d
      volumes:
        - name: conf
          configMap:
            name: ${$['web.conf'].metadata.name}
---
'&': service
apiVersion: v1
kind: Service
metadata:
  name: ${config.name}
spec:
  '&target': ${deployment.spec.template.spec.containers[0]}
  selector: ${deployment.spec.selector.matchLabels}
  ports:
    - name: ${target.name}
      port: 80
      targetPort: ${target.ports[0].containerPort}
---
'&': web.conf
apiVersion: v1
kind: ConfigMap
metadata:
  name: ${lib.conf(config.name)}
data:
  default.conf: |
    server { listen 8080; }
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: web-ingress
spec:
  podSelector:
    matchLabels: ${deployment.spec.selector.matchLabels}
  ingress:
    - {}
```

It means:

```jsonnet
{
  // the document of local keys
  local lib = import 'lib.libsonnet',
  local port = 8080,
  // document names: each local is the field itself
  local config = self.config,
  local deployment = self.deployment,
  local service = self.service,

  config:: { name: 'web', image: 'nginx:1.27', replicas: 2 },

  deployment:
    local yakl__root = {
      apiVersion: 'apps/v1',
      kind: 'Deployment',
      metadata: { name: config.name, labels: { app: config.name } },
      spec: {
        replicas: config.replicas,
        selector: { matchLabels: labels },
        template: {
          metadata: { labels: labels },
          spec: {
            containers: [lib.probes(port) + {
              name: 'web',
              image: config.image,
              ports: [{ containerPort: port }],
              volumeMounts: [{ name: 'conf', mountPath: '/etc/nginx/conf.d' }],
            }],
            volumes: [{ name: 'conf', configMap: { name: $['web.conf'].metadata.name } }],
          },
        },
      },
    }, labels = yakl__root.metadata.labels;
    yakl__root,

  service: {
    apiVersion: 'v1',
    kind: 'Service',
    metadata: { name: config.name },
    spec: {
      local target = deployment.spec.template.spec.containers[0],
      selector: deployment.spec.selector.matchLabels,
      ports: [{ name: target.name, port: 80, targetPort: target.ports[0].containerPort }],
    },
  },

  'web.conf': {
    apiVersion: 'v1',
    kind: 'ConfigMap',
    metadata: { name: lib.conf(config.name) },
    data: { 'default.conf': 'server { listen 8080; }\n' },
  },

  unnamed+: [{
    apiVersion: 'networking.k8s.io/v1',
    kind: 'NetworkPolicy',
    metadata: { name: 'web-ingress' },
    spec: { podSelector: { matchLabels: deployment.spec.selector.matchLabels }, ingress: [{}] },
  }],
}
```

`prod.yakl`:

```yaml
'&+': config
replicas: 5
image: nginx:1.29
---
'&+': service
spec+:
  type: LoadBalancer
---
'&+': web.conf
data:
  default.conf: |
    server { listen 8080; gzip on; }
---
'&': pdb
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: ${$.config.name}
spec:
  minAvailable: 2
  selector: ${$.deployment.spec.selector}
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ${$.config.name}
```

It means:

```jsonnet
{
  local pdb = self.pdb,
  config+: { replicas: 5, image: 'nginx:1.29' },
  service+: { spec+: { type: 'LoadBalancer' } },
  'web.conf'+: { data: { 'default.conf': 'server { listen 8080; gzip on; }\n' } },
  pdb: {
    apiVersion: 'policy/v1',
    kind: 'PodDisruptionBudget',
    metadata: { name: $.config.name },
    spec: { minAvailable: 2, selector: $.deployment.spec.selector },
  },
  unnamed+: [{ apiVersion: 'v1', kind: 'ServiceAccount', metadata: { name: $.config.name } }],
}
```

`kubecfg --alpha show web.yakl` and `kubecfg --alpha show web.yakl --overlay-code-file prod.yakl` give:

| Where | web.yakl alone | with prod.yakl |
| --- | --- | --- |
| `$.deployment` | Deployment web: 2 replicas, nginx:1.27, readiness probe and container port 8080, mounts `web-conf` | 5 replicas, nginx:1.29 |
| `$.pdb` | none | PodDisruptionBudget web, same selector as the Deployment |
| `$.service` | Service web: selects app=web, port `web` 80 to 8080 | `type: LoadBalancer` |
| `$.unnamed[0]` | NetworkPolicy web-ingress | unchanged |
| `$.unnamed[1]` | none | ServiceAccount web |
| `$['web.conf']` | ConfigMap web-conf | `default.conf` with `gzip on` |

- **The document of local keys** holds the import and the port. Neither is output.
- **`config` is a hidden document and a local.** The overlay only touches the `config` field,
  yet the Deployment gets 5 replicas and the new image through `${config.replicas}` and
  `${config.image}`, because a document name's local is the field. `'&+': config` keeps `config`
  hidden without repeating the colon.
- **`&labels` is a nested anchor:** the labels stay in `metadata`, and the same value fills the
  selector. An overlay that added a label to `metadata.labels` would leave the selector as it is.
- **`'&target'` is a local key** in the Service's `spec`. It names the container once and isn't
  output.
- **`'&': web.conf`** gives a field with a dot in its name. That isn't a variable name, so there's
  no local, and the Deployment reads it as `$['web.conf']`.
- **`<<+` starts the container from the library's probe** and adds its own fields.
- **The overlay is a separate file,** so it reaches the base through `$`, and its PDB takes the
  Deployment's selector.
- **`'&+': web.conf` with a plain `data:`** replaces all of `data`; `data+:` would merge it.

A larger example, a shop built on [k8s-libsonnet](https://github.com/kubecfg/k8s-libsonnet)
with dev and prod overlays that change every object, every pod, or one container, is in
[`examples/k8s-libsonnet`](https://github.com/kubecfg/kubecfg/tree/main/examples/k8s-libsonnet).

## Semantics

### Expressions

An expression evaluates to whatever Jsonnet returns: number, bool, null, string, array or
object. That value replaces the node.

### Keys

A key expression becomes a Jsonnet computed field name. It must evaluate to a string or
`null`, and `null` drops the field, which is how to include a field conditionally:

```yaml
${name}: value
${if debug then 'debugPort'}: 9229
```

`${if debug}:`, without `then`, and `${for ...}:` are [list items](#loops-and-conditions),
and errors as mapping keys.

Keys must be scalars. Complex keys (`? [a, b]`) are rejected. Duplicate keys are errors, also
when a computed key collides at runtime.

### Scope

Expressions are evaluated in the generated Jsonnet, so:

- `$` is the outermost object: the file in a file with names, and otherwise the root mapping
  of the current YAML document. `self` is the innermost mapping that contains the expression.
  Both are late-bound as usual in Jsonnet. Inside a YAML block nested in an expression,
  `self` is the block's innermost mapping, while `$` stays the same.
- File locals, the locals of document names, the anchors of the current document, the
  local keys of the mappings around the expression and the variables of the `${for}` items
  around it are in scope. An inner one hides an outer one with the same name, as in Jsonnet.
- `std` is available, and so are `std.extVar` (kubecfg `-V`, `--ext-code`, ...) and the
  kubecfg native functions.
- `import`, `importstr` and `importbin` resolve relative to the `.yakl` file, then the
  `-J` search path, just like in a `.jsonnet` file. URLs and `oci://` work too.

### Plain YAML is (almost) unchanged

A `.yakl` file without expressions means what `std.parseYaml` and kubectl make of it, so you
can rename most `.yaml` manifests to `.yakl` and get the same objects. The exceptions:

- **YAML 1.1 booleans are errors:** unquoted `y`, `n`, `yes`, `no`, `on`, `off`, in any case,
  as keys or values. kubectl reads them as booleans and kustomize as strings, so the file
  would mean two things. Write `true` or `false`, or quote them. A GitHub Actions workflow's
  `on:` key is one.
- **Keys ending in `+` or `:` are field modifiers.** Kubernetes keys never end that way.
- **The key `<<+` is reserved,** and so are keys starting with `&`. **`<<` must be the first
  key, and appear once.**
- **Strings starting with `$${` lose one `$`,** and **strings starting with `${` are
  expressions.**
- **Anchors become locals,** which is harmless, except that the same name twice in one
  document, and an anchor on a key, are now errors when the name is a Jsonnet variable name.

Other YAML 1.1 values keep their meaning, as kubectl and kustomize agree on them:

| YAML                         | Result                   |
|------------------------------|--------------------------|
| `0777`, `0x1F`, `1_000`      | `511`, `31`, `1000`      |
| `.inf`, `.nan`, `~:` (key)   | error (not representable)|

Quote a scalar to keep it a string (`'0777'`).

### Tags

`!!str` keeps its YAML meaning on ordinary scalars: `!!str 0777` is the string `"0777"`, as
kubectl reads it. It can't turn an expression into text: `!!str` on a string that starts with
`${` is an error that points to `$${`. `!!map` and `!!seq` are accepted and ignored. Other
tags (`!!int`, `!Ref`, the non-specific `!`, ...) are rejected.

### Encoding

Files must be UTF-8. A leading byte order mark is ignored, and `\r\n` and `\r` line breaks
are treated as `\n`. The Unicode line separators U+0085, U+2028 and U+2029, which YAML also
treats as line breaks, are rejected.

## Errors

Where a file could mean two things, yakl stops with an error and says why, with the source
line:

| Input | Why |
| --- | --- |
| a string that starts with `${` but isn't one whole expression, in any style | ambiguous; write `$${` for text, or build the string in Jsonnet |
| `!!str` on a string that starts with `${` | it can't turn an expression into text; write `$${` |
| unquoted `yes`, `no`, `on`, `off`, `y`, `n` | kubectl and kustomize read them differently |
| two keys for one name without `+` in a mapping (`x:` with `x::`), or two with `+` | two values or visibilities for one field |
| a `+` key with colons (`x+::`) next to a key without `+` | the field's visibility comes from the other key |
| `<<` that isn't the very first key, or a second `<<` | kubectl and kustomize give different results |
| `<<+` with an expression that isn't an object or `null`, including a list | `+` would silently join strings, arrays or numbers |
| two documents with the same name, unless one is plain and the other `+` (`'&': web` and `'&+': web`) | ambiguous |
| one local name declared twice in one scope: two anchors in a document, two local keys in a mapping, or a file local and a document name | a local can't have two values |
| an anchor on a key, on the value of `<<` or `<<+`, inside a merge source, or under a key expression | the node has no place of its own in the output |
| a key expression that uses the local of an anchor in the same mapping, or below it | Jsonnet needs the key to build the mapping that the local reads |
| an anchor inside the value of a local key, or on or inside a document of local keys | that value isn't in the output, so there's no node to name |
| a local key whose name isn't a Jsonnet variable name, is a keyword or `std`, or has dashes or `+`, such as `'&web.conf':` | the key declares a local, nothing else; a name is `'&': web.conf` |
| a local key without a value, such as `'&web':` | most likely meant as a name: `'&': web` |
| a name key (`'&'`, `'&+'`, `'&:'`, ...) below the top level of a document, or two in one document | it only names documents, once |
| a name that is an expression, empty, or ends in `+` or `:`, such as `'&': web+` | names are known before evaluation, and `+` and `:` go on the key: `'&+': web` |
| `$` in a file local of a file without names | there is no file object; Jsonnet would read it as the local's own object |
| `'&': unnamed` | `unnamed` is reserved for unnamed documents |
| `&labels: value`, a local key without quotes | YAML reads an empty key carrying the anchor `labels`; yakl says to quote it |
| `&web.conf:` without quotes | a YAML syntax error in kubectl, go-yaml v3, PyYAML and yamllint |
| `&a &b` on one node | YAML allows one anchor per node |
| a `${for}` or `${if}` list item with keys next to the clause, or without a body | the body goes under the clause; beside it, its keys would read as the item's own |
| a `${for}` or `${if}` key in a mapping | a mapping gets fields from elsewhere through `<<+`; clauses make list items |
| an anchor in the body of a `${for}` or `${if}`, or on a list item whose index depends on them, as after one in an `args:` that `args+:` extends | there's no single node to name |
| `${for ...}+:` | a string that starts with `${` is one whole expression |
| `${local x = ...}` | removed: see [Migrating](#migrating-from-jamlet) |

The `&a &b` error needs a warning of its own: on a document's first line, kubectl's decoder
silently reads it as a `null` document and skips the rest. Every other tool reports the error.

Allowed, with a note:

- **`'&': web` and `'&+': web` in one file** combine as `web: A + B`, by the same rule as two
  keys in a mapping; `'&:': web` with `'&+': web` gives a hidden `web`. It allows a vendored
  document followed by a local patch.
- **A document with only a name** gives `name: {}`, and kubecfg skips empty objects.
- **Nested anchors whose names aren't Jsonnet variable names,** like `&default-labels`, stay
  plain YAML anchors.
- **A file with only documents of local keys** has no output. A file imported for its helpers
  should export them as hidden fields (`'&:': helpers`), which importers can reach.
- **`<<+` at the top of a document using the document's own name or root anchor** is a loop,
  since the document is inside what `<<+` extends. Jsonnet stops with "max stack frames exceeded".

### Settings in a visible document

A visible document that isn't a Kubernetes object makes kubecfg fail. `'&': settings` with
`replicas: 3` puts a bare `3` where kubecfg looks for objects, and it stops with "Looking for
kubernetes object … but instead found float64". The same happens to a plain value without a
name, which lands in `unnamed`. Settings belong in a hidden document (`'&:': settings`), which
overlays can change, or in a document of local keys, which they can't.

## Corner cases

- **Unterminated or empty expressions** (`${` with no matching `}`, `${}`) are errors. They
  are never treated as literal text, because that would silently drop what the author
  meant.
- **Multi-line plain expressions** may use any indentation. YAML never sees them, and YAML
  error line numbers are mapped back to the source.
- **Jsonnet comments inside an expression.** A `#` or `//` comment runs to the end of the
  line, so `${ x # note }` swallows the `}`. Put the closing `}` on the next line or use
  `/* */`.
- **Source lines.** Jsonnet needs some code in a fixed place, so a few things are compiled out
  of source order, and Jsonnet errors inside them point at another line: file locals placed
  after a document in a file without names, `<<+` that isn't near the top of its mapping,
  and the clause of a `${for}`, which Jsonnet writes after its body.
  Anchored nodes stay in place; the code that reads them for their locals comes after the end
  of the document.
- **Reserved names.** yakl's own names start with `yakl__`, like `yakl__root`. Don't
  declare names with that prefix.
- **Private-use characters.** Placeholders are built from a Unicode private-use character
  that doesn't occur in the source, so any source text is safe.

## What tools do with it

Every construct here parses in all the tools listed at the end of this section. The losses
are in round trips, and in tools that need each document to be a Kubernetes object.

**Rewriting.** kubectl and kustomize parse a file and write new YAML, and so does prettier:

- **Expressions, `$${`, key modifiers, `<<+`, local keys and `'&'` names survive** as strings
  and keys.
  The spelling doesn't: both tools unquote `"nginx:${version}"`, and write a block expression
  as a plain or quoted string. That's why an expression means the same in every style.
- **Anchors don't survive kubectl or kustomize.** Both expand them, so locals written as
  anchors are gone after a round trip.
- **Keys are sorted.** kubectl and kustomize sort keys, and `&` sorts before letters, so a
  `'&'` name and local keys move to the top of their mapping.
- **Quote style changes, meaning doesn't.** kubectl and kustomize write `"&x"` as `'&x'`, and
  `_config::` as `'_config:':`. prettier writes `'&x'` as `"&x"`. On the worked example,
  prettier changes only spacing and quote style.
- **Merges are expanded.** A mapping with `<<: *app` and `resources+:` comes back with the
  copied `resources` and the `resources+` key, so it means the same.
- **`${for}` and `${if}` items survive,** with their bodies, and list items keep their order.

**kubectl** parses every document, but its commands need each one to be a Kubernetes object:

- A file with a document of local keys, hidden settings or overlay documents fails with
  "Object 'Kind' is missing". kubecfg, which applies yakl files, has no such limit.
- kubectl reads `metadata` into its typed form, so `metadata.labels: ${...}` fails with
  "cannot unmarshal string into Go struct field ObjectMeta.metadata.labels". Keep `metadata`
  labels and annotations as mappings; expressions elsewhere, like `spec.selector`, are fine.
  `<<+: ${...}` in `metadata.labels` is kept, since its value is a string, and so are loops
  in `env` or `args`.

**kustomize** needs more:

- **Every document must be a resource,** with `apiVersion`, `kind` and `metadata.name`. A file
  with a document of local keys or a `'&:': config` document fails with "missing Resource
  metadata".
- **Documents are reordered** by kind.
- **Transformers that write into a field holding an expression stop with an error.**
  `commonLabels` fails on `metadata.labels: ${labels}` and on a Service's `spec.selector:
  ${...}` with "expected MappingNode but got ScalarNode". Keep labels and selectors as mappings
  when label transformers must work. `labels` without selectors, `namespace` and patches that
  touch plain data work.
- **`namePrefix` and `nameSuffix` turn `${params.name}` into `dev-${params.name}`,** which is
  literal text, so the name silently stops being an expression. Use literal names, or apply
  the prefix in Jsonnet.
- **Resources are identified by kind and name text,** so two ConfigMaps both named
  `${params.name}` collide: "may not add resource with an already registered id".
- **A strategic-merge patch that merges `env` entries by `name` won't match a loop item,**
  because the item has no `name` until yakl expands it. Patch the expanded output instead,
  or keep such entries as plain items. Transformers such as `namespace` still apply.

**Editors and linters:**

- **A loop's body stays YAML,** so editors fold and highlight it, which they can't do inside a
  block expression.
- **The VS Code YAML server** shows an info-level "Unused anchor" hint on a `&labels` used
  only through `${labels}`, because it can't see uses inside expressions.
- **yamllint's default `indentation` rule** wants list items indented under their key, so
  kubectl-style unindented lists get flagged. That's style, unrelated to yakl.

**Mistakes the tools treat differently.** YAKL reports all of these, but other tools don't
always:

| Input | kubectl, kustomize | PyYAML, yamllint | eemeli/yaml, prettier | VS Code | tree-sitter |
| --- | --- | --- | --- | --- | --- |
| `&a &b`: two anchors on one node | the document is silently dropped | error | error | error | error only on a file's first line |
| `&labels: value` | kubectl: error; kustomize: not a resource | a `null` key | anchor `labels:` on `value`, with a warning | error | accepted |
| `&web.conf:` | error | error | anchor `web.conf:`, with a warning | error | accepted |
| `${local x = 1}` followed by more YAML | error | error | error | error | error |

YAML 1.2 lets an anchor hold `.` and `:`, so eemeli/yaml reads `&web.conf:` as an anchor
`web.conf:`; kubectl, kustomize and PyYAML allow only `[0-9A-Za-z_-]`.

**Versions tested:** kubectl 1.37.1 (through `kubectl annotate --local`, which reads and writes
files without a cluster), kustomize 5.8.3, go-yaml v3.0.5 and sigs.k8s.io/yaml v1.6.0, PyYAML
6.0.3, yamllint 1.37.0, eemeli/yaml 2.9.1, prettier 3.9.10, the VS Code YAML extension 1.24.0
(its bundled language server), tree-sitter-yaml 0.7.2, and the TextMate grammar of VS Code
1.136.1.

## Migrating from jamlet

YAKL used to be called jamlet. It is behind `--alpha`, so some meanings changed outright. Most
old files fail loudly where their meaning changed.

| Before | Now | Rewrite |
| --- | --- | --- |
| `.jamlet` files, `import 'jamlet:x.jamlet'` | `.yakl` files, `import 'yakl:x.yakl'` | rename the files and the imports |
| `${local x = ...}` lines at the top of a file | an error: not YAML | a document of local keys, `'&x': ...`. A later declaration that shadowed an earlier one becomes a duplicate-name error; rename one |
| `${local x = ...}` lines inside a mapping | an error | an `'&x': ...` key in the same mapping |
| `${local ...}` at the start of a `\{` block | an error | `'&x':` keys at the top of the block, or a Jsonnet `local` in front of it |
| `&spec` on a mapping: `local spec = self`, inside the mapping | a local for the whole document, holding the node as written; it may hide a file local with the same name | usually nothing; check for clashes with file locals |
| anchors on plain values and lists: plain YAML | locals too | nothing |
| the same anchor name twice in one document | an error | rename one |
| an anchor on a key, with a Jsonnet variable name | an error | a name that isn't a variable, like `&k-1`, or no anchor |
| `!!str '${HOME}'` | an error | `$${HOME}` |
| `'${v}-alpine'`: a quoted or block string that starts with `${` and has more text | an error | `$${v}-alpine` |
| unquoted `image: nginx:${version}` | literal text | `${'nginx:' + version}` if an expression was meant |
| quoted expressions only on one line | any number of lines, folded | nothing |
| white space around `${...}` in a quoted string, like `' ${HOME}'` | an expression, as in a block | `' $${HOME}'` for the text |
| `yes`, `no`, `on`, `off`, `y`, `n` unquoted | an error | `true`, `false`, or quotes |
| `<<: *base` | a YAML merge | nothing, or `<<+` for Jsonnet inheritance |
| keys ending in `+` or `:`, such as `a::` or `c++:` | field modifiers | `${'a:'}` for a literal key |
| a key starting with `&` | a local key; at the top of a document, `'&'`, `'&+'`, `'&:'` and the like name it | `${'&x'}` for a literal key |
| a `.jamlet` overlay, deep-merged by `toOverlay` | `base + overlay`, as written | add `+` to every key that should merge: `spec+:`, `_config+::` |

Three of these change meaning without an error:

- **Overlays.** A `.jamlet` overlay that wrote `spec: {replicas: 5}` merged into `spec`; as a
  `.yakl` overlay it replaces it. Existing overlays need their keys checked.
- **Keys ending in `:` or `+`.** `a:: 1` used to give a key `a:`, and now hides a field `a`.
  `c++:` used to give a key `c++`, and now extends `c+`. Kubernetes keys never end that way, so
  this only touches free-form data.
- **An anchor's local on a mapping** used to be the mapping itself, late-bound, and inside an
  alias copy it named the copy. Now it's the original node as written, everywhere in the
  document. The two differ when an overlay changes that mapping, or when an expression inside
  an alias copy used the name.

`$` keeps its meaning in every file without names: the document root in a single-document
file, and in each document of a list. It becomes the file only in files with names.

## Comparison with kro

| | kro | yakl |
|---|---|---|
| Expression language | CEL | Jsonnet |
| Where `${` is recognized | anywhere in string values, after YAML parsing | at the start of a string |
| Expression must also be valid YAML | yes (`${{a: 1}}` needs quoting) | yes, by the same quoting (not enforced yet) |
| `field: ${x}` | typed value | typed value |
| `field: "${x}"` | typed value | typed value |
| `name: app-${x}` | string template | literal text, write `${'app-' + x}` |
| Expressions in keys | no | yes, and `null` drops the field |
| Literal `${` | `${"${VAR}"}` | `$${VAR}`, or text before it |
| `${` in comments | (YAML parser drops comments first) | ignored |
| Variables | `schema`, resource ids | anchors (`&labels`), local keys (`'&port':`), named documents (`config`) |
| YAML inside an expression | no | `\{` and a line break open a YAML block |
| Unterminated `${` | error | error |

## kubecfg integration

`.yakl` files work wherever `.jsonnet` files do: `show`, `diff`, `update`, `delete`,
`validate`, `eval`, overlays (`--overlay-code-file x.yakl` is applied as
`base + overlay`, see [Overlays](#overlays); `.yaml` and `.json` overlays are still
deep-merged), OCI bundles, and `pack` (which ships the yakl source). `eval -o jsonnet` prints
the Jsonnet of a yakl file, see [Seeing the generated Jsonnet](#seeing-the-generated-jsonnet).

From Jsonnet, or from an expression in another yakl file, import a yakl file with the
`yakl:` scheme:

```jsonnet
(import 'yakl:deployment.yakl') + { spec+: { replicas: 5 } }
```

`importstr 'deployment.yakl'` and `importbin` return the file as it is. A plain
`import 'deployment.yakl'` parses the YAML as Jsonnet and fails with a Jsonnet syntax
error. The scheme is needed because go-jsonnet hands `import`, `importstr` and `importbin`
the same cached file contents, so compiled code needs a location of its own. The scheme
works with any location: `yakl:https://...`, `yakl:oci://...`.

## Open questions

- **A clearer kubecfg error when an overlay names a missing document,** such as "overlay field
  `service` matches no document". Today it shows as a fragment without `kind`.
- **Files that go through kustomize.** Should the docs recommend local keys instead of anchors
  there, since kustomize drops anchors? Documents of local keys and hidden settings documents
  can't go through kustomize at all.
- **String interpolation.** Allowing `${...}` inside strings (`dev-${params.name}`) would make
  `namePrefix` and `nameSuffix` work, but yakl deliberately has no interpolation.
- **Schema validation.** Kubernetes JSON schemas flag `spec+`, hidden keys, `${...}` values
  and `&` keys. A yakl-aware schema could allow them.
- **Looping over an object's keys and values,** like `${for k, v in params.env}`? Jsonnet has
  no such syntax, and `std.objectKeysValues` covers it, so the lean is to stay with plain
  Jsonnet.
- **Several items per iteration.** Should a list body be spliced, so one iteration can add a few
  items? The lean is no: one item per iteration, as in Jsonnet, with a second `for` in the
  clause for the rest.
- **Parameters.** A declared set of inputs for a template, like kro's `schema.spec`, instead
  of `std.extVar` or an imported parameters file.
- **Seeing the Jsonnet of imported files** while running `show` or `update`, for example with
  a flag that writes the compiled code of every yakl file to a directory. Today, `eval -o jsonnet`
  takes one file at a time.
