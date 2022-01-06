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

  // helmTemplate(releaseName, namespace, chartURL, values): Fetch and
  // expand helm chart at `chartURL` into jsonnet objects.  Chart URL
  // refers to chart .tgz file.  `values` is a jsonnet object that
  // conforms to a schema defined by the chart.
  helmTemplate:: std.native('helmTemplate'),
}
