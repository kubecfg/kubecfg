// expect: an array in a nested layer can only hold manifests
local knit = import 'knit/knit.libsonnet';
local cm(n) = { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: n } };
knit.render(knit.registry, [{ apps: [{ a: cm('a') }, { b: cm('b') }] }])
