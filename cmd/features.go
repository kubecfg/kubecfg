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
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(featuresCmd)
	featuresCmd.Flags().StringP(flagFormat, "o", "table", "Output format. Supported values are: table, json")
}

var featuresCmd = &cobra.Command{
	Use:   "features",
	Short: "List the feature gates, and which ones are on and why",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		format, err := cmd.Flags().GetString(flagFormat)
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		switch format {
		case "json":
			enc := json.NewEncoder(out)
			enc.SetIndent("", "  ")
			return enc.Encode(gates.List())
		case "table":
			onOff := map[bool]string{true: "on", false: "off"}
			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tSTAGE\tDEFAULT\tENABLED\tSOURCE\tSINCE")
			for _, s := range gates.List() {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", s.Name, s.Stage, onOff[s.Default], onOff[s.Enabled], s.Source, s.Since)
			}
			return w.Flush()
		default:
			return fmt.Errorf("unknown output format %q; want table or json", format)
		}
	},
}
