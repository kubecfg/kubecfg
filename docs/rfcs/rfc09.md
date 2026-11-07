# RFC09: YAKL, YAML with Jsonnet expressions

Status: **Draft**. Nothing here is decided; see [RFCs](README.md).

This RFC is written after the fact. YAKL shipped as an alpha feature, first as
jamlet ([#530](https://github.com/kubecfg/kubecfg/pull/530),
[#535](https://github.com/kubecfg/kubecfg/pull/535)), then renamed and reworked as YAKL
([#537](https://github.com/kubecfg/kubecfg/pull/537)). The user guide,
[YAKL](../Advanced-Usage/yakl.md), says what every construct does. This RFC says why it's
shaped that way, what was turned down, and what's still open.

## Background

kubecfg's language is Jsonnet. Jsonnet is good at what configuration needs: values that refer
to each other, inheritance with `+`, hidden fields, functions, imports. But most Kubernetes
users write YAML, and the manifests they copy from docs, charts and upstream projects are YAML.
To use kubecfg, they first have to rewrite those manifests in Jsonnet.

Other tools keep YAML and add expressions to it. [kro](https://kro.run) puts CEL expressions
in `${...}`. Helm puts Go templates around YAML text. Text templates have well-known problems:
the output is text, so indentation and quoting are the author's job (`nindent`, `toYaml`), and
a value can break the YAML around it.

## Goals

* **Plain YAML is (almost) valid YAKL.** Rename a `.yaml` manifest to `.yakl` and get the same
  objects. The few exceptions are errors, not silent changes.
* **A YAKL file is valid YAML.** kubectl, kustomize, prettier, yamllint and editors can read
  it. To them, an expression is a string, and a name (`^^: web`) and `spec+` are keys.
* **Expressions are values, not text.** `${...}` replaces a whole node with a Jsonnet value.
  Nothing is pasted into text, so nothing needs indenting or quoting.
* **Jsonnet's reuse features, spelled in YAML:** inheritance, hidden fields, locals, named
  fields that other parts of the file read, files that extend other files, overlays.
* **No new evaluator.** A YAKL file compiles to Jsonnet, so everything kubecfg does with
  Jsonnet (imports, OCI bundles, tracing, natives) works on it. Errors point at the `.yakl`
  line.

## Non-goals

* **String interpolation.** `name: app-${x}` is literal text. Write `${'app-' + x}`.
* **A new expression language.** The expression language is Jsonnet, unchanged.
* **Keeping meaning through every tool's round trip.** kustomize expands anchors. YAKL
  documents what survives and what doesn't, but doesn't bend its design around every tool.

## Design

The [user guide](../Advanced-Usage/yakl.md) has the details. In short:

| You write | It means |
| --- | --- |
| `${expr}` as a whole string, in any YAML quoting style | the Jsonnet expression's value |
| `$${...}` at the start of a string | the literal text `${...}` |
| `- ${for x in xs}:` with a body under it | the body once per `x` |
| `- ${if cond}:` with a body under it | the body, if `cond` holds |
| `spec+:`, `name::`, `name:::` | Jsonnet's `+:`, hidden and forced-visible fields |
| `<<+: S` | `S + {the mapping's own fields}` |
| `&labels` on a node | a local named `labels`; the node stays where it is |
| `'&port': 8080` | a local that isn't output |
| `^^: web` among a document's top-level keys | the document is the field `web` of the file, read as `$.web` |
| a first document of only `<<+: S` | the file extends `S` |

Four rules hold it together:

1. **Meaning lives in data and anchors, never in spelling or tags.** Tools rewrite quotes,
   reorder keys and drop tags. So a quoted, plain or block expression means the same thing,
   and key order never matters.
2. **`&name` always declares a local.** An anchor on a node keeps the node and names it. A
   quoted `'&name':` key only names a value.
3. **The `^^` key names a document,** and takes `+` and colons like a Jsonnet field:
   `^^+: service` extends `service`, `^^:: config` hides `config`. A dot starts a level
   ([#568](https://github.com/kubecfg/kubecfg/pull/568)): `^^: web.deployment` is
   `$.web.deployment`, and `web` adds to what a base has in it.
4. **One document is its value; several are named mappings.** A file of several documents,
   or one that extends another, is one object, as a Jsonnet file usually is, and `$` is that
   object. Each document is a field of it, so files extend and override each other's documents
   by name, never by position.

### Compiling

The compiler finds every `${` that starts a YAML node, reads the Jsonnet expression up to its
matching `}`, swaps it for a placeholder, parses the rest as YAML, and writes Jsonnet with the
expressions put back. YAML nodes and expressions stay on their source lines, so Jsonnet errors
and `kubecfg traceback` point at the `.yakl` file. `kubecfg eval -o jsonnet` prints the
generated code. `yaklfmt` formats the expressions.

### The hook for libraries

A file that extends an object lists its documents, in order, in the hidden field `documents`.
If the object has a `$named(name, plain)` method, each named document's field asks it for its
value. Without the method, the field is the document as written. This lets a library decide
what a name means. knit ([RFC07](rfc07.md)) uses it to make each named manifest a layer it
merges by Kubernetes key, and the field the object as it ends up.

## Decisions and what was turned down

* **Jsonnet, not CEL.** CEL can compute values but can't define them: no inheritance, no
  locals, no functions, no imports. kubecfg already runs Jsonnet.
* **No interpolation.** It would make `namePrefix` and similar tools work on names. But it
  would make `${` inside any string mean something, so plain YAML would no longer mean what it
  says, and an expression's value would be turned into text. Dropped on purpose.
* **`<<+`, not `<<: ${...}`.** kubectl and kustomize reject a `<<` merge key whose value isn't
  a mapping. `<<+` is an ordinary key to them. `<<` keeps its YAML meaning.
* **Overlays are `base + overlay`, as written.** jamlet deep-merged overlays with `toOverlay`.
  That meant a YAKL file read differently as a base and as an overlay. Now an overlay spells
  its own `+:` and says exactly what it does. This changed meaning without an error, which
  alpha allows; the migration table in the guide lists it.
* **Names are keys, not tags.** Names were first tags, `!web`. But a tag says what type a node
  is, and tools treat it that way: kustomize drops it, PyYAML's safe loader rejects it, the VS
  Code YAML server warns about it, and kubectl rejects `--- !web`. `^^` is an ordinary key to
  all of them ([#557](https://github.com/kubecfg/kubecfg/pull/557)).
* **Names only on documents.** Only a `^^` key among a document's top-level keys names it. The
  file's shape is then known without running anything, and a `^^` key elsewhere is an error, so
  nothing is silently ignored.
* **YAML 1.1 booleans are errors.** kubectl reads `yes` as `true` and kustomize as `"yes"`, so
  such a file means two things. YAKL refuses it.
* **Names are text, not expressions,** so tools and readers can see the file's fields.
* **No documents without a name in a stream** ([#564](https://github.com/kubecfg/kubecfg/pull/564)).
  They used to go to a list field, `unnamed`, which an overlay appended to. A list can't say
  which of its items an overlay means, so every document now has a name, and a document
  without one is an error.
* **`^^+:` only extends what the base has.** `^^+: servce` would otherwise add a fragment of a
  document with no error. To add a document, write `^^:`. A name with dots extends a field
  of a field: `^^+: web.conf` needs `web.conf` in the base.

## Feature gate

YAKL is the alpha feature `yakl` ([RFC05](rfc05.md)): `.yakl` files, `yakl:` imports,
`yaklfmt` and `eval -o jsonnet`.

## Open questions

The [guide's list](../Advanced-Usage/yakl.md#open-questions) is the current one. Two more came
up while building knit:

* **Name paths into keyed lists.** knit reads `$.api.spec.template.spec.containers_.api.env`,
  but the same path as a name, `^^+: api.spec.template.spec.containers_.api.env`, compiles
  today but its change is silently lost. See [RFC07](rfc07.md#addressing-list-entries). A `.`
  always splits the path (`%2E` too), which limits how list keys can be spelled in a name.
* **Names below a document's root.** A `^^` key on a list entry, `- {^^: api.container, ...}`,
  would give it a name that both overlays and expressions can use. It needs the
  documents-only rule lifted, and a
  meaning for it outside knit. See [RFC08](rfc08.md) for the larger idea that may make it
  unnecessary.
