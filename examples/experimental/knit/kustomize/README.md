# Importing a kustomization into knit

`kubecfg knit import -k <dir>` turns a kustomization into a
[YAKL](../../../../docs/Advanced-Usage/yakl.md) file on
[knit](../../../../docs/Advanced-Usage/knit.md). See
[Importing a kustomization](../../../../docs/Advanced-Usage/knit.md#importing-a-kustomization).

| File | What it is |
| --- | --- |
| `base/kustomization.yaml` | a kustomize base: the web app of `base/web.yaml`, and its `web` ConfigMap from a `configMapGenerator` |
| `overlays/prod/kustomization.yaml` | a kustomize overlay of the base: another log level in the `web` ConfigMap, three patches, a new PodDisruptionBudget and the tracing component |
| `components/tracing/kustomization.yaml` | a kustomize component: a ConfigMap, and a patch that sends the web container's traces there |
| `knit/base.yakl` | **generated** from the base |
| `knit/components/tracing.yakl` | **generated** from the component |
| `knit/overlays/prod.yakl` | **generated** from the overlay, as an overlay of `knit/base.yakl` |

`knit/` is a tree like the kustomizations', with a file where each kustomization has a
directory. The `.yakl` files are generated. To rebuild them, run in this directory:

```
kubecfg knit import -k overlays/prod --out-dir knit
```

`.kubecfgrc.jsonnet` turns on the knit and yakl features, so no flags are needed. They
render the same objects as `kustomize build base` and `kustomize build overlays/prod`:

```
kubecfg show knit/base.yakl
kubecfg show knit/overlays/prod.yakl
```

## What the import does

- **The base's resources become named objects,** as `kubecfg knit import base/web.yaml`
  would make them: `^^: web.Deployment`, `^^: web.Service`, and so on.
- **The `configMapGenerator` makes `^^: web.ConfigMap`,** after the resources, with the data
  of its literal and of `nginx.conf`, and the annotation
  `kustomize.config.k8s.io/needs-hash: "true"`: knit names it `web-<hash of the data>`, as
  kustomize does. The Deployment's references to it read that name:
  `${$.web.ConfigMap.metadata.name}`.
- **The overlay extends the base's file.** Its one directory resource, `../../base`, is
  `knit/base.yakl`, so `knit/overlays/prod.yakl` starts with
  `<<+: ${import 'yakl:../base.yakl'}`. Its own resource, `pdb.yaml`, is a new object.
- **The overlay's `configMapGenerator` with `behavior: merge` becomes a `^^+: web.ConfigMap`
  document** with its `LOG_LEVEL`. The ConfigMap's hash changes, and the Deployment's
  references follow.
- **A patch becomes a `^^+:` document** with the name of the object it patches. It loses
  `apiVersion`, `kind` and `metadata.name`, since the name says which object it is. The
  rest stays as written, `$patch: delete` on the `debug` container included: knit reads it
  the same way.
- **A patch with a target gets one document per object the target selects.** The patch
  of `kind: Service` adds its annotation to `web.Service` and to `web-metrics.Service`.
- **The component is a file of its own.** `knit/components/tracing.yakl` has its
  ConfigMap, `^^: tracing.ConfigMap`, and its patch of `web.Deployment`. It starts with
  `<<+: {}`: on its own, it extends nothing. The overlay adds it after the base:
  `<<+: ${(import 'yakl:../base.yakl') + import 'yakl:../components/tracing.yakl'}`.
  Then come the overlay's own objects and patches, as kustomize applies them.
- **A patch with `$patch: delete` on the whole object hides it** with a document of only
  `^^+:: web-debug.ConfigMap`, so it isn't output.

Without `--out-dir`, `kubecfg knit import -k overlays/prod` prints one file with everything:
the base's objects and patches, then the overlay's and the component's. There, the component's
patch and `deployment.yaml` are two `^^+: web.Deployment` documents, merged in turn.

## Checks

`go test ./cmd -run TestKnitImportExamples` fails when the generated files are out of date;
`go test ./cmd -run TestKnitImportExamples -update` rebuilds them. `go test ./cmd -run
TestKnitImportKustomize` checks that they render what kustomize builds, and `go test
./pkg/kubecfg -run TestExamples` checks what they render.
