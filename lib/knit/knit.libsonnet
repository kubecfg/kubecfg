// knit: kind-aware overlays for plain Kubernetes manifests.
//
// render(registry, layers) turns every list that has a known key into an
// object keyed by that key, stacks `templates + layer0 + layer1 + ...` with
// plain Jsonnet `+`, and flattens the result back into ordinary manifests.
// See docs/Advanced-Usage/knit.md for the user-facing model.

local kubecfg = import '../kubecfg.libsonnet';
local objectMeta = 'io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMeta';
local deleted = { '$deleted':: true };
local unsupported = ['$setElementOrder/', '$deleteFromPrimitiveList/', '$retainKeys'];
// Hidden fields knit keeps on values; user keys cannot take these names.
local reserved = std.set(['$at', '$deleted', '$list', '$mixin', '$n', '$pos', '$v']);

// Template levels, bottom to top. Data layers use 0, 1, 2, ... so template
// entries sort before entries from layers.
local L = { types: -5, objects: -4, groups: -3, groupKind: -2, groupVersionKind: -1 };

local str(v) = if std.isString(v) then v else std.manifestJsonMinified(v);
local isPatch(x, d) = std.isObject(x) && std.get(x, '$patch') == d;
local isDeleted(x) = std.isObject(x) && std.get(x, '$deleted', false);
local isKeyed(x) = std.isObject(x) && '$list' in x;
local isMergeable(x) = std.isObject(x) && !isDeleted(x) && !('$list' in x);
local isMarker(e) = std.isObject(e) && std.objectFields(e) == ['$patch'];
local isMixin(l) = std.isObject(l) && '$mixin' in l;
local isManifest(x) = std.isObject(x) && 'apiVersion' in x && 'kind' in x;
local isCrd(o) = std.startsWith(o.apiVersion, 'apiextensions.k8s.io/') && o.kind == 'CustomResourceDefinition';
// Whether o has a value at k that was not deleted, and that value or null.
local has(o, k) = std.isObject(o) && k in o && !isDeleted(o[k]);
local live(o, k) = if has(o, k) then o[k] else null;

local gkOf(group, kind) = (if group == '' then '' else group + '/') + kind;
local group(o) = local p = std.split(o.apiVersion, '/'); if std.length(p) == 1 then '' else p[0];
local groupKind(o) = gkOf(group(o), o.kind);
local groupVersionKind(o) = o.apiVersion + '/' + o.kind;

// Registry keys: 'apps/v1/Deployment', 'apps/Deployment', 'v1/ConfigMap' or
// 'ConfigMap'.
local isVersion(s) = std.length(s) > 1 && s[0] == 'v' && s[1] >= '0' && s[1] <= '9' && std.findSubstr('.', s) == [];
local parseKind(k) =
  local p = std.split(k, '/'), n = std.length(p);
  if n == 1 then { group: '', version: null, kind: p[0] }
  else if n == 2 && isVersion(p[0]) then { group: '', version: p[0], kind: p[1] }
  else if n == 2 then { group: p[0], version: null, kind: p[1] }
  else if n == 3 then { group: p[0], version: p[1], kind: p[2] }
  else null;

//
// OpenAPI (v3 components, or a CRD's structural schema) -> merge schema.
//
// A field schema is one of:
//   'io.k8s.api.core.v1.Container'          a named object type, body in defs
//   { fields: {...}, map: <field schema> }  an inline object
//   { list: 'map', keys, defaults, item }   a keyed list
//   { list: 'set' }                         a set of primitives
//   { list: 'atomic', item }                a list replaced whole
// An object can also be `free` (x-kubernetes-preserve-unknown-fields): its
// fields without a schema are free too. A field with no schema is a plain
// value: objects merge, lists are atomic.
//

local refName(s) =
  local r =
    if '$ref' in s then s['$ref']
    else if std.length(std.get(s, 'allOf', [])) == 1 && '$ref' in s.allOf[0] then s.allOf[0]['$ref']
    else null;
  if r == null then null else std.reverse(std.split(r, '/'))[0];
local isObjectSchema(s) = std.isObject(s) && (std.get(s, 'type') == 'object' || 'properties' in s);
local isFreeSchema(s) = std.get(s, 'x-kubernetes-preserve-unknown-fields', false) == true;
local deref(schemas, s) = local n = refName(s); if n == null then s else schemas[n];

