// The base seeds test data; the prod stack drops that Job.
local knit = import 'knit/knit.libsonnet';

knit.render(knit.registry, [
  import 'yaml:base.yaml',
  import 'yaml:prod.yaml',
])
