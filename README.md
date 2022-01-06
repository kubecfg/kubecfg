# kubecfg

[![Go Report Card](https://goreportcard.com/badge/github.com/kubecfg/kubecfg)](https://goreportcard.com/report/github.com/kubecfg/kubecfg)

A tool for managing Kubernetes resources as code.

`kubecfg` allows you to express the patterns across your
infrastructure and reuse these powerful "templates" across many
services, and then manage those templates as files in version control.
The more complex your infrastructure is, the more you will gain from
using kubecfg.

Yes, Google employees will recognise this as being very similar to a
similarly-named internal tool ;)

## Install

Pre-compiled executables exist for some platforms on
the [Github releases](https://github.com/kubecfg/kubecfg/releases)
page.

On macOS, it can also be installed via [Homebrew](https://brew.sh/):
`brew install kubecfg`

To build from source:

```console
% PATH=$PATH:$GOPATH/bin
% go get github.com/kubecfg/kubecfg
```

## Quickstart

```console
# Show generated YAML
% kubecfg show -o yaml examples/guestbook.jsonnet

# Create resources
% kubecfg update examples/guestbook.jsonnet

# Modify configuration (downgrade gb-frontend image)
% sed -i.bak '\,gcr.io/google-samples/gb-frontend,s/:v4/:v3/' examples/guestbook.jsonnet
# See differences vs server
% kubecfg diff examples/guestbook.jsonnet

# Update to new config
% kubecfg update examples/guestbook.jsonnet

# Clean up after demo
% kubecfg delete examples/guestbook.jsonnet
```

## Features

- Supports JSON, YAML or jsonnet files (by file suffix).
- Best-effort sorts objects before updating, so that dependencies are
  pushed to the server before objects that refer to them.
- Additional jsonnet builtin functions. See `lib/kubecfg.libsonnet`.
- Optional "garbage collection" of objects removed from config (see
  `--gc-tag`).
- Download and Expands `Helm Charts` into jsonnet objects. 

### Helm Chart

Example usage:

```
local kubecfg = import "kubecfg.libsonnet";

local url = "https://charts.jetstack.io/charts/cert-manager-v1.5.3.tgz";
local cm = kubecfg.helmTemplate("cert-manager", namespace, url, {
  // Example values.yaml
  webhook: {replicaCount: 2},
});

// ... returns a jsonnet object with filename keys from the helm chart
// and expanded/parsed Kubernetes objects as values.

// The result can be manipulated and used just like any other jsonnet
// value.
cm + {
  "cert-manager/templates/webhook-deployment.yaml": {
    spec+: {
      template+: {
        spec+: {
          nodeSelector+: {"kubernetes.io/arch": "amd64"},
        },
      },
    },
  },
}
```

*Caveats:*

- Helm 'hooks' are not supported and ignored.
- Chart sort order is ignored, and the usual kubecfg sort mechanism is
  used.
- `chartURL` argument uses the kubecfg URL-based importer, but
  will reject relative URLs by default[1].
- HTTP_PROXY is obeyed, but there is no other cache.  The helm chart
  is re-downloaded on every invocation (for now).
- Probably some other things.

[1]: Relative URLs can be enabled using a new
`--allow-relative-helm-urls` flag.  URLs are interpreted relative to
$PWD currently, even when used by jsonnet from remote URLs.  _This will
change_.  TODO: Make this consistent with usual relative `import`
semantics, and enable by default.


## Infrastructure-as-code Philosophy

The idea is to describe *as much as possible* about your configuration
as files in version control (eg: git).

Changes to the configuration follow a regular review, approve, merge,
etc code change workflow (github pull-requests, phabricator diffs,
etc).  At any point, the config in version control captures the entire
desired-state, so the system can be easily recreated in a QA cluster
or to recover from disaster.

### Jsonnet

Kubecfg relies heavily on [jsonnet](http://jsonnet.org/) to describe
Kubernetes resources, and is really just a thin Kubernetes-specific
wrapper around jsonnet evaluation.  You should read the jsonnet
[tutorial](http://jsonnet.org/docs/tutorial.html), and skim the functions available in the jsonnet [`std`](http://jsonnet.org/docs/stdlib.html)
library.

## Community

- [#jsonnet on Kubernetes Slack](https://kubernetes.slack.com/messages/jsonnet)

Click [here](http://slack.k8s.io) to sign up to the Kubernetes Slack org.
