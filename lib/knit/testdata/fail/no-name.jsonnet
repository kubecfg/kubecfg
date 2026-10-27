// expect: has no metadata.name
local knit = (import 'kubecfg.libsonnet').knit;
knit.render(knit.registry, [{ apiVersion: 'v1', kind: 'ConfigMap', metadata: {} }])
