# Cluster setup

Controllers, operators and their CRDs. Their CRs live one layer downstream,
in `deploy/setup-config/` (see its `kustomization.yaml`).

## Helm releases

Traefik and Multus are Flux `HelmRelease`s in `kube-system`. k3s's own
helm-controller is disabled (`ansible/group_vars/k3s_cluster.yml`).

- **Upgrade** by changing `chart.spec.version`. Versions are exact pins.
  On a `traefik` bump, check `traefik-crds` still matches (its file says how).
- **A failed upgrade** rolls back to the last good Helm revision after three
  retries. Check `flux get hr -n kube-system`, then retry with
  `flux reconcile hr -n kube-system <name> --force`.
- **Deleting a HelmRelease uninstalls its release**, including when Flux
  prunes it because it was removed from the repo. To stop managing one
  without uninstalling it, run `flux suspend hr -n kube-system <name>` first.
  A suspended HelmRelease is deleted without touching the release.
- **Renaming** a HelmRelease is a delete plus a create. Keep `releaseName`
  so the new object adopts the existing release, and suspend the old one first.
- **Never turn off a CRD group** in `traefik/helm-release-crds.yaml`. Helm
  deletes those CRDs, and every custom resource of those kinds.
- Drift detection is on. Flux reverts manual edits to anything a chart
  renders.
