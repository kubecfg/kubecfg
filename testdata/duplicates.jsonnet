{
  t1: {
    o1: {
      apiVersion: 'v1',
      kind: 'ConfigMap',
      metadata: {
        name: 'foo',
        namespace: 'myns',
      },
      data: {
        some: 'value1',
      },
    },
  },
  t2: {
    o2: {
      apiVersion: 'v1',
      kind: 'ConfigMap',
      metadata: {
        name: 'foo',
        namespace: 'myns',
      },
      data: {
        some: 'value2',
      },
    },
  },
}
