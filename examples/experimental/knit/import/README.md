# Importing YAML into knit

`kubecfg knit import` turns plain Kubernetes YAML into
[YAKL](../../../../docs/Advanced-Usage/yakl.md) files on
[knit](../../../../docs/Advanced-Usage/knit.md), with a name for each object. See
[Importing a YAML file](../../../../docs/Advanced-Usage/knit.md#importing-a-yaml-file).

| File | What it is |
| --- | --- |
| `base.yaml` | a web app: a ConfigMap, a Deployment and a Service |
| `prod.yaml` | production changes, as kustomize patches, and a new PodDisruptionBudget |
| `base.yakl` | **generated** from `base.yaml` |
| `prod.yakl` | **generated** from `prod.yaml`, as an overlay of `base.yakl` |

The `.yakl` files are generated. To rebuild them, run in this directory:

```
kubecfg knit import base.yaml > base.yakl
kubecfg knit import --extend base.yakl prod.yaml > prod.yakl
```

`.kubecfgrc.jsonnet` turns on the knit and yakl features, so no flags are needed. To render
them:

```
kubecfg show base.yakl
kubecfg show prod.yakl
```

## What the import does

- **`base.yakl`** starts with `<<+: ${import 'knit/knit.libsonnet'}`, and each object gets a
  name: `^^: web.Deployment`, `^^: web.Service`, `^^: web.ConfigMap`. They are together in
  `$.web`.
- **`prod.yakl`** starts with `<<+: ${import 'yakl:base.yakl'}`. The Deployment and the
  ConfigMap are in the base, so they get `^^+:` and the base's names: they patch the base's
  objects, and keep only what changes them. `apiVersion`, `kind` and `metadata` go, since
  the base has the same. The PodDisruptionBudget isn't in the base, so it gets `^^:` and is
  a new object.
- **Comments stay.** New objects keep their layout too: only the `^^` lines and the first
  document are added. Patches are written anew, two spaces per level.

From here on, the `.yakl` files are yours to edit: rename `web.Deployment` to
`web.deployment`, use `${...}` expressions, add overlays. `knit import --extend` uses the
base's names, so later imports against `base.yakl` keep working after a rename.

## Checks

`go test ./cmd -run TestKnitImportExamples` fails when the generated files are out of date;
`go test ./cmd -run TestKnitImportExamples -update` rebuilds them. `go test ./pkg/kubecfg -run
TestExamples` checks what they render.
