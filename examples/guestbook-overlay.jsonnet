// guestbook.jsonnet with a label on the redis-master Service.
//
//   kubecfg show guestbook-overlay.jsonnet
(import 'guestbook.jsonnet') + {
  master+: {
    svc+: {
      metadata+: {
        labels+: {
          demo: 'label',
        },
      },
    },
  },
}
