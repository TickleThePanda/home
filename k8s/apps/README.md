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

## Egress

Only services that hold credentials and have a short, fixed list of
destinations get a `restrict-egress` policy: LLDAP, TinyAuth, the SMTP
relay, Headlamp, Postgres and Pocket ID. Each allows DNS (kube-system's
CoreDNS) plus its known destinations:

- **API server:** `ipBlock: 192.168.1.32/32`, port `6443`. kube-proxy has
  already rewritten `kubernetes.default` to that endpoint when the policy
  is checked, so the Service IP never matches.
- **Internet only:** `0.0.0.0/0` except RFC 1918 and `100.64.0.0/10`
  (tailnet), on a single port.

Other workloads keep open egress. A blocked egress connection fails fast
(kube-router rejects it), so check the pod's `restrict-egress` first when
one of these services reports connection refused.

## Exceptions

- **Node-local traffic is always allowed.** kube-router accepts anything
  from the pod's own node, so kubelet probes work. A `hostNetwork` pod can
  therefore reach pods on its own node.
- **Masqueraded traffic looks node-local.** A LoadBalancer Service with
  `externalTrafficPolicy: Cluster` SNATs LAN clients to the node's address.
  Traefik's Services do this; Traefik itself has no policy.
- **Multus interfaces aren't filtered.** Kea's macvlan (`192.168.1.11`)
  bypasses the host's iptables. Policies govern only its `eth0`.
- **Unrestricted namespaces:**
  - `kube-system`, `cert-manager`, `metallb-system`, `cnpg-system`,
    `openebs`: the API server calls their webhooks from the control-plane
    node, and Traefik must reach every backend.
  - `cloudflared`, `kea`, `external-dns`, `reflector`: no inbound clients,
    so a policy would add risk without protecting anything.
  - `flux-system`: uses the policies the FluxInstance creates, plus
    `internal/apps/flux-operator/`.
- **`commonLabels` leak into selectors.** Kustomize adds them to every
  `podSelector`, including the Traefik one, which then matches nothing.
  Keep a policy out of any kustomization that sets them (see
  `home/network-policy.yaml`).
- **Denied traffic is rejected, not dropped.** A missing allow often looks
  like "connection refused" or an instant 502 from Traefik, not a timeout.

## Debugging

To test a path, run a throwaway pod in the client's namespace with
matching labels:

```sh
kubectl run np-probe -n geopulse --labels app=geopulse-backend \
  --image=busybox:1.37.0 --restart=Never --rm -it -- \
  sh -c 'sleep 15; nc -zv -w 3 postgres-rw.postgres 5432'
```

- kube-router takes several seconds to add a new pod's IP to the allow
  rules, so a pod's first connections can fail. Hence the `sleep`; retry
  a failure once before trusting it. Real apps retry through it.
- Don't copy labels a StatefulSet or CNPG selects on (`app: pocket-id`,
  `app: mosquitto`, `cnpg.io/cluster`). Their controllers may adopt the
  probe. Test from the real pod with `kubectl exec` instead.

Policies don't touch the API server, SSH or Traefik, so a bad policy can't
lock you out. To lift one fast, delete it; Flux restores it within 10
minutes, so suspend the `internal` Kustomization first if the fix needs
longer:

```sh
flux suspend kustomization internal
kubectl delete networkpolicy -n <ns> default-deny-ingress
```

Then fix it in git, push, and `flux resume kustomization internal`.
