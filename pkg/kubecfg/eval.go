package kubecfg

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/google/go-jsonnet"
	"github.com/kubecfg/kubecfg/pkg/yakl"
	"github.com/kubecfg/kubecfg/utils"
	"github.com/kubecfg/ursonnet"
	"github.com/kubecfg/yaml/v2"
)

// EvalCmd represents the eval subcommand
type EvalCmd struct {
	Exec     string // code to evaluate instead of a file, as with show -e
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

	// A newline, so that a // comment at the end of the code doesn't hide the parenthesis.
	root := "(" + c.Exec + "\n)"
	var pathURL string
	if c.Exec == "" {
		var err error
		if pathURL, err = utils.PathToURL(path); err != nil {
			return err
		}
		pathURL = utils.ImportPath(pathURL)
		root = fmt.Sprintf("(import %q)", pathURL)
	}

	eval := evalSnippet(root, c.Expr, tla)
	if c.ShowKeys {
		eval = fmt.Sprintf("std.objectFields(%s)", eval)
	}
	// The whole snippet, so that a top-level argument named kubecfg declares it.
	eval = yakl.PredefineKubecfg(eval)

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

// evalSnippet returns Jsonnet evaluating expr, with `$` being the result of the code root,
// called with the top-level arguments tla. Without expr, it evaluates to that result.
func evalSnippet(root, expr string, tla []string) string {
	if len(tla) > 0 {
		pairs := make([]string, len(tla))
		for i := range tla {
			pairs[i] = fmt.Sprintf("%s=%s", tla[i], tla[i])
		}
		// e.g. `function(foo,bar) (import %q)(foo=foo,bar=bar)`
		// the actuals are keyworded so that the order of TLA flags doesn't matter.
		root = fmt.Sprintf("%s(%s)", root, strings.Join(pairs, ","))
	}
	body := root
	if expr != "" {
		// `$` must be an object, while a YAML stream in a yakl file evaluates to an array.
		body = fmt.Sprintf("local Z____root__Z_ = %s; if Z____std__Z_.isObject(Z____root__Z_) then (Z____root__Z_ { Z____expr__Z_:: %s }).Z____expr__Z_ "+
			"else error '-x needs the input to evaluate to an object, not ' + Z____std__Z_.type(Z____root__Z_)", root, expr)
	}
	if len(tla) > 0 {
		body = fmt.Sprintf("function(%s) %s", strings.Join(tla, ","), body)
	}
	// std is bound before a top-level argument can shadow it.
	return "local Z____std__Z_ = std; " + body
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
