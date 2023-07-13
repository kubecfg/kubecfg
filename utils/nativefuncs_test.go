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

	_, err := vm.EvaluateSnippet("failtest", `std.native("parseHelmChart")("not_data", "rls", "myns", {})`)
	if err == nil {
		t.Errorf("helmTemplate succeeded with invalid data")
	}

	_, err = vm.EvaluateSnippet("failtest", `std.native("parseHelmChart")([1, 2, 3, 256], "myrls", "myns", {})`)
	if err == nil {
		t.Errorf("helmTemplate succeeded with invalid bytes")
	}

	vm = jsonnet.MakeVM()
	RegisterNativeFuncs(vm, NewIdentityResolver())

	x, err := vm.EvaluateSnippet("test", `
    local chrt = std.native("parseHelmChart")(import "../testdata/mysql-8.8.26.tgz.bin", "myrls", "myns", {primary: {resources: {limits: {cpu: "2"}}}});
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
