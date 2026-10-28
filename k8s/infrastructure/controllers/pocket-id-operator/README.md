# pocket-id-operator

Manages Pocket ID's OIDC clients from `PocketIDOIDCClient` resources in
`k8s/apps/internal/auth/pocketid-clients/`. Pocket ID itself stays the
hand-written StatefulSet in `k8s/apps/internal/auth/pocketid/`, adopted as an
external `PocketIDInstance`.

## The fork

The chart comes from the fork at `TickleThePanda/pocket-id-operator`, which
adds `operator.manageWorkloads`. Set to `false`, the operator never deploys
Pocket ID and the chart drops its RBAC on Deployments, StatefulSets,
Services, PVCs and HTTPRoutes. Upstream can create and delete those anywhere.

- Charts publish from the fork's `push-chart` workflow to
  `oci://ghcr.io/ticklethepanda/charts`, signed with Cosign. Flux pulls the
  chart through `oci-repository.yaml` and rejects it unless that workflow
  signed it. The chart pins the image by digest, so the tag there is the only
  pin.
- To publish a new build, run the fork's CI on the branch, set the image
  digest and a new `-ticklethepanda.N` version in the chart, then run
  `push-chart` on that branch.
- Once upstream releases `manageWorkloads`, point the OCIRepository at
  `oci://ghcr.io/aclerici38/charts/pocket-id-operator`, update the `verify`
  identity and drop the fork.

## Access

- **Kubernetes:** a ClusterRole that can read and write every Secret, `get`
  ConfigMaps, and manage the operator's CRDs, plus the leader-election Role in
  `pocket-id-operator`. No rules on any workload kind. Like Reflector, it can
  read every Secret, `sops-age` included.
- **Network:** `network-policy.yaml` denies all ingress, and all egress except
  the API server and the Pocket ID pod. It has no DNS: it calls Pocket ID at
  its pinned Service IP (`auth/pocketid/service.yaml`). So it can read every
  Secret but has nowhere to send them. Metrics are off for the same reason.
- **Pocket ID:** full admin, through Pocket ID's `STATIC_API_KEY`
  (`auth/pocketid/static-api-key.sops.yaml`). Reflector mirrors it into
  `pocket-id-clients`. The key crosses the LAN unencrypted (flannel VXLAN), as
  TinyAuth's token calls already do.

## Credentials

Headlamp, TinyAuth and GeoPulse read `client_secret` from
`<client>-oidc-credentials`, which the operator writes here and Reflector
mirrors into each app's namespace. Audiobookshelf keeps its secret in its own
database and Home Assistant is a public client, so the operator manages no
secret for either.

To replace a secret, annotate the client with
`pocketid.internal/regenerate-client-secret: "true"`. The old secret stays
valid for `clientSecretOverlap` (24h), and Reloader restarts the app when the
mirror changes. No rotation is scheduled.

## Deleting clients

Deleting a `PocketIDOIDCClient` deletes its client from Pocket ID. Flux never
prunes one (`prune: disabled` on the resources and on the `pocket-id-clients`
namespace), so removing a file from git leaves the client in place. To delete
one, remove its file and then `kubectl delete` the resource.

Uninstalling the chart doesn't delete clients: Helm keeps the CRDs, and with
the operator gone nothing removes the finalizers.

## Recovery

- **The operator or its namespace is lost:** Flux reinstalls it. It
  re-adopts every client by its `clientID` and changes nothing.
- **Pocket ID's database is lost:** restore it from backup together with
  `ENCRYPTION_KEY` (`auth/pocketid/secret.sops.yaml`). That keeps every client
  secret, including the ones apps outside the cluster hold. Without a backup,
  the operator recreates each client with its declared ID, but clients whose
  secret lives outside a Kubernetes Secret (Audiobookshelf) need a new secret
  pasted into the app.
- **The API key:** it lives in git under SOPS. To rotate it, edit
  `auth/pocketid/static-api-key.sops.yaml` and push; Pocket ID restarts with
  the new key and Reflector updates the mirror.
- **Kubernetes access never depends on Pocket ID.** The admin kubeconfig uses
  a client certificate; only Headlamp logins use Pocket ID tokens.
