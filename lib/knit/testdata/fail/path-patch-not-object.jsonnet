// expect: layer 1 at web: a patch of Deployment web is an object, not string
local knit = (import 'kubecfg.libsonnet').knit;
local deploy = { apiVersion: 'apps/v1', kind: 'Deployment', metadata: { name: 'web' } };
knit.render(knit.registry, [{ web: deploy }, { web: 'nginx' }])
