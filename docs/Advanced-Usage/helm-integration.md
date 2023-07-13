# Helm Integration

**Supported**: `from version v0.28.0`

---

Kubecfg supports rendering an Helm chart into Jsonnet at runtime using the `parseHelmChart` native extension.

* the Helm Chart source code *must be vendored* and imported as a binary blob using `importbin`
* the `values` passed to the Helm Chart are just a `jsonnet` object that can be created using any jsonnet feature
* the `standard` output of the `parseHelmChart` function is a flat objects with the `helm template path` as a key
  * Some functions are provided in `kubecfg.libsonnet` to convert the output to a more useful structure 
    * [kubecfg.layout.gvkName](https://github.com/kubecfg/kubecfg/blob/main/lib/kubecfg.libsonnet#L116) - Helper for 'fold'.  This accumulates a two-level collection of objects by 'apiVersion.kind' (GroupVersionKind) and then object 'name'.
    * [kubecfg.layout.gvkNsName](https://github.com/kubecfg/kubecfg/blob/main/lib/kubecfg.libsonnet#L127) - Helper for 'fold'.  This accumulates a three-level collection of objects by 'apiVersion.kind' (GroupVersionKind), object 'namespace', and then object 'name'. Namespace is '_' for non-namespaced objects.

## Vendoring helm chart version

Vendor 2 versions of the helm chart in the local directory
```
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
helm repo update
helm pull ingress-nginx/ingress-nginx --version 4.7.1
helm pull ingress-nginx/ingress-nginx --version 4.6.1
```

```
➜ ls -1
ingress-nginx-4.6.1.tgz
ingress-nginx-4.7.1.tgz
```

## Jsonnet Code

```jsonnet
local kubecfg = import 'kubecfg.libsonnet';

// To support multiple versions they need to be indepently vendored and imported 
// the selection will be done by passing the right key into `parseHelmChart`
local data = {
  '4.7.1': importbin './ingress-nginx-4.7.1.tgz',
  '4.6.1': importbin './ingress-nginx-4.6.1.tgz',
};

{
  _config:: {
    version: '4.6.1',
  },

  values:: {
    nameOverride: 'nginx-example',
    fullNameOverride: 'nginx-example',
    commonLabels: { app: 'nginx-example' },
    controller: {
      minReadySeconds: 10,
    },
  },

  valuesByVersion:: {
    '4.7.1': {
      commonLabels+: { extra: 'label' },
    },
    '4.6.1': {},
  },

  renderHelm::
    kubecfg.fold(
      kubecfg.layouts.gvkNsName,  // Render list output of ParseHelmChart into a hierarchy layout
      kubecfg.parseHelmChart(
        data[$._config.version],
        'nginx-example',
        'ingress',
        $.values + $.valuesByVersion[$._config.version]
      ),
      {}
    ),

  // Extend the Helm rendering by using Jsonnet
  // Access the resources based on the GVK NS NAME hirerarchy

  objects: $.renderHelm {
    'apps/v1.Deployment'+: {  //  apiversion.kind
      ingress+: {  // resource namespace
        'nginx-example-controller'+: {  // resource name
          spec+: {
            replicas: 20,
          },
        },
      },
    },
  },
}
```

## Render and Export manifests

`kubecfg show --export-dir output/ show.jsonnet`

```
➜ ls -1 output 
admissionregistration.k8s.io-v1.ValidatingWebhookConfiguration-ingress.nginx-example-admission.yaml
apps-v1.Deployment-ingress.nginx-example-controller.yaml
batch-v1.Job-ingress.nginx-example-admission-create.yaml
batch-v1.Job-ingress.nginx-example-admission-patch.yaml
networking.k8s.io-v1.IngressClass-ingress.nginx.yaml
rbac.authorization.k8s.io-v1.ClusterRole-ingress.nginx-example-admission.yaml
rbac.authorization.k8s.io-v1.ClusterRole-ingress.nginx-example.yaml
rbac.authorization.k8s.io-v1.ClusterRoleBinding-ingress.nginx-example-admission.yaml
rbac.authorization.k8s.io-v1.ClusterRoleBinding-ingress.nginx-example.yaml
rbac.authorization.k8s.io-v1.Role-ingress.nginx-example-admission.yaml
rbac.authorization.k8s.io-v1.Role-ingress.nginx-example.yaml
rbac.authorization.k8s.io-v1.RoleBinding-ingress.nginx-example-admission.yaml
rbac.authorization.k8s.io-v1.RoleBinding-ingress.nginx-example.yaml
v1.ConfigMap-ingress.nginx-example-controller.yaml
v1.Service-ingress.nginx-example-controller-admission.yaml
v1.Service-ingress.nginx-example-controller.yaml
v1.ServiceAccount-ingress.nginx-example-admission.yaml
v1.ServiceAccount-ingress.nginx-example.yaml
```

