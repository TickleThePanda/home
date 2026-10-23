---
paths:
  - "ansible/**"
---

# Ansible

- **Never scope a run with `--limit` alone.** `k8s-storage-anchor-01` is in
  both `pi` and `agent`, so limiting to `pi` still runs the k3s agent play on
  it and fails mid-reinstall. Run whole layer playbooks, as CI does.
- **Every play that gathers facts sets `gather_subset: [!all, min, network]`.**
  The fact cache is shared across layers, so a play that gathers less starves
  one that needs more. `ansible.cfg` can't set this; repeat it in every new
  play.
- Code shared between layers becomes a role in `ansible/roles/`,
  parameterised by `group_vars`. Check for per-host differences before
  merging (e.g. the Pi's extra LVM collector in `node_exporter`).
