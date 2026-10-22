+++
title = "Wasm controller"
summary = "A Kubernetes controller that renders templates with kubecfg-show.wasm"
aka = ["controller", "operator"]
kind = "embed"
intent = "aspiration"
research = "deferred"
status = "todo"

[[needs]]
on = "/embed/wasm-show"
why = "The renderer it runs"

[[needs]]
fuzzy = "a custom resource naming a template, its inputs and where the output goes"
why = "What users would create"

[[needs]]
fuzzy = "fetching the module from ghcr and keeping it compiled across restarts"
why = "Compiling takes about 20 seconds"
+++

The reason kubecfg-show.wasm exists. Compile the module once at startup, start
a fresh instance per render, and keep the compiled module on disk with
wazero's compilation cache.
