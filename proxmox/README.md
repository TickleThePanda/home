# proxmox

Host-level config for `proxmox-01` (`192.168.1.3`), the home hypervisor —
Proxmox VE 9 on Debian 13. The source of truth from here on; anything not in
this playbook is unmanaged.

Applied by the `proxmox` job in `.github/workflows/deploy.yaml` on every push
touching `proxmox/**`, over SSH as `root` through the same cloudflared tunnel
the `node` and `router` jobs use. The CI key is `NODE_SSH_KEY` — its public
half is already in the host's `authorized_keys` (a symlink into the
cluster-managed `/etc/pve/priv/`).

## What it manages

| Area | File |
|---|---|
| apt repos — enterprise off, no-subscription on (PVE + Ceph) | `tasks/apt-repos.yml` |

Not managed: the subscription key / "no valid subscription" nag, VMs and
containers and their storage, the Proxmox cluster config and `/etc/pve`, and
`authorized_keys`.

## Running it by hand

```sh
cd proxmox
ansible-playbook site.yml --syntax-check
ansible-playbook site.yml --diff
# then confirm it is idempotent:
ansible-playbook site.yml --check --diff
```

## Notes

- `known_hosts` pins the host's ed25519 key. After a reinstall regenerates it,
  refresh with `ssh-keyscan -t ed25519 192.168.1.3` and verify the fingerprint
  from the console.
- `vars/main.yml` holds `proxmox_apt_suite` (`trixie`); bump it on a major
  PVE / Debian upgrade.
- Enterprise repos are disabled in place (`Enabled: no`), not deleted —
  `pve-manager` can recreate the files on upgrade, and a redeploy re-asserts
  the state.
