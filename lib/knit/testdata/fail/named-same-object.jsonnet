// expect: knit: a and b are the same object, ConfigMap a; give one another name, kind or namespace
local knit = (import 'kubecfg.libsonnet').knit;
local cm = { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'a' } };
(knit { a: self['$merge'](null, cm), b: self['$merge'](null, cm) }).a
