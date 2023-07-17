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

### Code WalkThrough

* Importing the helm chart bundle
```
local data = {
  '4.7.1': importbin './ingress-nginx-4.7.1.tgz',
  '4.6.1': importbin './ingress-nginx-4.6.1.tgz',
  };
```

Jsonnet does not support `dynamic imports` so all versions that you might want to render must be imported separately

* Configuration of the helm values and version

```
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
```

We define a `common` values in jsonnet syntax and a `per-version` overlay , we also define a `version` key in the `_config` object to use to render the specific version and overlay the right configurations

* render the Helm chart into jsonnet using kubecfg

```
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
```

Here we wrap the actual `kubecfg.parseHelmChart` into a set of helper functions to get an output that is more flexible to be further extended using jsonnet since the output from the `parseHelmChart` function is a flat list of manifests that jsonnet is not very good at managing and mangling any further.

from the inside to the outside 

* we run `kubecfg.parseHelmChart` passing 
    * the `binary import of the **data[$._config.version]**` helm chart bundle. by setting the right `version` at runtime we can render different version of the helm chart
    * the revision name, as expected by helm
    * the namespace, as expected by helm
    * the `values` to pass to helm, here we use `jsonnet` to overlay the `common` `$.values` with the `version specific` `values`
* we call the `kubecfg.fold` function passsing which will iterate over the list ( the output of `parseHelmChart` ) and for each element will call the `layout` function and add the output to the `{}` empty dict
    * the `layout` we want to use `gvkNsName` or `gvkName`
    * the output of `parseHelmChart` 
    * the `initial` `empty dict` to add elements to

the output of such a wrap in `renderHelm` key is a structure like 

```yaml
renderHelm:
  "apps.v1": # GROUP.VERSION
    "deployments": # KIND
      "ingress": # NAMESPACE
        "ingress-nginx": # NAME
  "v1": #GROUP.VERSION
    "Secrets": # KIND
      "ingress": # NAMESPACE
        "some_secret_name": # NAME
    "ConfigMaps": # KIND
      "ingress": # NAMESPACE
        "some_configmap_name": # NAME
```

This structure is now easy to further work on using `jsonnet`

```jsonnet
{
  "renderHelm": "...",

  "objects": renderHelm + {
    "apps.v1"+: {
      "deployments"+: {
        "ingress"+: {
          "ingress-nginx"+: {
            "metadata"+: { "labels"+: { "a_label": "a_value" } },
          }
        }
      }
    }

  }
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

