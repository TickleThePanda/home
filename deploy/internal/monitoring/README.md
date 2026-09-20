# monitoring

Prometheus + Alertmanager + Grafana + kube-state-metrics. 1-minute scrape
interval, 30 days of retention. See the plan this was built from for the
full metric/alert inventory.

Sources: Glances' own Prometheus exporter on every host (`ansible/roles/glances/`,
port 9091), kubelet's `/metrics`, `/metrics/cadvisor` and `/metrics/resource`
(cluster pod/node resource usage, plus per-PVC usage), node_exporter's
textfile collector on the Pi (`ansible/roles/lvm_exporter/`, the `data`
volume group's free space), k3s's embedded etcd, kube-state-metrics (pod
restarts, node conditions, deployment/PVC status), and any pod carrying
`prometheus.io/scrape: "true"` annotations (Traefik, CloudNativePG).

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
  tinyauth authenticated you, so Grafana auto-provisions that user as an Org
  Admin on first visit.
- **Prometheus and Alertmanager** are ClusterIP-only (no IngressRoute), same
  trust model as the rest of this namespace.

## Email

Alertmanager sends through `deploy/internal/services/smtp-relay/` (an
in-cluster relay, its own service, not owned by this namespace) rather than
holding an SMTP credential itself -- see that service's README for the
one-time credential setup.

## Verifying metric names against the real exporters

A few PromQL expressions here (Glances' `[fs]`/`[network]`/`[diskio]`/
`[sensors]` field names, CNPG's `cnpg_backends_total`, Traefik's
`traefik_service_*`) were written from documentation and public issues, not
a live scrape -- Glances in particular has no single authoritative metric
list for its Prometheus exporter. After the first apply, check each source
directly and adjust `prometheus/rules.yml` or the dashboard JSON under
`grafana/dashboards/` if a name differs:

```sh
curl -s http://192.168.1.2:9091/metrics | grep glances_fs   # any host, any glances_* plugin
kubectl -n monitoring exec deploy/prometheus -- wget -qO- localhost:9090/api/v1/targets  # scrape target health
```
