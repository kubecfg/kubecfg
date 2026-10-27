// expect: duplicate keys ["A"]
local knit = import 'knit/knit.libsonnet';
knit.render(knit.registry, [{
  apiVersion: 'v1',
  kind: 'Pod',
  metadata: { name: 'p' },
  spec: { containers: [{ name: 'c', env: [{ name: 'A', value: '1' }, { name: 'A', value: '2' }] }] },
}])
