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

package utils

import (
	"bytes"
	"io"
	"strings"
	"testing"

	jsonnet "github.com/google/go-jsonnet"
	"github.com/kubecfg/kubecfg/pkg/features"
	log "github.com/sirupsen/logrus"
)

// check there is no err, and a == b.
func check(t *testing.T, err error, actual, expected string) {
	t.Helper()
	if err != nil {
		t.Errorf("Expected %q, got error: %q", expected, err.Error())
	} else if actual != expected {
		t.Errorf("Expected %q, got %q", expected, actual)
	}
}

func TestParseJson(t *testing.T) {
	vm := jsonnet.MakeVM()
	RegisterNativeFuncs(vm, NewIdentityResolver())

	_, err := vm.EvaluateSnippet("failtest", `std.native("parseJson")("barf{")`)
	if err == nil {
		t.Errorf("parseJson succeeded on invalid json")
	}

	x, err := vm.EvaluateSnippet("test", `std.native("parseJson")("null")`)
	check(t, err, x, "null\n")

	x, err = vm.EvaluateSnippet("test", `
    local a = std.native("parseJson")('{"foo": 3, "bar": 4}');
    a.foo + a.bar`)
	check(t, err, x, "7\n")
}

func TestParseYaml(t *testing.T) {
	vm := jsonnet.MakeVM()
	RegisterNativeFuncs(vm, NewIdentityResolver())

	_, err := vm.EvaluateSnippet("failtest", `std.native("parseYaml")("[barf")`)
	if err == nil {
		t.Errorf("parseYaml succeeded on invalid yaml")
	}

	x, err := vm.EvaluateSnippet("test", `std.native("parseYaml")("")`)
	check(t, err, x, "[ ]\n")

	x, err = vm.EvaluateSnippet("test", `
    local a = std.native("parseYaml")("foo:\n- 3\n- 4\n")[0];
    a.foo[0] + a.foo[1]`)
	check(t, err, x, "7\n")

	x, err = vm.EvaluateSnippet("test", `
    local a = std.native("parseYaml")("---\nhello\n---\nworld");
    a[0] + a[1]`)
	check(t, err, x, "\"helloworld\"\n")
}

func TestRegexMatch(t *testing.T) {
	vm := jsonnet.MakeVM()
	RegisterNativeFuncs(vm, NewIdentityResolver())

	_, err := vm.EvaluateSnippet("failtest", `std.native("regexMatch")("[f", "foo")`)
	if err == nil {
		t.Errorf("regexMatch succeeded with invalid regex")
	}

	x, err := vm.EvaluateSnippet("test", `std.native("regexMatch")("foo.*", "seafood")`)
	check(t, err, x, "true\n")

	x, err = vm.EvaluateSnippet("test", `std.native("regexMatch")("bar.*", "seafood")`)
	check(t, err, x, "false\n")
}

func TestRegexSubst(t *testing.T) {
	vm := jsonnet.MakeVM()
	RegisterNativeFuncs(vm, NewIdentityResolver())

	_, err := vm.EvaluateSnippet("failtest", `std.native("regexSubst")("[f",s "foo", "bar")`)
	if err == nil {
		t.Errorf("regexSubst succeeded with invalid regex")
	}

	x, err := vm.EvaluateSnippet("test", `std.native("regexSubst")("a(x*)b", "-ab-axxb-", "T")`)
	check(t, err, x, "\"-T-T-\"\n")

	x, err = vm.EvaluateSnippet("test", `std.native("regexSubst")("a(x*)b", "-ab-axxb-", "${1}W")`)
	check(t, err, x, "\"-W-xxW-\"\n")
}

