+++
title = "knit"
summary = "Merge plain Kubernetes manifests in layers, matching list entries by their Kubernetes key"
kind = "lang"
intent = "goal"
research = "outline"
refs = ["lib/knit/knit.libsonnet", "docs/Advanced-Usage/knit.md"]

[[feature]]
id = "library"
title = "The library, its schema and its tests, in lib/knit"
status = "wip"

[[feature]]
id = "yakl-layers"
title = "A YAKL file extends knit; its documents become layers, in order"
status = "wip"
needs = ["/lang/yakl#extension"]

[[feature]]
id = "examples"
title = "examples/experimental/knit: the guestbook and a shop, built on knit"
status = "wip"

[[feature]]
id = "names"
title = "Objects have names: !api is an object, !api+ patches it the knit way, $.api is the object as it ends up"
status = "wip"
note = "A prototype; documents without a name stay, found by identity, for YAML you don't write yourself"
needs = ["/lang/yakl#managed-names"]

[[feature]]
id = "gate"
title = "Its own knit feature gate instead of --alpha"
status = "wip"
needs = ["/config/feature-gates", "/config/require-feature"]

[[needs]]
on = "/lang/stdlib#object-paths"
why = "Uses groupBy and fromObjectPaths instead of its own copies"
+++

In #541.
A YAKL file extends the library object itself.
