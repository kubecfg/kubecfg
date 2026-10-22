// expect: atomic list entries cannot carry $patch
local knit = import 'knit/knit.libsonnet';
local pod(tolerations) = { apiVersion: 'v1', kind: 'Pod', metadata: { name: 'p' }, spec: { tolerations: tolerations } };
knit.render(knit.registry, [
  pod([{ key: 'a', operator: 'Exists' }]),
  pod([{ key: 'a', operator: 'Exists', '$patch': 'delete' }]),
])