func TestParseHelmChart(t *testing.T) {
	log.SetLevel(log.DebugLevel)

	vm := jsonnet.MakeVM()
	RegisterNativeFuncs(vm, NewIdentityResolver())

	_, err := vm.EvaluateSnippet("failtest", `std.native("parseHelmChart")("not_data", "rls", "myns", {}, {})`)
	if err == nil {
		t.Errorf("helmTemplate succeeded with invalid data")
	}

	_, err = vm.EvaluateSnippet("failtest", `std.native("parseHelmChart")([1, 2, 3, 256], "myrls", "myns", {}, {})`)
	if err == nil {
		t.Errorf("helmTemplate succeeded with invalid bytes")
	}

	vm = jsonnet.MakeVM()
	RegisterNativeFuncs(vm, NewIdentityResolver())

	x, err := vm.EvaluateSnippet("test", `
    local chrt = std.native("parseHelmChart")(import "../testdata/mysql-8.8.26.tgz.bin", "myrls", "myns", {primary: {resources: {limits: {cpu: "2"}}}}, {});
    local ss = chrt["mysql/templates/primary/statefulset.yaml"][0];
    [
      // from nested chart
      ss.apiVersion,
      // namespace arg
      ss.metadata.namespace,
      // Provided value
      ss.spec.template.spec.containers[0].resources.limits.cpu,
      // Default value
      ss.spec.template.spec.containers[0].image,
    ]
`)
	check(t, err, x, `[
   "apps/v1",
   "myns",
   "2",
   "docker.io/bitnami/mysql:8.0.28-debian-10-r23"
]
`)
}

func TestParseHelmChartDependencies(t *testing.T) {
	vm := jsonnet.MakeVM()
	RegisterNativeFuncs(vm, NewIdentityResolver())

	// umbrella has two copies of the "sub" subchart: one with a
	// condition, and one renamed with an alias and guarded by a tag.
	x, err := vm.EvaluateSnippet("test", `
    local render(values) = std.objectFields(std.native("parseHelmChart")(importbin "../testdata/umbrella-0.1.0.tgz", "rls", "myns", values, {}));
    {
      defaults: render({}),
      conditionOff: render({sub: {enabled: false}}),
      tagOff: render({tags: {extras: false}}),
    }
`)
	check(t, err, x, `{
   "conditionOff": [
      "umbrella/charts/tagged/templates/configmap.yaml",
      "umbrella/templates/configmap.yaml"
   ],
   "defaults": [
      "umbrella/charts/sub/templates/configmap.yaml",
      "umbrella/charts/tagged/templates/configmap.yaml",
      "umbrella/templates/configmap.yaml"
   ],
   "tagOff": [
      "umbrella/charts/sub/templates/configmap.yaml",
      "umbrella/templates/configmap.yaml"
   ]
}
`)
}

func TestValidateJSONSchema(t *testing.T) {
	vm := jsonnet.MakeVM()
	RegisterNativeFuncs(vm, NewIdentityResolver())

	res, err := vm.EvaluateSnippet("validObject", `
    local schema = {
      type: 'object',
      properties: {
        age: {
          description: 'Age in years which must be equal to or greater than zero.',
          type: 'integer',
          minimum: 0,
        },
      },
    };
    local obj = {
      age: 26,
    };

    std.native('validateJSONSchema')(obj, schema)`)
	check(t, err, res, "true\n")

	res, err = vm.EvaluateSnippet("invalidObjectErrors", `
    local schema = {
      type: 'object',
      properties: {
        projectName: {
          description: 'projectName is required for the name of a project',
          type: 'string',
        },
        language: {
          description: 'Programming language implementation',
          type: 'string',
        },
      },
      required: ["projectName"],
    };

    local obj = {
        language: 'go',
    };

    std.native('validateJSONSchema')(obj, schema)`)
	if err != nil {
		if !strings.Contains(err.Error(), "invalid against the schema") {
			t.Errorf("expected invalid schema error. got: %s\n", err.Error())
		}
	}
}

