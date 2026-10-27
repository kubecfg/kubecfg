# RFC07: knit, layered Kubernetes manifests in plain Jsonnet

Status: **Draft**. Nothing here is decided; see [RFCs](README.md).

This RFC is written alongside the first version of knit
([#541](https://github.com/kubecfg/kubecfg/pull/541)), which ships as the alpha feature
`knit`. The user guide, [knit](../Advanced-Usage/knit.md), says what knit does. This RFC says
why, records the design choices, and keeps the research on reading and patching list entries
by key, which isn't settled.

## Background

Most Kubernetes configs are a base plus a few variants: dev, staging, prod. Each variant
changes a little: more replicas, another env var, a different image. Two ways to write that
are common today:

* **kustomize with strategic merge patch.** A patch is plain YAML. Lists merge by their
  Kubernetes key: a patch that lists a container named `api` changes that container instead of
  adding a second one. But kustomize is a separate tool, outside Jsonnet, and has no
  expressions.
* **Jsonnet with a type library,** like Bitnami's
  [kube.libsonnet](https://github.com/bitnami-labs/kube-libsonnet). Jsonnet's `+` can't merge
  arrays by key: it joins them. So the library has you write `containers_: {api: {...}}`, an
  object keyed by name, and computes `containers` from it. That needs a library function per
  kind, loses the order of entries (Jsonnet objects have none), and plain YAML can't fill
  `containers_`.

What's missing: plain YAML manifests, merged in layers the way Kubernetes merges them, with
Jsonnet available for the logic.

## Goals

* **Layers of plain manifests.** A layer is YAML or Jsonnet data, as you'd give kubectl.
* **Lists merge by their Kubernetes key,** taken from the OpenAPI schema
  (`x-kubernetes-list-map-keys`, else `x-kubernetes-patch-merge-key`) and from CRDs found in
  the input. Nobody writes a library per kind.
* **The directives people already know.** `$patch: delete` and `$patch: replace`, spelled as
  in strategic merge patch.
* **Defaults for every object** (labels, a namespace, a node selector for every pod template),
  set once.
* **Plain Jsonnet,** with no Go code of its own, so it runs wherever kubecfg's Jsonnet runs.

## Non-goals

* Replacing kustomize. knit doesn't read `kustomization.yaml`.
* Talking to a cluster. The schema is bundled (Kubernetes 1.37), or extended by the user.
* Every strategic merge patch directive. `$setElementOrder`, `$deleteFromPrimitiveList` and
  `$retainKeys` are rejected.

## Design

The [guide](../Advanced-Usage/knit.md) has the details. In short:

1. **Normalize.** Each layer is read against the schema. A keyed list becomes an object keyed
   by its key, and each entry remembers where it first appeared (`$pos`). A deleted value
   becomes a marker.
2. **Stack.** Layers are applied in order with Jsonnet's `+`, on top of templates from a
   registry: per type (`PodTemplateSpec`), per kind, per group, and for every object.
3. **Match objects** across layers by group, kind, namespace and name, the identity
   `kubectl apply` uses.
4. **Render.** Keyed objects become arrays again, in their first-seen order, without deleted
   entries.

Around that:

* **Mixins** are Jsonnet applied to the composed stream, for changes that read other
  objects, like a checksum of a ConfigMap.
* **Migrations** turn an old API version into a newer one before matching.
* **CRDs in the input** teach knit their list keys and scope.
* **Templates follow the version.** If a layer changes an object's `apiVersion`, the object is
  rebuilt from its layers as the new version.

### With YAKL

A YAKL file can extend knit itself (`<<+: ${import 'knit/knit.libsonnet'}`), through the hook
described in [RFC09](rfc09.md#the-hook-for-libraries):

* `^^: api` makes a manifest a named object, and the field `api` of the file.
* `^^+: api` patches it the knit way, without repeating `apiVersion`, `kind` or `metadata.name`.
* `$.api` is the object as it ends up, after every layer of every file.
* Every document has a name, a mixin too: an object is where its name puts it, since its kind
  and `metadata.name` can be expressions an overlay changes. `kubecfg knit import` writes the
  names for YAML you didn't write.

### Design choices

* Lists with no schema are atomic: a later layer replaces them. Lists are never joined by
  accident.
* Keys made of several fields are written as JSON arrays: `'[8080,"TCP"]'`.
* A missing key field takes its default, when the schema has a real one, so
  `{containerPort: 80}` and `{containerPort: 80, protocol: TCP}` are the same port. The zero
  values the spec lists for required fields (`name: ""`) don't count as defaults.
* Duplicate keys in one list are an error.
* `null` takes back what lower layers set, down to the templates.
* An entry that is deleted and added again starts fresh, at its new position.
* An explicit `x-kubernetes-list-type` wins over a patch merge key, as in server-side apply.

## Feature gate

knit is the alpha feature `knit` ([RFC05](rfc05.md)). It can change in any release.

## Addressing list entries

A named object's field gives the object as a plain manifest: `containers` is an array. To read
one entry by key, each keyed list also has a hidden object next to it, with `_` after its
name, keyed by its key:

```yaml
env: ${$.api.deployment.spec.template.spec.containers_.api.env}
```

That works for reading. It has problems:

* **Keys of several fields are JSON text:** `ports_['[8080,"TCP"]']`. You have to know the key
  fields' order and the default protocol. A port named `metrics` can't be found by its name.
* **Two names for one list,** and the `_` one is never in the output, so nothing hints it
  exists.
* **Only on `$.name` views, and only for keyed lists,** not sets, plain arrays or values from
  elsewhere.
* **It could clash** with a real field whose name ends in `_`.

### What was tried

| Option | Reads | Patches | Verdict |
| --- | --- | --- | --- |
| A. `containers_.api` (today) | yes | no (see below) | keep, make writable |
| B. `find(containers, 'api')`, matching a subset of fields | yes | no | turned down |
| C. a path string, `at(o, 'spec.containers[name=api]')`, as kustomize writes it | yes | no | turned down |
| D. a name on the entry, `- {^^: api.container, ...}`, read as `$.api.container` | yes | yes | only for entries someone named |
| E. keyed lists as objects in `$.api`: `containers.api` | yes | yes | breaks reading |

* **B and C only read.** A function call can't be the target of an overlay. An overlay names a
  path of fields, so whatever reads an entry must also be a path of fields, and the same path
  must work as a name (`^^+: a.b.c`) and as a Jsonnet field (`a+: {b+: {c+: ...}}`). B did work
  well for reading in a test: a subset like `{containerPort: 9090}` finds an entry without
  knowing the default protocol, and `'metrics'` finds a port by name.
* **E isn't a manifest any more.** `for c in containers`, `std.length` and handing the value to
  code that expects arrays all break. knit does accept the keyed form as input, but in a test
  the order was lost: env `Z, A` came back as `A, Z`. Env order matters for `$(VAR)`
  references.
* **D is the cleanest to use,** but needs YAKL to allow names below a document's root, and only
  works for entries the base author named. Upstream YAML has no names.

### Selectors, for reading

YAKL now has selectors: `containers[@.name == 'api']` is the one entry for which the condition
is true ([YAKL](../Advanced-Usage/yakl.md#picking-a-list-entry-by-a-condition)). That is B with
a syntax of its own, and works on any list, with or without a schema: a port is found by its
name, or by `@.containerPort == 8080` without knowing the default protocol. `@` isn't valid
Jsonnet on its own, so YAKL turns each selector into a call to `kubecfg.pick` before Jsonnet sees it.

Selectors don't patch, for the same reason as B and C: they aren't a path of fields. So they
replace the `_` views for reading, and patching by key is still open.

### A bug found on the way

A name deeper than the object's name, like
`^^+: api.spec.template.spec.containers_.api.env_`, compiles, but knit only treats an exact
object name as a patch. The rest is plain YAKL, a Jsonnet `+` on the field `$.api`, which is
already the final view. In a test, the env change was silently dropped, and a path through
`volumeMounts_` put a real `volumeMounts_:` field into the Deployment.

### The proposal for A

1. **A name resolves by its longest prefix that is an object.** In the name above, `api` is the
   object and `spec.template.spec.containers_.api.env_` is a path inside it. The patch is
   wrapped at that path, as `^^+: api` patches are today. A name under an object that resolves to
   nothing is an error.
2. **`x_` is a way to write, in any layer.** Where the schema says `x` is a keyed list,
   `x_: {key: {...}}` means those entries of `x`, with the key fields filled in from the key.
   This is kube.libsonnet's way of writing, on top of knit's merge. knit already accepts the
   keyed form at `x`, but each entry must repeat its own key.
3. **Keys are spelled so a name can hold them.** A `.` always splits the path, and `,[]{}`
   would need quotes. Keys of several fields would be joined with `/`, the way kubectl prints
   ports: `ports_.8080/TCP`, and fields with defaults could be left off at the end:
   `ports_.8080`. Reads would use the same spelling. A key with a `.` in it (a `mountPath`, an
   IP, a `topologyKey`) can't go in a name. The name stops one step earlier, and the body names
   the entry: `volumeMounts_: {/var/lib.d: {readOnly: true}}`.

Some key values contain `/`, like the `topologyKey` `kubernetes.io/hostname`. The schema says
how many fields a key has, so the key would be split from the right, and the first field keeps
any extra slashes. That works for every key in the bundled schema: `containerPort/protocol`,
`port/protocol`, `status/reason`, `topologyKey/whenUnsatisfiable` and
`driver/device/pool/shareID`.

This is on hold. [RFC08](rfc08.md) proposes arrays that Jsonnet itself merges by key. If that
happens, `containers.api` is the one name for reading and writing, `x_` goes away, and most of
knit's normalize and render code goes with it.

## Open questions

* **The deep-name bug** should be an error now, whatever happens to A.
* **Placing new entries,** and removing a value from a set, aren't possible yet.
* **Type templates for CRDs with inline schemas,** whose types have no names.
* **`knit.explain(path)`,** to show which layer or template set a value.
