# YAKL with knit

A fake shop (a web frontend, an API, a nightly worker and Redis) written in
[yakl](../../../../docs/Advanced-Usage/yakl.md) as plain Kubernetes YAML, merged by
[knit](../../../../docs/Advanced-Usage/knit.md), with a dev and a prod overlay. knit comes with
kubecfg: `(import 'kubecfg.libsonnet').knit`, or `import 'knit/knit.libsonnet'`. Both are
alpha features; `.kubecfgrc.jsonnet` turns them on, so no flags are needed:

```
kubecfg show shop.yakl
kubecfg show dev.yakl
kubecfg show prod.yakl
kubecfg show prod-advanced.yakl
```

`dev.yakl` and `prod.yakl` start with `<<+: ${import 'yakl:shop.yakl'}`: each extends the
shop, so it renders on its own.

| File | What it is |
| --- | --- |
| `shop.yakl` | the shop |
| `redis.yaml` | Redis as published upstream, in plain YAML |
| `redis.yakl` | `redis.yaml` with a name on each object, made by `kubecfg knit import redis.yaml` |
| `dev.yakl`, `prod.yakl` | the overlays, each extending `shop.yakl` |
| `prod-advanced.yakl` | prod with the label `app: shop` on every pod and a `securityContext` on every container, extending `prod.yakl` |

## How the shop uses knit

- **Every object the shop writes has a name.** `^^: api.deployment` makes the API Deployment
  the field `deployment` of the field `api` of the file, next to `^^: api.service`. An overlay
  changes it with `^^+: api.deployment`, a patch that doesn't repeat its kind or name, and
  other documents read it as `$.api.deployment`: the object as it ends up, after every
  overlay. A document of only `^^+:: api` would leave out all of the API's objects.
- **Lists merge by their Kubernetes key,** the one `kubectl apply` uses: containers and env
  by `name`, ports by `containerPort`. An overlay changes one container by writing a
  container with the same name. Lists without a key, like `volumeClaimTemplates`, are
  replaced whole.
- **`^^+: registry` holds templates** that apply under every document: `objects` for the root of
  every object, `types` for every value of a Kubernetes type at any depth (`PodTemplateSpec`,
  `Container`), `kinds` for one kind. A value a document sets itself wins over a template.
- **Redis gets its names from `knit import`.** knit needs a name on every object, and
  `redis.yaml` is upstream YAML without any. `kubecfg knit import redis.yaml` makes
  `redis.yakl`, which names its objects `redis.StatefulSet` and `redis.Service`, and
  `shop.yakl` extends it, so the templates and overlays reach Redis too.
  `go test ./cmd -run TestKnitImportExamples -update` rebuilds it when `redis.yaml`
  changes; without `-update`, the test fails when it's out of date.
- **`^^:: config` holds the settings.** Documents read them as `$.config`, so they see what an
  overlay changed.

## What the overlays change

| Overlay document | Reaches | Used for |
| --- | --- | --- |
| `^^+: registry` with `objects+:` | every namespaced object | the namespace (dev, prod) |
| `^^+: registry.types.PodTemplateSpec.metadata.labels` | every pod | the label `app: shop` (prod-advanced) |
| `^^: registry.types.Container.securityContext` | every container | `securityContext` (prod-advanced) |
| `^^+: registry` with `types+: PodTemplateSpec+:` | every pod: Deployments, the CronJob, Redis | node selector and tolerations (prod) |
| `^^+: registry` with `types+: Container+:` | every container | `imagePullPolicy` and `LOG_LEVEL` (dev) |
| `^^+: registry` with `kinds+: Service+:` | every Service | an annotation (prod) |
| `^^+: config` | everything that reads the settings | image tag, replicas, domain |
| `^^+: api.deployment` | the API's container, by its name | resources and env (prod) |
| `^^+: redis.StatefulSet` | Redis, by the name `knit import` gave it | memory and disk size (prod) |
| `^^: api.budget` | a new object | the API's disruption budget (prod) |

## Things to watch

- **A misspelled name in an overlay is an error:** `^^+: api.deploymnet` extends a document
  that the base doesn't have. To add one, write `^^:`. A misspelled container name isn't: a
  container named `apii` is a new container without an image.
- **Atomic lists are replaced whole.** `prod.yakl` writes all of Redis's
  `volumeClaimTemplates` to change one size.
- **The worker reads the API as it ends up.** Its env is
  `${$.api.deployment.spec.template.spec.containers[@.name == 'api'].env}`, so the env
  `prod.yakl` adds to the API reaches the worker too. `[@.name == 'api']` picks the container
  named `api`.
- **Objects come out in the order of their names,** as the fields of a Jsonnet file do.