func TestValidateSimpleSchema(t *testing.T) {
	vm := jsonnet.MakeVM()
	RegisterNativeFuncs(vm, NewIdentityResolver())
	if _, err := vm.EvaluateAnonymousSnippet("test", `std.native('validateSimpleSchema')({}, {}, {})`); err == nil || !strings.Contains(err.Error(), "--features=simpleschema") {
		t.Errorf("want the native to need --features=simpleschema, got %v", err)
	}
	RegisterFeatures(vm, features.AllAlpha())
	vm.ExtCode("schema", `{
	  name: 'string | required=true pattern="^[a-z]+$"',
	  replicas: 'integer | default=1 minimum=1',
	  ratio: 'float',
	  labels: 'map[string]string',
	  extra: 'object',
	  ports: '[]Port | listType=map listMapKey=port',
	  limits: { cpu: 'string' },
	  range: 'Range | validation="!has(self.min) || self.min <= self.max"',
	}`)
	vm.ExtCode("types", `{
	  Port: { port: 'integer | required=true', name: 'string' },
	  Range: { min: 'integer', max: 'integer | default=10' },
	}`)

	for _, tc := range []struct{ obj, want string }{
		// Jsonnet numbers are float64, and pass as integers when they're whole.
		{`{name: 'web', replicas: 3, ratio: 1, labels: {a: 'b'}, extra: {any: [1]}, ports: [{port: 80}], limits: {cpu: '1'}, range: {max: 3}}`, ""},
		// The API server drops a null where the schema doesn't allow one, or fills in the default.
		{`{name: 'web', ratio: null, replicas: null}`, ""},
		{`{name: null}`, "- name: Required value\n"},
		{`{name: 'web', replicas: 0}`, "\n- replicas: Invalid value: 0: replicas in body should be greater than or equal to 1\n"},
		{`{replicas: 1.5}`, "- name: Required value\n"},
		{`{name: 'web', typo: 1, limits: {cpu: '1', mem: '1'}}`, ":\n- limits.mem: unknown field\n- typo: unknown field\n"},
		// obj is a spec, so these are unknown fields too, not the fields of a resource.
		{`{name: 'web', apiVersion: 'v1', kind: 'Foo', metadata: {}}`, ":\n- apiVersion: unknown field\n- kind: unknown field\n- metadata: unknown field\n"},
		{`{name: 'web', labels: {a: 1}}`, "- labels.a: Invalid value: \"integer\": labels.a in body must be of type string"},
		{`{name: 'web', ports: [{port: 80}, {port: 80, name: 'x'}]}`, "- ports[1]: Duplicate value: {\"port\":80}\n"},
		// The rule sees the default of max, 10.
		{`{name: 'web', range: {min: 11}}`, "- range: Invalid value: failed rule: !has(self.min) || self.min <= self.max\n"},
		// As in the API server, the rules don't run while there are errors like a missing field.
		{`{range: {min: 11}}`, ":\n- (top): the validation= rules were not checked, because of the other errors\n- name: Required value\n"},
		{`[]`, "- (top): Invalid value: \"array\":  in body must be of type object"},
	} {
		got, err := vm.EvaluateAnonymousSnippet("test", `std.native('validateSimpleSchema')(`+tc.obj+`, std.extVar('schema'), std.extVar('types'))`)
		if tc.want == "" {
			check(t, err, got, "true\n")
		} else if err == nil || !strings.Contains(err.Error(), "RUNTIME ERROR: object is invalid against the SimpleSchema:\n") || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: want an error with %q, got %v", tc.obj, tc.want, err)
		}
	}

	// kro reads a null schema or types as empty. A rule can have line breaks.
	for _, call := range []string{`({}, null, null)`, `({a: 5}, {a: 'integer | validation="self > 0 &&\n self < 10"'}, null)`} {
		got, err := vm.EvaluateAnonymousSnippet("test", `std.native('validateSimpleSchema')`+call)
		check(t, err, got, "true\n")
	}

	// The schema is checked whole, also where the object has nothing.
	for _, tc := range []struct{ schema, types, want string }{
		{`{a: 'Nope'}`, `{}`, "invalid SimpleSchema: field a: unknown type: Nope"},
		{`{ports: '[]Port | listType=map listMapKey=typo'}`, `{Port: {port: 'integer'}}`,
			"invalid SimpleSchema: the API server would reject the CRD that kro makes of it:\n- properties[ports].x-kubernetes-list-map-keys: Invalid value: [\"typo\"]: entries must all be names of item properties"},
		{`{a: 'string | validation="self.nope("'}`, `{}`, "- properties[a].x-kubernetes-validations[0].rule: Invalid value"},
		// kro gives range the default {}, which the API server checks without the defaults inside it.
		{`{range: 'Range | validation="self.min <= self.max"'}`, `{Range: {min: 'integer | default=0', max: 'integer | default=10'}}`,
			"- properties[range].default: Invalid value: \"object\": no such key: min evaluating rule: self.min <= self.max"},
	} {
		_, err := vm.EvaluateAnonymousSnippet("test", `std.native('validateSimpleSchema')({}, `+tc.schema+`, `+tc.types+`)`)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: want an error with %q, got %v", tc.schema, tc.want, err)
		}
	}
}

