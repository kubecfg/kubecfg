// expect: unsupported directive
local knit = (import 'kubecfg.libsonnet').knit;
knit.render(knit.registry, [{
  apiVersion: 'v1',
  kind: 'Pod',
  metadata: { name: 'p' },
  spec: { '$setElementOrder/containers': [{ name: 'c' }], containers: [{ name: 'c' }] },
}])
