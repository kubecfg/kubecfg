# Metacontroller Integration

**Supported**: `from version v0.29.0`

**Alpha Feature**

---

[Metacontroller](https://github.com/metacontroller/metacontroller) is an add-on for Kubernetes that makes it easy to write and deploy custom controllers.

For a more detailed explanation of how Metacontroller works and the many options of controllers it support please read the [Upstream Documentation](https://metacontroller.github.io/metacontroller/)

`Kubecfg` integrates with Metacontroller by exposing an `httpd` endpoint that accept `POSTs` from the metacontroller and respond with the expected JSON object.
By leveraging `kubecfg` to produce the response you can use all the extra features provided on top of standard jsonnet.

## Example usage

The following example will use code from [metacontroller-example](https://github.com/kubecfg/kubecfg/tree/main/examples/metacontroller)

## Overview

`kubecfg` supports exposing individual `.jsonnet` files over https as hooks by rendering the `TopLevelFunction` in those jsonnet files as expected by the `metacontroller`

the functionality is exposed by running `kubecfg --alpha httpd <jsonnet files> `

* each jsonnet file must expose a `TopLevelFunction` with a `request parameter` that will be called by kubecfg when the hook is called with a `POST`
* `kubecfg --alpha httpd sync.jsonnet` - will expose on port `:8080/sync` the `TopLevelFunction` defined in the `sync.jsonnet`
* `kubecfg --alpha httpd sync-pods.jsonnet sync-services.jsonnet` - will expose multiple endpoints `:8080/sync-pods` and `:8080/sync-jsonnet` calling each individial TLF for each files

See [sync.jsonnet](https://github.com/kubecfg/kubecfg/tree/main/examples/metacontroller/jsonnet/sync.jsonnet) in the `metacontroller` example for a working example

## Flow Diagram

``` mermaid
sequenceDiagram
  autonumber
  kubectl->>apiserver: Post CR Instance
  loop Validation
      apiserver->>apiserver: Validate CR vs CRD
  end
  loop ETCd
      apiserver->>apiserver: CR Persisted
  end
  metacontroller->>apiserver: Watch for instances of CRD
  Note right of metacontroller: CompositeController defines hooks
  metacontroller->>kubecfg-httpd: POST request to kubecfg-httpd
  Note right of kubecfg-httpd: kubecfg render json response
  kubecfg-httpd-->>metacontroller: response with manifests to create
  metacontroller-->>apiserver: Manifests to Create
```

## Create base cluster

Create a kind cluster using the provided `Makefile` in the examples directory

```shell
➜ cd examples/metacontroller
➜ make kind
...

➜ kubectl rollout status --timeout 180s -n metacontroller statefulset/metacontroller
partitioned roll out complete: 1 new pods have been updated...
```

## Kubecfg Controller

Create the example `useless-controller`

```shell
➜ make controller

➜ kubectl rollout status --timeout 180s -n metacontroller deployment/useless-controller
deployment "useless-controller" successfully rolled out
```

this Controller uses the CRD defined in `v1/crdv1.yaml` ( from `api/types.go` ) and the `compositeController` defined in the `manifests` directory

**CompositeController**

```yaml
---
apiVersion: metacontroller.k8s.io/v1alpha1
kind: CompositeController
metadata:
  name: useless-controller
spec:
  generateSelector: true
  parentResource:
    apiVersion: example.com/v1
    resource: uselesspods
    revisionHistory:
      fieldPaths:
      - spec.name
  childResources:
  - apiVersion: apps/v1
    resource: deployments
    updateStrategy:
      method: InPlace
  hooks:
    sync:
      webhook:
        url: http://useless-controller.metacontroller/sync
```

the compositeController defines how the `metacontroller` will `watch` for `example.com/v1/uselesspods` resources and post the request, with the CR specs, to the `kubecfg httpd` controller which will execute the `sync.jsonnet` code 

### sync.jsonnet hook

the code of the hook can be examined in the examples directory [sync.jsonnet](https://github.com/kubecfg/kubecfg/tree/main/examples/metacontroller/jsonnet/sync.jsonnet)

the hook **must** have a `Top Level Function` which is executed on each `POST` passing the `request body` as json 

```jsonnet

local process = function(request) {
...
...
  deployment:: { ... },

  resyncAfterSeconds: 30.0,
    status: {
        observedGeneration: std.get(request.parent.metadata, 'generation', 0),
        ready: if std.length(std.objectFields(request.children)) > 0 then 'true' else 'false',
    },
    children: [
      $.deployment,
    ],
}


//Top Level Function
function(request)
  local response = process(request);
    std.trace('request: ' + std.manifestJsonEx(request, '  ') + '\n\nresponse: ' + std.manifestJsonEx(response, '  '), response)
```


every object returned in the `children` key will be applied to Kubernetes by the metacontroller

## Example 

When an instance of the `UselessPod` is created 

1. ApiServer will validate the `CR` against the `CRD` definition
1. Persist the `CR` in etcd
1. Metacontroller watches the `UselessPods` `CRs` 
1. when an event on a watched CR is observed a `POST request` will be sent by the `metacontroller` to the `useless-controller` with the CR as part of the request
1. the `useless-controller` will run `kubecfg` on the `sync.jsonnet` code `Top Level Function` using the request as parameter 
1. the `response` to the metacontroller will include a `deployment` in the `children` field
1. the metacontroller will `apply` the `children resources` ( the deployment ) to the apiserver
1. kubernetes will create the generated deployment


```shell
➜ cat << EOF | kubectl apply -f -           
apiVersion: "example.com/v1"
kind: UselessPod
metadata:
  name: test1
  namespace: default
spec:
  name: instance1
EOF
uselesspod.example.com/test1 created
```

```shell
➜ kubectl rollout status -n default deployment/useless-instance1 
deployment "useless-instance1" successfully rolled out
```

## Cleanup

```shell
➜ make clean
```

