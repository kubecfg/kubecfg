// base.jsonnet in production: more web replicas, and a queue for the worker. The base's
// asserts check these settings too.
//
//   kubecfg show prod.jsonnet
(import 'base.jsonnet') + {
  settings+: {
    web+: { replicas: 5 },
    worker+: { env: { QUEUE: 'prod' } },
  },
}
