// Copyright 2023 The kubecfg authors
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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func ToDataURL(code string) string {
	return fmt.Sprintf("data:,%s", url.PathEscape(code))
}

type dataURIImporter struct {
}

func newDataURIImporter() *dataURIImporter {
	return &dataURIImporter{}
}

func (o *dataURIImporter) RoundTrip(req *http.Request) (*http.Response, error) {
	content, _, err := expandDataURL(req.URL)
	if err != nil {
		return nil, err
	}

	return simpleHTTPResponse(req, http.StatusOK, io.NopCloser(strings.NewReader(content))), nil
}

func expandDataURL(pathURL *url.URL) (string, string, error) {
	encoding, data, commaFound := strings.Cut(pathURL.Opaque, ",")
	if !commaFound {
		return "", "", fmt.Errorf("invalid data url: missing ','")
	}
	if encoding != "" {
		return "", "", fmt.Errorf("unsupported encoding %q", encoding)
	}
	content, err := url.PathUnescape(data)
	if err != nil {
		return "", "", err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", err
	}

	foundAt, err := PathToURL(cwd)
	if err != nil {
		return "", "", err
	}
	foundAt += "/"

	return content, foundAt, nil
}
