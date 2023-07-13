# Welcome to KubeCFG Documentation

**These Docs are a Work in progress and not complete by any means** - *PRs are welcome*

`Kubecfg` is a tool for managing Kubernetes resources as code.

`kubecfg` allows you to express the patterns across your
infrastructure and reuse these powerful "templates" across many
services, and then manage those templates as files in version control.
The more complex your infrastructure is, the more you will gain from
using kubecfg.

## Features

- Supports `JSON`, `YAML` or `jsonnet` files (by file suffix).
- Best-effort sorts objects before updating, so that dependencies are
  pushed to the server before objects that refer to them.
- Additional jsonnet builtin functions. See `lib/kubecfg.libsonnet`.
- Optional "garbage collection" of objects removed from config (see `--gc-tag`).
- **TODO**: ADD More features here

## Infrastructure-as-code Philosophy

The idea is to describe *as much as possible* about your configuration
as files in version control (eg: git).

Changes to the configuration follow a regular review, approve, merge,
etc code change workflow (github pull-requests, phabricator diffs,
etc).  At any point, the config in version control captures the entire
desired-state, so the system can be easily recreated in a QA cluster
or to recover from disaster.

## Jsonnet

Kubecfg relies heavily on [jsonnet](http://jsonnet.org/) to describe
Kubernetes resources, and is really just a thin Kubernetes-specific
wrapper around jsonnet evaluation.  You should read the jsonnet
[tutorial](http://jsonnet.org/docs/tutorial.html), and skim the functions available in the jsonnet [`std`](http://jsonnet.org/docs/stdlib.html)
library.
