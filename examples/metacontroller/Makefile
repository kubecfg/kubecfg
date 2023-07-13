SHELL=/bin/bash

all: generate_crds

.PHONY: generate_crds
generate_crds:
	@echo "+ Generating crds"
	@go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.12.0
	@controller-gen rbac:roleName=manager-role +crd:generateEmbeddedObjectMeta=true +paths="./api/..." +output:crd:stdout > v1/crdv1.yaml

.PHONY: controller
controller:
	@kubectl apply -k .

.PHONY: kind
kind:
	@kind create cluster --name="metacontroller"
	@kubectl apply -k https://github.com/metacontroller/metacontroller/manifests/production

.PHONY: clean
clean:
	@kind delete cluster --name="metacontroller"
