// A config checksum on the pod template, so pods restart when the ConfigMap
// changes. `self` in a mixin is the final stream, after every layer, so the
// checksum is of the ConfigMap the prod layer leaves.
local knit = import 'knit/knit.libsonnet';

knit.render(knit.registry, [
  import 'yaml:base.yaml',
  knit.mixin({
    local stream = self,
    [knit.id('apps/Deployment', 'web')]+: {
      spec+: { template+: { metadata+: { annotations+: {
        'checksum/config': std.md5(std.manifestJson(stream[knit.id('ConfigMap', 'web')].data)),
      } } } },
    },
  }),
  import 'yaml:prod.yaml',
])
