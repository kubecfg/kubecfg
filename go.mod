module github.com/kubecfg/kubecfg

require (
	github.com/Azure/go-autorest/autorest v0.10.0 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/Masterminds/sprig v2.22.0+incompatible
	github.com/elazarl/go-bindata-assetfs v1.0.1
	github.com/evanphx/json-patch v4.11.0+incompatible
	github.com/genuinetools/reg v0.16.1
	github.com/ghodss/yaml v1.0.0
	github.com/go-bindata/go-bindata v1.0.0
	github.com/go-openapi/spec v0.19.7 // indirect
	github.com/go-openapi/swag v0.19.8 // indirect
	github.com/golang/protobuf v1.5.2
	github.com/google/go-jsonnet v0.17.0
	github.com/googleapis/gnostic v0.5.5
	github.com/huandu/xstrings v1.3.2 // indirect
	github.com/imdario/mergo v0.3.9 // indirect
	github.com/mailru/easyjson v0.7.1 // indirect
	github.com/mattn/go-isatty v0.0.14
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/onsi/ginkgo v1.14.0
	github.com/onsi/gomega v1.10.1
	github.com/sergi/go-diff v1.2.0
	github.com/sirupsen/logrus v1.6.0
	github.com/spf13/cobra v1.0.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	golang.org/x/crypto v0.0.0-20211117183948-ae814b36b871
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d // indirect
	google.golang.org/grpc v1.28.1 // indirect
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.19.3
	k8s.io/apiextensions-apiserver v0.19.3
	k8s.io/apimachinery v0.22.4
	k8s.io/client-go v0.19.3
	k8s.io/klog v1.0.0
	k8s.io/kube-openapi v0.0.0-20211115234752-e816edb12b65
	k8s.io/kubectl v0.19.3
)

go 1.13

replace gopkg.in/yaml.v2 => github.com/mkmik/yaml v0.0.0-20210505221935-5a0cbc1c4094
