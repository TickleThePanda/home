# proxmox

Config for `proxmox-01` (`192.168.1.3`), the home hypervisor — Proxmox VE 9
on Debian 13 — and the k3s VMs on it. The source of truth from here on;
anything not covered is unmanaged.

Applied by the `proxmox` job in `.github/workflows/deploy.yaml` on every push
touching `proxmox/**`. Two access paths in the one play:

- **SSH as `root`** (through the same cloudflared tunnel as `node`/`router`,
  CI key `NODE_SSH_KEY`, already in `/etc/pve/priv/authorized_keys`) — the
  apt repos and the Debian 13 template build (`qm`; the API has no call to
  import a downloaded qcow2 as a VM disk).
- **Proxmox API** (`192.168.1.3:8006`, `delegate_to: localhost`) — the three
  k3s VMs, via `community.proxmox`. Auth is a `root@pam` API token
  (`PROXMOX_API_TOKEN_ID` / `PROXMOX_API_TOKEN_SECRET` in the `prod`
  environment).

## What it manages

| Area | File |
|---|---|
| apt repos — enterprise off, no-subscription on (PVE + Ceph) | `tasks/apt-repos.yml` |
| Debian 13 `genericcloud` template (VMID 9000) | `tasks/vm-template.yml` |
| the three k3s agent VMs (`k3s-vm-control-01` / `-worker-01` / `-worker-02`) | `tasks/vm-provision.yml` |

VM sizing, IPs and VMIDs are in `vars/main.yml` (`k3s_vms`). k3s *on* the VMs
is `k3s-cluster/`'s job, not this one.

Not managed: the subscription key / nag, LXC containers, non-k3s VMs and all
VM/CT storage, the cluster config and `/etc/pve`, `authorized_keys`, and
**VM deletion** — never automated (same principle as `node/` never owning the
partition table).

## Idempotence

- The template build is skipped once VMID 9000 exists. Rebuild = delete it by
  hand first.
- A VM is cloned + configured only if its VMID does not exist yet.
  `proxmox_kvm`'s update mode does not diff — it always reports changed — so
  reshaping an existing VM's CPU / RAM / IP means deleting it and re-running.
  The VMs are disposable by design.
- Disk resize and power state do diff, so the drift `--check` gate stays
  green in steady state.

## Prerequisites (not in this repo)

- Zero Trust: `192.168.1.3:22` **and** `:8006` on the CI service-token
  network policy.
- `pveum user token add root@pam ci --privsep 0` → set `PROXMOX_API_TOKEN_ID`
  to the token name (`ci`, not `root@pam!ci`) and `PROXMOX_API_TOKEN_SECRET`
  to the printed value. Full-admin token; treat like `NODE_SSH_KEY`.
- Confirm `vars/main.yml`'s `proxmox_vm_storage` / `proxmox_snippet_storage`
  / `proxmox_vm_bridge` / `proxmox_node_name` against `pvesm status`,
  `/etc/network/interfaces` and `pvesh get /nodes`; enable the "Snippets"
  content type on the snippet storage.

## Running it by hand

```sh
cd proxmox
ansible-galaxy collection install -r requirements.yml
pip install 'proxmoxer>=2.3.0' requests            # into the ansible venv
export PROXMOX_API_TOKEN_ID=ci
export PROXMOX_API_TOKEN_SECRET=...
export K3S_VM_SSH_PUBKEY="$(ssh-keygen -y -f ../deploy_key)"
ansible-playbook site.yml --syntax-check
ansible-playbook site.yml --diff --private-key ../deploy_key
ansible-playbook site.yml --check --diff --private-key ../deploy_key   # changed=0
```

Everything sensitive goes through the environment, not `-e`: an
`-e key=value` string is split on whitespace and would mangle the SSH key.
Use `--tags apt` / `--tags vm` (`--tags template` / `--tags provision`) to
scope a run.

## Notes

- `known_hosts` pins the host's ed25519 key. After a reinstall regenerates
  it, refresh with `ssh-keyscan -t ed25519 192.168.1.3` and verify the
  fingerprint from the console.
- `vars/main.yml` holds `proxmox_apt_suite` (`trixie`); bump it on a major
  PVE / Debian upgrade.
