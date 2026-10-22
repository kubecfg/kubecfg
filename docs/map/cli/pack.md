+++
title = "pack"
summary = "Pack a Jsonnet program and every file it imports into an OCI bundle"
aka = ["kubecfg pack", "OCI bundle"]
kind = "command"
intent = "goal"
research = "done"
refs = ["cmd/pack.go", "pkg/kubecfg/pack.go", "pkg/oci/repository.go", "docs/Advanced-Usage/OCI-support.md"]

[[feature]]
id = "push"
title = "Push the bundle to an OCI registry"
status = "done"
needs = ["/deps/oras"]

[[feature]]
id = "output-file"
title = "--output: write the bundle to a file instead"
status = "done"

[[feature]]
id = "docs"
title = "--docs-tar-file: ship a documentation bundle with it"
status = "done"

[[feature]]
id = "annotations"
title = "--annotations on the image manifest"
status = "done"

[[feature]]
id = "yakl-source"
title = "Ship .yakl and yaml: files as their source, not compiled"
status = "done"

[[needs]]
on = "/engine/vm"
why = "Finds every file the program imports"

[[needs]]
on = "/imports/universal"
why = "Reads the imported files"

[[needs]]
on = "/config/feature-gates"
why = "Alpha: the pack feature"
+++
