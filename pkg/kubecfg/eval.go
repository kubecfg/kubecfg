package kubecfg

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/google/go-jsonnet"
	"github.com/kubecfg/kubecfg/internal/acquire"
	"github.com/kubecfg/kubecfg/pkg/yakl"
	"github.com/kubecfg/kubecfg/utils"
	"github.com/kubecfg/ursonnet"
)

// EvalCmd represents the eval subcommand. It reads its inputs as show does, and prints their values
// instead of looking for Kubernetes objects in them.
type EvalCmd struct {
	ShowKeys bool
	Format   string
	Trace    bool
}

func (c EvalCmd) Run(vm *jsonnet.VM, paths []string, out io.Writer, opts ...utils.ReadOption) error {
	opt := acquire.MakeReadOptions(opts)
	if c.Trace && (len(paths) != 1 || strings.HasPrefix(paths[0], "data:") || opt.OverlayURL != "" || opt.OverlayCode != "") {
		return fmt.Errorf("--trace needs one file, without -e or overlays")
	}
	expr := opt.Expr
	if expr == "" {
		expr = "$"
	}
	if c.ShowKeys {
		opts = append(opts, utils.WithExpr(fmt.Sprintf("std.objectFields(%s)", expr)))
	}

	var values []interface{}
	err := readInputs(paths, opts, func(in string) error {
		v, err := utils.Evaluate(vm, in, opts...)
		values = append(values, v)
		return err
	})
	if err != nil {
		return err
	}
	for i, v := range values {
		if err := writeDoc(out, c.Format, i, v); err != nil {
			return err
		}
	}

	if c.Trace {
		pathURL, err := utils.PathToURL(paths[0])
		if err != nil {
			return err
		}
		if err := traceback(os.Stderr, vm, utils.ImportPath(pathURL), expr, false); err != nil {
			return err
		}
	}
	return nil
}

func traceback(w io.Writer, vm *jsonnet.VM, pathURL string, expr string, showAll bool) error {
	// The tracer extends the root with an object, so an array is traced as {docs: [...]}.
	isArray, err := vm.EvaluateAnonymousSnippet("", fmt.Sprintf("std.isArray(import %q)", pathURL))
	if err != nil {
		return err
	}
	if rest, ok := strings.CutPrefix(expr, "$"); ok && strings.TrimSpace(isArray) == "true" {
		pathURL, expr = utils.DocsScheme+pathURL, "$.docs"+rest
	}
	// The expression can be code from the command line.
	roots, err := ursonnet.Roots(vm, pathURL, yakl.PredefineKubecfg(expr))
	if err != nil {
		return err
	}
	for _, r := range roots {
		r = strings.TrimPrefix(r, utils.YAKLScheme) // compiled yakl files are local files too
		if showAll || strings.HasPrefix(r, "file://") {
			fmt.Fprintln(w, strings.TrimPrefix(r, "file://"))
		}
	}
	return nil
}
