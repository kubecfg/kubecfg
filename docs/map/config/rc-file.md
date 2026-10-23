+++
title = ".kubecfgrc.jsonnet"
summary = "A checked-in Jsonnet file sets flag defaults, so no wrapper scripts are needed"
aka = ["RFC02", "Flags From Files", "FFF", "flagspec", "kubecfgrc"]
kind = "config"
intent = "goal"
research = "done"
refs = ["https://github.com/kubecfg/kubecfg/pull/224", "docs/rfcs/rfc02.md", "cmd/root.go"]

[[feature]]
id = "search"
title = "Find .kubecfgrc.jsonnet next to the inputs or in a directory above"
status = "done"

[[feature]]
id = "defaults"
title = "flags and commands.<command> set flag defaults; the command line and env vars, KUBECONFIG included, still win"
status = "done"

[[feature]]
id = "paths"
title = "Path flags such as --jpath are relative to the rc file"
status = "done"

[[feature]]
id = "one-rc"
title = "All inputs of one run must find the same rc file"
status = "done"

[[feature]]
id = "bad-values"
title = "Reject values that don't fit the flag, such as 'dry-run': 'yes'"
status = "done"

[[feature]]
id = "features"
title = "features in the rc file, merged one feature at a time with the command line"
status = "done"
needs = ["/config/feature-gates"]
note = "RFC05: a list or an object, only under flags, evaluated with default gates; kubecfg features reads it too"

[[needs]]
on = "/config/flags"
why = "It sets their defaults"

[[needs]]
on = "/engine/vm"
why = "The rc file is Jsonnet"
+++

Lands with #224. Commands that evaluate files (show, update, delete, diff,
validate, eval) take their flags from it; every command takes alpha and
features from it, looking from the current directory when it has no input
files. Env vars that libraries read themselves, such as POD_NAMESPACE or
HTTPS_PROXY, lose to the rc file's matching flag, as they do to the command line.