local openapi = {
  field(schemas, s)::
    if !std.isObject(s) then null
    else
      local n = refName(s);
      if n != null then (if isObjectSchema(schemas[n]) then n else null)
      else if std.get(s, 'type') == 'array' then openapi.list(schemas, s)
      else if isObjectSchema(s) || std.isObject(std.get(s, 'additionalProperties')) || isFreeSchema(s) then
        local o = openapi.object(schemas, s);
        if o == {} then null else o
      else null,

  object(schemas, s)::
    local props = std.get(s, 'properties', {});
    // In a free-form object, declared fields and additionalProperties with
    // no merge schema stay listed (as {}), so they are not taken for free.
    local fields =
      {
        [k]: if f == null then {} else f
        for k in std.objectFields(props)
        for f in [openapi.field(schemas, props[k])]
        if f != null || isFreeSchema(s)
      }
      + (if std.get(s, 'x-kubernetes-embedded-resource', false) then { metadata: objectMeta } else {});
    local ap = std.get(s, 'additionalProperties');
    local mf = if std.isObject(ap) then openapi.field(schemas, ap) else null;
    local m = if mf == null && std.isObject(ap) && isFreeSchema(s) then {} else mf;
    (if fields == {} then {} else { fields: fields })
    + (if m == null then {} else { map: m })
    + (if isFreeSchema(s) then { free: true } else {}),

  // An explicit x-kubernetes-list-type wins; the strategic merge patch key
  // only counts for lists without one (as server-side apply does).
  list(schemas, s)::
    local lt = std.get(s, 'x-kubernetes-list-type');
    local merge = std.member(std.split(std.get(s, 'x-kubernetes-patch-strategy', ''), ','), 'merge');
    local pmk = std.get(s, 'x-kubernetes-patch-merge-key');
    local items = std.get(s, 'items', {});
    local itemDef = deref(schemas, items);
    local keys =
      if lt == 'map' then s['x-kubernetes-list-map-keys']
      else if lt == null && merge && pmk != null then [pmk]
      else null;
    if keys != null then
      local props = std.get(itemDef, 'properties', {});
      // The spec gives required fields their Go zero value as a default
      // (name: ""); only a real default (protocol: TCP) helps match entries.
      local defaults = {
        [k]: props[k].default
        for k in keys
        if k in props && 'default' in props[k] && !std.member(['', 0, false], props[k].default)
      };
      local item = openapi.field(schemas, items);
      { list: 'map', keys: keys }
      + (if defaults == {} then {} else { defaults: defaults })
      + (if item == null then {} else { item: item })
    else if lt == 'set' || (lt == null && merge && !isObjectSchema(itemDef)) then { list: 'set' }
    else
      // Atomic lists keep their item type, so type templates reach inside.
      local item = openapi.field(schemas, items);
      if item == null then null else { list: 'atomic', item: item },
};

// Every object has ObjectMeta metadata, whatever its schema says.
local rootSchema(s) = (if s == null then {} else s) + { fields+: { metadata: objectMeta } };

//
// Normalize: plain data -> overlay form.
//
// Objects merge into what lower layers set (`super`); keyed lists become
// objects tagged `$list` whose entries carry `$pos` (first position seen);
// deletions become `$deleted` markers that render skips. A field that no
// lower layer set gets its type template underneath; normalized objects
// carry `$n`. `t` is what templates
// alone give the value being normalized: what a null or `$patch: replace`
// goes back to.
//

local resolve(ctx, fs) = if std.isString(fs) then ctx.defs[fs] else fs;
local fieldOf(s, k) =
  if s == null then null
  else std.get(std.get(s, 'fields', {}), k, std.get(s, 'map', if std.get(s, 'free', false) then { free: true } else null));
local isFreeField(s, k) = s != null && std.get(s, 'free', false) && !(k in std.get(s, 'fields', {})) && !('map' in s);
local isKeyedSchema(ls) = std.isObject(ls) && std.member(['map', 'set'], std.get(ls, 'list'));
local atomicItem(ls) = if !std.isObject(ls) then null else if std.get(ls, 'free', false) then ls else std.get(ls, 'item');
local tmpl(ctx, fs) = if std.isString(fs) && !std.member(ctx.stack, fs) then std.get(ctx.tt, fs, {}) else {};

local listMode(xs, path) =
  local ms = std.set([e['$patch'] for e in xs if isMarker(e)]);
  assert std.length(ms) <= 1 && std.all([std.member(['replace', 'append'], m) for m in ms])
         : 'knit: %s: unknown list directive %s' % [path, ms];
  if ms == [] then 'merge' else ms[0];

// A missing (null, deleted) key field takes the value the entry gets from below:
// its type template's (`below`), else the schema's default (protocol: TCP), so
// `{containerPort: 80}` and `{containerPort: 80, protocol: TCP}` match.
local keyOf(ls, e, path, below) =
  if ls.list == 'set' then str(e)
  else
    assert std.isObject(e) : 'knit: %s: expected an object entry, got %s' % [path, std.type(e)];
    local vs = [
      local v = std.get(e, k);
      if v != null && !isDeleted(v) && !isPatch(v, 'delete') then v
      else std.get(below, k, std.get(std.get(ls, 'defaults', {}), k))
      for k in ls.keys
    ];
    assert std.any([v != null for v in vs]) : 'knit: %s: entry has no %s' % [path, std.join(', ', ls.keys)];
    if std.length(vs) == 1 then str(vs[0]) else std.manifestJsonMinified(vs);

