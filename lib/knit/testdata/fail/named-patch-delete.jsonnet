// expect: knit: !a+ deletes the object: to leave it out, hide it with !a+:: {}
local knit = import 'knit/knit.libsonnet';
local cm = { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'a' } };
(knit { documents: [{ name: 'a', plus: false, doc: cm }, { name: 'a', plus: true, doc: { '$patch': 'delete' } }] }).manifests
