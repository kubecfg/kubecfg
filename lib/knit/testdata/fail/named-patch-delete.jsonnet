// expect: knit: a patch deletes ConfigMap a: to leave it out, hide it with a document of only ^^+:: and its name
local knit = (import 'kubecfg.libsonnet').knit;
local cm = { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'a' } };
(knit { a: self['$merge'](self['$merge'](null, cm), { '$patch': 'delete' }) }).a
