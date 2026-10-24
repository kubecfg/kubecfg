// expect: a mixin must be a layer of its own
local knit = import 'knit/knit.libsonnet';
local cm = { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'web' } };
knit.render(knit.registry, [[cm, knit.mixin({ [knit.id('ConfigMap', 'web')]+: { data+: { a: 'b' } } })]])
