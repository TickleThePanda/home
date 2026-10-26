# Network policies

Each app namespace carries a `network-policy.yaml` next to its manifests.
k3s enforces it with its embedded kube-router policy controller.

## Conventions

- `default-deny-ingress` selects every pod in the namespace. Separate
  `allow-*` policies then admit specific clients on specific ports.
- Web UIs admit only Traefik, so TinyAuth can't be bypassed:

  ```yaml
  - from:
      - namespaceSelector:
          matchLabels:
            kubernetes.io/metadata.name: kube-system
        podSelector:
          matchLabels:
            app.kubernetes.io/name: traefik
  ```

- Ports are the pod's container ports, not the Service's.
- A pod that Prometheus scrapes by annotation needs an allow from
  `monitoring` / `app: prometheus` on its metrics port.
- A new in-cluster client of a protected service (another SMTP sender,
  another Postgres database user) needs an entry in that service's policy.
  Clients that go through Traefik need nothing.

## Exceptions

- **Node-local traffic is always allowed.** kube-router accepts anything
  from the pod's own node, so kubelet probes work. A `hostNetwork` pod can
  therefore reach pods on its own node.
- **Masqueraded traffic looks node-local.** A LoadBalancer Service with
  `externalTrafficPolicy: Cluster` SNATs LAN clients to the node's address.
  Traefik's Services do this; Traefik itself has no policy.
- **Multus interfaces aren't filtered.** Kea's macvlan (`192.168.1.11`)
  bypasses the host's iptables. Policies govern only its `eth0`.
- **Unrestricted namespaces:** `kube-system`, `cert-manager`,
  `metallb-system`, `cnpg-system` and `openebs`. Their webhooks are called
  by the API server from the control-plane node, and Traefik and
  cloudflared must reach any backend. `flux-system` uses the policies the
  FluxInstance creates, plus `apps/flux-operator/`.

## Debugging

A missing allow shows up as a client timeout, not a refusal. To test a
path, run a throwaway pod in the client's namespace with matching labels:

```sh
kubectl run np-probe -n geopulse --labels app=geopulse-backend \
  --image=busybox:1.37.0 --restart=Never --rm -it -- \
  sh -c 'sleep 8; nc -zv -w 3 postgres-rw.postgres 5432'
```

kube-router takes a few seconds to add a new pod's IP to the allow rules,
so a pod's first connections can time out. Hence the `sleep`. Real apps
retry through it.

Policies don't touch the API server, SSH or Traefik, so a bad policy can't
lock you out. To lift one fast, delete it; Flux restores it within 10
minutes, so suspend the `internal` Kustomization first if the fix needs
longer:

```sh
flux suspend kustomization internal
kubectl delete networkpolicy -n <ns> default-deny-ingress
```

Then fix it in git, push, and `flux resume kustomization internal`.
