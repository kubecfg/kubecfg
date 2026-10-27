+++
title = "knit"
summary = "Merge Kubernetes manifests in layers: objects by their name, list entries by their Kubernetes key"
aka = ["RFC07"]
kind = "lang"
intent = "goal"
research = "outline"
refs = ["https://github.com/kubecfg/kubecfg/pull/541", "docs/rfcs/rfc07.md", "lib/knit/knit.libsonnet", "docs/Advanced-Usage/knit.md"]

[[feature]]
id = "library"
title = "The library, its schema and its tests, in lib/knit"
status = "done"

[[feature]]
id = "yakl-layers"
title = "A YAKL file extends knit; its documents become layers, in order"
status = "done"
needs = ["/lang/yakl#extension", "/lang/yakl#library-hook"]

[[feature]]
id = "examples"
title = "examples/experimental/knit: the guestbook, a shop, and YAML files turned into YAKL by knit import"
status = "done"

[[feature]]
id = "names"
title = "Objects have names: ^^: api is an object, ^^+: api patches it the knit way, $.api is the object as it ends up"
status = "done"
needs = ["/lang/knit#yakl-layers"]

[[feature]]
id = "name-is-identity"
title = "The name is the object: overlays find it by name alone, so they can change its kind, name or namespace, and two names that give one Kubernetes object are an error"
status = "todo"
needs = ["/lang/knit#names"]
note = "Today each name maps to the group, kind, namespace and name read from its object: a ^^+: that sets metadata.namespace adds a second object, and ^^: a and ^^: b with the same kind and name silently become one"

[[feature]]
id = "nested-objects"
title = "Objects placed by indentation: ^^: api with the fields deployment and service holds the objects api.deployment and api.service"
status = "todo"
needs = ["/lang/knit#name-is-identity"]
note = "Today knit only sees an object that is a whole document: nested ones get no templates, and ^^+: api.deployment merges them with Jsonnet's +"

[[feature]]
id = "upstream-yaml"
title = "YAML you don't write gets the names knit import would give it as it's read, under the name of the document that reads it"
status = "todo"
needs = ["/lang/knit#nested-objects"]
note = "The names come from the file as written, not from the object after overlays, so nothing moves. Until then, run knit import once and keep its output, as the shop does with redis.yakl"

[[feature]]
id = "named-only"
title = "Every object in a knit YAKL file has a name, as YAKL requires of every document; knit import writes them for YAML"
status = "done"
needs = ["/lang/yakl#named-streams"]
note = "A mixin has a name too, and is a layer in its place. The shop's Redis is redis.yakl, which knit import makes from the upstream redis.yaml"

[[feature]]
id = "deep-name-error"
title = "A name deeper than an object's name, like ^^+: api.spec.template.spec.containers_.api.env_, is an error"
status = "todo"
note = "Today it compiles and its change is silently lost"

[[feature]]
id = "writable-keyed-views"
title = "Patch list entries by key: a deep name resolves by its longest object prefix, x_: {key: ...} writes entries of x, keys like ports_.8080/TCP"
status = "todo"
note = "On hold: t8y:/lang/typed-jsonnet would make containers.api the one name for reading and writing"

[[feature]]
id = "gate"
title = "Its own knit feature gate instead of --alpha"
status = "done"
needs = ["/config/feature-gates", "/config/require-feature"]

[[needs]]
on = "/cli/knit-import#name-everything"
soft = true
why = "named-only: YAML turned into YAKL must have no object left without a name"

[[needs]]
on = "/cli/knit-import#file"
soft = true
why = "upstream-yaml gives the names knit import gives"

[[needs]]
on = "/lang/stdlib#object-paths"
why = "Uses groupBy and fromObjectPaths instead of its own copies"
+++

A YAKL file extends the library object itself.

In a YAKL file, an object is where its name puts it: `^^: api.deployment` is
`$.api.deployment`, and that is how every overlay finds it. Its kind,
`metadata.name` and namespace are data. They can be expressions, and an
overlay can change them, so they can't also decide where the object is: an
overlay that changed them would move the object it is patching.

Group, kind, namespace and name are only read where they are plain text:
`t8y:/cli/knit-import` reads them once, to write the names into the file, and
`render` matches the Jsonnet layers it is given by them, as `kubectl apply`
does. List entries still match by their Kubernetes key, as `kubectl apply`
matches them.
