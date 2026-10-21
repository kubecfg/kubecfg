// Copyright 2026 The kubecfg authors
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
	"os"

	"github.com/kubecfg/kubecfg/pkg/yakl"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const flagInPlace = "in-place"

func init() {
	RootCmd.AddCommand(yaklfmtCmd)
	yaklfmtCmd.Flags().BoolP(flagInPlace, "i", false, "Rewrite the files instead of printing them")
}

var yaklfmtCmd = &cobra.Command{
	Use:   "yaklfmt <file.yakl>...",
	Short: "Format the Jsonnet expressions of yakl files",
	Long: `Format the Jsonnet code of the ${...} expressions in yakl files, as jsonnetfmt does, and
print the files, or rewrite them with --in-place. The YAML around the expressions stays as it
is; a YAML formatter like prettier keeps a yakl file's meaning. A plain expression whose
formatted code isn't a valid plain YAML scalar gets quoted.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !viper.GetBool(flagAlpha) {
			return fmt.Errorf("yakl is an alpha feature, please use --alpha")
		}
		inPlace, err := cmd.Flags().GetBool(flagInPlace)
		if err != nil {
			return err
		}
		for _, file := range args {
			src, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			out, err := yakl.Format(string(src))
			if err != nil {
				return fmt.Errorf("%s: %w", file, err)
			}
			if !inPlace {
				fmt.Fprint(cmd.OutOrStdout(), out)
				continue
			}
			if out == string(src) {
				continue
			}
			info, err := os.Stat(file)
			if err != nil {
				return err
			}
			if err := os.WriteFile(file, []byte(out), info.Mode().Perm()); err != nil {
				return err
			}
		}
		return nil
	},
}
