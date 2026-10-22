+++
title = "kubecfg-show.wasm"
summary = "show as a WebAssembly (wasip1) module: render templates from any Wasm host, offline"
aka = ["wasm", "wasip1", "kubecfg-show"]
kind = "embed"
intent = "goal"
research = "done"
refs = ["wasm/cmd/kubecfg-show/main.go", "wasm/hosttest/kubecfg_show_test.go"]

[[feature]]
id = "build"
title = "A separate, small entry point built for wasip1"
status = "done"

[[feature]]
id = "host-test"
title = "Tested in-process under wazero, in its own Go module"
status = "done"

[[feature]]
id = "oci-artifact"
title = "Pushed to ghcr as a Wasm OCI artifact on every release"
status = "done"

[[needs]]
on = "/engine/vm"
why = "Evaluates the templates"

[[needs]]
on = "/engine/objects"
why = "Flattens the result the same way show does"

[[needs]]
on = "/deps/wazero"
soft = true
why = "Only the host test uses it, so kubecfg's own go.mod doesn't depend on it"
+++

No network: `kubecfg.libsonnet` is built in, and imports only see the files
the host mounts. The host has to set `PWD`. Compiling the module takes about
20 seconds in wazero; each render after that about 100 ms.
