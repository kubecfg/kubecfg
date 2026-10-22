// expect: mixin layers [0] changed the object before
local knit = import 'knit/knit.libsonnet';
local hpa(v) = { apiVersion: 'autoscaling/' + v, kind: 'HorizontalPodAutoscaler', metadata: { name: 'web' } };
local web = knit.id('autoscaling/HorizontalPodAutoscaler', 'web');
knit.render(knit.registry, [
  knit.mixin({ [web]: hpa('v1') + { spec: { minReplicas: 2, maxReplicas: 5 } } }),
  hpa('v1'),
  hpa('v2'),
])
