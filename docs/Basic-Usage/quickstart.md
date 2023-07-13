# Quickstart

```console
# Show generated YAML
% kubecfg show -o yaml examples/guestbook.jsonnet

# Create resources
% kubecfg update examples/guestbook.jsonnet

# Modify configuration (downgrade gb-frontend image)
% sed -i.bak '\,gcr.io/google-samples/gb-frontend,s/:v4/:v3/' examples/guestbook.jsonnet
# See differences vs server
% kubecfg diff examples/guestbook.jsonnet

# Update to new config
% kubecfg update examples/guestbook.jsonnet

# Clean up after demo
% kubecfg delete examples/guestbook.jsonnet
```
