// expect: knit: registry.Service is a manifest in knit's field registry, so it isn't output; name it _registry.Service
local knit = (import 'kubecfg.libsonnet').knit;
(knit { registry+: { Service: $['$merge'](null, { apiVersion: 'v1', kind: 'Service', metadata: { name: 'registry' } }) } }).registry
