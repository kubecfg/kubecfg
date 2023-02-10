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

package oci

import (
	"context"

	"github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/config/credentials"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
)

// NewAuthenticatedRepository returns a new authenticated remote.Repository
func NewAuthenticatedRepository(ref string) (*remote.Repository, error) {
	repo, err := remote.NewRepository(ref)
	if err != nil {
		return nil, err
	}
	repo.Client, err = ociAuthClient()
	if err != nil {
		return nil, err
	}
	return repo, nil
}

// taken and adapted from oras internal package
func ociAuthConfig() (*configfile.ConfigFile, error) {
	cfg, err := config.Load(config.Dir())
	if err != nil {
		return nil, err
	}
	if !cfg.ContainsAuth() {
		cfg.CredentialsStore = credentials.DetectDefaultStore(cfg.CredentialsStore)
	}
	return cfg, nil
}

// taken and adapted from oras internal package
func ociAuthClient() (*auth.Client, error) {
	conf, err := ociAuthConfig()
	if err != nil {
		return nil, err
	}
	cli := &auth.Client{
		Credential: func(ctx context.Context, registry string) (auth.Credential, error) {
			authConf, err := conf.GetCredentialsStore(registry).Get(registry)
			if err != nil {
				return auth.EmptyCredential, err
			}
			cred := auth.Credential{
				Username:     authConf.Username,
				Password:     authConf.Password,
				AccessToken:  authConf.RegistryToken,
				RefreshToken: authConf.IdentityToken,
			}
			if cred != auth.EmptyCredential {
				return cred, nil
			}

			return auth.EmptyCredential, nil
		},
	}
	return cli, nil
}
