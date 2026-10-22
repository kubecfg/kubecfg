// expect: has no metadata.name
local knit = import 'knit/knit.libsonnet';
knit.render(knit.registry, [{ apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: null } }])
