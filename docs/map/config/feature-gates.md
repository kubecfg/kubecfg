+++
title = "Feature gates"
summary = "Turn experimental features on one at a time, each with a stage: alpha, beta, GA"
aka = ["RFC05", "--features", "KUBECFG_FEATURES", "kubecfg features"]
kind = "config"
intent = "goal"
research = "done"
refs = ["https://github.com/kubecfg/kubecfg/pull/538", "docs/rfcs/rfc05.md", "pkg/features/features.go", "cmd/features.go", "docs/Advanced-Usage/feature-gates.md"]

[[feature]]
id = "registry"
title = "A registry of features, each with a stage, a kind (additive or semantic) and a release"
status = "done"

[[feature]]
id = "flag"
title = "--features and KUBECFG_FEATURES: name=true, name=false, unknown names are errors"
status = "done"

[[feature]]
id = "alpha-compat"
title = "--alpha still turns on every alpha feature, including future ones"
status = "done"

[[feature]]
id = "migrate"
title = "The eight --alpha checks become features: eval, traceback, pack, httpd, overlay, yakl..."
status = "done"

[[feature]]
id = "errors"
title = "One error message for every gate, made from the registry"
status = "done"

[[feature]]
id = "command"
title = "kubecfg features: what is on, and which setting turned it on"
status = "done"

[[feature]]
id = "wasm"
title = "kubecfg-show.wasm takes --features too"
status = "done"

[[feature]]
id = "docs-page"
title = "A docs page listing every feature, generated from the registry"
status = "done"

[[needs]]
on = "/config/flags"

[[needs]]
on = "/config/alpha"
why = "Keeps --alpha working"
+++

Merged in #538; all eight features start at alpha. #541 added
a ninth, `knit`. Setting features in
`.kubecfgrc.jsonnet` comes with RFC02, `t8y:/config/rc-file#features`.
Still open from the RFC: how long a feature stays in beta before GA.
