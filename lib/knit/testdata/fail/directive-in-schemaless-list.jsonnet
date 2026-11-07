// expect: unsupported directive
local knit = (import 'kubecfg.libsonnet').knit;
knit.render(knit.registry, [{
  apiVersion: 'example.com/v1',
  kind: 'Thing',
  metadata: { name: 't' },
  spec: { items: [{ name: 'a', sub: { '$retainKeys': ['x'] } }] },
}])