// Entries keyed by ks, laid over what lower layers set: entry i is
// f(i, prev), prev being the live entry under the same key, or null.
local overlay(ks, f, dups) =
  assert std.length(std.set(ks)) == std.length(ks) : dups(std.set([k for k in ks if std.count(ks, k) > 1]));
  {
    [ks[i]]: f(i, if ks[i] in super && !isDeleted(super[ks[i]]) then super[ks[i]] else null)
    for i in std.range(0, std.length(ks) - 1)
  };

local norm = {
  ov(ctx, v, s, layer, path, t)::
    assert std.isObject(v) : 'knit: %s: expected an object, got %s' % [path, std.type(v)];
    // Callers act on `$patch: delete`; any other value would be dropped here.
    assert std.member([null, 'replace'], std.get(v, '$patch'))
           : 'knit: %s: $patch: %s is not supported here' % [path, v['$patch']];
    local taken = std.setInter(std.objectFields(v), reserved);
    assert taken == [] : 'knit: %s: %s are reserved for knit' % [path, taken];
    // `$n` marks a normalized object, which has its type templates.
    { '$n':: true } + {
      [k]:
        local x = v[k], fs = fieldOf(s, k), p = path + '.' + k, tk = live(t, k);
        assert [d for d in unsupported if std.startsWith(k, d)] == [] : 'knit: %s: unsupported directive' % p;
        if isPatch(x, 'delete') || isDeleted(x) then deleted
        else if x == null then
          // null takes back what lower layers set, down to what templates
          // give; with nothing below, or in free-form data, it is a value.
          if !(k in super) || isDeleted(super[k]) || isFreeField(s, k) then null
          else if has(t, k) then t[k]
          else deleted
        else if std.isObject(x) && !isKeyedSchema(resolve(ctx, fs)) then
          local base = if isMergeable(tk) then tk else tmpl(ctx, fs);
          local o = norm.ov(ctx, x, resolve(ctx, fs), layer, p, base);
          if !isPatch(x, 'replace') && k in super && isMergeable(super[k]) then super[k] + o
          else base + o
        else if std.isObject(x) then
          // A keyed list in the keyed form, as an object a mixin creates has.
          local ls = resolve(ctx, fs);
          local ks = std.sort(
            [e for e in std.objectFields(x) if e != '$patch'],
            function(e) if std.isObject(x[e]) then std.get(x[e], '$pos', [1e9, 0]) else [1e9, 0],
          );
          local kl = norm.keyed(ctx, [x[e] for e in ks], ls, layer, p, if isKeyed(tk) then tk else null, ks);
          if !isPatch(x, 'replace') && k in super && isKeyed(super[k]) then super[k] + kl else kl
        else if std.isArray(x) then
          local ls = resolve(ctx, fs), mode = listMode(x, p), xs = [e for e in x if !isMarker(e)];
          if isKeyedSchema(ls) then
            local kl = norm.keyed(ctx, xs, ls, layer, p, if isKeyed(tk) then tk else null);
            if mode != 'replace' && k in super && isKeyed(super[k]) then super[k] + kl else kl
          else
            // Atomic: this layer's list replaces the inherited one, but each
            // element still gets its type templates.
            assert [e for e in xs if std.isObject(e) && '$patch' in e] == []
                   : 'knit: %s: atomic list entries cannot carry $patch; the list is replaced whole, or appended to with a `$patch: append` marker' % p;
            local item = atomicItem(ls);
            local ys = [
              if std.isObject(xs[i]) then tmpl(ctx, item) + norm.ov(ctx, xs[i], resolve(ctx, item), layer, p + '[%d]' % i, tmpl(ctx, item))
              else xs[i]
              for i in std.range(0, std.length(xs) - 1)
            ];
            if mode == 'append' && k in super && std.isArray(super[k]) then super[k] + ys else ys
        else x
      for k in std.objectFields(v)
      if k != '$patch'
    },

  // xs are entries, or with `given` keys, the entries of a list in the keyed
  // form: set entries hold their value in `$v`, and deleted ones are markers.
  keyed(ctx, xs, ls, layer, path, t, given=null)::
    local isSet = ls.list == 'set', item = std.get(ls, 'item');
    local ks = if given != null then given else [keyOf(ls, e, path, tmpl(ctx, item)) for e in xs];
    local taken = std.setInter(std.set(ks), reserved);
    assert taken == [] : 'knit: %s: %s are reserved for knit' % [path, taken];
    { '$list':: ls.list } + overlay(ks, function(i, prev)
      local e = xs[i], k = ks[i], p = path + '[' + k + ']';
      if isDeleted(e) || !isSet && isPatch(e, 'delete') then deleted
      else if isSet then
        (if prev != null then prev else {})
        + { '$v':: if given == null then e else if std.isObject(e) && '$v' in e then e['$v'] else k }
        + { '$pos':: if prev == null then [layer, i] else std.get(prev, '$pos', [layer, i]) }
      else
        local own = keyOf(ls, e, p, tmpl(ctx, item));
        assert given == null || own == k : "knit: %s: the entry's own key is %s" % [p, own];
        // `$patch: replace` drops what lower layers set but keeps the place.
        // An entry deleted below comes back fresh, at its new position.
        local te = if live(t, k) != null then t[k] else tmpl(ctx, item);
        (if prev != null && !isPatch(e, 'replace') then prev else te)
        + norm.ov(ctx, e, resolve(ctx, item), layer, p, te)
        + { '$pos':: if prev == null then [layer, i] else std.get(prev, '$pos', [layer, i]) },
      function(dups) 'knit: %s: duplicate keys %s' % [path, dups]),
};

