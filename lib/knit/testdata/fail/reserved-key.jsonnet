// expect: ["$list"] are reserved for knit
local knit = (import 'kubecfg.libsonnet').knit;
knit.render(knit.registry, [{
  apiVersion: 'v1',
  kind: 'Pod',
  metadata: { name: 'p' },
  spec: { containers: [{ name: 'c', env: [{ name: '$list', value: '1' }] }] },
}])
