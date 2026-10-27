+++
title = "knit import"
summary = "Print a YAML file as a YAKL file on knit, with a name for each object"
aka = ["kubecfg knit import"]
kind = "command"
intent = "goal"
research = "done"
refs = ["cmd/knit.go", "docs/Advanced-Usage/knit.md", "examples/experimental/knit/import", "examples/experimental/knit/kustomize"]

[[feature]]
id = "file"
title = "One YAML file: ^^: name.Kind on each object, the namespace in front when two objects need it"
status = "done"

[[feature]]
id = "name-everything"
title = "Every object gets a name, and a document that can't get one is an error: a List, one without a kind or metadata.name, one with $patch"
status = "done"
note = "The error says what to write instead: each item of a List as a document, ^^+:: and the base's name to leave out an object"

[[feature]]
id = "extend"
title = "--extend base.yakl: an overlay; objects the base has get ^^+: with the base's name, new ones ^^: name.Kind"
status = "done"
note = "Finds each object's name in the base by group, kind, namespace and name, once, while the YAML is still plain text; names renamed by hand still match. A patch keeps only the fields that change what the base renders. With --delete, the file has every object in full, and what it leaves out is deleted"

[[feature]]
id = "kustomize"
title = "-k ./overlays/prod/ --out-dir knit: a file per kustomization in a tree like theirs; resources become objects, strategic merge patches ^^+: documents"
status = "done"
needs = ["/cli/knit-import#extend"]
note = "A kustomization with one directory resource extends that one's file (overlays/prod.yakl extends base.yakl); one with more has their objects in its own file, and without --out-dir it prints one file with all. A patch with a target gets one ^^+: per object it selects; $patch: delete on an object hides it. TestKnitImportKustomize checks the files render what kustomize builds"

[[feature]]
id = "kustomize-components"
title = "-k: components; with --out-dir, a component is a file that the file of a kustomization that lists it adds after its base"
status = "done"
needs = ["/cli/knit-import#kustomize"]
note = "<<+: ${(import 'yakl:../base.yakl') + import 'yakl:../components/tracing.yakl'}; the component's file starts with <<+: {} or its own components. One that patches the listing kustomization's own objects, which come after it, stays in that kustomization's file, with its other components; so does one listed by a directory resource whose objects are in another kustomization's file"

[[feature]]
id = "kustomize-join"
title = "-k: each patch is a ^^+: document of its own, in kustomize's order"
status = "done"
needs = ["/cli/knit-import#kustomize", "/lang/yakl#merge-chain"]
note = "A YAKL file's documents of one name are a chain of merges, so patches of one object stay apart, as kustomize applies them"

[[feature]]
id = "kustomize-json6902"
title = "-k: JSON 6902 patches that append to a list or remove a field or a keyed entry"
status = "done"
needs = ["/cli/knit-import#kustomize"]
note = "A ^^+: document per operation: add .../- is - $patch: append (or a new keyed entry), remove is $patch: delete. An index into a keyed list is the entry's key, from knit's schema ('$lists'); a strategic merge patch before it that adds or deletes entries of the list is an error"

[[feature]]
id = "kustomize-json6902-rest"
title = "-k: the other JSON 6902 operations: replace, add at an index, move, copy, test, an index into an atomic list"
status = "todo"
needs = ["/cli/knit-import#kustomize-json6902"]
note = "replace of a field is a plain value (with $patch: replace for a map or a list); an index into an atomic list needs the whole list"

[[feature]]
id = "kustomize-transformers"
title = "-k: namespace, labels, annotations, images, replicas and namePrefix"
status = "todo"
needs = ["/cli/knit-import#kustomize", "/lang/knit#name-is-identity"]
note = "They can become ^^+: registry documents; namePrefix changes metadata.name, which no longer moves an object once a name is the object"

[[feature]]
id = "kustomize-generators"
title = "-k: configMapGenerator and generatorOptions; references to a ConfigMap read its name through $"
status = "done"
needs = ["/cli/knit-import#kustomize", "/lang/knit#needs-hash"]
note = "A ConfigMap after the resources with the needs-hash annotation; behavior merge and replace are ^^+: documents of its data. A configMapRef, configMapKeyRef or volume configMap name gets ${$.web.ConfigMap.metadata.name}. TestKnitImportKustomize checks the hashes against kustomize build"

[[feature]]
id = "kustomize-secret-generator"
title = "-k: secretGenerator; references to a Secret read its name through $"
status = "done"
needs = ["/cli/knit-import#kustomize-generators"]
note = "A secretRef, secretKeyRef, a volume's secret (secretName) or projected secret (name), and imagePullSecrets"

[[feature]]
id = "kustomize-generator-base-refs"
title = "-k --out-dir: a base's reference to a ConfigMap an overlay generates"
status = "todo"
needs = ["/cli/knit-import#kustomize-generators"]
note = "An error for now: the base's file has the name as written. The overlay could patch the reference, or the base read it with a default"

[[needs]]
on = "/lang/knit#names"
why = "The names it writes are knit objects"

[[needs]]
on = "/lang/yakl#dotted-names"
why = "name.Kind is the field Kind of the field name"

[[needs]]
on = "/config/feature-gates"
why = "Alpha: the knit and yakl features"
+++