//
// Render: keyed objects -> arrays, in order, without deleted entries.
//

local liveKeys(v) = std.sort([k for k in std.objectFields(v) if !isDeleted(v[k])], function(k) std.get(v[k], '$pos', [1e9, 0]));

local value(v) =
  if std.isObject(v) then
    if '$list' in v then [if v['$list'] == 'set' then v[k]['$v'] else value(v[k]) for k in liveKeys(v)]
    else { [k]: value(v[k]) for k in std.objectFields(v) if !isDeleted(v[k]) }
  else if std.isArray(v) then [value(e) for e in v]
  else v;

// value, with each keyed list also as a hidden object keyed by its key,
// next to it: containers_.web, env_.LOG_LEVEL, ports_['[80,"TCP"]'].
local view(v) =
  if std.isObject(v) then
    if '$list' in v then [if v['$list'] == 'set' then v[k]['$v'] else view(v[k]) for k in liveKeys(v)]
    else
      local fs = [k for k in std.objectFields(v) if !isDeleted(v[k])];
      std.foldl(
        function(acc, k) acc + { [k + '_']:: { [e]: view(v[k][e]) for e in liveKeys(v[k]) } },
        [k for k in fs if isKeyed(v[k]) && v[k]['$list'] == 'map'],
        { [k]: view(v[k]) for k in fs },
      )
  else if std.isArray(v) then [view(e) for e in v]
  else v;

local groupBy = kubecfg.groupBy;

//
// Layers, identity and composition.
//

// A layer is a manifest, a v1 List, an array, or an object whose leaves are
// manifests. `at` records where each manifest sat, to keep the output shape.
local flatten(layer, x, path, arr) =
  local where = 'layer %d%s' % [layer, if path == [] then '' else ' at ' + std.join('.', path)];
  if x == null then []
  else if isMixin(x) then error 'knit: %s: a mixin must be a layer of its own' % where
  else if std.isArray(x) then
    std.flattenArrays([
      // Positions in an array are not kept, so in a nested layer an array
      // holds manifests only.
      assert path == [] || e == null || isManifest(e) || isMixin(e)
             : 'knit: %s: an array in a nested layer can only hold manifests' % where;
      flatten(layer, e, path, true)
      for e in x
    ])
  else if isManifest(x) then
    if x.kind == 'List' && 'items' in x then flatten(layer, x.items, path, true)
    else [{ obj: x, at: { path: path, arr: arr } }]
  else if std.isObject(x) then std.flattenArrays([flatten(layer, x[k], path + [k], false) for k in std.objectFields(x)])
  else error 'knit: %s: expected a manifest, got %s' % [where, std.type(x)];

// Migrations see every object a layer holds, partial ones and deletions
// included, without its `$patch`.
local migrate(reg, o, seen=[]) =
  local k = groupVersionKind(o), d = std.get(o, '$patch');
  if !(k in reg.migrations) then o
  else if d != null then migrate(reg, std.objectRemoveKey(o, '$patch'), seen) + { '$patch': d }
  else
    assert !std.member(seen, k) : 'knit: migrations loop: %s' % std.join(' -> ', seen + [k]);
    local m = reg.migrations[k](o);
    assert isManifest(m) && groupVersionKind(m) != k
           : 'knit: migration for %s must return an object with another apiVersion or kind' % k;
    migrate(reg, m, seen + [k]);

local id(gk, name, namespace) = gk + ':' + (if namespace == null || namespace == '' then '' else namespace + '/') + name;

local kindSchema(ctx, o) =
  local g = groupVersionKind(o), sk = ctx.reg.schema.kinds;
  // The registry's schema wins over in-stream CRDs, so it can override them.
  // CRDs themselves never take their schema from CRDs.
  local fs = if g in sk then sk[g] else if isCrd(o) then null else std.get(ctx.crd.kinds, g);
  if std.isString(fs) then fs else rootSchema(fs);

// ponytail: kinds with no bundled or in-stream schema count as namespaced;
// add a registry scope table or cluster discovery if that bites.
local isClusterScoped(ctx, o) =
  local gk = groupKind(o), sc = ctx.reg.schema.clusterScoped;
  if gk in sc then sc[gk] == true else isCrd(o) || std.get(ctx.crd.clusterScoped, gk, false);

