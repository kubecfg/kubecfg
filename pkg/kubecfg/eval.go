package kubecfg

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/go-jsonnet"
	"github.com/kubecfg/kubecfg/utils"
	"github.com/kubecfg/yaml/v2"
)

// EvalCmd represents the eval subcommand
type EvalCmd struct {
	Expr     string
	ShowKeys bool
	Format   string
}

func (c EvalCmd) Run(ctx context.Context, vm *jsonnet.VM, path string) error {
	expr := c.Expr
	if expr == "" {
		expr = "$"
	}
	pathURL, err := utils.PathToFileURL(path)
	if err != nil {
		return err
	}
	eval := fmt.Sprintf("((import %q) { Z____expr__Z_:: %s}).Z____expr__Z_", pathURL, expr)

	if c.ShowKeys {
		eval = fmt.Sprintf("std.objectFields(%s)", eval)
	}

	jsonstr, err := vm.EvaluateAnonymousSnippet(path, eval)
	if err != nil {
		return err
	}

	switch c.Format {
	case "json":
		fmt.Println(jsonstr)
	case "yaml":
		var jsontree interface{}
		if err := json.Unmarshal([]byte(jsonstr), &jsontree); err != nil {
			return err
		}
		b, err := yaml.Marshal(jsontree)
		if err != nil {
			return err
		}
		fmt.Println(string(b))
	default:
		return fmt.Errorf("unsupported format %q", c.Format)
	}

	return nil
}
