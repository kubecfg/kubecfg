package kubecfg

import (
	"fmt"
	"testing"
)

func TestFindCommonPathPrefix(t *testing.T) {
	testCases := []struct {
		paths []string
		want  string
	}{
		{
			[]string{"/foo/bar/a/b", "/foo/bar/a/c", "/foo/bar/c/a"},
			"/foo/bar/",
		},
		{
			[]string{"/foo/bar/a/b", "/foo/bar/a/c", "/foo/bar/c/a", "/foo/zar/c/a"},
			"/foo/",
		},
		{
			[]string{"/foo/bar/a/b", "/foo/bar/a/c", "/foo/bar/c/a", "/fox/zar/c/a"},
			"/",
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			got := findCommonPathPrefix(tc.paths)
			if want := tc.want; got != want {
				t.Errorf("got: %q, want: %q", got, want)
			}
		})
	}
}
