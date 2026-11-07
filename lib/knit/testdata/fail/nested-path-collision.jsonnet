// expect: objects at the same path, or one inside another
local knit = (import 'kubecfg.libsonnet').knit;
local cm(n) = { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: n } };
knit.render(knit.registry, [{ config: cm('a') }, { config: cm('b') }])
