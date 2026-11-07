// expect: knit: a patch of ConfigMap a- finds no object: it has no metadata.name
local knit = (import 'kubecfg.libsonnet').knit;
(knit { a: self['$merge'](self['$merge'](null, { apiVersion: 'v1', kind: 'ConfigMap', metadata: { generateName: 'a-' } }), { data: { x: '1' } }) }).a
