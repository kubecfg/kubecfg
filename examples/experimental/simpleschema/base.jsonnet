// Two apps made from their settings. The asserts check the settings against a kro
// SimpleSchema, app-schema.yaml, as an overlay leaves them: see prod.jsonnet.
//
//   kubecfg show base.jsonnet
local kubecfg = import 'kubecfg.libsonnet';

// A Deployment, and a Service if the app has ports.
local app(name, s) = {
  deployment: {
    apiVersion: 'apps/v1',
    kind: 'Deployment',
    metadata: { name: name },
    spec: {
      replicas: s.replicas,
      selector: { matchLabels: { app: name } },
      template: {
        metadata: { labels: { app: name } },
        spec: {
          containers: [{
            name: name,
            image: s.image,
            [if 'args' in s then 'args']: s.args,
            [if 'env' in s then 'env']: [{ name: k, value: s.env[k] } for k in std.objectFields(s.env)],
            [if 'ports' in s then 'ports']: [{ name: p.name, containerPort: p.port } for p in s.ports],
          }],
        },
      },
    },
  },
  [if 'ports' in s then 'service']: {
    apiVersion: 'v1',
    kind: 'Service',
    metadata: { name: name },
    spec: {
      selector: { app: name },
      ports: [{ name: p.name, port: p.port } for p in s.ports],
    },
  },
};

{
  // An overlay can change the schema too: appSchema+: {spec+: {...}}.
  appSchema:: import 'yaml:app-schema.yaml',

  // The settings. An overlay changes them with settings+: {web+: {...}}.
  settings:: {
    web: { image: 'nginx:1.27', replicas: 2, ports: [{ name: 'http', port: 80 }] },
    worker: { image: 'busybox:1.36', replicas: 1, args: ['sleep', 'infinity'], env: { QUEUE: 'dev' } },
  },

  assert kubecfg.validateSimpleSchema(self.settings.web, self.appSchema.spec, self.appSchema.types),
  assert kubecfg.validateSimpleSchema(self.settings.worker, self.appSchema.spec, self.appSchema.types),

  web: app('web', $.settings.web),
  worker: app('worker', $.settings.worker),
}
