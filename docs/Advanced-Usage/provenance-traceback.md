# Provenance and Tracking

When manifests are generated from a big , nested, codebase it can be hard sometimes to identify where in jsonnet a specific manifest was generated or where a specific field was defined. 

`kubecfg` supports 2 functionatilities to help with this 

## Provenance 

when rendering kubernetes manifests `kubecfg` can add a custom annotations to help identify which file and which key in the file was used to generate the manifest


**`kubecfg show code.jsonnet --show-provenance`**
```jsonnet
➜ cat code.jsonnet
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

```yaml
➜ kubecfg show code.jsonnet --show-provenance              
---
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  annotations:
    kubecfg.github.com/provenance-file: code.jsonnet
    kubecfg.github.com/provenance-path: $.deployment
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

the 2 `kubecfg.github.com/provenance-` annotations will automatically be added to help identify where a manifest came from in the codebase

## Traceback

**Alpha Feature** 

when the annotations are added to the rendered manifests is possible to traceback the line in the jsonnet code that did add a specific field 

for example, to find which line in jsonnet added the `replicas: 1` (which is line 12 in the manifest ) we can

`kubecfg --alpha traceback RENDERED.yaml:LINE_NUMBER`

```
➜ kubecfg --alpha traceback output.yaml:12   
INFO  Tracing file="code.jsonnet", path="$.deployment.spec.replicas"
/dev/shm/kubecfg/example/code.jsonnet:21 
```

and we will know it was `line 21` of the `code.jsonnet` that added the replicas field

```
➜ sed -n '21p' code.jsonnet
      replicas: 1,
```

If the replicas value gets updated by setting the replicas to 3 , the traceback will tell us the correct line

```
➜ tail code.jsonnet 
    },
  },
} + {

  deployment+: {
    spec+: {
      replicas: 3,
    },
  },
}


➜ kubecfg show code.jsonnet --show-provenance > output.yaml

➜ kubecfg --alpha traceback output.yaml:12
INFO  Tracing file="code.jsonnet", path="$.deployment.spec.replicas"
/dev/shm/kubecfg/example/code.jsonnet:34 

➜ sed -n '34p' code.jsonnet                             
      replicas: 3,
```
