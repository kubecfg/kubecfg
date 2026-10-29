// expect: knit: ^^: registry.Service: registry is one of knit's fields
local knit = import 'knit/knit.libsonnet';
(knit { documents: [{ name: 'registry.Service', plus: false, doc: { apiVersion: 'v1', kind: 'Service', metadata: { name: 'registry' } } }] }).manifests
