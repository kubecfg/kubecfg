// args is atomic: a later layer replaces it unless it asks to append.
local knit = (import 'kubecfg.libsonnet').knit;

knit.render(knit.registry, [
  import 'yaml:base.yaml',
  import 'yaml:prod.yaml',
])
