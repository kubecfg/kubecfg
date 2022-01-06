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
	"testing"

	jsonnet "github.com/google/go-jsonnet"
	log "github.com/sirupsen/logrus"
)

// check there is no err, and a == b.
func check(t *testing.T, err error, actual, expected string) {
	if err != nil {
		t.Errorf("Expected %q, got error: %q", expected, err.Error())
	} else if actual != expected {
		t.Errorf("Expected %q, got %q", expected, actual)
	}
}

func TestParseJson(t *testing.T) {
	vm := jsonnet.MakeVM()
	RegisterNativeFuncs(vm, NewIdentityResolver(), false)

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
	RegisterNativeFuncs(vm, NewIdentityResolver(), false)

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
	RegisterNativeFuncs(vm, NewIdentityResolver(), false)

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
	RegisterNativeFuncs(vm, NewIdentityResolver(), false)

	_, err := vm.EvaluateSnippet("failtest", `std.native("regexSubst")("[f",s "foo", "bar")`)
	if err == nil {
		t.Errorf("regexSubst succeeded with invalid regex")
	}

	x, err := vm.EvaluateSnippet("test", `std.native("regexSubst")("a(x*)b", "-ab-axxb-", "T")`)
	check(t, err, x, "\"-T-T-\"\n")

	x, err = vm.EvaluateSnippet("test", `std.native("regexSubst")("a(x*)b", "-ab-axxb-", "${1}W")`)
	check(t, err, x, "\"-W-xxW-\"\n")
}

func TestHelmTemplate(t *testing.T) {
	log.SetLevel(log.DebugLevel)

	vm := jsonnet.MakeVM()
	RegisterNativeFuncs(vm, NewIdentityResolver(), false)

	_, err := vm.EvaluateSnippet("failtest", `std.native("helmTemplate")("rls", "myns", "not_a_url", {})`)
	if err == nil {
		t.Errorf("helmTemplate succeeded with invalid URL")
	}

	_, err = vm.EvaluateSnippet("failtest", `std.native("helmTemplate")("myrls", "myns", "../testdata/kubernetes-dashboard-5.0.0.tgz", {})`)
	if err == nil {
		t.Errorf("helmTemplate succeeded with relative URL")
	}

	vm = jsonnet.MakeVM()
	vm.Importer(MakeUniversalImporter(nil))
	RegisterNativeFuncs(vm, NewIdentityResolver(), true /* allowRelativeURLs */)

	x, err := vm.EvaluateSnippet("test", `
    local chrt = std.native("helmTemplate")("myrls", "myns", "../testdata/kubernetes-dashboard-5.0.0.tgz", {replicaCount: 7});
    local sa = chrt["kubernetes-dashboard/charts/metrics-server/templates/metrics-server-serviceaccount.yaml"];
    local d = chrt["kubernetes-dashboard/templates/deployment.yaml"];
    [
      // Uses releaseName arg, from a nested chart
      sa.metadata.name,
      // namespace arg
      sa.metadata.namespace,
      // Provided value
      d.spec.replicas,
      // Default value
      d.spec.template.spec.containers[0].image,
    ]
`)
	check(t, err, x, "[\n   \"myrls-metrics-server\",\n   \"myns\",\n   7,\n   \"kubernetesui/dashboard:v2.3.1\"\n]\n")
}
