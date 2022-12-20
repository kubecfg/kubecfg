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

	"github.com/kubecfg/kubecfg/pkg/kubecfg"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	flagListenAddr = "listen-addr"
)

func init() {
	RootCmd.AddCommand(httpdCmd)
	httpdCmd.PersistentFlags().StringP(flagListenAddr, "l", ":8080", "address:port to listen")
}

var httpdCmd = &cobra.Command{
	Use:   "httpd",
	Short: "process https requests with jsonnet",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		alpha := viper.GetBool(flagAlpha)
		if !alpha {
			return fmt.Errorf("httpd is an alpha feature, please use --alpha")
		}

		flags := cmd.Flags()
		var err error
		c := kubecfg.HttpdCmd{}

		c.ListenAddr, err = flags.GetString(flagListenAddr)
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

		return c.Run(cmd.Context(), vm, args)
	},
}
