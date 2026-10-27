// expect: matches no object, and is not a manifest
local knit = import 'knit/knit.libsonnet';
local cm = { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'web' } };
knit.render(knit.registry, [cm, knit.mixin({ [knit.id('ConfigMap', 'nope')]+: { data+: { a: 'b' } } })])