// What the registry gives every object of o's version, under any layer.
local rootTemplate(ctx, o) =
  local fs = kindSchema(ctx, o), s = resolve(ctx, fs), reg = ctx.reg, gk = groupKind(o), gvk = groupVersionKind(o);
  local objects =
    if isClusterScoped(ctx, o) then std.mergePatch(reg.objects, { metadata: { namespace: null } })
    else reg.objects;
  // Each level lays over the ones below; `$patch: replace` at its root
  // drops them.
  std.foldl(
    function(acc, l) (if isPatch(l.v, 'replace') then tmpl(ctx, fs) else acc) + norm.ov(ctx, l.v, s, l.layer, l.path, null),
    [
      { v: objects, layer: L.objects, path: 'objects' },
      { v: std.get(reg.groups, group(o), {}), layer: L.groups, path: 'groups.' + group(o) },
      { v: std.get(reg.kinds, gk, {}), layer: L.groupKind, path: 'kinds.' + gk },
      { v: std.get(reg.kinds, gvk, {}), layer: L.groupVersionKind, path: 'kinds.' + gvk },
    ],
    tmpl(ctx, fs),
  );
local root(ctx, o) = local g = groupVersionKind(o); if g in ctx.roots then ctx.roots[g] else rootTemplate(ctx, o);

// Objects match across layers by group, kind, namespace and name. The
// namespace is the one the object ends up with: its own (even ''), none if
// it deletes it, else the one templates give it, and none for
// cluster-scoped kinds. An object with only
// generateName gets an identity no other layer can match.
local idOf(ctx, o, layer, i) =
  local md = if std.isObject(std.get(o, 'metadata')) then o.metadata else {};
  local name = std.get(md, 'name'), gen = std.get(md, 'generateName'), own = std.get(md, 'namespace');
  local ns =
    if isClusterScoped(ctx, o) then null
    else if std.isString(own) then own
    else if isPatch(own, 'delete') || isDeleted(own) then null
    else
      local tm = live(root(ctx, o), 'metadata');
      local tns = std.get(if isMergeable(tm) then tm else tmpl(ctx, objectMeta), 'namespace');
      if std.isString(tns) then tns else null;
  if std.isString(name) && name != '' then id(groupKind(o), name, ns)
  else if std.isString(gen) && gen != '' then '%s:%s~%d.%d' % [groupKind(o), gen, layer, i]
  else error 'knit: %s object has no metadata.name' % groupVersionKind(o);

// What one data layer adds to one object.
local contribution(ctx, o, layer, k) = norm.ov(ctx, o, resolve(ctx, kindSchema(ctx, o)), layer, k, root(ctx, o));

// Templates follow the version an object has at each point of the stack.
// When a layer changes the apiVersion, the object is rebuilt from the data
// layers since it was last added or replaced, read as the new version, so
// only that version's schema and templates apply. Mixin changes cannot be
// replayed, so they are rejected.
local rebase(ctx, k, o, layer, history) =
  local seen = [e for e in std.get(history, k, []) if e.layer < layer];
  local resets = [i for i in std.range(0, std.length(seen) - 1) if isPatch(seen[i].obj, 'delete') || isPatch(seen[i].obj, 'replace')];
  local r = if resets == [] then null else resets[std.length(resets) - 1];
  local replay = if r == null then seen else seen[r + (if isPatch(seen[r].obj, 'delete') then 1 else 0):];
  // Mixins count back to the last delete or replace, including one that
  // created the object before any data layer did.
  local since = if r == null then -1 else seen[r].layer;
  local mixins = [m.layer for m in ctx.mixins if m.layer > since && m.layer < layer && std.objectHasAll(m.obj, k)];
  assert mixins == []
         : 'knit: %s: layer %d changes apiVersion to %s, but mixin layers %s changed the object before; move those mixins after layer %d'
           % [k, layer, o.apiVersion, mixins, layer];
  std.foldl(function(acc, e) acc + contribution(ctx, e.obj { apiVersion: o.apiVersion }, e.layer, k), replay, root(ctx, o));

// The stream of objects is one more keyed list, keyed by identity.
local stream(ctx, entries, layer, history) =
  overlay([e.id for e in entries], function(i, prev)
    local e = entries[i], o = e.obj, k = e.id;
    if isPatch(o, 'delete') then deleted
    else
      local base =
        if prev == null || isPatch(o, 'replace') then root(ctx, o)
        else if std.get(prev, 'apiVersion') != o.apiVersion then rebase(ctx, k, o, layer, history)
        else prev;
      base
      + contribution(ctx, o, layer, k)
      + {
        '$pos':: if prev == null then [layer, i] else std.get(prev, '$pos', [layer, i]),
        '$at':: if prev == null then e.at else std.get(prev, '$at', e.at),
      },
    function(dups) 'knit: layer %d: duplicate objects %s' % [layer, dups]);

// CRDs teach knit their kinds' list keys and scope.
local crdSchemas(crds) = std.foldl(function(acc, o) acc + {
  local gk = o.spec.group + '/' + o.spec.names.kind,
  kinds+: {
    [o.spec.group + '/' + v.name + '/' + o.spec.names.kind]: openapi.field({}, std.get(v.schema, 'openAPIV3Schema'))
    for v in std.get(o.spec, 'versions', [])
    if std.isObject(std.get(v, 'schema'))
  },
  clusterScoped+: { [gk]: std.get(o.spec, 'scope') == 'Cluster' },
}, [
  o
  for o in crds
  if std.isObject(std.get(o, 'spec')) && 'group' in o.spec && std.isObject(std.get(o.spec, 'names')) && 'kind' in o.spec.names
], { kinds: {}, clusterScoped: {} });

