package kubecfg

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/google/go-jsonnet"
	"github.com/kubecfg/kubecfg/utils"
	"github.com/kubecfg/ursonnet"
	"github.com/kubecfg/yaml/v2"
)

// EvalCmd represents the eval subcommand
type EvalCmd struct {
	Expr     string
	ShowKeys bool
	Format   string
	Trace    bool
}

func (c EvalCmd) Run(ctx context.Context, vm *jsonnet.VM, path string, tla []string) error {
	expr := c.Expr
	if expr == "" {
		expr = "$"
	}

	pathURL, err := utils.PathToURL(path)
	if err != nil {
		return err
	}
	var eval string
	if err != nil {
		return err
	}
	if len(tla) > 0 {
		formals := strings.Join(tla, ",")
		pairs := make([]string, len(tla))
		for i := range tla {
			pairs[i] = fmt.Sprintf("%s=%s", tla[i], tla[i])
		}
		actuals := strings.Join(pairs, ",")
		// e.g. `function(foo,bar) (import %q)(foo=foo,bar=bar)`
		// the actuals are keyworded so that the order of TLA flags doesn't matter.
		eval = fmt.Sprintf("function(%s) ((import %q)(%s) { Z____expr__Z_:: %s}).Z____expr__Z_", formals, pathURL, actuals, expr)
	} else {
		eval = fmt.Sprintf("((import %q) { Z____expr__Z_:: %s}).Z____expr__Z_", pathURL, expr)
	}

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

	if c.Trace {
		if err := traceback(os.Stderr, vm, pathURL, expr, false); err != nil {
			return err
		}
	}

	return nil
}

func traceback(w io.Writer, vm *jsonnet.VM, pathURL string, expr string, showAll bool) error {
	roots, err := ursonnet.Roots(vm, pathURL, expr)
	if err != nil {
		return err
	}
	for _, r := range roots {
		if showAll || strings.HasPrefix(r, "file://") {
			fmt.Fprintln(w, strings.TrimPrefix(r, "file://"))
		}
	}
	return nil
}
