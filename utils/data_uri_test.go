package utils

import (
	"fmt"
	"net/url"
	"testing"
)

func TestExpandDataURL(t *testing.T) {
	testCases := []struct {
		url     string
		content string
		wantErr error
	}{
		{"data:,foo", "foo", nil},
		{"data:text/plain,foo", "", fmt.Errorf("unsupported encoding %q", "text/plain")},
		{"data:foo", "", fmt.Errorf("invalid data url: missing ','")},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			u, err := url.Parse(tc.url)
			if err != nil {
				t.Fatal(err)
			}

			content, _, err := expandDataURL(u)
			if got, want := fmt.Sprint(err), fmt.Sprint(tc.wantErr); got != want {
				t.Errorf("got: %q, want: %q", got, want)
			} else {
				if got, want := content, tc.content; got != want {
					t.Errorf("got: %q, want: %q", got, want)
				}
			}
		})
	}
}
