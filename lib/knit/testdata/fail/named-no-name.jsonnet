// expect: knit: ^^: a has no metadata.name
local knit = import 'knit/knit.libsonnet';
(knit { documents: [{ name: 'a', plus: false, doc: { apiVersion: 'v1', kind: 'ConfigMap', metadata: { generateName: 'a-' } } }] }).manifests
