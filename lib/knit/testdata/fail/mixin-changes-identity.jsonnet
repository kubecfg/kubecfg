// expect: the mixin makes ConfigMap:a the object ConfigMap:b; a mixin can't change an object's kind, name or namespace
local knit = import 'knit/knit.libsonnet';
local cm(n) = { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: n } };
knit.render(knit.registry, [cm('a'), knit.mixin({ [knit.id('ConfigMap', 'a')]+: { metadata+: { name: 'b' } } }), cm('b')])
