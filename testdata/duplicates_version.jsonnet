{
  t1: {
    o1: {
      apiVersion: 'networking.k8s.io/v1beta1',
      kind: 'Ingress',
      metadata: {
        name: 'foo',
        namespace: 'myns',
      },
    },
  },
  t2: {
    o2: {
      apiVersion: 'networking.k8s.io/v1',
      kind: 'Ingress',
      metadata: {
        name: 'foo',
        namespace: 'myns',
      },
    },
  },
}
