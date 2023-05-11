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

package cmd

import (
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kubecfg/kubecfg/pkg/kubecfg"
)

const (
	flagCreate          = "create"
	flagSkipGc          = "skip-gc"
	flagGcTag           = "gc-tag"
	flagGcTagsFromInput = "gc-tags-from-input"
	flagGcAllNs         = "gc-all-namespaces"
	flagDryRun          = "dry-run"
	flagValidate        = "validate"
)

func init() {
	cmd := updateCmd
	RootCmd.AddCommand(cmd)
	cmd.PersistentFlags().Bool(flagCreate, true, "Create missing resources")
	cmd.PersistentFlags().Bool(flagSkipGc, false, "Don't perform garbage collection, even with --"+flagGcTag)
	cmd.PersistentFlags().String(flagGcTag, "", "Add this tag to updated objects, and garbage collect existing objects with this tag and not in config")
	cmd.PersistentFlags().Bool(flagGcTagsFromInput, false, "Garbage collect existing objects not in input and with any of the gc-tags present in input")
	cmd.PersistentFlags().Bool(flagGcAllNs, true, "Ignore namespace scope for garbage collection")
	cmd.PersistentFlags().Bool(flagDryRun, false, "Perform only read-only operations")
	cmd.PersistentFlags().Bool(flagValidate, true, "Validate input against server schema")
	cmd.PersistentFlags().Bool(flagIgnoreUnknown, false, "Don't fail validation if the schema for a given resource type is not found")

	addCommonEvalFlags(cmd)
}

var updateCmd = &cobra.Command{
	Use:         "update",
	Short:       "Update Kubernetes resources with local config",
	Args:        cobra.ArbitraryArgs,
	Annotations: map[string]string{evalCmdAnno: ""},
	RunE: func(cmd *cobra.Command, args []string) error {
		flags := cmd.Flags()
		var err error
		c := kubecfg.UpdateCmd{}

		validate, err := flags.GetBool(flagValidate)
		if err != nil {
			return err
		}

		c.Create, err = flags.GetBool(flagCreate)
		if err != nil {
			return err
		}

		c.GcTag, err = flags.GetString(flagGcTag)
		if err != nil {
			return err
		}

		c.GcTagsFromInput, err = flags.GetBool(flagGcTagsFromInput)
		if err != nil {
			return err
		}

		c.SkipGc, err = flags.GetBool(flagSkipGc)
		if err != nil {
			return err
		}

		c.DryRun, err = flags.GetBool(flagDryRun)
		if err != nil {
			return err
		}

		c.Client, c.Mapper, c.Discovery, err = getDynamicClients(cmd)
		if err != nil {
			return err
		}

		c.DefaultNamespace, err = defaultNamespace(clientConfig)
		if err != nil {
			return err
		}

		gcAllNamespaces, err := flags.GetBool(flagGcAllNs)
		if err != nil {
			return err
		} else if gcAllNamespaces {
			c.GcNamespace = metav1.NamespaceAll
		} else {
			c.GcNamespace = c.DefaultNamespace
		}

		objs, err := readObjs(cmd, args)
		if err != nil {
			return err
		}

		if validate {
			v := kubecfg.ValidateCmd{
				Mapper:    c.Mapper,
				Discovery: c.Discovery,
			}

			v.IgnoreUnknown, err = flags.GetBool(flagIgnoreUnknown)
			if err != nil {
				return err
			}

			if err := v.Run(objs, cmd.OutOrStdout()); err != nil {
				return err
			}
		}

		return c.Run(cmd.Context(), objs)
	},
}
