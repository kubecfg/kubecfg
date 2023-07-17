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

{
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

  // parseHelmChart(chartData, releaseName, namespace, values): Expand
  // helm chart into jsonnet objects.  `chartData` should be valid
  // chart .tgz as an array of numbers (bytes).  `values` is a jsonnet
  // object that conforms to a schema defined by the chart.
  parseHelmChart:: std.native('parseHelmChart'),

  // validateJSONSchema(obj, schema): Validates a given object against the provided
  // schema. Returns 'true' is the schema is valid. If this is not the case, an error stream
  // is omitted based on the given schema's rules.
  validateJSONSchema:: std.native('validateJSONSchema'),

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
  },
}
