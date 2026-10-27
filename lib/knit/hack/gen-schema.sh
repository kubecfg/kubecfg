#!/bin/sh
# Regenerates schema/k8s-<minor>.libsonnet from a Kubernetes release tag.
# Usage: hack/gen-schema.sh v1.37.1
set -eu
v=${1:?usage: hack/gen-schema.sh <kubernetes tag, e.g. v1.37.1>}
cd "$(dirname "$0")/.."
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

git -c advice.detachedHead=false clone -q --depth 1 --filter=blob:none --sparse --branch "$v" https://github.com/kubernetes/kubernetes "$tmp/k8s"
git -C "$tmp/k8s" sparse-checkout set api/openapi-spec/v3

# One spec file per group version; merge them and drop descriptions.
jq -s '{
  schemas: (map(.components.schemas // {}) | add | del(.. | .description?)),
  paths: (map(.paths // {}) | add | del(.. | .description?))
}' "$tmp"/k8s/api/openapi-spec/v3/*.json >"$tmp/spec.json"

minor=$(echo "${v#v}" | cut -d. -f1-2)
jq .schemas "$tmp/spec.json" >"$tmp/schemas.json"
jq .paths "$tmp/spec.json" >"$tmp/paths.json"
# kubecfg, built from this repo: knit needs its stdlib and --features=knit.
go run ../.. --features=eval,knit eval -o json --ext-code-file schemas="$tmp/schemas.json" --ext-code-file paths="$tmp/paths.json" --ext-str version="$v" \
  hack/gen-schema.jsonnet >"$tmp/out.libsonnet"
mv "$tmp/out.libsonnet" "schema/k8s-$minor.libsonnet"
echo "wrote schema/k8s-$minor.libsonnet"
