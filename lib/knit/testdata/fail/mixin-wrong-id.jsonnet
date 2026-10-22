// expect: the mixin's ConfigMap:b is the object ConfigMap:c
local knit = import 'knit/knit.libsonnet';
local cm(n) = { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: n } };
knit.render(knit.registry, [cm('a'), knit.mixin({ [knit.id('ConfigMap', 'b')]: cm('c') })])
