+++
title = "Bundle features"
summary = "pack records the features a bundle needs; importing it names the missing ones"
kind = "config"
intent = "aspiration"
research = "outline"
status = "todo"

[[needs]]
on = "/cli/pack"
why = "Writes them next to the kubecfg version it already records"

[[needs]]
on = "/imports/oci"
why = "Checks them on import"

[[needs]]
on = "/config/feature-gates"
+++

A follow-up in RFC05. Importing never turns features on: remote code
shouldn't be able to enable alpha features.
