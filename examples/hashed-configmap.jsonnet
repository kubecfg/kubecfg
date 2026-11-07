// A ConfigMap named with a hash of its data, as kustomize's configMapGenerator names one, and a
// Deployment that refers to it as $.config.metadata.name. When the data changes, so does the
// name, and the Deployment rolls out new pods that read the new data.
// hashed-configmap-overlay.jsonnet changes the data.
//
//   kubecfg show hashed-configmap.jsonnet
local kubecfg = import 'kubecfg.libsonnet';

{
  config: {
    // The ConfigMap as it ends up, with what overlays change.
    local config = self,
    apiVersion: 'v1',
    kind: 'ConfigMap',
    metadata: {
      // web-<10 characters of a hash of the data>, as kustomize computes it.
      name: kubecfg.hashedName('web', config),
    },
    data: {
      LOG_LEVEL: 'info',
    },
  },

  deployment: {
    apiVersion: 'apps/v1',
    kind: 'Deployment',
    metadata: { name: 'web' },
    spec: {
      selector: { matchLabels: { app: 'web' } },
      template: {
        metadata: { labels: { app: 'web' } },
        spec: {
          containers: [{
            name: 'web',
            image: 'nginx:1.29',
            envFrom: [{ configMapRef: { name: $.config.metadata.name } }],
          }],
        },
      },
    },
  },
}
