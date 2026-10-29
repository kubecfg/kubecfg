# Eval

**Alpha Feature**: turn it on with `--features=eval` (see [Feature gates](../../Advanced-Usage/feature-gates.md)).

`eval` reads its inputs exactly as [`show`](show.md) does: the same files, `-e` code, overlays,
`-x` expressions and top-level arguments. The difference is what it prints. `show` looks for
Kubernetes objects in the result and prints those. `eval` prints the result as it is.

This helps when `show` fails because part of the output isn't a Kubernetes object, or when you
want to see the shape of the output. Unlike the `jsonnet` command, it knows kubecfg's native
functions and import schemes.

## Jsonnet Code

```jsonnet
local kubecfg = import 'kubecfg.libsonnet';

{
  local outer = self,

  container:: {
    name: 'busybox',
    image: 'busybox:latest',
  },

  deployment: {
    local this = self,

    apiVersion: 'extensions/v1beta1',
    kind: 'Deployment',
    metadata: {
      name: 'busybox',
      labels: { name: 'busybox' },
    },
    spec: {
      replicas: 3,
      template: {
        metadata: { labels: this.metadata.labels },
        spec: {
          containers: [outer.container],
        },
      },
    },
  },
}
```

## Basic Rendering

render jsonnet to stdout in YAML format

`kubecfg --features=eval eval source.jsonnet`

```yaml
➜ kubecfg --features=eval eval code.jsonnet
---
deployment:
  apiVersion: extensions/v1beta1
  kind: Deployment
  metadata:
    labels:
      name: busybox
    name: busybox
  spec:
    replicas: 3
    template:
      metadata:
        labels:
          name: busybox
      spec:
        containers:
        - image: busybox:latest
          name: busybox
```

render to stdout in JSON format

`kubecfg --features=eval eval -o json source.jsonnet`
```json
{
  "deployment": {
    "apiVersion": "extensions/v1beta1",
    "kind": "Deployment",
    "metadata": {
      "labels": {
        "name": "busybox"
      },
      "name": "busybox"
    },
    "spec": {
      "replicas": 3,
      "template": {
        "metadata": {
          "labels": {
            "name": "busybox"
          }
        },
        "spec": {
          "containers": [
            {
              "image": "busybox:latest",
              "name": "busybox"
            }
          ]
        }
      }
    }
  }
}
```

## Inputs

Each input prints as one YAML document, or one JSON value. As with `show`, the inputs are the files
on the command line and the code of `-e` (`--exec`), which is evaluated on its own, as with
`jsonnet -e`:

```console
$ kubecfg --features=eval eval -e '{replicas: 1 + 2}'
---
replicas: 3
```

`--overlay-code` and `--overlay-code-file` add to each input, and `-A`/`--tla-code` set its
top-level arguments, the same as in `show`. So `eval` prints what `show` looks for objects in.

## Evaluating an expression

`-x` (`--expr`) evaluates an expression instead of each whole input. In the expression, `$` is the
value of the input, even inside an object you write in the expression:

```console
$ kubecfg --features=eval eval code.jsonnet -x '$.deployment.spec.replicas'
---
3
```

`-x` works in `show` and the other commands too, for example to show only some of the objects:
`kubecfg show --features=eval code.jsonnet -x '$.deployment'`.

`-k` (`--show-keys`) prints the names of the fields of each input (or of the result of `-x`)
instead of the whole value. It doesn't evaluate the fields, so it works when one of them fails.

### Using `kubecfg` without importing it

In the expression, `kubecfg` is already defined, so you can call the functions of
`kubecfg.libsonnet` without importing it:

```console
$ kubecfg --features=eval eval code.jsonnet -x 'kubecfg.fold(kubecfg.layouts.gvkName, $, {})'
---
extensions/v1beta1.Deployment:
  busybox:
    apiVersion: extensions/v1beta1
    ...
```

It's always the copy of `kubecfg.libsonnet` built into kubecfg. A file with that name on the search
path (`-J`) or next to the evaluated file doesn't change it. A name you define yourself wins, such
as a `local kubecfg` or a top-level argument named `kubecfg`. So to use another copy, import it:
`-x 'local kubecfg = import "kubecfg.libsonnet"; ...'` finds it the usual way.

The same goes for the code of `--exec`, `--overlay-code`, `--ext-code` and `--tla-code`, and for
the expressions in [YAKL](../../Advanced-Usage/yakl.md) files. It doesn't for Jsonnet files: they
still need the import, so editors, linters and other Jsonnet tools can read them.
[RFC06](../../rfcs/rfc06.md) explains why.

## Seeing the Jsonnet of a YAKL file

`kubecfg --features=eval,yakl eval -o jsonnet app.yakl` prints the Jsonnet that a [YAKL](../../Advanced-Usage/yakl.md)
file compiles to, without evaluating it. It's part of the `yakl` feature, and takes one file without
`-e`, `-x`, `-k`, `--trace` or overlays. See [Seeing the generated Jsonnet](../../Advanced-Usage/yakl.md#seeing-the-generated-jsonnet).

## Tracing a value to its source

`--trace` prints the source lines that the value came from. It's part of the `traceback` feature,
so turn it on with `--features=eval,traceback`, and it takes one file, without `-e` or overlays.
