package yamloc

import (
	"fmt"
	"testing"
)

func TestLineToPath(t *testing.T) {
	src := `apiVersion: v1
kind: Deployment
spec:
  selector:
    matchLabels:
      foo: bar
  template:
    spec:
      containers:
      - name: foo
        env:
        - name: BAR
          value: baz
        - name: QUX
          value: QUX
      - name: bar
        env:
        - name: BAR2
          value: baz2
        - name: QUX2
          value: QUX2
  replicas: 1
  someArrayOfScalars:
  - foo
`

	testCases := []struct {
		line int
		want string
	}{
		{1, "$.apiVersion"},
		{2, "$.kind"},
		{3, "$.spec"},
		{4, "$.spec.selector"},
		{5, "$.spec.selector.matchLabels"},
		{6, "$.spec.selector.matchLabels.foo"},
		{7, "$.spec.template"},
		{8, "$.spec.template.spec"},
		{9, "$.spec.template.spec.containers"},
		{10, "$.spec.template.spec.containers[0].name"},
		{11, "$.spec.template.spec.containers[0].env"},
		{12, "$.spec.template.spec.containers[0].env[0].name"},
		{13, "$.spec.template.spec.containers[0].env[0].value"},
		{14, "$.spec.template.spec.containers[0].env[1].name"},
		{15, "$.spec.template.spec.containers[0].env[1].value"},
		{16, "$.spec.template.spec.containers[1].name"},
		{17, "$.spec.template.spec.containers[1].env"},
		{18, "$.spec.template.spec.containers[1].env[0].name"},
		{19, "$.spec.template.spec.containers[1].env[0].value"},
		{20, "$.spec.template.spec.containers[1].env[1].name"},
		{21, "$.spec.template.spec.containers[1].env[1].value"},
		{22, "$.spec.replicas"},
		{23, "$.spec.someArrayOfScalars"},
		{24, "$.spec.someArrayOfScalars[0]"},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			got, err := LineToPath([]byte(src), tc.line)
			if err != nil {
				t.Fatal(err)
			}
			if want := tc.want; got != want {
				t.Errorf("got: %q, want: %q", got, want)
			}
		})
	}
}
