package utils

import "testing"

func TestParseImageName(t *testing.T) {
	tests := []struct {
		input   string
		want    ImageName
		str     string
		wantErr bool
	}{
		{input: "nginx", want: ImageName{Registry: "docker.io", Repository: "library/nginx", Tag: "latest"}, str: "docker.io/library/nginx:latest"},
		{input: "fedora/httpd:1.0", want: ImageName{Registry: "docker.io", Repository: "fedora/httpd", Tag: "1.0"}, str: "docker.io/fedora/httpd:1.0"},
		{input: "myhost:5000/fedora/httpd:version1.0", want: ImageName{Registry: "myhost:5000", Repository: "fedora/httpd", Tag: "version1.0"}, str: "myhost:5000/fedora/httpd:version1.0"},
		{
			input: "gcr.io/foo/bar@sha256:0000000000000000000000000000000000000000000000000000000000000000",
			want:  ImageName{Registry: "gcr.io", Repository: "foo/bar", Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000"},
			str:   "gcr.io/foo/bar@sha256:0000000000000000000000000000000000000000000000000000000000000000",
		},
		{input: "UPPER/case", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := ParseImageName(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
			if s := got.String(); s != tc.str {
				t.Errorf("String() = %q, want %q", s, tc.str)
			}
		})
	}
}
