+++
title = "kubecfg.libsonnet"
summary = "The functions kubecfg adds to Jsonnet: parsing, regexes, overlays, layouts, images"
aka = ["stdlib", "native functions", "std.native"]
kind = "lang"
intent = "goal"
research = "done"
refs = ["lib/kubecfg.libsonnet", "utils/nativefuncs.go"]

[[feature]]
id = "parse-manifest"
title = "parseJson, parseYaml, manifestJson, manifestYaml"
status = "done"

[[feature]]
id = "regex"
title = "regexMatch, regexSubst, escapeStringRegex"
status = "done"

[[feature]]
id = "overlays"
title = "toOverlay, deepMap, fold, getPath, objectHasPath"
status = "done"

[[feature]]
id = "layouts"
title = "layouts: index objects by group, version, kind, namespace and name"
status = "done"

[[feature]]
id = "json-schema"
title = "validateJSONSchema"
status = "done"
needs = ["/deps/jsonschema"]

[[feature]]
id = "simple-schema"
title = "validateSimpleSchema: check an object against a kro SimpleSchema, behind --features=simpleschema"
status = "done"
needs = ["/deps/kro"]

[[feature]]
id = "resolve-image"
title = "resolveImage: turn image tags into digests (--resolve-images)"
status = "done"
needs = ["/deps/go-containerregistry"]

[[feature]]
id = "object-paths"
title = "groupBy, toObjectPaths, fromObjectPaths, isNamespaced, layouts.gkNsName"
status = "done"

[[feature]]
id = "pick"
title = "pick: the one element of an array for which a condition is true"
status = "done"

[[feature]]
id = "hashed-name"
title = "hashedName: a name with kustomize's hash of a ConfigMap's or Secret's data, as its generators make"
status = "done"
note = "std.native('kustomizeHash') calls kustomize's own hasher, so names match kustomize build"

[[needs]]
on = "/deps/go-jsonnet"
why = "The Go functions are registered as native functions on the VM"
+++

`parseHelmChart` lives here too; it has its own component,
`t8y:/lang/helm`.
