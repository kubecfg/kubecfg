+++
title = "Objects"
summary = "Walks the evaluated value into a flat list of Kubernetes objects"
aka = ["walk", "provenance", "duplicates", "yamloc"]
kind = "engine"
intent = "induced"
research = "done"
refs = ["utils/acquire.go", "utils/duplicates.go", "pkg/yamloc/yamloc.go"]

[[feature]]
id = "walk"
title = "Nested objects and arrays become one flat list"
status = "done"

[[feature]]
id = "dedupe"
title = "Identical duplicates are dropped; conflicting ones are an error"
status = "done"

[[feature]]
id = "provenance"
title = "Record the file and field path each object came from"
status = "done"

[[feature]]
id = "yaml-lines"
title = "Map a line of rendered YAML back to a field path"
status = "done"

[[needs]]
on = "/deps/kubernetes"
why = "Objects are apimachinery's unstructured objects"
+++
