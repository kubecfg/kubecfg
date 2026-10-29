+++
title = "YAKL"
summary = "YAML with Jsonnet expressions: plain YAML is valid YAKL, ${...} adds the logic"
aka = ["RFC09", "jamlet", "yannet", ".yakl", "yet another konfig language"]
kind = "lang"
intent = "goal"
research = "done"
refs = ["docs/rfcs/rfc09.md", "pkg/yakl/yakl.go", "docs/Advanced-Usage/yakl.md", "examples/experimental/yakl/guestbook.yakl"]

[[feature]]
id = "expressions"
title = "${...} values: plain, quoted and block expressions"
status = "done"

[[feature]]
id = "yaml-in-expressions"
title = '\{ and a line break open YAML inside an expression'
status = "done"

[[feature]]
id = "loops"
title = "${for} and ${if} clauses as list items and mapping keys"
status = "done"

[[feature]]
id = "extension"
title = "+ keys, <<+ and hidden fields"
status = "done"

[[feature]]
id = "names"
title = "Documents named with a key (^^: web), read as $.name"
status = "done"

[[feature]]
id = "dotted-names"
title = "A dot in a name starts a level: ^^: web.conf is $.web.conf"
status = "done"

[[feature]]
id = "named-streams"
title = "One document is any value; several are named mappings, and the file is an object of them"
status = "done"

[[feature]]
id = "locals"
title = "Anchors are locals; local keys and file locals"
status = "done"

[[feature]]
id = "selectors"
title = "list[@.name == 'api'] reads the one entry of a list for which the condition is true"
status = "done"
needs = ["/lang/stdlib#pick"]

[[feature]]
id = "library-hook"
title = "The hook for libraries: the object a file extends gets its documents, in order, in documents, and its $named decides what $.name reads"
status = "done"
note = "For knit. The file decides where each document goes, with its ^^ key; the hook only decides what is there"

[[feature]]
id = "file-extends"
title = "A file extends another with <<+"
status = "done"

[[feature]]
id = "fuzzing"
title = "The compiler is fuzzed"
status = "done"

[[feature]]
id = "kubecfg-locals"
title = "KubecfgLocals: a pseudo-resource that gives kustomize output its imports"
status = "wip"
note = "Uncommitted in the structured-stargazing-beacon worktree; still written against jamlet"

[[feature]]
id = "missing-document-error"
title = "A clear error when an overlay names a document that doesn't exist"
status = "done"

[[feature]]
id = "entry-names"
title = "Names below a document's root: a ^^ key on a list entry, - {^^: api.container}, names it"
status = "todo"
note = "An open question in RFC09; t8y:/lang/typed-jsonnet may make it unnecessary"

[[feature]]
id = "interpolation"
title = "${...} inside strings, like app-${x}"
status = "dropped"
note = "On purpose: write ${'app-' + x}"

[[needs]]
on = "/deps/yaml"
why = "Parses the YAML, keeping positions for error messages"

[[needs]]
on = "/config/feature-gates"
why = "Alpha: the yakl feature"
+++

Compiles each file to Jsonnet, so everything else in kubecfg sees Jsonnet.
Was called yannet, then jamlet, before it became YAKL. The open questions
at the end of `docs/Advanced-Usage/yakl.md` are where new ideas start:
two of them are `t8y:/lang/yakl-schema` and `t8y:/lang/yakl-params`.
