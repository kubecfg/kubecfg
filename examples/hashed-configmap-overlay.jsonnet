// hashed-configmap.jsonnet with another log level. The ConfigMap gets another name, and the
// Deployment's reference follows it.
//
//   kubecfg show hashed-configmap-overlay.jsonnet
(import 'hashed-configmap.jsonnet') + {
  config+: { data+: { LOG_LEVEL: 'debug' } },
}
