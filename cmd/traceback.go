package cmd

import (
	"fmt"

	"github.com/kubecfg/kubecfg/pkg/kubecfg"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	flagAll = "all"
)

func init() {
	cmd := tracebackCmd
	RootCmd.AddCommand(cmd)
	cmd.PersistentFlags().Bool(flagAll, false, "Do not hide non file:// urls")
}

var tracebackCmd = &cobra.Command{
	Use:   "traceback <filename.yaml>:<line>",
	Short: "Output jsonnet file line locations that may affect a given yaml output",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		alpha := viper.GetBool(flagAlpha)
		if !alpha {
			return fmt.Errorf("eval is an alpha feature, please use --alpha")
		}

		vm, err := JsonnetVM(cmd)
		if err != nil {
			return err
		}

		c := kubecfg.TracebackCmd{}

		flags := cmd.Flags()
		c.ShowAll, err = flags.GetBool(flagAll)
		if err != nil {
			return err
		}

		return c.Run(cmd.Context(), vm, args[0])
	},
}