//
// Mixins.
//

// A mixin writes the internal form directly. What it leaves is brought back
// to that form: deletions become markers, keyed lists stay keyed (each entry
// under its own key), and an entry it adds (one without a `$pos`) goes after
// what was there. Values it adds (objects without `$n`, entries without
// `$pos`) get their type templates. Fields are read through `super` and
// nothing is read from the stream below, so `self` in a mixin still means
// the final value.
local settle(ctx, v, s, layer, path) =
  local rs = resolve(ctx, s);
  local typed(fs, x) = if isMergeable(x) && !('$n' in x) then tmpl(ctx, fs) + x + { '$n':: true } else x;
  if isPatch(v, 'delete') then deleted
  else if std.isArray(v) then
    assert [e for e in v if isMarker(e)] == [] : 'knit: %s: in a mixin, replace a list with `:` and append with `+`' % path;
    if isKeyedSchema(rs) then norm.keyed(ctx, [value(e) for e in v], rs, layer, path, null)
    else
      local item = atomicItem(rs);
      [typed(item, settle(ctx, v[i], item, layer, path + '[%d]' % i)) for i in std.range(0, std.length(v) - 1)]
  else if !std.isObject(v) || isDeleted(v) then v
  else
    assert !('$patch' in v) : 'knit: %s: in a mixin, replace with `:` rather than $patch' % path;
    local ks = std.objectFields(v);
    if isKeyedSchema(rs) then
      local item = std.get(rs, 'item');
      { '$list':: rs.list } + v + {
        [ks[j]]:
          local k = ks[j], e = super[k], p = path + '[' + k + ']';
          local pos = if std.isObject(e) then std.get(e, '$pos') else null;
          if e == null || isPatch(e, 'delete') || isDeleted(e) then deleted
          else if rs.list == 'set' then { '$v':: if std.isObject(e) && '$v' in e then e['$v'] else k, '$pos':: if pos == null then [layer, j] else pos }
          else
            local own = keyOf(rs, e, p, tmpl(ctx, item));
            assert own == k : "knit: %s: the entry's own key is %s" % [p, own];
            local se = settle(ctx, e, item, layer, p);
            if pos != null then se else typed(item, se) + { '$pos':: [layer, j] }
        for j in std.range(0, std.length(ks) - 1)
      }
    else
      v + { [k]: local fs = fieldOf(rs, k); typed(fs, settle(ctx, super[k], fs, layer, path + '.' + k)) for k in ks };

// An object a mixin creates is added like one from a data layer, in the
// mixin's place: matched by identity, on its kind's templates, deletions in
// it kept. It is read like a data layer, once, so `self` in it is its own.
local created(ctx, v, layer, i, k) =
  assert isManifest(v) : 'knit: layer %d: the mixin\'s %s matches no object, and is not a manifest' % [layer, k];
  // Migrations take plain manifests.
  local o = if groupVersionKind(v) in ctx.reg.migrations then migrate(ctx.reg, value(v)) else v;
  local own = idOf(ctx, o, layer, i);
  assert own == k : 'knit: layer %d: the mixin\'s %s is the object %s' % [layer, k, own];
  root(ctx, o) + contribution(ctx, o, layer, k) + { '$pos':: [layer, i], '$at':: { path: [k], arr: false } };

local applyMixin(ctx, acc, m, layer) =
  local ks = std.objectFields(m);
  acc
  // What a key held before the mixin, read through `super`, so it is
  // evaluated against the final stream like the mixin itself.
  + { '$before':: function(k) super[k] }
  + m
  + {
    [ks[i]]:
      local k = ks[i], v = super[k];
      local prev = if k in acc && !isDeleted(super['$before'](k)) then super['$before'](k) else null;
      if v == null || isPatch(v, 'delete') || isDeleted(v) then deleted
      else if prev == null then created(ctx, v, layer, i, k)
      else
        // The object keeps its place, whatever the mixin put there.
        settle(ctx, v, kindSchema(ctx, v), layer, k)
        + { '$pos':: std.get(prev, '$pos', [layer, i]), '$at':: std.get(prev, '$at', { path: [k], arr: false }) }
    for i in std.range(0, std.length(ks) - 1)
  };

//
// Registry.
//

local typeName(schema, k) =
  if k in schema.defs then k
  else if k in schema.aliases then schema.aliases[k]
  else error 'knit: types.%s: unknown or ambiguous type name' % k;

