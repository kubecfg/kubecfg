+++
title = "kro"
summary = "kro's SimpleSchema, checked with the API server's code: apiextensions-apiserver and its CEL"
aka = ["kubernetes-sigs/kro", "SimpleSchema", "cel-go"]
kind = "dep"
intent = "induced"
research = "done"
status = "done"
refs = ["https://github.com/kubernetes-sigs/kro", "utils/simpleschema.go"]
+++

Only `pkg/simpleschema` is linked. The checks are apiextensions-apiserver's, which bring cel-go and
the API server's metrics: about 17 MB more in the kubecfg binary, from 87 MB to 104 MB. 5 MB of it
is the CRD validation that checks the schema itself, which also checks conversion webhook settings
and so brings the API server's webhook client.
