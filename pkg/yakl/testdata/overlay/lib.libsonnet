{
  conf(name):: name + '-conf',
  probes(port):: { readinessProbe: { httpGet: { path: '/healthz', port: port } } },
}
