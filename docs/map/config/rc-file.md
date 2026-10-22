+++
title = ".kubecfgrc.jsonnet"
summary = "A checked-in Jsonnet file sets flag defaults, so no wrapper scripts are needed"
aka = ["RFC02", "Flags From Files", "FFF", "flagspec", "kubecfgrc"]
kind = "config"
intent = "goal"
research = "done"
refs = ["https://github.com/kubecfg/kubecfg/pull/224", "docs/rfcs/rfc02.md on the flagspec branch"]

[[feature]]
id = "search"
title = "Find .kubecfgrc.jsonnet next to the inputs or in a directory above"
status = "wip"

[[feature]]
id = "defaults"
title = "flags and commands.<command> set flag defaults; flags and env vars still win"
status = "wip"

[[feature]]
id = "paths"
title = "Path flags such as --jpath are relative to the rc file"
status = "wip"

[[feature]]
id = "one-rc"
title = "All inputs of one run must find the same rc file"
status = "wip"

[[feature]]
id = "bad-values"
title = "Reject values that don't fit the flag, such as 'dry-run': 'yes'"
status = "wip"

[[feature]]
id = "features"
title = "features in the rc file, merged one feature at a time with the command line"
status = "todo"
needs = ["/config/feature-gates"]
note = "RFC05: accept an object, only under flags, evaluated with default gates"

[[needs]]
on = "/config/flags"
why = "It sets their defaults"

[[needs]]
on = "/engine/vm"
why = "The rc file is Jsonnet"
+++

On the `flagspec` branch, rebased and fixed after review on 2026-10-22.
Only commands that evaluate files (show, update, delete, diff, validate,
eval) read it.
