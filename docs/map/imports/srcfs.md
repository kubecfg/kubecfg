+++
title = "srcfs"
summary = "Import a file as it was at another commit of the same repository"
aka = ["RFC04", "local git urls", "history channel", "srcfs://"]
kind = "engine"
intent = "aspiration"
research = "outline"
status = "todo"
refs = ["https://github.com/kubecfg/kubecfg/pull/357", "docs/rfcs/rfc04.md on the localgituri branch"]

[[needs]]
on = "/imports/universal"
why = "A new URL scheme"

[[needs]]
fuzzy = "a git client that reads old commits, fetching lazily from treeless clones"
why = "go-git can't fetch lazily, so commits must be fetched first"

[[needs]]
on = "/imports/cache"
soft = true
why = "srcfs imports could be vendored like any remote import, which allows shallow clones"
+++

Lets staging and production use different versions of the same templates
without copying them: `import 'srcfs:///app/app.jsonnet?ref-from=history-channel/prod'`.
