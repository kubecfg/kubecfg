local process = function(request) {
  _config:: {
    image: 'k8s.gcr.io/pause:3.9',
    instanceName: 'useless-' + request.parent.spec.name,
  },

  deployment:: {
    apiVersion: 'apps/v1',
    kind: 'Deployment',
    metadata: {
      name: $._config.instanceName,
    },
    spec: {
      replicas: 1,
      selector: {
        matchLabels: {
          app: $._config.instanceName,
        },
      },
      template: {
        metadata: {
          labels: {
            app: $._config.instanceName,
          },
        },
        spec: {
          containers: [
            {
              name: 'useless',
              image: $._config.image,
              imagePullPolicy: 'Always',
            },
          ],
        },
      },
    },
  },

  resyncAfterSeconds: 30.0,
  status: {
    observedGeneration: std.get(request.parent.metadata, 'generation', 0),
    ready: if std.length(std.objectFields(request.children)) > 0 then 'true' else 'false',
  },
  children: [
    $.deployment,
  ],
};

//Top Level Function
function(request)
  local response = process(request);
  std.trace('request: ' + std.manifestJsonEx(request, '  ') + '\n\nresponse: ' + std.manifestJsonEx(response, '  '), response)
