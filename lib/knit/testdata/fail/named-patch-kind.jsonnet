// expect: knit: !a+ changes the kind to Service
local knit = import 'knit/knit.libsonnet';
local cm = { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'a' } };
(knit { documents: [{ name: 'a', plus: false, doc: cm }, { name: 'a', plus: true, doc: { kind: 'Service' } }] })['$named']('a', function() null)
