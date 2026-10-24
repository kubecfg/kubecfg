// expect: entry has no name
local knit = import 'knit/knit.libsonnet';
local pod = { apiVersion: 'v1', kind: 'Pod', metadata: { name: 'p' }, spec: { containers: [{ name: 'c' }] } };
knit.render(knit.registry, [
  pod,
  knit.mixin({ [knit.id('Pod', 'p')]+: { spec+: { containers+: { c+: { env+: { FOO: { value: '1' } } } } } } }),
])
