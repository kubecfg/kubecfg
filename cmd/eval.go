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

	"github.com/google/go-jsonnet"
	"github.com/kubecfg/kubecfg/pkg/features"
	"github.com/kubecfg/kubecfg/pkg/kubecfg"
	"github.com/kubecfg/kubecfg/pkg/yakl"
	"github.com/kubecfg/kubecfg/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
	cmd.PersistentFlags().StringP(flagFormat, "o", "yaml", "Output format.  Supported values are: json, yaml, jsonnet (the unevaluated code of a .yakl file)")
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
	Use:         "eval",
	Short:       "eval jsonnet expression",
	Args:        cobra.ArbitraryArgs,
	Annotations: map[string]string{evalCmdAnno: ""},
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := gates.Require(features.Eval); err != nil {
			return err
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

		if c.Format == "jsonnet" {
			if c.Expr != "" || c.ShowKeys || c.Trace {
				return fmt.Errorf("-o jsonnet prints the file's code without evaluating it, so it can't be combined with -e, -k or --trace")
			}
			return printJsonnet(cmd, vm, args[0])
		}

		tla, err := tlaNames(flags)
		if err != nil {
			return err
		}

		return c.Run(cmd.Context(), vm, args[0], tla)
	},
}

// printJsonnet prints the code that kubecfg evaluates for the file at path, without evaluating it,
// so it works when evaluating fails. A yakl file's code is formatted to be read: names become
// fields, anchors and keys like '&x' become locals, key modifiers become +:, :: and :::, and YAML
// comments become // comments. kubecfg evaluates the same code, laid out to keep the source lines.
func printJsonnet(cmd *cobra.Command, vm *jsonnet.VM, file string) error {
	// Through the importer, like the other commands, so URLs and /dev/stdin work too.
	path, err := utils.PathToURL(file)
	if err != nil {
		return err
	}
	src, _, err := vm.ImportData(path, path)
	if err != nil {
		return err
	}
	if utils.ImportPath(path) == path { // not a yakl file, so it's its own code
		fmt.Fprint(cmd.OutOrStdout(), src)
		return nil
	}
	code, err := yakl.Desugar(src)
	if err != nil && code == "" {
		return fmt.Errorf("%s: %w", file, err)
	}
	// Unformatted code that doesn't parse still shows what the file means.
	fmt.Fprintln(cmd.OutOrStdout(), strings.TrimSuffix(code, "\n"))
	if err != nil {
		return fmt.Errorf("%s:%w", file, err) // a Jsonnet syntax error, at "line:column"
	}
	return nil
}