// Registry keys that name nothing. CRD groups always have a dot, so only
// groups without one (the core group, apps, batch, ...) can be checked.
local registryErrors(reg) =
  local known = [parseKind(k) for k in std.objectFields(reg.schema.kinds)];
  local groups = std.set([p.group for p in known]), gks = std.set([gkOf(p.group, p.kind) for p in known]);
  local isCrdGroup(g) = std.length(std.findSubstr('.', g)) > 0;
  local groupErrors(what, g) =
    if isCrdGroup(g) || std.member(groups, g) then [] else ["knit: %s: no API group %s (the core group is '')" % [what, g]];
  local kindErrors(what, k) =
    local p = parseKind(k);
    if p == null then ['knit: %s: expected [group/][version/]kind' % what]
    else if isCrdGroup(p.group) || std.member(gks, gkOf(p.group, p.kind)) then []
    else if !std.member(groups, p.group) then groupErrors(what, p.group)
    else
      local alt = [gk for gk in gks if std.endsWith('/' + gk, '/' + p.kind)];
      ['knit: %s: no kind %s in the %s group%s' % [
        what,
        p.kind,
        if p.group == '' then 'core' else p.group,
        if alt == [] then '' else '; did you mean %s?' % std.join(' or ', alt),
      ]];
  local names = [typeName(reg.schema, k) for k in std.objectFields(reg.types)];
  std.flattenArrays(
    [kindErrors('kinds.' + k, k) for k in std.objectFields(reg.kinds)]
    + [groupErrors('groups.' + g, g) for g in std.objectFields(reg.groups)]
  )
  // Migrations come from APIs the schema may no longer have.
  + [
    'knit: migrations.%s: expected group/version/kind, or version/kind in the core group' % k
    for k in std.objectFields(reg.migrations)
    for p in [parseKind(k)]
    if p == null || p.version == null
  ]
  + [
    'knit: types.%s name the same type' % std.join(' and types.', [k for k in std.objectFields(reg.types) if typeName(reg.schema, k) == n])
    for n in std.set(names)
    if std.count(names, n) > 1
  ];

local compile(reg, layers) =
  local idx = std.range(0, std.length(layers) - 1);
  local stub(g) = local p = std.split(g, '/'), n = std.length(p); { apiVersion: std.join('/', p[:n - 1]), kind: p[n - 1] };
  {
    local ctx = self,
    local all = [e for es in ctx.data if es != null for e in es],
    reg: reg,
    defs: reg.schema.defs,
    // The type templates being built, so one that holds its own type ends.
    stack: [],
    // Each type template is closed: its fields are values, so laying it
    // under a value that already has it changes nothing, even where it
    // appends to a list.
    tt: {
      [typeName(reg.schema, k)]:
        local n = typeName(reg.schema, k);
        local t = norm.ov(ctx { stack+: [n] }, reg.types[k], resolve(ctx, n), L.types, 'types.' + k, null);
        { [f]: t[f] for f in std.objectFields(t) }
      for k in std.objectFields(reg.types)
    },
    // The objects of each data layer, null for mixins.
    data: [
      if isMixin(layers[i]) then null
      else
        local flat = flatten(i, layers[i], [], true);
        [
          local o = migrate(reg, flat[j].obj);
          { obj: o, at: flat[j].at, layer: i, id: idOf(ctx, o, i, j) }
          for j in std.range(0, std.length(flat) - 1)
        ]
      for i in idx
    ],
    mixins: [{ layer: i, obj: layers[i]['$mixin'] } for i in idx if isMixin(layers[i])],
    history: groupBy(all, function(e) e.id),
    roots: { [g]: rootTemplate(ctx, stub(g)) for g in std.set([groupVersionKind(e.obj) for e in all]) },
    // The CRDs in the stream as the data layers compose them. Mixins are
    // left out: a mixin can see every object, so the schema of the objects
    // it sees cannot depend on it.
    crd:
      local data = [if es == null then null else [e for e in es if isCrd(e.obj)] for es in ctx.data];
      local history = groupBy([e for es in data if es != null for e in es], function(e) e.id);
      local composed = std.foldl(function(acc, i) if data[i] == null then acc else acc + stream(ctx, data[i], i, history), idx, {});
      crdSchemas([value(composed[k]) for k in std.objectFields(composed) if !isDeleted(composed[k])]),
  };

// The layers composed into the stream of objects, in the internal form.
local compose(registry, layers) =
  local errors = registryErrors(registry);
  assert errors == [] : std.join('\n', errors);
  local ctx = compile(registry, layers);
  {
    ctx: ctx,
    layers: layers,
    names: {},
    stream: std.foldl(
      function(acc, i)
        if ctx.data[i] == null then applyMixin(ctx, acc, layers[i]['$mixin'], i)
        else acc + stream(ctx, ctx.data[i], i, ctx.history),
      std.range(0, std.length(layers) - 1),
      { '$list':: 'stream' },
    ),
  };

// The manifests of a composed stream, but those with a name, which their
// fields output. They keep the first data layer's shape: an array, or an
// object with each manifest at the path where it was first introduced.
local output(c) =
  local composed = c.stream, data = [l for l in c.layers if l != null && !isMixin(l)];
  local nested = data != [] && std.isObject(data[0]) && !isManifest(data[0]);
  local named = std.set([c.names[n] for n in std.objectFields(c.names)]);
  local ks = [k for k in liveKeys(composed) if !std.member(named, k)];
  // Manifests that came from an array go back into it, in stream order.
  if nested then
    kubecfg.fromObjectPaths([
      local k = ks[i], at = std.get(composed[k], '$at', { path: [], arr: false });
      { path: if at.path == [] then [k] else if at.arr then at.path + [i] else at.path, obj: value(composed[k]) }
      for i in std.range(0, std.length(ks) - 1)
    ])
  else [value(composed[k]) for k in ks];

