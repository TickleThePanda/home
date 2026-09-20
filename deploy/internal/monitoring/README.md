# monitoring

Prometheus + Alertmanager + Grafana + kube-state-metrics. 1-minute scrape
interval, 30 days of retention. See the plan this was built from for the
full metric/alert inventory.

Sources: node_exporter on every host (`ansible/roles/node_exporter/`, port
9100 -- real host metrics with device/mountpoint/interface as labels, plus a
textfile collector on the Pi for the `data` volume group's free space, the
only host with it), kubelet's `/metrics`, `/metrics/cadvisor` and
`/metrics/resource` (cluster pod/node resource usage, plus per-PVC usage),
k3s's embedded etcd, kube-state-metrics (pod restarts, node conditions,
deployment/PVC status), and any pod carrying `prometheus.io/scrape: "true"`
annotations (Traefik, CloudNativePG).

Six dashboards: **Hosts** (fleet overview + a `$host`-filtered detail
section), **Storage** (LVM VG/PV, per-mount, per-PVC), **Cluster** (pod
CPU/memory top-10, restarts, node/deployment health), **Traefik**,
**Postgres**, **etcd** -- split out from a single "Platform" dashboard that
tried to cover all three at once.

Host metrics used to come from Glances, replaced because it encodes
device/mount/interface identity in the metric *name* instead of a label --
broke `rate()`/`topk()`/`sum by`, forced per-Glances-version query variants
(v3 on the Pi, v4 everywhere else, different metric names), and produced
15-140-line dashboard panels with no way to filter. node_exporter's labels
fix all of that.

Only Grafana has an IngressRoute
(`grafana.internal.ticklethepanda.co.uk`). Prometheus and Alertmanager stay
ClusterIP-only:

```sh
kubectl -n monitoring port-forward svc/prometheus 9090:9090
kubectl -n monitoring port-forward svc/alertmanager 9093:9093
```

## Auth

Neither Grafana, Prometheus nor Alertmanager has its own credential to
create or rotate.

- **Grafana** trusts tinyauth's forward-auth headers (`auth.proxy` in
  `grafana/deployment.yaml`) and has its own login form disabled --
  reaching `grafana.internal.ticklethepanda.co.uk` at all already means
  tinyauth authenticated you, so Grafana auto-provisions that user as a
  Viewer on first visit. Dashboards are provisioned from this repo, not
  clicked together in the UI, and there's no PVC to persist UI-made changes
  past a restart anyway (`grafana/deployment.yaml`'s `data` volume is an
  emptyDir -- dropped so Grafana isn't pinned to the Pi, the only node with
  `lvm-data`, alongside Prometheus).
- **Prometheus and Alertmanager** are ClusterIP-only (no IngressRoute), same
  trust model as the rest of this namespace.

## Email

Alertmanager sends through `deploy/internal/services/smtp-relay/` (an
in-cluster relay, its own service, not owned by this namespace) rather than
holding an SMTP credential itself -- see that service's README for the
one-time credential setup.

## Checking a scrape target directly

```sh
curl -s http://192.168.1.2:9100/metrics | grep node_load    # any host's node_exporter
kubectl -n monitoring exec deploy/prometheus -- wget -qO- localhost:9090/api/v1/targets  # scrape target health
```
