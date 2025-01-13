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
	"context"
	"fmt"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// ImageName represents the parts of a docker image name
type ImageName struct {
	// eg: "myregistryhost:5000/fedora/httpd:version1.0"
	Registry   string // "myregistryhost:5000"
	Repository string // "fedora/httpd"
	Tag        string // "version1.0"
	Digest     string
}

// String implements the Stringer interface
func (n ImageName) String() string {
	buf := bytes.Buffer{}
	if n.Registry != "" {
		buf.WriteString(n.Registry)
		buf.WriteString("/")
	}
	if n.Repository != "" {
		buf.WriteString(n.Repository)
	}
	if n.Digest != "" {
		buf.WriteString("@")
		buf.WriteString(n.Digest)
	} else {
		buf.WriteString(":")
		buf.WriteString(n.Tag)
	}
	return buf.String()
}

// ParseImageName parses a docker image into an ImageName struct.
func ParseImageName(image string) (ImageName, error) {
	ret := ImageName{}

	ref, err := name.ParseReference(image)
	if err != nil {
		return ret, fmt.Errorf("parsing reference %q: %w", image, err)
	}

	if t, ok := ref.(name.Tag); ok {
		ret.Registry = t.RegistryStr()
		ret.Repository = t.RepositoryStr()
		ret.Tag = t.TagStr()
	}

	if d, ok := ref.(name.Digest); ok {
		ret.Registry = d.RegistryStr()
		ret.Repository = d.RepositoryStr()
		ret.Digest = d.DigestStr()
	}

	return ret, nil
}

// Resolver is able to resolve docker image names into more specific forms
type Resolver interface {
	Resolve(image *ImageName) error
}

// NewIdentityResolver returns a resolver that does only trivial
// :latest canonicalisation
func NewIdentityResolver() Resolver {
	return identityResolver{}
}

type identityResolver struct{}

func (r identityResolver) Resolve(image *ImageName) error {
	return nil
}

// NewRegistryResolver returns a resolver that looks up a docker
// registry to resolve digests
func NewRegistryResolver() Resolver {
	return &registryResolver{
		cache: make(map[string]string),
	}
}

type registryResolver struct {
	cache map[string]string
}

func (r *registryResolver) Resolve(n *ImageName) error {
	// TODO: get context from caller.
	ctx := context.Background()

	if n.Digest != "" {
		// Already has explicit digest
		return nil
	}

	image := n.String()
	if digest, ok := r.cache[image]; ok {
		n.Digest = digest
		return nil
	}

	ref, err := name.ParseReference(image)
	if err != nil {
		return fmt.Errorf("parsing reference %q: %w", image, err)
	}

	dsc, err := remote.Get(ref,
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
	)
	if err != nil {
		return fmt.Errorf("fetching manifest of %q: %w", image, err)
	}

	n.Digest = dsc.Digest.String()
	r.cache[image] = n.Digest

	return nil
}
