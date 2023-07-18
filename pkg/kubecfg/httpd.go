package kubecfg

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

  log "github.com/sirupsen/logrus"
	"github.com/google/go-jsonnet"
	"github.com/kubecfg/kubecfg/utils"
)

// HttpdCmd represents the eval subcommand
type HttpdCmd struct {
	ListenAddr string
}

func (c HttpdCmd) Run(ctx context.Context, mkVM func() (*jsonnet.VM, error), paths []string) error {
  log.Info("Staring Kubecfg HTTPD")
	for _, path := range paths {

		base := strings.TrimSuffix(path, ".jsonnet")

		filename, err := utils.PathToURL(path)
		if err != nil {
      log.Fatalf("cannot convert path to filename %q: %v", path, err)
		}

		filedata, err := ioutil.ReadFile(path)
		if err != nil {
      log.Fatalf("cannot read filename %q: %v", path, err)
		}
		hookcode := string(filedata)

    log.Infof("HTTPD Hook: /%s - Filename: %s", base, filename)
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

			vm, err := mkVM()
			if err != nil {
				http.Error(w, fmt.Sprintf("%v", err), http.StatusInternalServerError)
				return
			}
			vm.TLACode("request", string(body))
			result, err := vm.EvaluateSnippet(filename, hookcode)

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
