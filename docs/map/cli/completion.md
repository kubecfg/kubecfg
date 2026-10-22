+++
title = "completion"
summary = "Shell completion scripts for bash, zsh and fish"
aka = ["kubecfg completion"]
kind = "command"
intent = "goal"
research = "done"
status = "done"
refs = ["cmd/completion.go"]

[[needs]]
on = "/deps/cobra"
why = "Cobra generates the scripts"
+++
