# kubelan

kubelan provides an unmanaged Layer 2 network to a set of pods (specifically
all the pods pointed to by a configured set of services). It's implemented
with VXLAN, similar to how CNI's such as Flannel work. It's up to the pod to
decide how to use this network (no IPAM is provided, something like
[dnsmasq-k8s](https://github.com/devplayer0/dnsmasq-k8s)) might be useful.

## Example

kubelan is deployed as a sidecar along with each of your application's pods.
See [`k8s-example.yaml`](k8s-example.yaml) for a sample StatefulSet containing
3 replicas that will be networked together (by a headless Service, since the
application isn't exposing anything to Kubernetes).

To deploy the example, run `kubectl apply -f k8s-example.yaml`. Each pod in the
StatefulSet will have an IP address of the form `192.168.69.<replica+1>`. To
prove the layer 2 network is working, run the following:

```bash
kubectl exec kubelan-0 -c alpine -- arping -I kubelan 192.168.69.3
```

You should see something similar to the following:

```
ARPING 192.168.69.3 from 192.168.69.1 kubelan
Unicast reply from 192.168.69.3 [5e:7c:00:f7:51:e6] 0.023ms
Unicast reply from 192.168.69.3 [5e:7c:00:f7:51:e6] 0.035ms
Unicast reply from 192.168.69.3 [5e:7c:00:f7:51:e6] 0.044ms
Unicast reply from 192.168.69.3 [5e:7c:00:f7:51:e6] 0.042ms
```

(You can remove the deployment with `kubectl delete -f k8s-example.yaml`)

## Configuration

Configuration is done by either a YAML file in a ConfigMap (see the
sample), or with environment variables. For all configuration options and what
they do, see [`kubelan.sample.yaml`](kubelan.sample.yaml). Any configuration
option can be set by environment variable by adding a `KL_` prefix, replacing
`.`s in YAML paths with `_` and using uppercase. For example, `vxlan.interface`
would become `KL_VXLAN_INTERFACE`.

Note that `ip` and `namespace` are set by using pod field values exposed as
environment variables (see
[here](https://kubernetes.io/docs/tasks/inject-data-application/environment-variable-expose-pod-information/))
for an explanation). The `kubelan` container also needs `CAP_NET_ADMIN` to
create and manage the VXLAN interface. If you want to manage the interface
through your own container (e.g. add to add an IP address), you'll need to add
the same capability.

Also worth noting is that `kubelan` needs to be able to watch
[EndpointSlices](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.19/#watch-list-endpointslice-v1beta1-discovery-k8s-io)
in your cluster, so you'll need an appropriate `ClusterRole` and
`ClusterRoleBinding` as given in the example.
