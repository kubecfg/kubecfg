# YAKL with k8s-libsonnet

A fake shop (a web frontend, an API, a nightly worker and Redis) written in
[yakl](../../docs/Advanced-Usage/yakl.md) on top of
[k8s-libsonnet](https://github.com/kubecfg/k8s-libsonnet), with a dev and a prod overlay.

```
kubecfg --features=yakl show shop.yakl
kubecfg --features=yakl,overlay show shop.yakl --overlay-code-file dev.yakl
kubecfg --features=yakl,overlay show shop.yakl --overlay-code-file prod.yakl
```

Their output is in `testdata/shop.yaml`, `testdata/shop-dev.yaml` and
`testdata/shop-prod.yaml`, so `diff testdata/shop.yaml testdata/shop-prod.yaml` shows what
an overlay changes.

| File | What it is |
| --- | --- |
| `k8s.libsonnet` | imports the library from GitHub, at a fixed commit |
| `shop.yakl` | the shop |
| `redis.yaml` | Redis as published upstream, in plain YAML |
| `dev.yakl`, `prod.yakl` | the overlays |
| `testdata/` | the output of the commands above, and a copy of the library and its license for the tests |

## How the shop uses the library

- **`^^:: k8s` makes the library a hidden field of the file,** `k8s`, with a label added for
  every object. Expressions read it as `$.k8s`, so they follow overlays. Each
  object starts from a type in it: `<<+: ${$.k8s.apps.v1.Deployment}`. The type sets
  `apiVersion` and `kind`, and fills in defaults.
- **Keys that the library also sets take a `+`:** `metadata+:`, `spec+:`, `template+:`. The
  `+` adds the document's fields to the library's. Without it, the document's value replaces
  the library's, and what the library adds is lost.
- **Lists of named items are mappings keyed by the name,** under a key ending in `_`:
  `containers_+: {web: {...}}`, `env_+: {API_URL: {value: ...}}`, `ports_+`. The library turns
  them into the lists Kubernetes expects, sorted by name. An overlay can then change one item
  by its name, which it can't do in a list.
- **`redis.yaml` stays plain YAML.** `$.k8s.adopt` turns its objects into library objects,
  named lists included, so the overlays reach Redis too.

## What the overlays change

| Overlay document | Reaches | Used for |
| --- | --- | --- |
| `^^+: k8s` with `ObjectMeta+:` | every object | the namespace (dev, prod) |
| `^^+: k8s` with `v1+: PodTemplateSpec+:` | every pod: Deployments, the CronJob, Redis | node selector and tolerations (prod) |
| `^^+: k8s` with `v1+: Container+:` | every container | `imagePullPolicy` and `LOG_LEVEL` (dev) |
| `^^+: k8s` with `v1+: Service+:` | every Service | an annotation (prod) |
| `^^+: config` | everything that reads the settings | image tag, replicas, domain |
| `^^+: api` with `containers_+: api+:` | one container | resources (prod) |
| `^^+: redis` | the adopted Redis | memory and disk size (prod) |
| `^^: api-pdb` | a new object | a PodDisruptionBudget (prod) |

The overlays reach the shop through names: `${$.k8s...}`, `${$.config...}` and `${$.api...}`
in `shop.yakl` read the fields, not copies, so they see what an overlay changed. The overlays
read the shop the same way, like `${$.api.spec.selector}`, since `$` is the combined file.

The library's types are the bottom layer: a `^^+: k8s` overlay gives defaults, and a value
that a document sets itself wins. dev's `LOG_LEVEL` reaches every container because none of
them sets it.

## Things to watch

- **A missing `+` is silent.** `metadata:` instead of `metadata+:` drops the namespace from
  that one object, which then goes to the default namespace. `spec:` instead of `spec+:`
  outputs `containers_` as it is, and no `containers`.
- **A `containers:` list instead of `containers_+:` works,** but overlays that change a
  container by name, or all containers, skip it without an error.
- **A misspelled name in an overlay adds an item:** `containers_+: apii+:` adds a container
  without an image. A misspelled document name fails: `^^+: apii` in `prod.yakl` gives
  "^^+: apii extends a document that the base doesn't have".
- **Lists without a name to key by,** like Redis's `volumeClaimTemplates`, take a loop to
  change, `- ${for t in super.volumeClaimTemplates}:` with `<<+: ${t}` in its body: see
  `prod.yakl`.
- **Named lists come out sorted by name,** not in file order, which matters for environment
  variables that use `$(OTHER)`. Unused ones come out empty, like `volumes: []`.

## Tests

`pkg/kubecfg/examples_test.go` checks that the commands above give the files in `testdata`.
It serves the pinned library from `testdata/k8s.libsonnet`, so it runs offline. To move to
another commit, change the URL in `k8s.libsonnet` and in the test, copy the library into
`testdata`, and run the commands above to update the outputs.
