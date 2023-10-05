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

// Run me with `kubecfg show kubecfg_test.jsonnet`

// NB: These tests are in a separate dir to kubecfg.libsonnet to verify
// that kubecfg.libsonnet is found along the usual search path, and
// not via a current-directory relative import.
local kubecfg = import 'kubecfg.libsonnet';

local result =

  std.assertEqual(kubecfg.parseJson('[3, 4]'), [3, 4]) &&

  std.assertEqual(kubecfg.parseYaml(|||
                    ---
                    - 3
                    - 4
                    ---
                    foo: bar
                    baz: xyzzy
                  |||),
                  [[3, 4], { foo: 'bar', baz: 'xyzzy' }]) &&

  std.assertEqual(kubecfg.manifestJson({ foo: 'bar', baz: [3, 4] }),
                  |||
                    {
                        "baz": [
                            3,
                            4
                        ],
                        "foo": "bar"
                    }
                  |||) &&

  std.assertEqual(kubecfg.manifestJson({ foo: 'bar', baz: [3, 4] }, indent=2),
                  |||
                    {
                      "baz": [
                        3,
                        4
                      ],
                      "foo": "bar"
                    }
                  |||) &&

  std.assertEqual(kubecfg.manifestJson('foo'), '"foo"\n') &&

  std.assertEqual(kubecfg.manifestYaml({ foo: 'bar', baz: [3, 4] }),
                  |||
                    baz:
                    - 3
                    - 4
                    foo: bar
                  |||) &&

  std.assertEqual(kubecfg.resolveImage('busybox'),
                  'docker.io/library/busybox:latest') &&

  std.assertEqual(kubecfg.regexMatch('o$', 'foo'), true) &&

  std.assertEqual(kubecfg.escapeStringRegex('f[o'), 'f\\[o') &&

  std.assertEqual(kubecfg.regexSubst('e', 'tree', 'oll'),
                  'trolloll') &&

  std.assertEqual(std.clamp(42, 0, 10), 10) &&

  local testObj = {
    a: {
      b: {
        c: 1,
        d: 10,
      },
    },
  };
  local expectedOverlayObj = {
    a: {
      b: {
        c: 2,
        d: 10,
      },
    },
  };
  std.assertEqual(testObj + kubecfg.toOverlay(import 'overlay.json'), expectedOverlayObj) &&

  // Testing import of pre-converted chart with standard import
  local chartData = import 'mysql-8.8.26.tgz.bin';
  local testChart = kubecfg.parseHelmChart(
    chartData, 'foo', 'myns', {
      auth: { password: 'foo' },
    }
  );
  local testValue = [
    testChart['mysql/templates/primary/statefulset.yaml'][0].spec.serviceName,
    testChart['mysql/templates/secrets.yaml'][0].metadata.namespace,
  ];
  std.assertEqual(testValue, ['foo-mysql', 'myns']) &&

  // Testing import of chart with importbin from go-jsonnet 0.19.0
  local importBin = importbin './mysql-8.8.26.tgz';
  local testChartBin = kubecfg.parseHelmChart(
    importBin, 'foo', 'myns', {
      auth: { password: 'foo' },
    }
  );
  local testValueBin = [
    testChartBin['mysql/templates/primary/statefulset.yaml'][0].spec.serviceName,
    testChartBin['mysql/templates/secrets.yaml'][0].metadata.namespace,
  ];
  std.assertEqual(testValueBin, ['foo-mysql', 'myns']) &&
  std.assertEqual('7f94f699bd5353f1ba023bcd391b5068', std.md5(std.base64(importBin))) &&
  std.assertEqual(std.base64(chartData), std.base64(importBin)) &&

  // Testing import of chart using import binary:// alpha feature
  local importBinary = import 'binary://mysql-8.8.26.tgz';
  std.assertEqual('7f94f699bd5353f1ba023bcd391b5068', std.md5(std.base64(importBinary))) &&
  std.assertEqual(std.base64(chartData), std.base64(importBinary)) &&

  std.assertEqual(kubecfg.isK8sObject('bogus'), false) &&

  std.assertEqual(kubecfg.isK8sObject({ apiVersion: 'v1', kind: 'Pod' }), true) &&

  local obj(n) = { apiVersion: 'example.com/v1alpha1', kind: 'Test', name: n };
  local f(o) = o { name+: '2' };
  local input = {
    a: obj('a'),
    b: [
      { b1: obj('b1') },
      obj('b2'),
    ],
    c: null,
    d: {
      apiVersion: 'v1',
      kind: 'List',
      extrakey: 'foo',
      items: [obj('d')],
    },
  };
  local expected = {
    a: obj('a2'),
    b: [
      { b1: obj('b12') },
      obj('b22'),
    ],
    c: null,
    d: {
      apiVersion: 'v1',
      kind: 'List',
      extrakey: 'foo',
      items: [obj('d2')],
    },
  };
  std.assertEqual(kubecfg.deepMap(f, input), expected) &&

  local obj(n) = { apiVersion: 'example.com/v1alpha1', kind: 'Test', name: n };
  local names(accum, o) = accum + [o.name];
  local input = {
    a: obj('a'),
    b: [
      { b1: obj('b1') },
      obj('b2'),
    ],
    c: null,
    d: {
      apiVersion: 'v1',
      kind: 'List',
      extrakey: 'foo',
      items: [obj('d')],
    },
  };
  local expected = ['a', 'b1', 'b2', 'd'];
  std.assertEqual(kubecfg.fold(names, input, []), expected) &&

  local obj(n) = { apiVersion: 'example.com/v1alpha1', kind: 'Test', metadata: { name: n } };
  local one = kubecfg.layouts.gvkName({}, obj('a'));
  local two = kubecfg.layouts.gvkName(one, obj('b'));
  std.assertEqual(two, { 'example.com/v1alpha1.Test': { a: obj('a'), b: obj('b') } }) &&

  local obj(n) = { apiVersion: 'example.com/v1alpha1', kind: 'Test', metadata: { name: n } };
  local one = kubecfg.layouts.gvkNsName({}, obj('a'));
  local two = kubecfg.layouts.gvkNsName(one, obj('b'));
  std.assertEqual(two, { 'example.com/v1alpha1.Test': { _: { a: obj('a'), b: obj('b') } } }) &&

  local nested_obj = {
    foo: {
      bar: {
        baz: 'nested!',
      },
      hidden:: {
        qux:: 'sneaky!',
      },
    },
  };
  std.assertEqual(kubecfg.getPath(nested_obj, 'foo.bar.baz'), 'nested!') &&
  std.assertEqual(kubecfg.getPath(nested_obj, 'foo.hidden.qux'), 'sneaky!') &&
  std.assertEqual(kubecfg.getPath(nested_obj, 'foo.hidden.qux', inc_hidden=false), null) &&
  std.assertEqual(kubecfg.getPath(nested_obj, 'foo.not.exist', default="hello!"), "hello!") &&
  std.assertEqual(kubecfg.getPath(nested_obj, 'path.not.exist', 'default!'), 'default!') &&
  std.assertEqual(kubecfg.objectHasPath(nested_obj, 'foo.bar.baz'), true) &&
  std.assertEqual(kubecfg.objectHasPath(nested_obj, 'foo.not.exist'), false) &&
  std.assertEqual(kubecfg.objectHasPath(nested_obj, 'foo.hidden.qux'), false) &&
  std.assertEqual(kubecfg.objectHasPathAll(nested_obj, 'foo.hidden.qux'), true) &&

  true;

// Kubecfg wants to see something that looks like a k8s object
{
  apiVersion: 'test',
  kind: 'Result',
  // result==false assert-aborts above, but we should use the value
  // somewhere here to ensure the expression actually gets evaluated.
  result: if result then 'SUCCESS' else 'FAILED',
}
