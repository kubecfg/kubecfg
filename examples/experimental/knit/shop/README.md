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
```

`dev.yakl` and `prod.yakl` start with `<<+: ${import 'yakl:shop.yakl'}`: each extends the
shop, so it renders on its own.

| File | What it is |
| --- | --- |
| `shop.yakl` | the shop |
| `redis.yaml` | Redis as published upstream, in plain YAML |
| `dev.yakl`, `prod.yakl` | the overlays, each extending `shop.yakl` |

## How the shop uses knit

- **Every object the shop writes has a name.** `!api` makes the API Deployment the field
  `api` of the file. An overlay changes it with `!api+`, a patch that doesn't repeat its kind
  or name, and other documents read it as `$.api`: the object as it ends up, after every
  overlay.
- **Lists merge by their Kubernetes key,** the one `kubectl apply` uses: containers and env
  by `name`, ports by `containerPort`. An overlay changes one container by writing a
  container with the same name. Lists without a key, like `volumeClaimTemplates`, are
  replaced whole.
- **`!registry+` holds templates** that apply under every document: `objects` for the root of
  every object, `types` for every value of a Kubernetes type at any depth (`PodTemplateSpec`,
  `Container`), `kinds` for one kind. A value a document sets itself wins over a template.
- **`redis.yaml` stays plain YAML.** `${std.parseYaml(importstr 'redis.yaml')}` makes it a
  layer, so the templates and overlays reach Redis too. Its objects have no names, so
  `prod.yakl` finds the StatefulSet by kind and name, as `kubectl apply` would.
- **`!config::` holds the settings.** Documents read them as `$.config`, so they see what an
  overlay changed.

## What the overlays change

| Overlay document | Reaches | Used for |
| --- | --- | --- |
| `!registry+` with `objects+:` | every namespaced object | the namespace (dev, prod) |
| `!registry+` with `types+: PodTemplateSpec+:` | every pod: Deployments, the CronJob, Redis | node selector and tolerations (prod) |
| `!registry+` with `types+: Container+:` | every container | `imagePullPolicy` and `LOG_LEVEL` (dev) |
| `!registry+` with `kinds+: Service+:` | every Service | an annotation (prod) |
| `!config+` | everything that reads the settings | image tag, replicas, domain |
| `!api+` | the API's container, by its name | resources and env (prod) |
| a StatefulSet named `redis` | Redis, by kind and name | memory and disk size (prod) |
| `!apiBudget` | a new object | the API's disruption budget (prod) |

## Things to watch

- **A misspelled name in an overlay adds an object or an item:** `!apii+` has no object to
  patch, and kubecfg stops at its fields; a container named `apii` is a new container
  without an image.
- **Atomic lists are replaced whole.** `prod.yakl` writes all of Redis's
  `volumeClaimTemplates` to change one size.
- **The worker reads the API as it ends up.** Its env is
  `${$.api.spec.template.spec.containers_.api.env}`, so the env `prod.yakl` adds to the API
  reaches the worker too. `containers_` is `containers` keyed by name.
- **Objects come out in the order of their names,** as the fields of a Jsonnet file do;
  Redis's, which have none, come out in `manifests`.
