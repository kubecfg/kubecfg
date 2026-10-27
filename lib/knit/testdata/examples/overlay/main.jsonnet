local knit = import 'knit/knit.libsonnet';

local registry = knit.registry {
  objects+: { metadata+: { labels+: { team: 'platform' } } },
  types+: { PodTemplateSpec+: { spec+: { nodeSelector+: { 'kubernetes.io/os': 'linux' } } } },
};

knit.render(registry, [
  import 'yaml:base.yaml',
  import 'yaml:prod.yaml',
])
