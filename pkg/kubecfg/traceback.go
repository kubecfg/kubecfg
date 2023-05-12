package kubecfg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/go-jsonnet"
	"github.com/kubecfg/kubecfg/pkg/yamloc"
	"github.com/kubecfg/kubecfg/utils"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// TracebackCmd represents the traceback subcommand
type TracebackCmd struct {
	ShowAll bool
}

func (c TracebackCmd) Run(ctx context.Context, vm *jsonnet.VM, fileloc string) error {
	filename, loc, found := strings.Cut(fileloc, ":")
	if !found {
		return fmt.Errorf("argument must be <filename.yaml>:<linenumber>")
	}
	line, err := strconv.Atoi(loc)
	if err != nil {
		return err
	}
	b, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	innerPath, err := yamloc.LineToPath(b, line)
	if err != nil {
		return err
	}

	var res K8sResource
	if err := yaml.Unmarshal(b, &res); err != nil {
		return err
	}
	provenanceFile, found := res.Metadata.Annotations[ProvenanceFileAnnotation]
	if !found {
		return fmt.Errorf("provenance annotation %q not found in target yaml file", ProvenanceFileAnnotation)
	}

	provenancePath, found := res.Metadata.Annotations[ProvenancePathAnnotation]
	if !found {
		return fmt.Errorf("provenance annotation %q not found in target yaml file", ProvenanceFileAnnotation)
	}

	rootDir := "."
	if gitRoot, foundGit, err := utils.SearchUp(".git", filename); err != nil {
		return err
	} else if foundGit {
		rootDir = filepath.Dir(gitRoot)
	}
	provenanceFile = filepath.Join(rootDir, provenanceFile)

	fullPath := fmt.Sprint(provenancePath, strings.TrimPrefix(innerPath, "$"))
	log.Infof("Tracing file=%q, path=%q", provenanceFile, fullPath)

	provenanceFileURL, err := utils.PathToURL(provenanceFile)
	if err != nil {
		return err
	}

	return traceback(os.Stdout, vm, provenanceFileURL, fullPath, c.ShowAll)
}

const (
	ProvenanceFileAnnotation = "kubecfg.github.com/provenance-file"
	ProvenancePathAnnotation = "kubecfg.github.com/provenance-path"
)

type K8sResource struct {
	Metadata K8sMetadata `json:"metadata"`
}

type K8sMetadata struct {
	Annotations map[string]string `json:"annotations"`
}
