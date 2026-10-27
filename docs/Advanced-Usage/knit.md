# knit

knit merges plain Kubernetes manifests in layers (a base, then prod, staging,
...) and matches list entries by key: containers by `name`, volume mounts by
`mountPath`, container ports by `containerPort` + `protocol`. The keys come from
the Kubernetes OpenAPI schema and from CRDs found in the input, so nobody has
to write a type library per kind. It is plain Jsonnet, with no Go code of
its own.

knit comes with kubecfg: in Jsonnet it's `(import 'kubecfg.libsonnet').knit`,
and in a YAKL file, where `kubecfg` is predefined, `kubecfg.knit`. Nothing is
downloaded and no `-J` path is needed. knit is an alpha feature: it needs `kubecfg --features=knit`. Its source
and tests are in [`lib/knit`](https://github.com/kubecfg/kubecfg/tree/main/lib/knit).

## Quick start

```jsonnet
local knit = (import 'kubecfg.libsonnet').knit;

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
  An array inside such an object holds manifests only. A leaf can also be a
  patch of the object an earlier layer has at the same path (see below);
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

In an object layer, a value at the path of an earlier layer's object is a
patch of that object, as Jsonnet's `+` finds a field by its path. The patch
doesn't repeat the object's `apiVersion`, `kind` or `metadata.name`: knit
takes them from the object at that path. So each file of a chain can add one
layer, a tree shaped like the base's:

```jsonnet
// base.jsonnet
{
  layers:: [{ web: { apiVersion: 'apps/v1', kind: 'Deployment', metadata: { name: 'web' }, spec: {...} } }],
  manifests: knit.render(knit.registry, self.layers),
}
// prod.jsonnet
(import 'base.jsonnet') + {
  layers+:: [{ web: { spec: { template: { spec: { containers: [{ name: 'web', image: 'nginx:1.29' }] } } } } }],
}
```

The patch merges as any patch does: maps field by field, lists by their
Kubernetes key, and `$patch` directives as written, so `{ web: knit.delete }`
deletes `web`. A patch that sets another kind or name is an error. Only
objects outside arrays have a path. A path that no earlier layer has an
object at is read as before: its leaves must be manifests.

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
before that point is an error, since its change cannot be replayed, and so
is a mixin that changes the version itself.

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

knit keeps a few hidden fields on values: `$at`, `$born`, `$deleted`, `$list`,
`$mixin`, `$n`, `$plain`, `$pos` and `$v`. They cannot be used as keys in the input.

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
appended to with `+`, not with `$patch`. A mixin can't change an object's
kind, name, namespace or `apiVersion`: that is an error. Change them in a data
layer, or delete the object and create another.

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

A `^^` key gives an object a name, as a field of the file, the way a Jsonnet
file names its objects. The name is what an overlay changes, and what other
documents read:

```yaml
# base.yakl
<<+: ${kubecfg.knit}
---
^^:: config
replicas: 2
---
^^: api
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
^^: worker
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
              image: ${$.api.spec.template.spec.containers[@.name == 'api'].image}
              env: ${$.api.spec.template.spec.containers[@.name == 'api'].env}
```

```yaml
# prod.yakl
<<+: ${import 'yakl:base.yakl'}
---
^^+: registry
objects+:
  metadata+: {namespace: prod}
---
^^+: config
replicas: 5
---
^^+: api
spec:
  template:
    spec:
      containers:
        - name: api
          env: [{name: CACHE_TTL, value: '3600'}]
```

- **`^^+: api` changes the object named `api`,** the knit way: the container is
  found by its name, and its env gets one more entry. The patch doesn't repeat
  `apiVersion`, `kind` or `metadata.name`. A patch can set `apiVersion` to
  upgrade the object, and later patches without one keep the new version. A
  patch can also change the kind, `metadata.name` or namespace: the name is the
  object, wherever it ends up. Two names that end up as one Kubernetes object
  are an error.
- **A file can patch a name several times.** The documents of a name are a
  chain: `^^: api` first, then each `^^+: api` in file order, each merged into
  what comes before.
  As anywhere in YAKL, the base must have `api`: a misspelled `^^+: apii` is an
  error, and `^^: apii` adds an object.
- **`$.api` is the object as it ends up,** after every layer of every file.
  The worker reads the API's env, so prod's `CACHE_TTL` reaches the worker
  too. `containers[@.name == 'api']` picks the container named `api`: see
  [selectors](yakl.md#picking-a-list-entry-by-a-condition). Next to each list
  with a Kubernetes key there is also a hidden field with `_` after its name,
  keyed by that key: `containers_.api`, `env_.REDIS_URL`,
  `ports_['[8080,"TCP"]']`.
- **A value can't depend on itself.** Reading a field of `$.api` from that
  same field, or from what makes the object's identity (`apiVersion`, `kind`,
  `metadata.name` and `namespace`), never ends: Jsonnet stops with "max stack
  frames exceeded".
- **`^^: api` without `+` gives the name again,** as a Jsonnet field does: the
  object is replaced. A `^^+: api` in the same file patches that `api`, even
  when it comes first.
- **A dot groups objects:** `^^: api.deployment` and `^^: api.service` are
  `$.api.deployment` and `$.api.service`. An overlay patches one with
  `^^+: api.deployment`, and a document of only `^^+:: api` leaves out all of
  them.
- **`^^:: api` is hidden:** other documents can read it, but it isn't output.
  An overlay leaves an object out with a document of only `^^+:: api`. Other
  documents still read it, and a later `^^+: api` still applies and keeps it
  hidden, as a Jsonnet field keeps the visibility it inherits; a document of
  only `^^+::: api` outputs it again. `^^+: api` can't carry `$patch: delete`: `$.api` would become `null`,
  and a later `^^+: api` would start a new object with only its own fields.
- **A name is an object when its document is a manifest,** with `apiVersion`
  and `kind`. Other names, like `^^:: config` and `^^+: registry`, are plain
  YAKL, extended with Jsonnet's `+`.
  A document named `config` is read as `$.config` and sees the last file's
  settings. An object can't be named in one of knit's own fields, like
  `^^: registry.Service`: it would be a field of the registry, and never output,
  so knit stops with an error.

Each object is output once, by its field, so objects come out in the order
of their names, as the fields of a Jsonnet file do.

### Every object has a name

In a YAKL file, every document needs a name, as in any YAKL file of several
documents, and a document without one is an error. An object is where its name puts it. Its kind and `metadata.name` are
data: they can be expressions, and an overlay can change them, and the object
stays where its name is.

For YAML you don't write yourself, like upstream manifests, `kubecfg knit
import` (below) writes the names. The shop extends `redis.yakl`, made this
way from Redis's upstream `redis.yaml`, and its prod overlay patches Redis by
the name the import gave it:

```yaml
<<+: ${import 'yakl:redis.yakl'}
---
^^+: redis.StatefulSet
spec:
  template:
    spec:
      containers:
        - name: redis
          resources: {requests: {memory: 2Gi}}
```

There are no mixins in a YAKL file: `$.mixin` is an error. A document reads
any object as it ends up through `$`, so a value computed from other objects is
an ordinary patch:

```yaml
^^+: web.Deployment
spec:
  template:
    metadata:
      annotations:
        checksum/config: ${std.md5(std.manifestJson($.web.ConfigMap.data))}
```

A copy is an ordinary document, `^^: backup.ConfigMap` with
`<<+: ${$.web.ConfigMap}` and a new `metadata.name`, and a document of only
`^^+:: name` leaves an object out. The file is knit's own object, so a document
cannot take the names of knit's fields: `registry` (made to be extended),
`render`, `mixin` and the others. See
[`lib/knit/testdata/examples/named`](https://github.com/kubecfg/kubecfg/tree/main/lib/knit/testdata/examples/named)
and
[`lib/knit/testdata/examples/yakl`](https://github.com/kubecfg/kubecfg/tree/main/lib/knit/testdata/examples/yakl).

### A name with a hash of the data

kustomize's `configMapGenerator` names a ConfigMap with a hash of its data, `web-hf678c7m2b`, and
changes every reference to it. When the data changes, so does the name, and a Deployment that
uses the ConfigMap rolls out new pods that read the new data. On knit, an object annotated
`kustomize.config.k8s.io/needs-hash: "true"` gets such a name:

```yaml
^^: web.ConfigMap
apiVersion: v1
kind: ConfigMap
metadata:
  name: web
  annotations:
    kustomize.config.k8s.io/needs-hash: "true"
data:
  LOG_LEVEL: info
---
^^: web.Deployment
# ...
          envFrom:
            - configMapRef:
                name: ${$.web.ConfigMap.metadata.name}
```

- **The name gets a dash and kustomize's hash,** of the kind and the data (and `binaryData`; a
  Secret's `type` and `stringData`) as they end up, after every layer. The annotation isn't
  output. `render` does the same.
- **Other objects read the name through `$`,** so an overlay that changes the data changes the
  references too: `^^+: web.ConfigMap` with `data: {LOG_LEVEL: debug}` gives the ConfigMap and
  the Deployment's `envFrom` another name.
- **Layers match the object by the name as written,** `web`. A patch doesn't repeat the hash.
- **Why an annotation:** in plain Jsonnet, `metadata: { name: kubecfg.hashedName('web', config) }`
  with `local config = self` names a ConfigMap from its own data. On knit, the name is what
  matches the layers whose data gets merged, so it can't depend on that data: Jsonnet would stop
  with "max stack frames exceeded". The annotation is the one kustomize reads on what its
  generator plugins make.

See [`examples/experimental/knit/hashed-configmap`](https://github.com/kubecfg/kubecfg/tree/main/examples/experimental/knit/hashed-configmap).

### Importing a YAML file

`kubecfg --features=knit,yakl knit import app.yaml` prints a YAML file of
Kubernetes objects as a YAKL file on knit, with a name for each object, so an
overlay can change it:

```yaml
# app.yaml
apiVersion: v1
kind: ConfigMap
metadata: {name: web, namespace: a}
---
apiVersion: v1
kind: ConfigMap
metadata: {name: web, namespace: b}
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: web}
```

```yaml
# kubecfg knit import app.yaml
<<+: ${kubecfg.knit}
---
^^: a.web.ConfigMap
apiVersion: v1
kind: ConfigMap
metadata: {name: web, namespace: a}
---
^^: b.web.ConfigMap
apiVersion: v1
kind: ConfigMap
metadata: {name: web, namespace: b}
---
^^: web.Deployment
apiVersion: apps/v1
kind: Deployment
metadata: {name: web}
```

- **A name is the object's name, then its kind, as written:** `web.Deployment`.
  Objects named alike come together under one field: `$.web.Deployment`,
  `$.web.Service`, and a document of only `^^+:: web` leaves them all out.
  A dot in the object's name nests too: `foos.example.com.CustomResourceDefinition`.
- **The namespace goes in front only when it's needed,** for objects whose name
  and kind are the same: `a.web.ConfigMap`.
- **A name that would start with one of knit's fields gets a `_` in front.** A
  Service named `registry` is `_registry.Service`, because `^^: registry.Service`
  would be a field of knit's `registry`, which knit reports as an error. The same
  goes for `manifests`, `documents` and knit's other fields. Kubernetes names
  never start with `_`.
- **Two objects that would get the same name are an error,** and so is a key or
  string that YAKL reads differently from YAML: one starting with `${` is an
  expression, and a key like `c++`, `a:` or `&x` is a field with a modifier or a
  local. The error says how to write it, like `${"c++"}`.
- **A document that can't get a name is an error:** one without a kind or
  `metadata.name`, a `v1` `List` (write its items as documents of their own), one
  with `$patch`, and one that says `null`. To leave out an object of the base,
  write a document of only `^^+::` and the base's name for it. Empty documents,
  and everything else, comments included, stay as they are.
- **The objects come out in the order of their names,** not of the file.

`--extend base.yakl` makes the file an overlay of `base.yakl` instead. The first
document extends the base, `<<+: ${import 'yakl:../base/base.yakl'}`, with the
path from the YAML file, and each object is looked up in the base by identity:

- **An object the base has gets `^^+:` with the base's name,** so the
  document patches it. The base's name wins, even if it was changed after the
  base was imported: a Deployment `web` that the base calls `api.deployment`
  is patched by `^^+: api.deployment`.
- **The patch keeps only what changes the base's object,** as the base renders
  it: a field with the same value goes, and so do `apiVersion`, `kind` and
  `metadata.name`. A changed entry of a keyed list keeps its key, like
  `- {name: web, image: 'web:2'}`; an atomic list is kept whole when it
  changes at all. An object with no change is a document of only its `^^+:`
  line. What the file leaves out stays as the base has it, unless you pass
  `--delete` (see below). The patch is written
  anew, two spaces per level, with its comments; the comments above it stay as
  they are. An object with anchors, aliases or merge keys is kept whole.
- **Any other object is new,** and gets `^^:` as above. An object with an explicit
  `namespace: ''` is looked up with no namespace, not with the base's default one. A new name can't clash
  with one of the base's, as `web.Deployment` would with a base's `^^: web`.

`--delete` is for a file that has every object in full, as `kubectl get` or a
rendered Helm chart gives them, not only what changes. What the base has and the
file doesn't is taken out, so the overlay renders what the file says:

- **A field the file leaves out gets `null`,** which takes it back to what
  templates give, if any. In free-form data, where `null` is a value, it gets
  `$patch: delete`.
- **A keyed list entry the file leaves out gets deleted:**
  `- {name: metrics, $patch: delete}`. A set can't delete one value, so a set
  that loses one is written whole, with `- $patch: replace`.
- **An object of the base that the file doesn't have is left out** with a
  document of only `^^+:: name`.

[`examples/experimental/knit/import`](https://github.com/kubecfg/kubecfg/tree/main/examples/experimental/knit/import)
has a base and a prod overlay imported this way.

### Importing a kustomization

`kubecfg --features=knit,yakl knit import -k overlays/prod --out-dir knit`
turns a kustomization and its bases into YAKL files on knit. It reads the
kustomizations, not the objects `kustomize build` makes, so the files keep the
patches as patches. `knit/` gets a tree like the kustomizations', with a file
where each has a directory. The tree starts at the directory that holds all the
kustomizations it reads, and it prints the files it writes:

```
base/kustomization.yaml               knit/base.yakl
components/tracing/kustomization.yaml knit/components/tracing.yakl
overlays/prod/kustomization.yaml      knit/overlays/prod.yakl
```

```yaml
# overlays/prod/kustomization.yaml
resources: [../../base, pdb.yaml]
components: [../../components/tracing]
patches:
  - path: deployment.yaml
  - target: {kind: Service}
    patch: |-
      apiVersion: v1
      kind: Service
      metadata:
        name: any
        annotations: {cloud.example.com/internal: "true"}
```

```yaml
# knit/overlays/prod.yakl
<<+: ${(import 'yakl:../base.yakl') + import 'yakl:../components/tracing.yakl'}
---
^^: web.PodDisruptionBudget
apiVersion: policy/v1
kind: PodDisruptionBudget
...
---
^^+: web.Deployment
spec:
  replicas: 3
---
^^+: web.Service
metadata:
  annotations: {cloud.example.com/internal: "true"}
---
^^+: web-metrics.Service
metadata:
  annotations: {cloud.example.com/internal: "true"}
```

- **Resources become named objects,** as with a YAML file. A file resource is
  read as it is, comments included.
- **A kustomization with one directory resource extends that one's file,** so a
  base is written once and each overlay extends it. Importing another overlay
  of the same base writes the base's file again, the same. A kustomization with
  more directory resources has their objects and patches first, in its own file.
- **Without `--out-dir`, it prints one file** with every kustomization in it, the
  bases' objects and patches first.
- **A component is a file of its own,** which a kustomization's file adds
  after its base, as `(import base) + import component`, and before its own
  objects and patches. The component's file starts with `<<+: {}`, since it
  extends nothing on its own, or with the components it lists. Its patches are
  `^^+:` documents of the objects before it: the base's, and those of the
  components listed before it. Without `--out-dir`, a component's objects and
  patches go in the one file, after the kustomization's resources.
- **A component that patches an object of the kustomization that lists it stays
  in that kustomization's file.** kustomize adds a component after the
  kustomization's resources, but its file comes before them, so it can't patch
  them. Then all of that kustomization's components go in its file, in order,
  and `knit import` says so on stderr. So does a component listed by a directory
  resource that has its objects in another kustomization's file.
- **A strategic merge patch becomes a `^^+:` document** with the name of the
  object it patches, without `apiVersion`, `kind` and `metadata.name`. The rest
  stays as written: knit reads `$patch: delete`, `$patch: replace` and keyed
  list entries as kustomize does. Patches come from `patches` and the older
  `patchesStrategicMerge`, as files or inline.
- **A patch finds its object as kustomize finds it:** by group, kind, namespace
  and name, or by its `target`, which can select many objects (`name` is a
  regular expression, and `labelSelector` and `annotationSelector` work). Each
  object the target selects gets a document. A patch without a target that
  finds no object is an error, as in kustomize.
- **A patch with `$patch: delete` on the whole object hides it:** the
  kustomization's own object gets `^^::`, and a base's object a document of
  only `^^+::`.
- **Each patch is a document of its own,** in kustomize's order, so two
  patches of one object are two `^^+:` documents, merged in turn.
- **A `configMapGenerator` or `secretGenerator` makes a ConfigMap or Secret** after the
  resources, as kustomize does, from its `literals`, `files` and `envs`, with the
  [needs-hash annotation](#a-name-with-a-hash-of-the-data) unless `disableNameSuffixHash` is set,
  and the labels, annotations and `immutable` of its options and `generatorOptions`. A Secret has
  its `type` (`Opaque` if none) and its data in base64. A reference to it reads its name:
  `${$.web.ConfigMap.metadata.name}`. References are a `configMapRef`, a `configMapKeyRef` or a
  volume's `configMap`; a `secretRef`, a `secretKeyRef`, a volume's `secret`, or
  `imagePullSecrets`.
- **`behavior: merge` and `behavior: replace` patch the ConfigMap or Secret** with a `^^+:`
  document of its data, which `replace` starts with `$patch: replace`. The hash follows the data.
- **A JSON 6902 patch is a `^^+:` document for each operation,** with the operation as a comment
  above it. Two kinds of operation are read. `add` at the end of a list (a path ending in `/-`)
  appends: `- {$patch: append}` and the value, or, in a list with keys, the new entry. `remove`
  deletes a field with `{$patch: delete}`, or an entry of a list with keys with its key and
  `$patch: delete`: `remove /spec/template/spec/containers/1` becomes
  `- {name: debug, $patch: delete}`. An index into a list with keys, on the way to a field, is
  the entry's key too. The index is of the object as its file has it, or as its base renders it,
  after the JSON patches before; a strategic merge patch before it that adds or deletes entries
  of that list is an error, since kustomize puts new entries first.

Only `resources`, `bases`, `components`, `patches`, `patchesStrategicMerge`,
`configMapGenerator`, `secretGenerator` and `generatorOptions` are read so far. Anything else
in a kustomization, like `namespace` or `images`, is an error, and so are the other JSON 6902
operations: `replace`, `add` at an index, `move`, `copy` and `test`, and an index into a list
without keys. So is a base's reference to a ConfigMap or Secret an overlay generates: the base's
file can't read it. Two
differences from `kustomize build` remain: a new entry of a keyed list goes
after the others, where kustomize puts it first, and a `target` selects
objects by the labels their files give them, not the ones an earlier patch adds.

[`examples/experimental/knit/kustomize`](https://github.com/kubecfg/kubecfg/tree/main/examples/experimental/knit/kustomize)
has a base, a prod overlay and a component imported this way.

### How it works

In a file that extends something, YAKL writes the documents of a name as a
chain of merges: `web: merge(merge(null, d1), d2)` for `^^: web` and
`^^+: web`, with `kubecfg.merge`. That merges with Jsonnet's `+`, unless the
file has a `$merge` method, and knit has one. Merged into nothing, a manifest
gets the registry's templates; merged into an object, a patch merges the knit
way, by the object's keys. Each field is its object, so there is no stream of
layers: `$.web` is the field, after every file's documents. See
[YAKL: files that extend](yakl.md#a-file-extends-another).

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
- Finding named objects by name alone. Today a name stands for the identity of
  its object, so a `^^+:` that changes `metadata.namespace` adds a second object
  instead of moving it (`t8y:/lang/knit#name-is-identity`).

## Development

```sh
go test ./lib/knit                             # edge cases, expected errors, golden examples
go test ./lib/knit -run TestExamples -update   # rewrite testdata/examples/*/expected.yaml
lib/knit/hack/gen-schema.sh v1.37.1            # regenerate lib/knit/schema/k8s-1.37.libsonnet
```

`schema/k8s-<minor>.libsonnet` is generated from the Kubernetes OpenAPI v3
spec by `hack/gen-schema.jsonnet`, which uses the same converter that reads
CRDs at render time.
