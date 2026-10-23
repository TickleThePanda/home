---
paths:
  - "ansible/proxmox/**"
  - "ansible/group_vars/proxmox.yml"
---

# Proxmox layer

- `ansible/proxmox/` manages `proxmox-01` (`192.168.1.3`), Proxmox VE 9 on
  Debian 13, as `root`.
- Two access paths in one play. apt repos and the Debian 13 template build
  go over SSH (`qm`, since the API can't import a qcow2 as a disk). The three
  k3s VMs are cloned and configured through the Proxmox API (port 8006,
  `community.proxmox`, `delegate_to: localhost`) with a `root@pam` token:
  `PROXMOX_API_TOKEN_ID` is the token *name*, `PROXMOX_API_TOKEN_SECRET` its
  value. CI needs `192.168.1.3:22` and `:8006` in the Zero Trust policy.
- The template build is skipped once VMID 9000 exists. VM config runs only
  for a VMID that doesn't exist yet, since `proxmox_kvm` update mode always
  reports changed. To reshape a VM, delete it and re-run. For
  `k3s-vm-control-01` that also means restoring etcd from a snapshot (see
  `ansible/k3s/RECOVERY.md`).
- Cloud-init's resolver is Quad9 (`proxmox_vm_nameservers`), not Pi-hole,
  so a fresh clone doesn't depend on a cluster workload.
- apt sources are deb822 `.sources` via `deb822_repository`. Enterprise
  repos are disabled in place (`Enabled: no`), not deleted, because
  `pve-manager` recreates them on upgrade. Bump `proxmox_apt_suite`
  (`vars/main.yml`) on a major upgrade.
- Not managed: the subscription key/nag, LXC containers, non-k3s VMs, VM/CT
  storage, `/etc/pve` other than `authorized_keys`, and VM deletion (never
  automated).
- The I219-V NIC has hung before (e1000e TX hang). The offload/EEE-off,
  `nic-watchdog` and `iTCO_wdt` workarounds here are deliberate.
