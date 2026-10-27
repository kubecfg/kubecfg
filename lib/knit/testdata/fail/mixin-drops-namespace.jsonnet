// expect: the mixin makes ConfigMap:prod/a the object ConfigMap:a
local knit = (import 'kubecfg.libsonnet').knit;
local reg = knit.registry { objects+: { metadata+: { namespace: 'prod' } } };
local cm = { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'a' } };
knit.render(reg, [cm, knit.mixin({ [knit.id('ConfigMap', 'a', 'prod')]+: { metadata: { name: 'a' } } })])
