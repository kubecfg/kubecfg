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

package cmd

import (
	"fmt"

	"github.com/kubecfg/kubecfg/pkg/kubecfg"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	flagOutput           = "output"
	flagInsecureRegistry = "insecure-registry"
	flagDocsTarFile      = "docs-tar-file"
)

func init() {
	cmd := packCmd
	RootCmd.AddCommand(cmd)
	cmd.PersistentFlags().String(flagOutput, "", "Output archive file. Don't push to OCI but just dump into a file")
	cmd.PersistentFlags().Bool(flagInsecureRegistry, false, "Use HTTP instead of HTTPS to access the OCI registry")
	cmd.PersistentFlags().String(flagDocsTarFile, "", "Optional tar.gz file containing a documentation bundle")
}

var packCmd = &cobra.Command{
	Use:   "pack [flags] oci_package root_jsonnet_file",
	Short: "Create and push an OCI package",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		alpha := viper.GetBool(flagAlpha)
		if !alpha {
			return fmt.Errorf("eval is an alpha feature, please use --alpha")
		}

		vm, err := JsonnetVM(cmd)
		if err != nil {
			return err
		}

		flags := cmd.Flags()
		c := kubecfg.PackCmd{}

		c.OutputFile, err = flags.GetString(flagOutput)
		if err != nil {
			return err
		}

		c.InsecureRegistry, err = flags.GetBool(flagInsecureRegistry)
		if err != nil {
			return err
		}

		c.DocsTarFile, err = flags.GetString(flagDocsTarFile)
		if err != nil {
			return err
		}

		return c.Run(cmd.Context(), vm, args[0], args[1])
	},
}
