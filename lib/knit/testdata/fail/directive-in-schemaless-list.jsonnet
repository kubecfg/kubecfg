// expect: unsupported directive
local knit = import 'knit/knit.libsonnet';
knit.render(knit.registry, [{
  apiVersion: 'example.com/v1',
  kind: 'Thing',
  metadata: { name: 't' },
  spec: { items: [{ name: 'a', sub: { '$retainKeys': ['x'] } }] },
}])
