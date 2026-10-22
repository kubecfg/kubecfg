+++
title = "Garbage collection"
summary = "Find and delete objects that carry kubecfg's tag but left the config"
aka = ["gc", "--gc-tag"]
kind = "cluster"
intent = "induced"
research = "done"
refs = ["pkg/kubecfg/update.go"]

[[feature]]
id = "tag"
title = "--gc-tag labels what update applies, and deletes tagged objects not in the config"
status = "done"

[[feature]]
id = "tags-from-input"
title = "--gc-tags-from-input: use the tags found in the input"
status = "done"

[[feature]]
id = "all-namespaces"
title = "Search every namespace, or only the config's"
status = "done"

[[feature]]
id = "robust"
title = "Tolerate partial API discovery and types that vanish mid-run"
status = "done"

[[needs]]
on = "/cluster/client"
+++
