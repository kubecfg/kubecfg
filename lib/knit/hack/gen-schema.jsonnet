// Builds schema/k8s-<minor>.libsonnet from the Kubernetes OpenAPI v3 spec.
// Run hack/gen-schema.sh rather than this file directly.
local knit = import '../knit.libsonnet';
local schemas = std.extVar('schemas');
local paths = std.extVar('paths');

local types = [n for n in std.objectFields(schemas) if knit.openapi.isObject(schemas[n])];
local obj(g) = { apiVersion: (if g.group == '' then '' else g.group + '/') + g.version, kind: g.kind };
local short(n) = std.reverse(std.split(n, '.'))[0];
local shorts = [short(n) for n in types];

local byGvk = {
  [knit.groupVersionKind(obj(g))]: n
  for n in types
  for g in std.get(schemas[n], 'x-kubernetes-group-version-kind', [])
};

// Resource paths (not watch, not subresources) name the real kinds, and
// whether they sit under /namespaces/{namespace}/ gives their scope.
local ops = [
  { o: obj(op['x-kubernetes-group-version-kind']), namespaced: std.member(segs, '{namespace}') }
  for p in std.objectFields(paths)
  for segs in [std.split(p, '/')]
  if !std.member(segs, 'watch') && (segs[std.length(segs) - 1] == '{name}' || !std.member(segs, '{name}'))
  for m in std.objectFields(paths[p])
  for op in [paths[p][m]]
  if std.isObject(op) && 'x-kubernetes-group-version-kind' in op
];
local namespaced = std.set([knit.groupKind(o.o) for o in ops if o.namespaced]);

{
  version: std.extVar('version'),
  kinds: { [g]: byGvk[g] for g in std.set([knit.groupVersionKind(o.o) for o in ops]) if g in byGvk },
  clusterScoped: { [gk]: true for gk in std.set([knit.groupKind(o.o) for o in ops]) if !std.member(namespaced, gk) },
  // Short names usable in registry.types when they are unique.
  aliases: { [short(n)]: n for n in types if std.count(shorts, short(n)) == 1 },
  defs: { [n]: knit.openapi.object(schemas, schemas[n]) for n in types },
}
