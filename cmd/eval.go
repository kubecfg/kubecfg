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
	"strings"

	"github.com/kubecfg/kubecfg/pkg/kubecfg"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const (
	flagExpr     = "expr"
	flagShowKeys = "show-keys"
	flagTrace    = "trace"
)

func init() {
	cmd := evalCmd
	RootCmd.AddCommand(cmd)
	cmd.PersistentFlags().StringP(flagExpr, "e", "", "jsonnet expression to evaluate")
	cmd.PersistentFlags().BoolP(flagShowKeys, "k", false, "instead of rendering an object, list it's keys")
	cmd.PersistentFlags().StringP(flagFormat, "o", "yaml", "Output format.  Supported values are: json, yaml")
	cmd.PersistentFlags().Bool(flagTrace, false, "print a causal trace")

	addCommonEvalFlags(cmd, withoutShortEvalFlag())
}

func tlaNames(flags *pflag.FlagSet) ([]string, error) {
	var names []string
	for _, flagName := range []string{flagTLAVar, flagTLAVarFile, flagTLACode, flagTLACodeFile} {
		entries, err := flags.GetStringArray(flagName)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			key, _, _ := strings.Cut(entry, "=")
			names = append(names, key)
		}
	}
	return names, nil
}

var evalCmd = &cobra.Command{
	Use:   "eval",
	Short: "eval jsonnet expression",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		alpha := viper.GetBool(flagAlpha)
		if !alpha {
			return fmt.Errorf("eval is an alpha feature, please use --alpha")
		}

		flags := cmd.Flags()
		var err error
		c := kubecfg.EvalCmd{}

		c.Expr, err = flags.GetString(flagExpr)
		if err != nil {
			return err
		}

		c.Format, err = flags.GetString(flagFormat)
		if err != nil {
			return err
		}

		c.ShowKeys, err = flags.GetBool(flagShowKeys)
		if err != nil {
			return err
		}

		c.Trace, err = flags.GetBool(flagTrace)
		if err != nil {
			return err
		}

		vm, err := JsonnetVM(cmd)
		if err != nil {
			return err
		}

		if len(args) < 1 {
			return fmt.Errorf("jsonnet filename required")
		}

		tla, err := tlaNames(flags)
		if err != nil {
			return err
		}

		return c.Run(cmd.Context(), vm, args[0], tla)
	},
}
