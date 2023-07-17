# Eval

**Alpha Feature**

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

`kubecfg --alpha eval source.jsonnet`

```yaml
➜ kubecfg --alpha eval code.jsonnet
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

`kubecfg --alpha eval -o json source.jsonnet`
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
