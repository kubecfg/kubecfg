# Show

Show expanded resource definitions

It will traverse the visible elements in the `jsonnet` output and render any object that `looks` like a kubernetes resource 

* has a `kind` field
* has an `apiVersion` field

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
      replicas: 1,
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

`kubecfg show source.jsonnet`

```yaml
---
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  labels:
    name: busybox
  name: busybox
spec:
  replicas: 1
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

`kubecfg show -o json source.jsonnet`
```json
{
  "apiVersion": "extensions/v1beta1",
  "kind": "Deployment",
  "metadata": {
    "labels": {
      "name": "busybox"
    },
    "name": "busybox"
  },
  "spec": {
    "replicas": 1,
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
```

## Export Manifests to files

It is possible to export the manifests as a set of split files ( 1 per resource ) to a directory.
This can be usefl when using other tools , like Flux or ArgoCD, to actually deploy manifests to a cluster

`kubecfg show --export-dir output/ --export-filename-format "{{.apiVersion}}.{{.kind}}-{{default \"default\" .metadata.namespace}}.{{.metadata.name}}" show.jsonnet`

```yaml
➜ ls -1 output 
extensions-v1beta1.Deployment-default.busybox.yaml

➜ cat output/extensions-v1beta1.Deployment-default.busybox.yaml 
---
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  labels:
    name: busybox
  name: busybox
spec:
  replicas: 1
  template:
    metadata:
      labels:
        name: busybox
    spec:
      containers:
      - image: busybox:latest
        name: busybox
```

## Inline usage 

It is possible to exec inline code with kubecfg, this is useful for debugging purposes

`kubecfg show -e "(import 'show.jsonnet') + { deployment+: { metadata+: {namespace: 'foo'}}}"`

```yaml
---
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  labels:
    name: busybox
  name: busybox
  namespace: foo
spec:
  replicas: 1
  template:
    metadata:
      labels:
        name: busybox
    spec:
      containers:
      - image: busybox:latest
        name: busybox
```