//
// Names: the documents of YAKL files.
//

// A YAKL file lists its documents in `documents`, in order, as {name, plus,
// doc}. A name is an object when the last of its documents without + is a
// manifest; objects maps it to that document's index.
local namedObjects(docs) =
  local defs = std.foldl(
    function(acc, i) if docs[i].name != null && !docs[i].plus then acc + { [docs[i].name]: i } else acc,
    std.range(0, std.length(docs) - 1),
    {},
  );
  { [n]: defs[n] for n in std.objectFields(defs) if isManifest(docs[defs[n]].doc) };

// What a !name+ document patches: the object of its name, which it doesn't
// have to repeat the identity of.
local patchOf(name, def, p) =
  local md = def.metadata;
  assert std.isObject(p) : 'knit: !%s+: expected an object, got %s' % [name, std.type(p)];
  // A later !name+ would start the object again with only its own fields.
  assert !isPatch(p, 'delete') : 'knit: !%s+ deletes the object: to leave it out, hide it with !%s+:: {}' % [name, name];
  assert std.get(p, 'kind', def.kind) == def.kind : 'knit: !%s+ changes the kind to %s' % [name, p.kind];
  assert !std.isObject(std.get(p, 'metadata')) || std.get(p.metadata, 'name', md.name) == md.name
         : 'knit: !%s+ changes the name to %s' % [name, p.metadata.name];
  p + {
    apiVersion: std.get(p, 'apiVersion', def.apiVersion),
    kind: def.kind,
    metadata+: { name: md.name } + (if 'namespace' in md then { namespace: md.namespace } else {}),
  };

// The layers that documents give, one per document. A document with a name
// that isn't an object (settings, the registry) gives none, and neither do
// the documents of an object before its last definition, which replaces
// whatever the object was, as a Jsonnet field does.
local documentLayers(docs, objects) = [
  local d = docs[i];
  if d.name == null then d.doc
  else if !(d.name in objects) || i < objects[d.name] then null
  else if i == objects[d.name] then
    assert std.isString(std.get(std.get(d.doc, 'metadata', {}), 'name')) : 'knit: !%s has no metadata.name' % d.name;
    d.doc + { '$patch': 'replace' }
  else patchOf(d.name, docs[objects[d.name]].doc, d.doc)
  for i in std.range(0, std.length(docs) - 1)
];

{
  assert kubecfg.requireFeature('knit'),

  // A YAKL file that extends this object adds its documents, in order, to
  // `documents`, and its documents without a name to `unnamed`; a Jsonnet
  // file adds layers to `unnamed`. Every document is a layer on top of its
  // parent's, and late binding renders the whole chain from the last file.
  documents:: [],
  unnamed:: [],
  manifests: output(self['$composed']),
  '$names':: namedObjects(self.documents),
  '$composed'::
    local docs = self.documents, objects = self['$names'];
    local c = compose(self.registry, if std.length(docs) == 0 then self.unnamed else documentLayers(docs, objects));
    c {
      names: { [n]: idOf(c.ctx, migrate(c.ctx.reg, docs[objects[n]].doc), objects[n], 0) for n in std.objectFields(objects) },
    },

  // A named document's field (see YAKL's documents): for an object, the
  // object as it ends up, after every layer of every file, so $.web reads
  // the final web. Keyed lists are lists, and also hidden objects keyed by
  // their key, next to them: containers_.web.env_.LOG_LEVEL. Other names
  // are plain YAKL.
  '$named'(name, plain)::
    if name in self['$names'] then
      local c = self['$composed'], k = c.names[name];
      if k in c.stream && !isDeleted(c.stream[k]) then view(c.stream[k]) else null
    else plain(),

  registry:: {
    schema: import 'schema/k8s-1.37.libsonnet',
    types: {},
    objects: {},
    groups: {},
    kinds: {},
    migrations: {},
  },

  // Directives for Jsonnet layers; YAML layers spell them out.
  delete:: { '$patch': 'delete' },
  replace(items):: [{ '$patch': 'replace' }] + items,
  append(items):: [{ '$patch': 'append' }] + items,

  // A layer applied as-is to the composed stream, in the internal keyed form.
  // `super` is the stream the layers below it leave, `self` the final one.
  mixin(obj):: { '$mixin':: obj },
  id(groupKind, name, namespace=null):: id(groupKind, name, namespace),

  // For hack/gen-schema.jsonnet.
  openapi:: openapi { isObject(s):: isObjectSchema(s) },
  groupKind:: groupKind,
  groupVersionKind:: groupVersionKind,

  render(registry, layers):: output(compose(registry, layers)),
}
