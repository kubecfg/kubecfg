# Show

Show expanded resource definitions

It will traverse the visible elements in the `jsonnet` output and render any object that `looks` like a kubernetes resource 

* has a `kind` field
* has an `apiVersion` field

## Help

```
Show expanded resource definitions                                                                                                                                                                                 
                                                                                                         
Usage:                                                                                                                                                                                                             
  kubecfg show [flags]                                                                                                                                                                                             
                                                                                                         
Flags:                                                                                                   
  -e, --exec string                        Inline code
      --export-dir string                  Split yaml stream into multiple files and write files into a directory. If the directory exists it must be empty.
      --export-filename-extension string   Override the file extension used when creating filenames when using export-filename-format
      --export-filename-format string      Go template expression used to render path names for resources. (default "{{.apiVersion}}.{{.kind}}-{{default \"default\" .metadata.namespace}}.{{.metadata.name}}")
  -o, --format string                      Output format.  Supported values are: json, yaml (default "yaml")
  -h, --help                               help for show
      --overlay-code string                Inline Jsonnet code to compose to each of the input files
      --overlay-code-file string           Jsonnet file to compose to each of the input files
      --reorder string                     --reorder=server: Reorder resources like the 'update' command does. --reorder=client: TODO
      --show-provenance                    Add provenance annotations showing the file and the field path to each rendered k8s object
```

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
