package kubecfg

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/go-jsonnet"
	"github.com/kubecfg/kubecfg/utils"
)

// HttpdCmd represents the eval subcommand
type HttpdCmd struct {
	ListenAddr string
}

func evaluateFile(vm *jsonnet.VM, path string) (string, error) {
	pathURL, err := utils.PathToFileURL(path)
	if err != nil {
		return "", err
	}
	// TODO(mkm): figure out why vm.EvaluateFile and vm.EvaluateAnonymousSnippet don't work with our custom importers.
	snippet := fmt.Sprintf("(import %q)", path)
	jsonstr, err := vm.EvaluateSnippet(pathURL, snippet)
	if err != nil {
		return "", err
	}
	return jsonstr, nil
}

func (c HttpdCmd) Run(ctx context.Context, vm *jsonnet.VM, paths []string) error {
	for _, path := range paths {
		base := strings.TrimSuffix(path, ".jsonnet")
		http.HandleFunc(fmt.Sprint("/", base), func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Can't read body", http.StatusInternalServerError)
				return
			}
			vm.TLACode("request", string(body))
			result, err := evaluateFile(vm, path)

			if err != nil {
				http.Error(w, fmt.Sprintf("%v", err), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, result)
		})
	}

	return http.ListenAndServe(c.ListenAddr, nil)
}
