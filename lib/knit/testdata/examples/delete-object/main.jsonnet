// The base seeds test data; the prod stack drops that Job.
local knit = (import 'kubecfg.libsonnet').knit;

knit.render(knit.registry, [
  import 'yaml:base.yaml',
  import 'yaml:prod.yaml',
])
