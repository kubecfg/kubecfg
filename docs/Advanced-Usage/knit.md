# knit

knit merges plain Kubernetes manifests in layers (a base, then prod, staging,
...) and matches list entries by key: containers by `name`, volume mounts by
`mountPath`, container ports by `containerPort` + `protocol`. The keys come from
the Kubernetes OpenAPI schema and from CRDs found in the input, so nobody has
to write a type library per kind. It is plain Jsonnet, with no Go code of
its own.

knit comes with kubecfg. Import it as `(import 'kubecfg.libsonnet').knit`, or
as `import 'knit/knit.libsonnet'`; nothing is downloaded and no `-J` path is
needed. knit is an alpha feature: it needs `kubecfg --features=knit`. Its source
and tests are in [`lib/knit`](https://github.com/kubecfg/kubecfg/tree/main/lib/knit).

## Quick start

```jsonnet
local knit = import 'knit/knit.libsonnet';

local registry = knit.registry {
  objects+: { metadata+: { labels+: { team: 'platform' } } },
  types+: { PodTemplateSpec+: { spec+: { nodeSelector+: { 'kubernetes.io/os': 'linux' } } } },
};

knit.render(registry, [
  import 'yaml:base.yaml',
  import 'yaml:prod.yaml',
])
```

Render it with `kubecfg show main.jsonnet`. See
[`lib/knit/testdata/examples`](https://github.com/kubecfg/kubecfg/tree/main/lib/knit/testdata/examples) for the full input and the
`expected.yaml` output of each case, and
[`examples/experimental/knit/shop`](https://github.com/kubecfg/kubecfg/tree/main/examples/experimental/knit/shop) for a
larger example.

## Layers

`render(registry, layers)` takes an ordered array. A layer can be:

- a single manifest, a `v1` `List`, or an array of manifests (a parsed YAML stream);
- an object whose leaves are manifests, like `{ deployment: {...}, service: {...} }`.
  An array inside such an object holds manifests only;
- a mixin made with `knit.mixin(...)` (see below). A mixin is always a layer
  of its own.

Objects are matched across layers by group, kind, namespace and name. The API
version is not part of the match. The namespace is the one the object ends up
with: its own (an explicit `namespace: ''` included), or else the one the
registry templates give it. Cluster-scoped kinds have no namespace. So with `objects: { metadata: { namespace: 'shop' } }`,
a base object written with `namespace: shop` and an overlay object with no
namespace are the same object. An object that appears only in a later layer is
added. The output has the same shape as the first layer: an array, or an
object with each manifest at the path where it first appeared. In that second
case, two objects that land on the same path (or one inside another) are an
error, since they would otherwise merge into one.

## Registry

Templates sit under every layer. A layer always wins over a template, and
higher rows win over lower ones:

| Registry field                    | Applies to                                      |
| --------------------------------- | ----------------------------------------------- |
| `kinds['apps/v1/Deployment']`     | that exact version                              |
| `kinds['apps/Deployment']`        | that kind in every version                      |
| `groups['apps']`                  | every kind in the group (`''` is the core group) |
| `objects`                         | the root of every object                        |
| `types['PodTemplateSpec']`        | every value of that schema type, at any depth   |

Registry keys are checked: `kinds.Deployment` is an error (the key is
`apps/Deployment`), and so is `groups.core` (the core group is `''`). Groups
with a dot may belong to CRDs defined elsewhere, so their keys are not checked.
Migration keys must include a version, and may name APIs the schema no longer
has (`extensions/v1beta1/Deployment`).

`types` takes full names (`io.k8s.api.core.v1.Container`) or short names when
they are unique (`Container`, `ObjectMeta`). Two keys that name the same type
are an error. A type template is applied to
values that some layer or template sets. It does not create them: a `Probe`
template does not add probes to containers that have none. Type templates also
reach into atomic lists, such as a StatefulSet's `volumeClaimTemplates`; the
list is still replaced whole by a later layer. A type template that holds a
value of its own type (a `JSONSchemaProps` inside a `JSONSchemaProps`) does not
apply to that inner value.

A template can start with `$patch: replace` to drop the template levels below
it.

Kind and version templates follow the version an object has at each layer.
If a later layer changes `autoscaling/v1` to `autoscaling/v2`, the object is
rebuilt from the data layers read as `v2`, on top of the `v2` templates, so
only the `v2` schema and templates apply. A mixin that changed the object
before that point is an error, since its change cannot be replayed.

A namespace in `objects` is not applied to cluster-scoped kinds (ClusterRole,
Namespace, CRDs, ...).

`migrations['policy/v1beta1/PodDisruptionBudget']` is a function from an old
object to a newer one. It runs before matching and before templates, and
migrations chain. It runs on every object a layer holds, including partial
overlays and `$patch: delete` markers (without their `$patch`), so keep the
fields you do not change: `function(o) o { apiVersion: 'policy/v1' }`.

`schema` holds the merge schema. It defaults to the bundled Kubernetes 1.37
schema, and can be extended for CRDs that are not in the input. Every kind
gets the `ObjectMeta` schema for `metadata`, so an override only describes the
rest:

```jsonnet
knit.registry {
  schema+: { kinds+: {
    'example.com/v1/Thing': { fields: { spec: { fields: { items: { list: 'map', keys: ['name'] } } } } },
  } },
}
```

## How lists merge

| List kind | Where the key comes from                                              | Rule                                         |
| --------- | --------------------------------------------------------------------- | -------------------------------------------- |
| keyed     | `x-kubernetes-list-map-keys`, else `x-kubernetes-patch-merge-key`     | entries with the same key merge field by field |
| set       | `x-kubernetes-list-type: set`                                         | union of values                              |
| atomic    | `x-kubernetes-list-type: atomic`, or no schema at all                 | a later layer replaces the list              |

Keys come from the registry's `schema` (the bundled Kubernetes schema plus any
overrides), which wins over CRDs found in the layers. A CRD is read as the
data layers compose it, so an overlay can change a CRD's schema without
repeating its group and names. Mixins do not change how custom resources
merge. A list knit knows nothing about is atomic, so lists are never joined
by accident.

A missing key field takes the value the entry gets from its type template, or
else the schema's default when there is a real one, so `{containerPort: 80}`
and `{containerPort: 80, protocol: TCP}` are the same port.

Entries keep the position of the layer that first added them. Entries that a
later layer adds go after the existing ones.

## Directives

The same spelling as strategic merge patch, where one exists:

| Directive (YAML)                   | Jsonnet                     | Effect                                         |
| ---------------------------------- | --------------------------- | ---------------------------------------------- |
| `$patch: delete` on a list entry   | `{ name: 'x' } + knit.delete` | removes the inherited entry                 |
| `$patch: delete` on a field        | `knit.delete`              | removes the inherited field                    |
| `null` on a field                  | `null`                      | takes back what lower layers set (see below)   |
| `$patch: delete` on an object      | `obj + knit.delete`        | removes the whole object                       |
| `- $patch: replace` in a list      | `knit.replace([...])`      | this list replaces the inherited one           |
| `$patch: replace` on a map         | `{ '$patch': 'replace', ... }` | this map replaces the inherited one         |
| `$patch: replace` on a list entry or object | `{ name: 'x', '$patch': 'replace', ... }` | drops the inherited fields, keeps the position |
| `- $patch: append` in an atomic list | `knit.append([...])`     | appends to the inherited list                  |

A `null` takes back what lower layers set: the field goes back to what the
registry templates give it, or goes away if they give it nothing.
`$patch: delete` removes the field even when a template sets it. A `null` with
nothing below it is kept as a value, and so is a `null` in free-form data
(`x-kubernetes-preserve-unknown-fields`), as in a Helm release's `values`.

`$patch: replace` drops what lower layers set. What the registry templates
give stays. An entry that is deleted and then added again by a later layer
starts from scratch, at its new position. Entries of atomic lists cannot carry
`$patch`; such a list is replaced whole, or appended to with the marker.
`$setElementOrder`, `$deleteFromPrimitiveList` and `$retainKeys` are rejected
with an error.

knit keeps a few hidden fields on values: `$at`, `$deleted`, `$list`,
`$mixin`, `$n`, `$pos` and `$v`. They cannot be used as keys in the input.

## Mixins

A data layer can read the final objects by their [names](#objects-have-names),
but it can only give values. A mixin can also change and delete what other layers
wrote: it is applied with `+` to the composed stream.
Inside it, `super` is the stream as the layers below left it, and `self` is
the final stream, after every layer. Bind `self` at the top of the mixin; `$`
would point at the outermost object of the file, which is not the stream once
`render` is called inside an object:

```jsonnet
knit.mixin({
  local stream = self,
  [knit.id('apps/Deployment', 'web')]+: {
    spec+: { template+: { metadata+: { annotations+: {
      'checksum/config': std.md5(std.manifestJson(stream[knit.id('ConfigMap', 'web')].data)),
    } } } },
  },
})
```

Use `super` for a value the mixin changes (`replicas: super.replicas * 2`);
reading it through `self` would read the mixin's own result.

Objects are keyed by `knit.id(groupKind, name, namespace)`, with the
namespace the object ends up with. A mixin sees the internal form: keyed lists
are objects keyed by their key (`containers+: { web+: { image: 'web:2' } }`),
and composite keys are written as JSON arrays (`'[80,"TCP"]'`). A new entry
carries its own key (`env+: { FOO: { name: 'FOO', value: '1' } }`).
`knit.delete` removes an object or an entry. Lists are replaced with `:` and
appended to with `+`, not with `$patch`.

What a mixin adds is treated like what a data layer adds: new values get
their type templates, new entries go after the existing ones (an entry copied
from another, `side: super.web + { name: 'side' }`, keeps the place of the one
it copies), and a new
object gets its kind's templates and goes after the objects already there (in
nested output, at a top-level key named after its id). Assigning to an id that
matches no object creates that object, so the value must be a whole manifest
with that identity. A created object is read once, like a data layer: `self`
inside it is its own value, and later layers do not change what it computed.
In objects that already exist, `self` keeps meaning the final value.

## YAKL files

Instead of a `main.jsonnet` that lists the layers, each
[YAKL](yakl.md)
file can extend its parent, as a Jsonnet file would with `+`, and
`kubecfg show prod.yakl` renders the whole chain. A first document that holds
only `<<+: ...` says what the file extends. The first file extends knit
itself.

### Objects have names

A name tag gives an object a name, as a field of the file, the way a Jsonnet
file names its objects. The name is what an overlay changes, and what other
documents read:

```yaml
# base.yakl
<<+: ${import 'knit/knit.libsonnet'}
---
!config::
replicas: 2
---
!api
apiVersion: apps/v1
kind: Deployment
metadata: {name: api}
spec:
  replicas: ${$.config.replicas}
  template:
    spec:
      containers:
        - name: api
          image: api:1
          env: [{name: REDIS_URL, value: 'redis://redis:6379'}]
---
!worker
apiVersion: batch/v1
kind: CronJob
metadata: {name: worker}
spec:
  schedule: '@daily'
  jobTemplate:
    spec:
      template:
        spec:
          containers:
            - name: worker
              image: ${$.api.spec.template.spec.containers_.api.image}
              env: ${$.api.spec.template.spec.containers_.api.env}
```

```yaml
# prod.yakl
<<+: ${import 'yakl:base.yakl'}
---
!registry+
objects+:
  metadata+: {namespace: prod}
---
!config+
replicas: 5
---
!api+
spec:
  template:
    spec:
      containers:
        - name: api
          env: [{name: CACHE_TTL, value: '3600'}]
```

- **`!api+` changes the object named `api`,** the knit way: the container is
  found by its name, and its env gets one more entry. The patch doesn't repeat
  `apiVersion`, `kind` or `metadata.name`.
- **`$.api` is the object as it ends up,** after every layer of every file.
  The worker reads the API's env, so prod's `CACHE_TTL` reaches the worker
  too. Next to each list with a Kubernetes key there is a hidden field with
  `_` after its name, keyed by that key: `containers_.api`,
  `env_.REDIS_URL`, `ports_['[8080,"TCP"]']`.
- **A value can't depend on itself.** Reading a field of `$.api` from that
  same field, or from what makes the object's identity (`apiVersion`, `kind`,
  `metadata.name` and `namespace`), never ends: Jsonnet stops with "max stack
  frames exceeded".
- **`!api` without `+` gives the name again,** as a Jsonnet field does: the
  object is replaced.
- **`!api::` is hidden:** other documents can read it, but it isn't output.
  An overlay leaves an object out with `!api+:: {}`. Other documents still
  read it, and a later `!api+` still applies and keeps it hidden, as a
  Jsonnet field keeps the visibility it inherits; `!api+::: {}` outputs it
  again. `!api+` can't carry `$patch: delete`: `$.api` would become `null`,
  and a later `!api+` would start a new object with only its own fields.
- **A name is an object when its document is a manifest.** Other names, like
  `!config::` and `!registry+`, are plain YAKL, extended with Jsonnet's `+`.
  A document named `config` is read as `$.config` and sees the last file's
  settings.

Each object is output once, by its field, so objects come out in the order
of their names, as the fields of a Jsonnet file do.

### Documents without a name

Objects can also go without a name, for YAML you don't write yourself, like
a chart's output or upstream manifests. Such a document is a layer at its
place in the file, and knit finds its objects by identity, as `kubectl apply`
would: group, kind, the namespace they end up with, and name. A document
without a name can change a named object too, and an overlay can change
objects that have no name:

```yaml
# Redis as published upstream: a parsed YAML stream is a layer too.
${import 'yaml:redis.yaml'}
---
apiVersion: apps/v1
kind: StatefulSet
metadata: {name: redis}
spec:
  template:
    spec:
      containers:
        - name: redis
          resources: {requests: {memory: 2Gi}}
```

Objects without a name come out in the field `manifests`. A document can
also be a `knit.mixin(...)`. The file is knit's own object, so its helpers
are there as `$.mixin(...)`, `$.id(...)` and `$.delete`, and a document
cannot take the names knit renders with: `registry` (made to be extended),
`documents`, `unnamed`, `manifests` and `render`. See
[`lib/knit/testdata/examples/named`](https://github.com/kubecfg/kubecfg/tree/main/lib/knit/testdata/examples/named)
and
[`lib/knit/testdata/examples/yakl`](https://github.com/kubecfg/kubecfg/tree/main/lib/knit/testdata/examples/yakl).

### How it works

YAKL lists a file's documents in order in the hidden field `documents`, and
a named document's field asks the object the file extends for its value
(`'$named'`). knit stacks every document of every file as layers, in order:
a named document's layer carries its object's identity, and a `+` document
is a patch of it. A named object's field is that object, read from the
final stream. See [YAKL: files that extend](yakl.md#a-file-extends-another).

## Design choices

- Lists with no schema are atomic.
- Composite keys are JSON arrays; single keys are the plain value.
- Zero values in the spec (`name: ""`, `containerPort: 0`) are not treated as
  defaults. The v3 spec lists them as defaults for every required field.
- Duplicate keys in one list are an error.
- `null` in a data layer takes back what lower layers set, down to the
  templates; with nothing below, it is a value.
- Deleted-then-re-added entries start fresh.
- Namespace is part of identity, as the object ends up with it: the templates'
  namespace counts for an object without one.
- An explicit `x-kubernetes-list-type` beats a patch merge key (as server-side
  apply does). This only matters for two status fields in Kubernetes 1.37.
- Whether a kind is cluster-scoped comes from the OpenAPI paths: a kind is
  namespaced when its resource path contains `/namespaces/{namespace}/`.
  Kinds with no schema count as namespaced.

## Not done yet

- `knit.explain(path)`, live-cluster schema lookups, Go natives.
- Removing a value from a set, and explicit placement of new entries.
- Type templates for CRDs with inline schemas (their types have no names).
- Matching nested-object layers by field path; all layers match by identity.

## Development

```sh
go test ./lib/knit                             # edge cases, expected errors, golden examples
go test ./lib/knit -run TestExamples -update   # rewrite testdata/examples/*/expected.yaml
lib/knit/hack/gen-schema.sh v1.37.1            # regenerate lib/knit/schema/k8s-1.37.libsonnet
```

`schema/k8s-<minor>.libsonnet` is generated from the Kubernetes OpenAPI v3
spec by `hack/gen-schema.jsonnet`, which uses the same converter that reads
CRDs at render time.
