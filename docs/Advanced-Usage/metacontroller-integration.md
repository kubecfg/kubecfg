# Metacontroller Integration

**Supported**: `from version v0.29.0`

**Alpha Feature**

---

[Metacontroller](https://github.com/metacontroller/metacontroller) is an add-on for Kubernetes that makes it easy to write and deploy custom controllers.

For a more detailed explanation of how Metacontroller works and the many options of controllers it support please read the [Upstream Documentation](https://metacontroller.github.io/metacontroller/)

`Kubecfg` integrates with Metacontroller by exposing an `httpd` endpoint that accept `POSTs` from the metacontroller and respond with the expected JSON object.
By leveraging `kubecfg` to produce the response you can use all the extra features provided on top of standard jsonnet.

## Architecture

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
