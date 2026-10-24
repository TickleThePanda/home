# Home infrastructure monorepo

Four-node k3s cluster running the owner's home services, plus the Ansible,
Terraform and app source behind it. `README.md` has the network map (node
IPs, ingress IPs, address assignments). Area-specific notes live in
`.claude/rules/` and load when you touch the matching files.

## Workflow

Commit straight to `main`; don't branch.

## Writing style

Documentation and comments: simple, concise, practical.

- Use short sentences and paragraphs. Assume a competent developer.
- Explain only what the reader needs. Skip the obvious, avoid repetition and
  filler.
- Prefer concrete examples over long explanations. Use bullets when they
  improve scanning.
- Prefer self-explanatory code to comments. Comments should explain
  non-obvious behaviour, constraints, invariants, or rationale.
- Document the current code for developers without the chat context. Explain
  relevant behaviour, constraints, and rationale, not implementation history,
  rejected approaches, or the conversation that produced it.
- Use active voice. Make the actor clear.
- Prioritise clarity and completeness over brevity when they conflict.

## Cluster

- **Server:** `k3s-vm-control-01` (`192.168.1.32`), amd64 Debian 13 VM on
  `proxmox-01`. Sole control-plane, single-member embedded etcd. Tainted
  `ticklethepanda.dev/control-plane:NoSchedule`.
- **Agents:** `k8s-storage-anchor-01` (`192.168.1.2`, Raspberry Pi, arm64)
  and `k3s-vm-worker-01` / `-02` (`192.168.1.33`–`.34`, amd64). The Pi holds
  every `lvm-data` volume on its SSD; its `PreferNoSchedule` taint steers
  portable pods to the workers.
- Every image must be multi-arch (`linux/amd64` + `linux/arm64`), since a
  stateless pod can land on either. Their `lvm-data` PVC, not a
  `nodeSelector`, pins Pi-bound workloads to the Pi.
- SSH by IP, no host aliases. `panda@192.168.1.2` for the Pi; every node
  takes the `deploy` CI key, and control-plane work goes through
  `deploy@192.168.1.32`. `panda`'s sudo on the Pi is deliberately narrow
  (`k3s-agent` control, reading k3s config). Anything broader needs the
  owner.
- The local kubeconfig's client cert doesn't auto-rotate. On "server has
  asked for the client to provide credentials", restart k3s on
  `k3s-vm-control-01`, re-pull `/etc/rancher/k3s/k3s.yaml` into
  `~/.kube/config`, and set `server:` to `192.168.1.32`.

## Networking

- Five VLANs, each an untagged bridge on the router: `homelab`
  (`192.168.1.0/24`, every node and cluster IP), `trusted`
  (`192.168.10.0/24`, open to `homelab` for now), and `iot_inet` /
  `iot_local` / `iot_echo` (`192.168.20/30/40.0/24`). IoT VLANs can be
  reached from `homelab` / `trusted` / tailnet but can't initiate back or
  reach each other.
- `homelab` address bands: `.1` gateway, `.2–.9` physical hosts, `.10–.31`
  cluster service IPs (MetalLB + Kea), `.32–.63` Proxmox guests, `.64–.127`
  other static, `.128–.254` DHCP. Put new addresses in their band. Only the
  Kea pool and `deploy/setup-config/metallb/` enforce it.
- **MetalLB** L2 pools: `external`, `internal`, `pihole`. **Traefik** is the
  only ingress. Internal apps use
  ``Host(`<app>.internal.ticklethepanda.co.uk`)`` on `int-web-secure`
  (wildcard cert, `internal` pool); external apps use `ext-web` (`external`
  pool). **tinyauth** forward-auth (pocket-id OIDC + lldap) gates most
  internal apps. **cloudflared** tunnels selected services to the internet.
- DNS: clients → Pi-hole → Unbound → CoreDNS, which is authoritative for
  `internal.ticklethepanda.co.uk` only. Its wildcard already points at the
  internal ingress, so a new internal app needs no DNS change. BIND serves
  `home.arpa` and reverse zones, fed by Kea DDNS and ExternalDNS.
- **CI reaches the LAN only through the in-cluster `cloudflared`** (Zero Trust
  private-network routes, two replicas on the workers). With the server or
  both workers down, CI is blind. Humans aren't: the router runs its own
  Tailscale subnet router and exit node. See `ansible/node/RECOVERY.md` and
  `ansible/k3s/RECOVERY.md`.

## Deploying

- `.github/workflows/deploy.yaml` runs on push to `main`, in order:
  `preflight` (pin guard, tunnel/API readiness), `infra` (all Ansible
  layers), `cluster` (Flux), `terraform` (Cloudflare Zero Trust, last). Each
  layer is skipped when nothing under its tree changed.
- `cluster` pushes the repo as an OCI artifact and moves its `deploy` tag.
  Flux reconciles only that tag, never `main`, through Kustomizations
  `setup` → `setup-config` → `home` / `internal` (plus `flux-system`), with
  `prune` and `wait`, every 10m.
- Flux manages `flux-system/` itself; CI never applies it. Flux Operator
  installs the controllers from the FluxInstance in
  `flux-system/instance/`. Never delete it: that uninstalls Flux.
  `flux-system/bootstrap.sh` is the manual path for a cold start or a Flux
  too broken to fix itself.
- **Flux reverts manual changes.** `kubectl edit`/`patch` or
  `./apply-subset.sh` on anything under `deploy/` is undone within 10
  minutes unless committed and pushed. Objects Flux doesn't own (PVCs
  created by operators, cert-manager CertificateRequests) are not reverted.
- For an out-of-band fix to one component, use
  `./apply-subset.sh <deploy-subdir> [kubectl apply args...]`, never
  `kubectl apply -f` or `kubectl apply -k deploy/<subdir>`.
- Flux image automation bumps third-party image tags and commits them to
  `main`. Majors are manual.

## Administration

- Run Ansible by whole layer playbook (`ansible/site.yml` lists them), never
  with `--limit` alone. The Pi is in both `pi` and `agent`, so `--limit pi`
  still runs the k3s agent play on it.
- k3s re-applies its bundled addons from
  `/var/lib/rancher/k3s/server/manifests/` on every restart. If the repo
  owns a resource k3s also bundles, disable the addon in `server_config_yaml`
  (`ansible/group_vars/k3s_cluster.yml`) and vendor the resource, as with
  `deploy/setup/traefik/`. Disabling an addon is a real Helm uninstall and
  can cascade (Traefik's took its CRDs, and every IngressRoute/TLSStore with
  them). Afterwards, reapply affected apps and re-check TLS, since a missing
  `TLSStore` silently falls back to a self-signed cert.
- Objects stuck `Terminating` drift invisibly while `apply` keeps
  "succeeding". Spot check:
  `kubectl get svc -A -o json | jq -r '.items[] | select(.metadata.deletionTimestamp != null) | "\(.metadata.namespace)/\(.metadata.name)"'`
  (swap the kind). Clearing a finalizer is a real mutation; ask the owner
  first.
- A `raw.githubusercontent.com` URL in a kustomize `resources:` list can be
  misparsed as a git clone and fail with 429/503. Check
  `https://www.githubstatus.com/api/v2/status.json` before assuming local
  throttling.
