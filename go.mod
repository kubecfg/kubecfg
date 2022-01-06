module github.com/kubecfg/kubecfg

require (
	github.com/Masterminds/sprig/v3 v3.2.2
	github.com/elazarl/go-bindata-assetfs v1.0.1
	github.com/evanphx/json-patch v4.12.0+incompatible
	github.com/genuinetools/reg v0.16.1
	github.com/ghodss/yaml v1.0.0
	github.com/go-bindata/go-bindata v1.0.0
	github.com/golang/protobuf v1.5.2
	github.com/google/go-jsonnet v0.18.0
	github.com/googleapis/gnostic v0.5.5
	github.com/huandu/xstrings v1.3.2 // indirect
	github.com/mattn/go-isatty v0.0.14
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.17.0
	github.com/sergi/go-diff v1.2.0
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.3.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	golang.org/x/crypto v0.0.0-20211215153901-e495a2d5b3d3
	gopkg.in/yaml.v2 v2.4.0
	helm.sh/helm/v3 v3.6.3
	k8s.io/api v0.23.1
	k8s.io/apiextensions-apiserver v0.23.1
	k8s.io/apimachinery v0.23.1
	k8s.io/client-go v0.23.1
	k8s.io/klog v1.0.0
	k8s.io/kube-openapi v0.0.0-20211115234752-e816edb12b65
	k8s.io/kubectl v0.23.1
)

go 1.13

replace gopkg.in/yaml.v2 => github.com/mkmik/yaml v0.0.0-20210505221935-5a0cbc1c4094
