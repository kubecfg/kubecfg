// Copyright 2018 The kubecfg authors
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
	"path/filepath"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
)

const (
	flagShell = "shell"
)

func guessShell(path string) string {
	ret := filepath.Base(path)
	ret = strings.TrimRightFunc(ret, unicode.IsNumber)
	return ret
}

func init() {
	cmd := completionCmd
	RootCmd.AddCommand(cmd)
	cmd.PersistentFlags().String(flagShell, "", "Shell variant for which to generate completions.  Supported values are bash,zsh")
}

var completionCmd = &cobra.Command{
	Use:   "completion",
	Short: "Generate shell completions for kubecfg",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		flags := cmd.Flags()

		shell, err := flags.GetString(flagShell)
		if err != nil {
			return err
		}
		if shell == "" {
			shell = guessShell(os.Getenv("SHELL"))
		}

		out := cmd.OutOrStdout()

		switch shell {
		case "bash":
			if err := RootCmd.GenBashCompletion(out); err != nil {
				return err
			}
		case "zsh":
			// See https://github.com/spf13/cobra/issues/1529
			// using the same workaround as kubectl https://github.com/kubernetes/kubernetes/blob/bf8c918e0be340637e63520eb3b58df493a230f9/staging/src/k8s.io/kubectl/pkg/cmd/completion/completion.go#L174-L175
			zshHead := "#compdef kubecfg\n#compdef _kubecfg kubecfg\n"
			out.Write([]byte(zshHead))
			if err := RootCmd.GenZshCompletion(out); err != nil {
				return err
			}
		case "fish":
			if err := RootCmd.GenFishCompletion(out, true); err != nil {
				return err
			}
		default:
			return fmt.Errorf("Unknown shell %q, try --%s", shell, flagShell)
		}

		return nil
	},
}
