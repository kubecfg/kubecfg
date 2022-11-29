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
	"runtime/debug"

	jsonnet "github.com/google/go-jsonnet"
	"github.com/spf13/cobra"
	"k8s.io/client-go/pkg/version"
)

func init() {
	RootCmd.AddCommand(versionCmd)
}

// Default version if not overriden by build parameters.
const DevVersion = "(dev build)"

// Version is overridden by main
var Version = DevVersion

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		out := cmd.OutOrStdout()
		kubecfgVersion := Version
		if bi, ok := debug.ReadBuildInfo(); ok {
			if v := bi.Main.Version; v != "" && v != "(devel)" {
				kubecfgVersion = v
			}
		}
		fmt.Fprintln(out, "kubecfg version:", kubecfgVersion)
		fmt.Fprintln(out, "jsonnet version:", jsonnet.Version())
		fmt.Fprintln(out, "client-go version:", version.Get())
	},
}
