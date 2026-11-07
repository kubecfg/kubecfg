// expect: layer 1 at wbe.spec.replicas: expected a manifest, or a patch at the path of an earlier layer's object
local knit = (import 'kubecfg.libsonnet').knit;
local deploy = { apiVersion: 'apps/v1', kind: 'Deployment', metadata: { name: 'web' } };
knit.render(knit.registry, [{ web: deploy }, { wbe: { spec: { replicas: 2 } } }])
