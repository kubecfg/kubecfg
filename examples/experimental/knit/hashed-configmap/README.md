# A ConfigMap named with a hash of its data

kustomize's `configMapGenerator` names a ConfigMap with a hash of its data, `web-hf678c7m2b`,
and changes every reference to it. When the data changes, the name changes, so a Deployment that
uses the ConfigMap rolls out new pods that read the new data. The old ConfigMap stays until it's
deleted, so pods that haven't been replaced yet keep theirs.

In kubecfg, an object refers to the ConfigMap by its field, so it reads the name as it ends up:

| File | What it is |
| --- | --- |
| [`../../../hashed-configmap.jsonnet`](../../../hashed-configmap.jsonnet) | Jsonnet: `name: kubecfg.hashedName('web', config)`, with `local config = self`; the Deployment reads `$.config.metadata.name` |
| [`../../../hashed-configmap-overlay.jsonnet`](../../../hashed-configmap-overlay.jsonnet) | a Jsonnet overlay that changes the data, and so the name |
| `base.yakl` | YAKL on knit: the annotation `kustomize.config.k8s.io/needs-hash: "true"`; the Deployment reads `${$.web.ConfigMap.metadata.name}` |
| `prod.yakl` | a YAKL overlay that changes the data, and so the name |

```
kubecfg show base.yakl
kubecfg show prod.yakl
```

On knit, the hash comes from an annotation, not from `kubecfg.hashedName` in `metadata.name`:
knit matches an object across layers by its name, so the name can't depend on the data the
layers merge. The annotation is the one kustomize reads on what its generator plugins make.

`kubecfg knit import -k` writes a `configMapGenerator` this way: see
[`../kustomize`](../kustomize).
