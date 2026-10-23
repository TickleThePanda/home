---
paths:
  - ".github/**"
  - "scripts/check-traefik-pin.sh"
---

# CI pipeline

- `deploy.yaml` holds the trigger, the `dorny/paths-filter` step and the
  `preflight` job. `infra`, `cluster` and `terraform` are separate files
  (`deploy-infra.yaml`, `deploy-cluster.yaml`, `deploy-terraform.yaml`)
  called via `workflow_call`. Keep them split: paths-filter works per file,
  so with one file, editing any job's section reran every layer.
  `preflight` stays in `deploy.yaml` because it computes every other job's
  gating outputs.
- **`infra` is one job for all four Ansible layers. Don't split it.** Runner
  setup, checkout, the WARP connect and `ansible-setup` are most of a no-op
  deploy's cost. The layers are sequential `ansible-playbook` calls under
  `set -euo pipefail`, which already stops later layers on a failure.
- Layer order: `proxmox` (creates VMs) → `node` (Pi mounts that
  `k3s-agent` requires) → `k3s` (before `cluster`, since a k3s bump changes
  the served Traefik chart) → `router` last, since it is the one layer that
  can drop CI's own egress.
- `terraform` runs last for the same reason: it owns the tunnel and the
  private-network routes CI connects through. It doesn't need WARP itself.
- Downstream `if:`s treat an upstream *skip* as a pass and a failure as a
  block. `concurrency: group: cluster` prevents overlapping runs.
- The `shared` filter (`ansible/inventory.yml`, `group_vars/`, `host_vars/`,
  `roles/`, `ansible.cfg`, `site.yml`, `requirements.yml`, `deploy.yaml`,
  `.github/actions/**`) runs every Ansible layer. This is load-bearing:
  without it a shared-role change would apply nowhere. `workflow_dispatch`
  runs everything.
- `.github/actions/ansible-setup` installs a pinned `ansible-core`
  (`requirements.txt`, bumped in its own commit) plus collections, with
  caches. `.github/actions/ansible-apply` applies one layer, then re-runs it
  with `--check` to prove idempotence, but only if the apply changed
  something.
- `deploy-cluster.yaml`'s "Confirm the promoted revision reconciles" step is
  `continue-on-error`. It reports a failed Kustomization as error
  annotations and a run summary rather than blocking `terraform`.
- `scripts/check-traefik-pin.sh` (run in `preflight`) enforces the
  k3s-version / Traefik-chart pair. See the k3s rules.
