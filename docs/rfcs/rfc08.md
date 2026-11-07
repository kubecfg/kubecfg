# RFC08: Keyed arrays and types for Jsonnet

Status: **Draft**. Nothing here is decided; see [RFCs](README.md).

This RFC records an idea, not a plan to build it now. It came out of knit's problem of
reading and patching list entries by key ([RFC07](rfc07.md#addressing-list-entries)). It
needs changes to the Jsonnet interpreter, so it would be a project of its own: a fork of
go-jsonnet, or a proposal upstream.

## Background

Many Kubernetes lists are really maps. A Deployment's `containers` is a list, but each
container is found by its `name`. `env` is found by `name`, `volumeMounts` by `mountPath`, and
container ports by `containerPort` and `protocol`. The OpenAPI schema says so
(`x-kubernetes-list-type: map` and `x-kubernetes-list-map-keys`), and Kubernetes merges these
lists by key in strategic merge patch and server-side apply.

Jsonnet doesn't know any of this. `+` on two arrays joins them, and an array can only be read
by position. So there are two ways to work with these lists in Jsonnet today, and neither is
complete:

* **kube.libsonnet** has you write the list as an object keyed by name, `containers_`, and
  computes the array `containers` from it. Reading and patching both use the object:
  `containers_+: {api+: {env_+: {FOO: 'x'}}}`. That's pleasant to use, but the data model is
  objects, not the arrays YAML has. The order of entries is lost. Plain YAML can't fill
  `containers_`. Writing `containers` directly silently drops whatever is in `containers_`.
* **knit** keeps plain arrays in the input and output, and converts keyed lists to objects
  inside. It gives a read-only object next to each list, `containers_`, but an overlay can't
  use that path to patch ([RFC07](rfc07.md#addressing-list-entries)).

What's wanted: kube.libsonnet's way of reading and patching by key, on the array data model
YAML uses. One name, `containers`, for reading, patching and output.

## Keyed arrays

Add to the interpreter one function, `std.keyed(keys, arr)`. It returns an array that knows
which fields are its key. For containers, `std.keyed(['name'], [...])`. On such an array:

| Expression | Plain array | Keyed array |
| --- | --- | --- |
| `arr[0]` | the first element | the first element |
| `arr.api`, `arr['api']` | error | the element whose key is `api` |
| `arr + [{name: 'api', image: 'x'}]` | joined | merged by key: a matching element is `old + new`, others go at the end, order kept |
| `arr + {api+: {...}, FOO+: {...}, A: null}` | error | patched as if it were an object keyed by its key; a new element gets its key fields from its name; `null` removes one |
| `for e in arr`, `std.length(arr)`, output | an array | an array |

Because a field written `f+: v` means `super.f + v`, kube.libsonnet's way of patching works on
the array itself:

```jsonnet
api+: { spec+: { template+: { spec+: { containers+: { api+: { env+: {
  FOO+: { value: 'foo2' },   // new: gets name: 'FOO'
  Z+: { value: 'z2' },       // patched in place
  A: null,                   // removed
} } } } } } },
```

and reading is a path of fields, late-bound like any other:

```jsonnet
env: $.api.spec.template.spec.containers.api.env,
```

The same path works as a YAKL name, `^^+: api.spec.template.spec.containers.api.env`, because
the name compiles to those `+:` fields. Reading and patching use the same name.

There's no new syntax. Only arrays made by `std.keyed` behave differently, and no array is
keyed today, so no existing program changes meaning. `jsonnetfmt`, parsers and editors keep
working.

### A prototype

A throwaway prototype on go-jsonnet v0.22.0 took a new file of 168 lines plus 13 changed lines
in `value.go`, `builtins.go` and `interpreter.go`. go-jsonnet's own tests still passed. In a
test with a base, two overlays and a second container reading the first one's env:

* The env order was kept: `Z` stayed before the new `FOO`, and `A: null` removed `A`.
* An overlay written as an array changed the image and added a `sidecar` container.
* The reader saw every change, late-bound.
* `std.isArray`, `std.length`, `arr[1]` and comprehensions behaved like on any array.

The prototype wasn't kept. Nothing in kubecfg depends on it.

## Where keys come from: types

A keyed array only helps if something calls `std.keyed` on every keyed list before an overlay
merges into it, including lists that an overlay adds (a new container's `env`). Tagging every
list by hand is what kube.libsonnet's library functions do, kind by kind. knit does it from the
OpenAPI schema, by walking every layer.

The facts needed are schema facts: which lists are keyed, by which fields, with which defaults
(`protocol: TCP`), and which lists are sets or atomic. knit already turns the OpenAPI schema
into exactly that: a field is a named type, an inline object, a keyed list, a set or an atomic
list. That's a type, so the idea grows into **types for Jsonnet**:

* **A value can carry a type.** An object's type gives its fields' types, so reading a field
  gives a typed value. Only the root needs a type, found from `apiVersion` and `kind`. Every
  list below it knows its keys without being tagged.
* **`+` keeps the left side's type,** as keyed arrays do. A value merged into a typed field
  takes that field's type, so a container added by an overlay gets typed `env` and `ports`.
* **Untyped values behave as today.** Types are opt-in, value by value.
* **The type says how a field merges:** by key, as a set, or replaced whole. That's the rule
  strategic merge patch and server-side apply follow, now in the language.
* **More can hang off a type later:** defaults per type (what knit's `types` templates do, like
  a node selector for every `PodTemplateSpec`), checks when a value is read or output, and
  maybe static checking and editor completion from the same OpenAPI and CRD schemas.

With types, knit gets much smaller. Layering is plain Jsonnet `+`. What stays is the schema,
the templates, and matching the objects of Jsonnet layers by identity. The normalize and render
code, `$pos`, and the `_` views go away.

## Prior art

* **Strategic merge patch and server-side apply:** the merge rules, and the schema
  annotations that drive them.
* **kube.libsonnet:** the `x_` objects, and the experience this RFC wants on arrays.
* **CUE:** keyed lists are usually written as structs and turned into lists with a
  comprehension, the same trade-off as kube.libsonnet.
* **Pkl:** member predicates, `[[name == "api"]] { ... }`, address entries of a list by a
  condition when amending it.
* **Nickel:** contracts, types checked at run time, and merging with priorities.

Each of these needs a closer look before a design is final.

## Open questions

* **Fork or upstream.** A fork means rebasing on go-jsonnet releases, and files that only run
  on kubecfg: not on the C++ jsonnet, jrsonnet or sjsonnet, and not understood by
  jsonnet-language-server or go-jsonnet's linter. Upstream may be slow or say no. Since it's
  additive, a proposal is worth trying either way. ursonnet, kubecfg's tracer, builds on
  go-jsonnet too.
* **Overlays need a deep merge.** In Jsonnet, `env: [...]` inside a patch replaces the env. A
  YAML overlay means "merge". A `^^+: api` document would need `+:` on every field, like
  kubecfg's `toOverlay`, and an explicit way to replace. The prototype's merge by key is one
  level deep.
* **Order of entries added by an object patch.** Jsonnet objects keep no field order, so the
  prototype adds them in key order. The interpreter does know the order fields were written in,
  and could keep it.
* **Keys of several fields.** The prototype joins them with `/` (`ports['8080/TCP']`), the
  spelling [RFC07](rfc07.md#the-proposal-for-a) proposes for names. Defaults should let
  `ports['8080']` find `8080/TCP`, and a new entry should get every key field back from its
  name, with the right types (`8080` is a number).
* **Laziness.** Finding entries by key evaluates every element's key fields. A key that
  depends on the list it's in would loop. That's rare, but the error should say what
  happened.
* **What drops keys.** `std.map`, `std.filter`, `std.sort`, and `[] + arr` return plain arrays.
  Should some of them keep the key? Should `'api' in arr` work?
* **Removing entries in the array form,** with `$patch: delete` as in YAML, or only with
  `null` in the object form?
* **Speed.** Finding an entry by key scans the array. A cached index may be needed for big
  lists.
* **Type syntax.** Does a type stay a value built from a schema (`std.typed(schema, v)`), or
  does Jsonnet get syntax for it? A value is enough for the Kubernetes case.
