// Copyright 2026 The kubecfg authors
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

package cmd

import (
	"encoding/json"
	"testing"

	"github.com/kubecfg/kubecfg/pkg/features"
)

func TestFeatures(t *testing.T) {
	t.Setenv("KUBECFG_FEATURES", "yakl,pack")
	for _, tc := range []struct {
		args []string
		want map[string]string // name -> source of the enabled features
	}{
		{nil, map[string]string{"yakl": "--features", "pack": "--features"}},
		// the flag replaces the env var, as for any flag
		{[]string{"--features=eval", "--features", "overlay"}, map[string]string{"eval": "--features", "overlay": "--features"}},
		{[]string{"--alpha", "--features=httpd=false"}, map[string]string{
			"eval": "--alpha", "traceback": "--alpha", "pack": "--alpha", "overlay": "--alpha",
			"yakl": "--alpha", "kustomize-import": "--alpha",
		}},
	} {
		var list []features.Status
		if err := json.Unmarshal([]byte(cmdOutput(t, append([]string{"features", "-o", "json"}, tc.args...))), &list); err != nil {
			t.Fatal(err)
		}
		got := map[string]string{}
		for _, s := range list {
			if s.Enabled {
				got[s.Name] = s.Source
			}
		}
		if len(got) != len(tc.want) {
			t.Errorf("%v: got %v, want %v", tc.args, got, tc.want)
		}
		for name, source := range tc.want {
			if got[name] != source {
				t.Errorf("%v: %s: got source %q, want %q", tc.args, name, got[name], source)
			}
		}
	}
}

func TestFeatureRequired(t *testing.T) {
	defer resetFlags()
	RootCmd.SetArgs([]string{"--features=yakl", "eval", "-e", "1"})
	err := RootCmd.Execute()
	if want := "eval is an alpha feature; turn it on with --features=eval"; err == nil || err.Error() != want {
		t.Errorf("got %v, want %q", err, want)
	}
}
