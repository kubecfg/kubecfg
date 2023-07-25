package kubecfg

import (
	"fmt"
	"net/url"
	"reflect"
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
		{
			[]string{"/foo/bar/file1.txt", "/foo/bar/file1.txt"},
			"/foo/bar/",
		},
		{
			[]string{"/foo/bar/file1.txt"},
			"/foo/bar/",
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

func TestShortNames(t *testing.T) {
	testCases := []struct {
		urls            []string
		rootURL         string
		short           []string
		shortEntrypoint string
	}{
		{
			[]string{"file:///Users/mkm/tmp/dummy.txt", "file:///Users/mkm/tmp/shell.jsonnet"}, "file:///Users/mkm/tmp/shell.jsonnet",
			[]string{"dummy.txt", "shell.jsonnet"}, "shell.jsonnet",
		},
		{
			[]string{"file:///Users/mkm/tmp/shell.jsonnet"}, "file:///Users/mkm/tmp/shell.jsonnet",
			[]string{"shell.jsonnet"}, "shell.jsonnet",
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			var urls []*url.URL
			for _, u := range tc.urls {
				parsed, err := url.Parse(u)
				if err != nil {
					t.Fatal(err)
				}
				urls = append(urls, parsed)
			}
			rootURL, err := url.Parse(tc.rootURL)
			if err != nil {
				t.Fatal(err)
			}

			short, shortEntrypoint := shortNames(urls, rootURL)
			if got, want := short, tc.short; !reflect.DeepEqual(got, want) {
				t.Errorf("got: %q, want: %q", got, want)
			}
			if got, want := shortEntrypoint, tc.shortEntrypoint; got != want {
				t.Errorf("got: %q, want: %q", got, want)
			}
		})
	}

}
