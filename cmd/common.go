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

	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const (
	flagExec    = "exec"
	flagOverlay = "overlay"
)

type commonFlagOpts struct {
	noShortEval bool
}

type commonEvalFlagOpt func(opts *commonFlagOpts)

func withoutShortEvalFlag() commonEvalFlagOpt {
	return func(opts *commonFlagOpts) {
		opts.noShortEval = true
	}
}

// Most commands evaluate jsonnet files and expose flags to control
// how to evaluate them. We cannot put those flags in the root command because
// we also have commands that wouldn't honour them.
func addCommonEvalFlags(flags *flag.FlagSet, opt ...commonEvalFlagOpt) {
	var opts commonFlagOpts
	for _, o := range opt {
		o(&opts)
	}

	shortEval := "e"
	if opts.noShortEval {
		shortEval = ""
	}
	flags.StringP(flagExec, shortEval, "", "Inline code") // like `jsonnet -e`
	flags.String(flagOverlay, "", "Jsonnet file to compose to each of the input files")
}

func processCommonEvalFlags(flags *flag.FlagSet, args *[]string) error {
	exec, err := flags.GetString(flagExec)
	if err != nil {
		return err
	}
	if exec != "" {
		*args = append(*args, toDataURL(exec))
	}

	overlay, err := flags.GetString(flagOverlay)
	if err != nil {
		return err
	}
	if overlay != "" {
		alpha := viper.GetBool(flagAlpha)
		if !alpha {
			return fmt.Errorf("--%s is an alpha feature please use --%s", flagOverlay, flagAlpha)
		}
		overlayExpression := func(src string) string {
			return toDataURL(fmt.Sprintf(`(import %q) + (import %q)`, src, overlay))
		}
		for i := range *args {
			(*args)[i] = overlayExpression((*args)[i])
		}
	}
	return nil
}
