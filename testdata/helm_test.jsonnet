local kubecfg = import 'kubecfg.libsonnet';

local data = importbin './mysql-8.8.26.tgz';

kubecfg.parseHelmChart(data, 'rls', 'myns', {
  auth: {
    forcePassword: false,
    password: 'foo',
  },
})
