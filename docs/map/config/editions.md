+++
title = "Editions"
summary = "A language version for each file or module, like the go line in go.mod"
kind = "config"
intent = "aspiration"
research = "outline"
status = "todo"

[[needs]]
on = "/config/feature-gates"
why = "A change in meaning is tried behind an alpha gate, and ships as an edition"

[[needs]]
on = "/config/rc-file"
why = "A file's edition comes from the nearest rc-like file above it"

[[needs]]
fuzzy = "how an OCI bundle records its edition"
why = "Imported bundles keep the meaning they were written for"
+++

A follow-up in RFC05. Feature gates are per run, so they can't change what
existing files mean; editions can, file by file.
