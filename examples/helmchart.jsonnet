// Copyright 2017 The kubecfg authors
//
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

local kubecfg = import 'kubecfg.libsonnet';

{
  // this Url is relative to the CWD where kubecfg is run
  local chartUrl = 'testdata/kubernetes-dashboard-5.0.0.tgz',
  local name = 'kubernetes-dashboard',
  local namespace = 'kubernetes-dashboard',
  local values = {
    replicaCount: 2,
    labels: {
      foo: 'bar',
    },
    ingress: {
      enabled: true,
    },
  },

  local chart = kubecfg.helmTemplate(name, namespace, chartUrl, values),

  objects: chart {
    'kubernetes-dashboard/templates/deployment.yaml'+: {
      spec+: {
        template+: {
          spec+: {
            nodeSelector+: { 'kubernetes.io/arch': 'amd64' },
          },
        },
      },
    },
  },
}
