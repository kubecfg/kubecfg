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

	"github.com/kubecfg/kubecfg/pkg/kubecfg"
	"github.com/kubecfg/kubecfg/utils"
)

const (
	flagIgnoreUnknown = "ignore-unknown"
	flagRepeatEval    = "repeat-eval"
)

func init() {
	cmd := validateCmd
	RootCmd.AddCommand(cmd)
	cmd.PersistentFlags().Bool(flagIgnoreUnknown, true, "Don't fail if the schema for a given resource type is not found")
	cmd.PersistentFlags().Bool(flagRepeatEval, true, "Repeat evaluation twice to verify idempotency")

	addCommonEvalFlags(cmd)
}

var validateCmd = &cobra.Command{
	Use:         "validate",
	Short:       "Compare generated manifest against server OpenAPI spec",
	Args:        cobra.ArbitraryArgs,
	Annotations: map[string]string{evalCmdAnno: ""},
	RunE: func(cmd *cobra.Command, args []string) error {
		flags := cmd.Flags()
		var err error

		c := kubecfg.ValidateCmd{}

		_, c.Mapper, c.Discovery, err = getDynamicClients(cmd)
		if err != nil {
			return err
		}

		c.IgnoreUnknown, err = flags.GetBool(flagIgnoreUnknown)
		if err != nil {
			return err
		}

		repeatEval, err := flags.GetBool(flagRepeatEval)
		if err != nil {
			return err
		}

		objs, err := readObjs(cmd, args, utils.WithReadTwice(repeatEval))
		if err != nil {
			return err
		}

		return c.Run(objs, cmd.OutOrStdout())
	},
}
