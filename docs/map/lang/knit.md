+++
title = "knit"
summary = "Merge plain Kubernetes manifests in layers, matching list entries by their Kubernetes key"
aka = ["skuba", "stack"]
kind = "lang"
intent = "goal"
research = "outline"
refs = ["https://github.com/kubecfg/skuba", "docs/Advanced-Usage/knit.md"]

[[feature]]
id = "library"
title = "The library, its schema and its tests, moved here from kubecfg/skuba"
status = "wip"

[[feature]]
id = "yakl-layers"
title = "A YAKL file extends knit; its unnamed documents become layers"
status = "wip"
needs = ["/lang/yakl#extension"]

[[feature]]
id = "examples"
title = "examples/yakl and examples/guestbook-new.jsonnet, built on knit"
status = "wip"

[[feature]]
id = "gate"
title = "Its own knit feature gate instead of --alpha"
status = "todo"
needs = ["/config/feature-gates"]

[[needs]]
on = "/lang/stdlib#object-paths"
why = "Uses groupBy and fromObjectPaths instead of its own copies"

[[needs]]
on = "/config/alpha#jsonnet"
why = "Asserts that --alpha is on"
+++

Committed on the `worktree-abstract-snuggling-pixel` branch; no PR yet.
The separate skuba repository goes away once this lands. The name is
still open (knit or stack), and so is whether the stream root is needed.
