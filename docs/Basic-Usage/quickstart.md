# Quickstart

All commands in this quickstart are run in the [examples](https://github.com/kubecfg/kubecfg/tree/main/examples) directory using the `guestbook` code

**WARNING** All commands will use the currently defined context similar to `kubectl` , be sure not to run these commands with the wrong context

```console
# Show generated YAML
% kubecfg show -o yaml examples/guestbook.jsonnet
```

```console
# Create resources in the kubernetes cluster defined in the current context
% kubecfg update examples/guestbook.jsonnet
```

```console
# Modify _something_ (downgrade gb-frontend image)
% sed -i.bak '\,gcr.io/google-samples/gb-frontend,s/:v4/:v3/' examples/guestbook.jsonnet

# See differences vs server
% kubecfg diff examples/guestbook.jsonnet
```

```console
# Update the kubernetes resources to the new configuration
% kubecfg update examples/guestbook.jsonnet
```

```console
# Clean up after demo
% kubecfg delete examples/guestbook.jsonnet
```
