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

	"github.com/spf13/cobra"
)

const (
	flagExec            = "exec"
	flagOverlay         = "overlay" // deprecated
	flagOverlayCode     = "overlay-code"
	flagOverlayCodeFile = "overlay-code-file"

	evalCmdAnno = "evalCmd"
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
func addCommonEvalFlags(cmd *cobra.Command, opt ...commonEvalFlagOpt) {
	flags := cmd.PersistentFlags()

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
	flags.MarkDeprecated(flagOverlay, fmt.Sprintf("please use %s instead", flagOverlayCodeFile))
	flags.String(flagOverlayCode, "", "Inline Jsonnet code to compose to each of the input files")
	flags.String(flagOverlayCodeFile, "", "Jsonnet file to compose to each of the input files")
	cmd.MarkFlagsMutuallyExclusive(flagOverlay, flagOverlayCode, flagOverlayCodeFile)
}
