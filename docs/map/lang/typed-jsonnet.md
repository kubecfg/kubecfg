+++
title = "Keyed arrays and types for Jsonnet"
summary = "Arrays that Jsonnet reads and merges by key, growing into types taken from OpenAPI schemas"
aka = ["RFC08", "keyed arrays", "std.keyed", "type system", "go-jsonnet fork"]
kind = "lang"
intent = "aspiration"
research = "outline"
refs = ["docs/rfcs/rfc08.md"]

[[feature]]
id = "keyed-arrays"
title = "std.keyed(keys, arr): arr.api finds an entry by key, arr + [...] and arr + {api+: ...} merge by key, output stays an array"
status = "todo"
note = "A throwaway prototype worked: 168 new lines in go-jsonnet, its tests passing"

[[feature]]
id = "types"
title = "Values carry a type: fields of a typed value are typed, + keeps the left side's type, untyped values behave as today"
status = "todo"

[[feature]]
id = "openapi-types"
title = "Types from the Kubernetes OpenAPI schema and CRDs, found from apiVersion and kind"
status = "todo"

[[feature]]
id = "deep-overlays"
title = "Overlays that merge deep, with an explicit way to replace"
status = "todo"
note = "In Jsonnet, env: [...] in a patch replaces; a YAML overlay means merge"

[[feature]]
id = "upstream"
title = "Propose keyed arrays to go-jsonnet"
status = "todo"
note = "Additive: no array is keyed today"

[[needs]]
on = "/deps/go-jsonnet"
why = "Keyed arrays need the interpreter: a fork, or an upstream change"

[[needs]]
fuzzy = "where the fork lives, and how it follows go-jsonnet releases"
why = "Its own subproject"
+++

A subproject of its own. Files using it would only run on kubecfg's
Jsonnet, not on the C++ jsonnet, jrsonnet or sjsonnet, and
jsonnet-language-server wouldn't know it.

If it lands, `t8y:/lang/knit` shrinks to its schema and templates, and YAKL's
`$.a.b` and `^^+: a.b` use the same path into keyed lists (`t8y:/lang/yakl`).
Objects already find each other by name, so `^^+: api` could be YAKL's own
`+`, without `t8y:/lang/yakl#library-hook`.
