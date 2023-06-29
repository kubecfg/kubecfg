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
	"fmt"
	"sort"

	"github.com/kubecfg/kubecfg/pkg/kubecfg"
	"github.com/kubecfg/kubecfg/utils"
	"github.com/spf13/cobra"
)

const (
	flagFormat               = "format"
	flagExportDir            = "export-dir"
	flagExportFileNameFormat = "export-filename-format"
	flagExportFileNameExt    = "export-filename-extension"
	flagShowProvenance       = "show-provenance"
	flagReorder              = "reorder"
)

func init() {
	cmd := showCmd
	RootCmd.AddCommand(cmd)
	cmd.PersistentFlags().StringP(flagFormat, "o", "yaml", "Output format.  Supported values are: json, yaml")
	cmd.PersistentFlags().String(flagExportDir, "", "Split yaml stream into multiple files and write files into a directory. If the directory exists it must be empty.")
	cmd.PersistentFlags().String(flagExportFileNameFormat, kubecfg.DefaultFileNameFormat, "Go template expression used to render path names for resources.")
	cmd.PersistentFlags().String(flagExportFileNameExt, "", fmt.Sprintf("Override the file extension used when creating filenames when using %s", flagExportFileNameFormat))
	cmd.PersistentFlags().Bool(flagShowProvenance, false, "Add provenance annotations showing the file and the field path to each rendered k8s object")
	cmd.PersistentFlags().String(flagReorder, "", "--reorder=server: Reorder resources like the 'update' command does. --reorder=client: TODO")

	addCommonEvalFlags(cmd)
}

var showCmd = &cobra.Command{
	Use:   "show",
	Short: "Show expanded resource definitions",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		flags := cmd.Flags()

		outputFormat, err := flags.GetString(flagFormat)
		if err != nil {
			return err
		}
		exportDir, err := flags.GetString(flagExportDir)
		if err != nil {
			return err
		}
		exportFileNameFormat, err := flags.GetString(flagExportFileNameFormat)
		if err != nil {
			return err
		}
		exportFileNameExt, err := flags.GetString(flagExportFileNameExt)
		if err != nil {
			return err
		}
		reorder, err := flags.GetString(flagReorder)
		if err != nil {
			return err
		}

		c, err := kubecfg.NewShowCmd(outputFormat, exportDir, exportFileNameFormat, exportFileNameExt)
		if err != nil {
			return err
		}

		showProvenance, err := flags.GetBool(flagShowProvenance)
		if err != nil {
			return err
		}

		objs, err := readObjs(cmd, args, utils.WithProvenance(showProvenance))
		if err != nil {
			return err
		}

		switch reorder {
		case "":
			// no reordering
		case "server":
			_, mapper, discovery, err := getDynamicClients(cmd)
			if err != nil {
				return err
			}

			depOrder, err := utils.DependencyOrder(discovery, mapper, objs)
			if err != nil {
				return err
			}
			sort.Sort(depOrder)
		default:
			return fmt.Errorf("unsupported %q reordering", reorder)
		}

		return c.Run(objs, cmd.OutOrStdout())
	},
}
