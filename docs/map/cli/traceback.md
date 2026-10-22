+++
title = "traceback"
summary = "Find the Jsonnet source behind a line of rendered YAML"
aka = ["kubecfg traceback"]
kind = "command"
intent = "goal"
research = "done"
status = "done"
refs = ["cmd/traceback.go", "pkg/kubecfg/traceback.go", "docs/Advanced-Usage/provenance-traceback.md"]

[[needs]]
on = "/engine/objects#yaml-lines"
why = "Turns the line number into a field path"

[[needs]]
on = "/engine/vm"
why = "Re-evaluates the file to find the source"

[[needs]]
on = "/config/feature-gates"
why = "Alpha: the traceback feature"
+++