// An assert in a base checks the object as an overlay leaves it.
func TestValidateSimpleSchemaOverlay(t *testing.T) {
	vm := jsonnet.MakeVM()
	RegisterNativeFuncs(vm, NewIdentityResolver())
	RegisterFeatures(vm, features.AllAlpha())
	vm.ExtCode("base", `{
	  schema:: { replicas: 'integer | minimum=1' },
	  settings:: { replicas: 2 },
	  assert std.native('validateSimpleSchema')(self.settings, self.schema, {}),
	  replicas: $.settings.replicas,
	}`)
	got, err := vm.EvaluateAnonymousSnippet("test", `(std.extVar('base') + {settings+: {replicas: 3}}).replicas`)
	check(t, err, got, "3\n")
	if _, err := vm.EvaluateAnonymousSnippet("test", `std.extVar('base') + {settings+: {replicas: 0}}`); err == nil || !strings.Contains(err.Error(), "replicas in body should be greater than or equal to 1") {
		t.Errorf("want the overlay's replicas: 0 to fail the base's assert, got %v", err)
	}
	// The overlay can change the schema too.
	got, err = vm.EvaluateAnonymousSnippet("test", `(std.extVar('base') + {schema+: {replicas: 'integer'}, settings+: {replicas: 0}}).replicas`)
	check(t, err, got, "0\n")
}

func TestArrayReader(t *testing.T) {
	buf := make([]byte, 2)
	var r io.Reader

	assertRead := func(expected_n int, expected_err error, expected []byte) {
		n, err := r.Read(buf)
		if n != expected_n || err != expected_err {
			t.Errorf("(%d, %v) != (%d, %v)", n, err, expected_n, expected_err)
			return
		}
		if !bytes.Equal(expected, buf[:n]) {
			t.Errorf("%v != %v", buf, expected)
		}
	}

	// Shorter than buf
	r = &ArrayReader{[]interface{}{42.0}}
	assertRead(1, nil, []byte{42})
	assertRead(0, io.EOF, nil)

	// Longer than buf
	r = &ArrayReader{[]interface{}{0.0, 1.0, 2.0, 3.0, 255.0}}
	assertRead(2, nil, []byte{0, 1})
	assertRead(2, nil, []byte{2, 3})
	assertRead(1, nil, []byte{255})
	assertRead(0, io.EOF, nil)

	// Bad numbers
	r = &ArrayReader{[]interface{}{256.0}}
	assertRead(0, errBadByte, nil)

	// Not numbers
	r = &ArrayReader{[]interface{}{"bogus"}}
	assertRead(0, errBadByte, nil)
}

func TestFeature(t *testing.T) {
	vm := jsonnet.MakeVM()
	RegisterNativeFuncs(vm, NewIdentityResolver())
	if _, err := vm.EvaluateAnonymousSnippet("feature", `std.native("requireFeature")("knit")`); err == nil || !strings.Contains(err.Error(), "--features=knit") {
		t.Errorf("knit is off by default; got %v", err)
	}
	RegisterFeatures(vm, features.AllAlpha())
	x, err := vm.EvaluateAnonymousSnippet("feature", `std.native("requireFeature")("knit")`)
	check(t, err, x, "true\n")
	if _, err = vm.EvaluateAnonymousSnippet("feature", `std.native("requireFeature")("nope")`); err == nil {
		t.Error("an unknown feature should be an error")
	}
}
