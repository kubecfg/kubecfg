+++
title = "OCI imports"
summary = "oci:// imports read Jsonnet out of bundles made with kubecfg pack"
aka = ["oci://"]
kind = "engine"
intent = "goal"
research = "done"
status = "done"
refs = ["utils/oci.go", "docs/Advanced-Usage/OCI-support.md"]

[[needs]]
on = "/imports/universal"
why = "Registered as one of its URL schemes"

[[needs]]
on = "/deps/containerd"
why = "Fetches from the registry with the user's docker login"
+++
