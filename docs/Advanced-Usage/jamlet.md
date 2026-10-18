# Jamlet: YAML with Jsonnet expressions

!!! warning "Prototype"
    Jamlet is an experimental format, enabled with `--alpha` (or `KUBECFG_ALPHA=true`). The
    syntax and semantics below may change.

Jsonnet is a great language, but many people would rather write YAML. Tools like
[kro](https://kro.run) keep YAML as the outer language and embed `${...}` expressions
in it. Jamlet does the same, but the language inside `${...}` is Jsonnet instead of CEL.
Files use the `.jamlet` extension.

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

The model is: **a jamlet file is YAML in which any node can be written as a Jsonnet
expression.** Jamlet is not a text template language. Expressions produce *values* that
are placed in the document tree, so there is no indentation or quoting to get right in the
output (no `nindent`, no `toYaml`), and YAML strings are never interpolated.

## YAML compatibility

**A jamlet file is valid YAML**, as read by `kubectl` (`sigs.k8s.io/yaml`), so editors,
linters and other YAML tools can work with it. To them, every expression is a single scalar
node.

Most expressions are valid plain scalars as they are (`replicas: ${params.replicas}`). Those
containing `: ` or ` #`, and those inside flow collections (`[...]`, `{...}`), are written as
[quoted expressions](#quoted-expressions). Those spanning lines are written as
[block expressions](#block-expressions).

This is not complete yet:

- The compiler doesn't enforce it: it still accepts unquoted expressions that a YAML
  parser would reject or split, like `a: ${ {b: 1}.b }`.
- [Local declarations](#local-declarations) have no valid YAML form yet.

The examples and the snippets in this document are checked with kubectl's YAML parser,
except those with local declarations.

## Implementation model

A jamlet file is compiled to Jsonnet and then evaluated like any other Jsonnet file:

1. **Lex.** A YAML-aware lexer finds every `${` that starts a YAML node and reads the
   Jsonnet expression up to the matching `}`. It skips comments, quoted scalars and block
   scalars, except those whose whole value is an expression. YAML blocks nested inside expressions are lexed the same way, recursively.
   Expressions are replaced by opaque placeholders. Local declarations at the start of a
   document are set aside, and those inside a mapping become placeholder keys.
2. **Parse.** The result, now plain YAML, is parsed as a YAML stream. The parser also checks
   the lexer: every placeholder must come back as a whole plain or block scalar, and the documents
   must match. Any disagreement is an error, never silently wrong output.
3. **Generate.** The YAML tree is translated to Jsonnet: mappings become objects, sequences
   become arrays, scalars become literals, placeholders become the original expressions,
   local declarations at the start of a document wrap the documents that follow them, and
   placeholder keys and anchors of mappings become object locals. YAML blocks inside
   expressions are compiled in place.

The generated code keeps every YAML node and expression on its original source line, so
Jsonnet errors and `kubecfg traceback` / `eval --trace` point at the right line of the
`.jamlet` file.

## Syntax

### Expressions

`${` at the start of a YAML node begins an expression, which runs to the matching `}`. The
body is any Jsonnet expression. Braces are matched with awareness of Jsonnet lexical
syntax, so braces inside these do not count:

- string literals: `"..."`, `'...'` (with backslash escapes)
- verbatim strings: `@"..."`, `@'...'` (quotes escaped by doubling)
- text blocks: `|||` ... `|||`
- comments: `// ...`, `# ...`, `/* ... */`

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
apply. A quoted expression must be on one line, and can't contain escaped line breaks
(`"${a\n}"`) or YAML blocks: YAML folds the lines of a quoted scalar, so Jsonnet errors
couldn't point at the right line. Write multi-line code as a
[block expression](#block-expressions).

### Block expressions

A block scalar (`|` or `>`) whose content is a single `${...}` is that expression too. Its
lines are the Jsonnet code as written, so unlike a quoted expression it can hold comments,
text blocks and [YAML blocks](#yaml-inside-expressions), and Jsonnet errors point at the
right line. This is how to write a multi-line expression:

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

### An expression is a whole node

An expression can be a mapping value, a mapping key, a sequence item, an item of a flow
collection, or the entire document:

```yaml
replicas: ${params.replicas}
${name}: value
items:
- ${item}
flow: ['${a}', '${b}']
```

It is always the *whole* node. Text around an expression is an error:

```yaml
image: nginx:${version}       # error: ${ inside a plain scalar
image: ${version}-alpine      # error: unexpected text after an expression
image: ${'nginx:' + version}  # build strings in Jsonnet
name: ${'%s-%d' % [app, n]}
```

After the closing `}`, only these may follow on the line: a `:` (the expression is a key),
flow punctuation (`,`, `]`, `}`), a comment, or nothing. An expression can carry an anchor
(`&a ${x}`) but no tag. In a quoted or block scalar, text around the expression makes the scalar a
plain string, see below.

### Strings and comments are never interpolated

Except for a quoted or block scalar that is a whole expression, `${` inside a quoted scalar (`'...'`,
`"..."`), inside a block scalar (`|`, `>`), or inside a comment is plain text. So shell
variables need no escaping:

```yaml
command: [sh, -c, 'echo ${HOME}']   # the string "echo ${HOME}"
script: |
  #!/bin/sh
  echo "${GREETING:-hello}"
# ${ in a comment is just a comment
```

The `!!str` tag makes a quoted or block scalar a string even when it looks like an
expression: `!!str '${HOME}'` is the string `${HOME}`. A quoted or block scalar that starts
with `${` and ends with `}` but isn't a well-formed expression, like `'${"x}'`, is an error rather than a
string, as it was most likely meant as an expression. To put computed values into a
multi-line string, build it in Jsonnet, e.g. with a text block:

```yaml
config.ini: |-
  ${|||
    [server]
    port = %d
  ||| % params.port}
```

### Local declarations

A `${local ...}` *without a body*, on its own line(s), declares Jsonnet locals. At the start
of a YAML document it declares locals for the rest of the file:

```yaml
${local params = import 'params.libsonnet'}
${local labels = {app: 'web'}, port = 8080}
${local name(suffix) = 'web-' + suffix;}
apiVersion: v1
kind: Service
metadata:
  name: ${name('svc')}
  labels: ${labels}
```

The trailing `;` is optional. Several locals can share a declaration (`local a = 1, b = 2`),
and declarations can span lines. A `${local x = 1; x}` *with* a body is an ordinary
expression.

Anywhere else in a block mapping, written where a key could be, it declares a local of that
mapping, visible to every expression in the mapping, however deep:

```yaml
spec:
  ${local labels = {app: 'web'}}
  selector:
    matchLabels: ${labels}
  template:
    metadata:
      labels: ${labels}
    spec:
      containers:
      - ${local port = 8080}
        name: web
        args: [--port, '${std.toString(port)}']
        ports: [{containerPort: '${port}'}]
```

This compiles to a Jsonnet object local (`{ local labels = {app: 'web'}, selector: ... }`).
Such a declaration declares a single variable (`local a = 1, b = 2` is an error, write two declarations), and
the line after it can't be indented more than it. A mapping can consist of local declarations
only, and then it is `{}`. A local declaration inside a flow collection, or with text before
or after it on its line (other than the `- ` of a sequence item), is an error.

### Anchors name mappings

An anchor on a mapping also names the mapping in the expressions inside it:

```yaml
spec: &spec
  replicas: 3
  template:
    spec:
      containers:
      - name: web
        env: [{name: REPLICAS, value: '${std.toString(spec.replicas)}'}]
```

Only anchor names that are Jsonnet identifiers (`[A-Za-z_][A-Za-z0-9_]*`, not a keyword like
`if` or `self`) are bound. Others, like `&default-labels`, stay plain YAML anchors, as do
anchors on scalars and sequences. An alias `*spec` still copies the node, and the copy binds
the name to itself.

### YAML inside expressions

Inside an expression, `\{` followed by a line break opens a YAML block. So `${` always means
"Jsonnet here" and `\{` always means "YAML here", and the two languages nest in each other to
any depth. As with Jsonnet `|||` text blocks, the first content line sets the block's
indentation. The block ends at the first line indented less
than that, which must start with `}`. How deep the block sits in the surrounding code
doesn't matter.

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
Jsonnet scope, like the comprehension variable `k` above. Everything that works in a file
works in a block: any YAML node as its root, expressions, further nested blocks, and local
declarations at its start. Blocks also work in local declarations:

```yaml
${local labels = \{
    app: web
    tier: frontend
  }}
```

Rules:

- `\{` must end its line; only a YAML comment may follow it. A backslash means nothing in
  Jsonnet outside string literals, so `\{` can't clash with Jsonnet code. Inside strings it is
  just text.
- The block's content must be indented more than its closing `}`. After the `}`, the
  Jsonnet code continues on the same line: `} for k in ...`, `} + {...}`, `},`.
- Blank and comment lines don't count for indentation.
- A block holds a single YAML document. An empty block is `null`.

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

Keys must be scalars. Complex keys (`? [a, b]`) are rejected.

### Scope

Expressions are evaluated in the generated Jsonnet, so:

- `$` is the root mapping of the current YAML document, and `self` is the innermost
  mapping that contains the expression. Both are late-bound as usual in Jsonnet. Inside a
  YAML block nested in an expression, `self` is the block's innermost mapping, while `$`
  stays the document root, as `$` is always the outermost object in Jsonnet.
- Locals declared earlier in the file are in scope (see below), and so are the locals and
  anchors of enclosing mappings. These follow Jsonnet scoping: an inner declaration shadows
  an outer one with the same name, and since they bind `self`, they are late-bound too.
- `std` is available, and so are `std.extVar` (kubecfg `-V`, `--ext-code`, ...) and the
  kubecfg native functions.
- `import`, `importstr` and `importbin` resolve relative to the `.jamlet` file, then the
  `-J` search path, just like in a `.jsonnet` file. URLs and `oci://` work too.

### Local declarations and streams

A local declaration means exactly what Jsonnet's `local x = e; rest` means, where `rest` is
**the rest of the file**. In a stream of documents, declarations scope over all the
documents that follow them:

```yaml
${local params = import 'params.libsonnet'}   # visible in both documents
---
kind: Deployment
spec: {replicas: '${params.replicas}'}
---
${local port = 8080}                           # visible here and in later documents
kind: Service
spec: {ports: [{port: '${port}'}]}
```

compiles to

```jsonnet
local params = import 'params.libsonnet'; [
  { kind: 'Deployment', spec: { replicas: params.replicas } },
] + (local port = 8080; [
  { kind: 'Service', spec: { ports: [{ port: port }] } },
])
```

A preamble of declarations at the top of the file therefore covers the whole stream. A
later declaration can shadow an earlier one. An explicit (`---`) document that contains
only declarations is an empty document, so it becomes `null`.

### Documents

- A file with a single YAML document evaluates to that document.
- A file with a `---` document marker, or with more than one document, evaluates to an
  array of documents. Empty documents become `null`.
- An empty file evaluates to `null`.

These rules match `std.parseYaml`. kubecfg accepts all of these shapes.

### Plain YAML is unchanged

**A `.jamlet` file whose only `${` occurrences are in strings and comments evaluates exactly
like `std.parseYaml` of the same text**, which is also what `kubectl` makes of it. You can
rename any `.yaml` manifest to `.jamlet` and get the same objects. Three exceptions: a plain
(unquoted) scalar containing `${` is an error in jamlet, so quote it; a quoted or block scalar
whose whole value is `${...}` is an expression, so tag it `!!str` to keep the string; and merge keys
(`<<`) are not supported.

This means YAML 1.1 scalar resolution:

| YAML                         | Result                   |
|------------------------------|--------------------------|
| `yes`, `on`, `y` (values)    | `true`                   |
| `no`, `off`, `n` (values)    | `false`                  |
| `n: 1` (key)                 | `{"false": 1}`           |
| `0777`, `0x1F`, `1_000`      | `511`, `31`, `1000`      |
| `.inf`, `.nan`, `~:` (key)   | error (not representable)|

Quote a scalar to keep it a string (`'yes'`, `'0777'`).

### Anchors and aliases

An anchor on a mapping also names it in Jsonnet, see [Anchors name mappings](#anchors-name-mappings).

An alias (`*x`) is a copy of the anchored node. Expressions in it are evaluated again at the
alias position, so `self` and `$` refer to the alias's surroundings. An alias can't repeat a
multi-line expression (that would shift the line numbers of later errors), so bind it with a
local declaration instead. Like `std.parseYaml`, jamlet rejects an anchor that contains an
alias to itself, and aliases that expand to more than 100,000 nodes ("billion laughs").
Merge keys (`<<: *base`) are rejected. Use Jsonnet inheritance instead:
`${base + {image: 'nginx'}}`.

### Tags

`!!str` forces a string, also on a quoted or block scalar that would be an expression, and `!!map`
and `!!seq` may mark mappings and sequences. Other tags (`!!int`, `!Ref`, the non-specific
`!`, ...) are rejected, and so are tags on unquoted expressions.

### Encoding

Files must be UTF-8. A leading byte order mark is ignored, and `\r\n` and `\r` line breaks
are treated as `\n`. The Unicode line separators U+0085, U+2028 and U+2029, which YAML also
treats as line breaks, are rejected.

## Comparison with kro

| | kro | jamlet |
|---|---|---|
| Expression language | CEL | Jsonnet |
| Where `${` is recognized | anywhere in string values, after YAML parsing | at the start of a node, by a YAML-aware lexer |
| Expression must also be valid YAML | yes (`${{a: 1}}` needs quoting) | yes, by the same quoting (not enforced yet) |
| `field: ${x}` | typed value | typed value |
| `field: "${x}"` | typed value | typed value |
| `name: app-${x}` | string template | error, write `${'app-' + x}` |
| Expressions in keys | no | yes, and `null` drops the field |
| Literal `${` | `${"${VAR}"}` | `!!str '${VAR}'`, or text around it |
| `${` in comments | (YAML parser drops comments first) | ignored |
| Variables | `schema`, resource ids | `${local ...}` in a file or a mapping, anchors of mappings |
| YAML inside an expression | no | `\{` and a line break open a YAML block |
| Unterminated `${` | error | error |

## Corner cases

- **Unterminated or empty expressions** (`${` with no matching `}`, `${}`) are errors. They
  are never treated as literal text, because that would silently drop what the author
  meant.
- **Multi-line expressions** may use any indentation, except in a block scalar. YAML never sees them, and YAML
  error line numbers are mapped back to the source.
- **Jsonnet comments inside an expression.** A `#` or `//` comment runs to the end of the
  line, so `${ x # note }` swallows the `}`. Put the closing `}` on the next line or use
  `/* */`.
- **Multi-line plain scalars.** `${` on a continuation line of a plain scalar is inside that
  scalar, and so an error.
- **Duplicate keys** are an error, also when a computed key collides at runtime.
- **Private-use characters.** Placeholders are built from a Unicode private-use character
  that doesn't occur in the source, so any source text is safe. (A YAML `` escape that
  happens to forge a placeholder is not detected. The author controls the source, so this
  is not a security issue.)

## kubecfg integration

`.jamlet` files work wherever `.jsonnet` files do: `show`, `diff`, `update`, `delete`,
`validate`, `eval`, overlays (`--overlay-code-file x.jamlet` is applied like a YAML
overlay), OCI bundles, and `pack` (which ships the jamlet source).

From Jsonnet, or from an expression in another jamlet file, import a jamlet file with the
`jamlet:` scheme:

```jsonnet
(import 'jamlet:deployment.jamlet') + { spec+: { replicas: 5 } }
```

`importstr 'deployment.jamlet'` and `importbin` return the file as it is. A plain
`import 'deployment.jamlet'` parses the YAML as Jsonnet and fails with a Jsonnet syntax
error. The scheme is needed because go-jsonnet hands `import`, `importstr` and `importbin`
the same cached file contents, so compiled code needs a location of its own. The scheme
works with any location: `jamlet:https://...`, `jamlet:oci://...`.

## Open questions

- **A YAML form for local declarations**, see [YAML compatibility](#yaml-compatibility).
- **Merge keys.** `<<: *base` could compile to Jsonnet inheritance (`base + {own fields}`),
  and `<<: ${expr}` would then let a YAML mapping inherit from a Jsonnet object.
- **Parameters.** A schema-like way to declare a template's inputs (like kro's
  `schema.spec`), instead of `std.extVar` or importing a parameters file.
- **Inline YAML blocks.** `\{a: 1}` on one line inside an expression is reserved (it is an
  error today). It adds little over a Jsonnet object literal.
- **Debugging.** A command to print the generated Jsonnet.
