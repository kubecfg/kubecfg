// The CRD in the stream is the schema: listeners merge by name.
local knit = (import 'kubecfg.libsonnet').knit;

knit.render(knit.registry, [
  import 'yaml:crd.yaml',
  import 'yaml:base.yaml',
  import 'yaml:prod.yaml',
])
