+++
title = "Helm charts"
summary = "parseHelmChart: render a Helm chart inside Jsonnet, then change it like any object"
aka = ["parseHelmChart", "helm"]
kind = "lang"
intent = "goal"
research = "done"
refs = ["utils/nativefuncs.go", "docs/Advanced-Usage/helm-integration.md"]

[[feature]]
id = "render"
title = "Render a chart from its archive, with a release name, namespace and values"
status = "done"

[[feature]]
id = "capabilities"
title = "Pass capabilities, such as the Kubernetes version"
status = "done"

[[feature]]
id = "dependency-conditions"
title = "Honour dependencies.condition, so alertmanager.enabled: false drops the subchart"
status = "done"
note = "Also tags, alias and import-values: the same step helm install runs. Started as contributor PR #446"
refs = ["https://github.com/kubecfg/kubecfg/pull/446"]

[[needs]]
on = "/deps/helm"
why = "Helm's own engine renders the chart"
+++
