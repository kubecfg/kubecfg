# Eval

**Alpha Feature**: turn it on with `--features=eval` (see [Feature gates](../../Advanced-Usage/feature-gates.md)).

`Eval` is very similar to `show` but it differs in a fundamental way, it does not traverse the code in search of kubernetes manifests, it instead behaves more like `jsonnet` command and does just evaluate the jsonnet code to output 

This is useful in order to troubleshoot some jsonnet code that , because of the use of kubecfg native extension, cannot be rendered using standard jsonnet

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

## Evaluating an expression

`-e` evaluates an expression instead of the whole file. In the expression, `$` is the value of the
file:

```console
$ kubecfg --features=eval eval code.jsonnet -e '$.deployment.spec.replicas'
3
```

### Using `kubecfg` without importing it

In the expression, `kubecfg` is already defined, so you can call the functions of
`kubecfg.libsonnet` without importing it:

```console
$ kubecfg --features=eval eval code.jsonnet -e 'kubecfg.fold(kubecfg.layouts.gvkName, $, {})'
extensions/v1beta1.Deployment:
  busybox:
    apiVersion: extensions/v1beta1
    ...
```

It's always the copy of `kubecfg.libsonnet` built into kubecfg. A file with that name on the search
path (`-J`) or next to the evaluated file doesn't change it. A name you define yourself wins, such
as a `local kubecfg` or a top-level argument named `kubecfg`. So to use another copy, import it:
`-e 'local kubecfg = import "kubecfg.libsonnet"; ...'` finds it the usual way.

The same goes for the code of `--exec`, `--overlay-code`, `--ext-code` and `--tla-code`, and for
the expressions in [YAKL](../../Advanced-Usage/yakl.md) files. It doesn't for Jsonnet files: they
still need the import, so editors, linters and other Jsonnet tools can read them.
[RFC06](../../rfcs/rfc06.md) explains why.

## Seeing the Jsonnet of a YAKL file

`kubecfg --features=eval,yakl eval -o jsonnet app.yakl` prints the Jsonnet that a [YAKL](../../Advanced-Usage/yakl.md)
file compiles to, without evaluating it. See [Seeing the generated Jsonnet](../../Advanced-Usage/yakl.md#seeing-the-generated-jsonnet).
