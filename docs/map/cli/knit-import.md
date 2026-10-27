+++
title = "knit import"
summary = "Print a YAML file as a YAKL file on knit, with a name for each object"
aka = ["kubecfg knit import"]
kind = "command"
intent = "goal"
research = "done"
refs = ["cmd/knit.go", "docs/Advanced-Usage/knit.md", "examples/experimental/knit/import"]

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
title = "-k ./overlays/prod/: import a kustomize tree, bases once and each overlay as a file that extends them"
status = "todo"
needs = ["/cli/knit-import#extend", "/lang/knit#name-is-identity"]
note = "kustomize is plain text when it's imported, so it can be resolved then: strategic merge patches become ^^+: documents, and a patch with a target selector one ^^+: per object it selects; namespace, commonLabels, images and replicas become ^^+: registry documents; namePrefix changes metadata.name, which no longer moves an object. JSON 6902 patches and generators are open"

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
