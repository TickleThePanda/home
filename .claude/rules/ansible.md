---
paths:
  - "ansible/**"
  - "bootstrap/**"
---

# Ansible project

## Layout

- `ansible/` is one project for every managed host: one `ansible.cfg`,
  `inventory.yml`, `known_hosts`, `requirements.yml` (union of all
  collections), and shared `group_vars/`, `host_vars/`, `roles/`.
- Each layer has its own directory with a playbook named after it, plus
  `tasks/`, `templates/`, `vars/`: `proxmox/`, `node/`, `k3s/`, `router/`.
  `site.yml` imports them in dependency order. `bootstrap/router/` is
  hand-flashed and not part of this.
- Connection differences are `group_vars`: the router is `root` with
  `ansible_python_interpreter: /dev/null`, proxmox-01 is `root`, everything
  else is `deploy`.
- `localhost` is in its own `control` group, for `community.proxmox`'s
  `delegate_to: localhost`. No play uses `hosts: all`, since that would run
  `become` tasks on the runner.
- Code shared between layers becomes a role in `ansible/roles/`,
  parameterised by `group_vars`. Watch for per-host differences when
  merging. `node_exporter` is identical everywhere except the Pi, which also
  runs the LVM textfile collector (`node_exporter_lvm_collector`).

## Rules

- **Every play that gathers facts uses `gather_subset: [!all, min, network]`.**
  The fact cache is shared and `gathering = smart` lets a cached entry win,
  so a play that gathered less would starve one that needs more
  (`k3s.orchestration` reads `all_ipv6_addresses` and `default_ipv4`). There
  is no `ansible.cfg` key for this: `gather_subset` under `[defaults]` is
  silently ignored. Repeat it per play.
- **Never scope a run with `--limit` alone.** `k8s-storage-anchor-01` is in
  both `pi` and `agent`, so limiting to `pi` still matches the k3s agent
  play. That reinstalls k3s on the Pi, then fails on an undefined `token`.
  Select whole layer playbooks instead, as CI does.

## SSH keys

- `ansible/files/ssh-authorized-keys` is the single list of keys trusted as
  the automation user (`deploy` / `root`). It holds public keys and is
  committed. It is exposed as `ssh_authorized_keys` in `group_vars/all.yml`.
- The `ssh_keys` role applies it with `exclusive: true`, so removing a key
  and re-applying revokes it. It is wired into `node/node.yml` (Pi),
  `k3s/k3s.yml` (the VMs, not the Pi) and `proxmox/proxmox.yml`
  (`/etc/pve/priv/authorized_keys`).
- The router has no Python, so `router/tasks/ssh-keys.yml` templates the
  same file into `/etc/dropbear/authorized_keys`.
- Cloud-init (`proxmox/tasks/vm-provision.yml`) and the router image bake
  (`bootstrap/router/build.sh`) read the same file, so new hosts trust it
  from first boot.
- A key for one host only goes in `ssh_authorized_keys_extra` in that
  group's vars, e.g. proxmox-01's own self-key in `group_vars/proxmox.yml`.
  Otherwise `exclusive: true` strips it.
- `panda`'s interactive login to the Pi is managed separately, out of band.
