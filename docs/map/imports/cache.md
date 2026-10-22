+++
title = "Caching and vendoring"
summary = "Keep downloaded imports in a cache that can be checked in as a vendor directory"
aka = ["RFC03", "vendoring", "--cache-dir", "--ci"]
kind = "engine"
intent = "aspiration"
research = "outline"
refs = ["https://github.com/kubecfg/kubecfg/pull/235", "docs/rfcs/rfc03.md on the vendoring branch"]

[[feature]]
id = "cache"
title = "Remote imports are saved under the user's cache directory"
status = "todo"

[[feature]]
id = "search-path"
title = "Several --cache-dir directories, searched in order"
status = "todo"

[[feature]]
id = "vendoring"
title = "A checked-in cache directory is the vendor directory"
status = "todo"

[[feature]]
id = "pruning"
title = "Remove cached files that no import needs any more"
status = "todo"
note = "Open: an explicit prune step, a fresh cache per batch, or an access log"

[[feature]]
id = "ci"
title = "--ci: fail instead of downloading"
status = "todo"

[[feature]]
id = "file-names"
title = "Escape file names so they work on Linux, macOS and Windows"
status = "wip"
note = "Contributed by jenniferplusplus on the vendoring branch"

[[needs]]
on = "/imports/universal"
why = "Sits between it and the network"

[[needs]]
on = "/config/rc-file"
soft = true
why = "Vendoring is meant to be set up in .kubecfgrc.jsonnet"

[[needs]]
on = "/imports/pinned-hashes"
soft = true
why = "Pinned imports can be served from the cache safely"

[[needs]]
on = "/config/feature-gates"
soft = true
why = "Could ship as the alpha feature vendoring"
+++
