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

// Simple example to demonstrate kubecfg.

// This is a simple port to jsonnet of the standard guestbook example
// https://github.com/kubernetes/kubernetes/tree/master/examples/guestbook
//
// ```
// kubecfg update guestbook.jsonnet
// # poke at $(minikube service --url frontend), etc
// kubecfg delete guestbook.jsonnet
// ```

// This example uses knit, which comes with kubecfg. knit merges
// plain Kubernetes objects in layers, and matches list entries by
// their Kubernetes key: containers and env by name, ports by
// containerPort. See docs/Advanced-Usage/knit.md. knit is an alpha
// feature; .kubecfgrc.jsonnet turns it on.
local knit = (import 'kubecfg.libsonnet').knit;

// Templates under every object: every container gets these resource
// requests unless it sets its own.
local registry = knit.registry {
  types+: {
    Container+: { resources: { requests: { cpu: '100m', memory: '100Mi' } } },
  },
};

local deployment(name, container) = {
  apiVersion: 'apps/v1',
  kind: 'Deployment',
  metadata: { name: name, labels: { name: name } },
  spec: {
    selector: { matchLabels: { name: name } },
    template: {
      metadata: { labels: { name: name } },
      spec: { containers: [container] },
    },
  },
};

// A Service for every port of a Deployment's containers.
local service(deploy) = {
  apiVersion: 'v1',
  kind: 'Service',
  metadata: deploy.metadata,
  spec: {
    selector: deploy.spec.selector.matchLabels,
    ports: [
      { port: p.containerPort, targetPort: p.containerPort }
      for c in deploy.spec.template.spec.containers
      for p in c.ports
    ],
  },
};

// A function that returns 2 k8s objects: a redis Deployment and Service
local redis(name) = {
  // "self" is this object, so the Service follows the Deployment.
  deploy: deployment(name, {
    name: 'redis',
    image: 'bitnami/redis:4.0.9',
    ports: [{ containerPort: 6379 }],
    env: [
      { name: 'REDIS_REPLICATION_MODE', value: 'master' },
      { name: 'ALLOW_EMPTY_PASSWORD', value: 'yes' },
    ],
  }),
  svc: service(self.deploy),
};

local frontend = deployment('frontend', {
  name: 'php-redis',
  image: 'gcr.io/google-samples/gb-frontend:v3',
  ports: [{ containerPort: 80 }],
  readinessProbe: { httpGet: { path: '/', port: 80 } },
  livenessProbe: self.readinessProbe { initialDelaySeconds: 10 },
}) { spec+: { replicas: 3 } };

// Note the jsonnet file evaluates to the last object in the file.
// Kubecfg expects this object to be a possibly nested collection
// (array or object) of Kubernetes API objects. knit.render keeps
// the shape of the first layer.
knit.render(registry, [
  {
    frontend: {
      deploy: frontend,
      svc: service(frontend) { spec+: { type: 'LoadBalancer' } },
    },
    master: redis('redis-master'),
    slave: redis('redis-slave'),
  },

  // A second layer "overrides" some parameters in the slave redis
  // Deployment. It's plain Kubernetes: knit finds the Deployment by
  // kind and name, the container by name, and the env entries by name.
  {
    apiVersion: 'apps/v1',
    kind: 'Deployment',
    metadata: { name: 'redis-slave' },
    spec: {
      replicas: 2,
      template: { spec: { containers: [{
        name: 'redis',
        env: [
          { name: 'REDIS_REPLICATION_MODE', value: 'slave' },
          { name: 'REDIS_MASTER_HOST', value: 'redis-master' },
          { name: 'REDIS_MASTER_PORT_NUMBER', value: '6379' },
        ],
      }] } },
    },
  },
])
