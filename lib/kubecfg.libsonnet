// Copyright 2017 The kubecfg authors
//
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

// The group of an object's apiVersion: 'apps' for apps/v1, '' for v1.
local groupOf(o) = local p = std.split(o.apiVersion, '/'); if std.length(p) == 1 then '' else p[0];
// 'apps/Deployment', or 'ConfigMap' in the core group.
local groupKind(o) = (if groupOf(o) == '' then '' else groupOf(o) + '/') + o.kind;
local pathString(path) = if path == [] then '(top)' else std.join('.', [std.toString(s) for s in path]);

{
  // requireFeature(name): true when the named feature is on (kubecfg
  // --features, see `kubecfg features`), else an error that says how to turn
  // it on. A library can require a feature, but not read whether it is on:
  //   assert kubecfg.requireFeature('knit'),
  requireFeature(name):: std.native('requireFeature')(name),

  // parseJson(data): parses the `data` string as a json document, and
  // returns the resulting jsonnet object.
  parseJson:: std.native('parseJson'),

  // parseYaml(data): parse the `data` string as a YAML stream, and
  // returns an *array* of the resulting jsonnet objects.  A single
  // YAML document will still be returned as an array with one
  // element.
  parseYaml:: std.native('parseYaml'),

  // manifestJson(value, indent): convert the jsonnet object `value`
  // to a string encoded as "pretty" (multi-line) JSON, with each
  // nesting level indented by `indent` spaces.
  manifestJson(value, indent=4):: (
    local f = std.native('manifestJson');
    f(value, indent)
  ),

  // manifestYaml(value): convert the jsonnet object `value` to a
  // string encoded as a single YAML document.
  manifestYaml:: std.native('manifestYaml'),

  // escapeStringRegex(s): Quote the regex metacharacters found in s.
  // The result is a regex that will match the original literal
  // characters.
  escapeStringRegex:: std.native('escapeStringRegex'),

  // resolveImage(image): convert the docker image string from
  // image:tag into a more specific image@digest, depending on kubecfg
  // command line flags.
  resolveImage:: std.native('resolveImage'),

  // regexMatch(regex, string): Returns true if regex is found in
  // string. Regex is as implemented in golang regexp package
  // (python-ish).
  regexMatch:: std.native('regexMatch'),

  // regexSubst(regex, src, repl): Return the result of replacing
  // regex in src with repl.  Replacement string may include $1, etc
  // to refer to submatches.  Regex is as implemented in golang regexp
  // package (python-ish).
  regexSubst:: std.native('regexSubst'),

  // parseHelmChart(chartData, releaseName, namespace, values, capabilities={}): Expand
  // helm chart into jsonnet objects.  `chartData` should be valid
  // chart .tgz as an array of numbers (bytes).  `values` is a jsonnet
  // object that conforms to a schema defined by the chart.
  // `capabilities` is an object matching the struct format of Helm's
  // "Capabilities" struct. Only "KubeVersion" is supported. For more
  // information, see:
  // https://helm.sh/docs/chart_template_guide/builtin_objects/
  parseHelmChart(chartData, releaseName, namespace, values, capabilities={}):: (
    std.native('parseHelmChart')(
      chartData,
      releaseName,
      namespace,
      values,
      capabilities
    )
  ),

  // validateJSONSchema(obj, schema): Validates a given object against the provided
  // schema. Returns 'true' is the schema is valid. If this is not the case, an error stream
  // is omitted based on the given schema's rules.
  validateJSONSchema:: std.native('validateJSONSchema'),

  // toOverlay is a function that takes a normal JSON/YAML object {a:{b:c:{d:1}}}
  // and rewrites it as if it was written as {a+:{b+:c+:{d:1}}}.
  // Care is taken to use the + operator only for objects, since for scalars and
  // array it doesn't have the semantic of "extend".
  //
  // This behaves similar to std.mergePatch but mergePatch breaks lazy evaluation.
  toOverlay(v):: (
    local t = std.type(v);
    if t == 'object' then {
      [kv.key]+: $.toOverlay(kv.value)
      for kv in std.objectKeysValues(v)
      if std.type(kv.value) == 'object'
    } + {
      [kv.key]: kv.value
      for kv in std.objectKeysValues(v)
      if std.type(kv.value) != 'object'
    } else v
  ),

  // knit: kind-aware overlays of plain Kubernetes manifests, keyed by the
  // Kubernetes OpenAPI schema. See docs/Advanced-Usage/knit.md.
  // Also importable directly as 'knit/knit.libsonnet'. Alpha: it needs
  // --features=knit.
  knit:: import 'knit/knit.libsonnet',

  // isK8sObject(o): Return true iff o is a Kubernetes object.
  isK8sObject(o):: (
    std.isObject(o) &&
    std.objectHas(o, 'apiVersion') &&
    std.objectHas(o, 'kind')
  ),

  // Private helper function for map/fold.
  local isK8sList(o) = $.isK8sObject(o) && o.apiVersion == 'v1' && o.kind == 'List',

  // deepMap(func, o): Apply the given function to each Kubernetes
  // object in nested collection o, preserving the structure of o.
  deepMap(func, o):: (
    if isK8sList(o) then
      o { items: [func(item) for item in super.items] }
    else if $.isK8sObject(o) then
      func(o)
    else if std.isObject(o) then
      { [k]: $.deepMap(func, o[k]) for k in std.objectFields(o) }
    else if std.isArray(o) then
      [$.deepMap(func, elem) for elem in o]
    else if o == null then
      null
    else
      error ('o must be an object or array of k8s objects, found ' + std.type(o))
  ),

  // fold(func, o, init): Apply the given function to each Kubernetes
  // object in nested collection o, accumulating a result as we go.
  // Function arg is invoked with arguments (accumulator, object).
  fold(func, o, init):: (
    if isK8sList(o) then
      $.fold(func, o.items, init)
    else if $.isK8sObject(o) then
      func(init, o)
    else if std.isObject(o) then
      $.fold(func, [o[k] for k in std.objectFields(o)], init)
    else if std.isArray(o) then
      std.foldl(function(running, elem) $.fold(func, elem, running), o, init)
    else if o == null then
      init
    else
      error ('o must be an object or array of k8s objects, found ' + std.type(o))
  ),

  // Helper function for deciding which standard library implementation to use.
  local hidden_fields_wrapper(o, f, inc_hidden) = (
    if inc_hidden then
      !std.objectHasAll(o, f)
    else
      !std.objectHas(o, f)
  ),

  local objectGetDeep(o, f, default, inc_hidden) = (

    local objectGetDeep_(o, ks) =
      if hidden_fields_wrapper(o, ks[0], inc_hidden) then
        default
      else if std.length(ks) == 1 then
        o[ks[0]]
      else
        objectGetDeep_(o[ks[0]], ks[1:]);

    objectGetDeep_(o, std.split(f, '.'))
  ),

  local objectHasDeep(o, f, inc_hidden) = (
    local objectHasDeep_(o, ks) =
      if hidden_fields_wrapper(o, ks[0], inc_hidden) then
        false
      else if std.length(ks) == 1 then
        true
      else
        objectHasDeep_(o[ks[0]], ks[1:]);

    objectHasDeep_(o, std.split(f, '.'))
  ),

  // getPath(obj, path, default, inc_hidden): Similar to std.get, but with a nested
  // jsonpath. If a value is not available, then a default or null is returned.
  getPath(obj, path, default=null, inc_hidden=true):: objectGetDeep(obj, path, default, inc_hidden),

  // objectHasPath(obj, path, inc_hidden): Similar to std.objectHasAll, but with a nested jsonpath.
  objectHasPath(obj, path, inc_hidden=false):: objectHasDeep(obj, path, inc_hidden),

  // objectHasPathAll(obj path): Shorthand for objectHasPath(obj, path, inc_hidden=true)
  objectHasPathAll(obj, path):: objectHasDeep(obj, path, inc_hidden=true),

  // groupBy(arr, keyF): splits arr into an object of arrays, one per key.
  // keyF(x) must return a string. Each array keeps the order of arr. Runs
  // in O(n log n).
  //   groupBy(objs, function(o) o.kind) == { ConfigMap: [cm1, cm2], Deployment: [web] }
  groupBy(arr, keyF):: (
    local n = std.length(arr), ks = [keyF(x) for x in arr];
    local s = std.sort(std.range(0, n - 1), function(i) [ks[i], i]);
    local starts = [j for j in std.range(0, n - 1) if j == 0 || ks[s[j]] != ks[s[j - 1]]] + [n];
    { [ks[s[starts[g]]]]: [arr[i] for i in s[starts[g]:starts[g + 1]]] for g in std.range(0, std.length(starts) - 2) }
  ),

  // toObjectPaths(o): every Kubernetes object in the nested collection o,
  // with the path that leads to it: an array of { path, obj }. A path is a
  // list of object keys (strings) and array indexes (numbers). A v1 List
  // counts as an array of its items. Objects come in the order fold visits
  // them. null is skipped, and any other value is an error, as in fold.
  //   toObjectPaths({ web: { deploy: d, svc: s }, extra: [cm] }) == [
  //     { path: ['extra', 0], obj: cm },
  //     { path: ['web', 'deploy'], obj: d },
  //     { path: ['web', 'svc'], obj: s },
  //   ]
  toObjectPaths(o):: (
    local walk(o, path) =
      local each(xs) = std.flattenArrays([walk(xs[i], path + [i]) for i in std.range(0, std.length(xs) - 1)]);
      if isK8sList(o) then each(o.items)
      else if $.isK8sObject(o) then [{ path: path, obj: o }]
      else if std.isObject(o) then std.flattenArrays([walk(o[k], path + [k]) for k in std.objectFields(o)])
      else if std.isArray(o) then each(o)
      else if o == null then []
      else error ('o must be an object or array of k8s objects, found ' + std.type(o));
    walk(o, [])
  ),

  // fromObjectPaths(entries): the reverse of toObjectPaths. Builds a nested
  // collection from an array of { path, obj }: a string step makes an
  // object, a number step an array. Under one array, entries go in index
  // order and gaps are closed, so entries can be dropped without
  // renumbering. Two entries at the same path, an entry inside another
  // one's object, or strings and numbers at one level are an error. No
  // entries give [].
  //   fromObjectPaths(toObjectPaths(o)) == o, except a v1 List comes back as an array.
  //   // Drop every Secret, keeping the shape:
  //   fromObjectPaths([e for e in toObjectPaths(o) if e.obj.kind != 'Secret'])
  fromObjectPaths(entries):: (
    local describe(e) =
      local what = if $.isK8sObject(e.obj) then '%s %s' % [e.obj.kind, std.get(std.get(e.obj, 'metadata', {}), 'name', '')] else std.type(e.obj);
      '%s at %s' % [what, pathString(e.path)];
    local build(es, d) =
      if std.length([e for e in es if std.length(e.path) == d]) > 0 then
        assert std.length(es) == 1
               : 'fromObjectPaths: objects at the same path, or one inside another: ' + std.join(', ', [describe(e) for e in es]);
        es[0].obj
      else
        local types = std.set([std.type(e.path[d]) for e in es]);
        assert types == ['number'] || types == ['string']
               : 'fromObjectPaths: a level mixes object keys and array indexes: ' + std.join(', ', [describe(e) for e in es]);
        local g = $.groupBy(es, function(e) std.toString(e.path[d]));
        if types == ['number'] then [build(g[k], d + 1) for k in std.sort(std.objectFields(g), std.parseJson)]
        else { [k]: build(g[k], d + 1) for k in std.objectFields(g) };
    if entries == [] then [] else build(entries, 0)
  ),

  // isNamespaced(o, crds=[]): whether objects of o's kind live in a
  // namespace. Built-in kinds are looked up in the Kubernetes API schema
  // that comes with kubecfg, which lists the cluster-scoped ones
  // (Namespace, ClusterRole, ...). For custom resources, pass their CRDs:
  // each one's spec.scope decides. A kind found in neither counts as
  // namespaced. The group must match: a Namespace in example.com is not
  // the core Namespace.
  //   deepMap(function(o) if isNamespaced(o) then o { metadata+: { namespace: 'prod' } } else o, objs)
  isNamespaced(o, crds=[]):: (
    local gk = groupKind(o);
    local scopes = [c.spec.scope for c in crds if c.spec.group + '/' + c.spec.names.kind == gk];
    if scopes != [] then scopes[0] != 'Cluster'
    else !std.get((import 'knit/schema/k8s-1.37.libsonnet').clusterScoped, gk, false)
  ),

  layouts:: {
    // gvkName(accum, o): Helper for 'fold'.  This accumulates a
    // two-level collection of objects by 'apiVersion.kind'
    // (GroupVersionKind) and then object 'name'.  NB: use gvkNsName
    // if namespace is required for uniqueness.
    gvkName(accum, o):: accum {
      [o.apiVersion + '.' + o.kind]+: {
        assert !( o.metadata.name in super),
        [o.metadata.name]: o,
      },
    },

    // gvkNsName(accum, o): Helper for 'fold'.  This accumulates a
    // three-level collection of objects by 'apiVersion.kind'
    // (GroupVersionKind), object 'namespace', and then object 'name'.
    // Namespace is '_' for non-namespaced objects.
    gvkNsName(accum, o):: accum {
      [o.apiVersion + '.' + o.kind]+: {
        [std.get(o.metadata, 'namespace', default='_')]+: {
          assert !( o.metadata.name in super),
          [o.metadata.name]: o,
        },
      },
    },

    // gkNsName(accum, o): Helper for 'fold', like gvkNsName, but keyed by
    // group and kind without the version: 'apps/Deployment' (core kinds
    // have no group: 'ConfigMap'), then namespace ('_' for none), then
    // name. This is how Kubernetes tells objects apart, so the apps/v1 and
    // apps/v1beta1 versions of one Deployment are a duplicate, and fail;
    // gvkNsName takes them for two objects.
    gkNsName(accum, o):: accum {
      [groupKind(o)]+: {
        [std.get(o.metadata, 'namespace', default='_')]+: {
          assert !(o.metadata.name in super) : 'duplicate object %s %s' % [groupKind(o), o.metadata.name],
          [o.metadata.name]: o,
        },
      },
    },
  },
}
