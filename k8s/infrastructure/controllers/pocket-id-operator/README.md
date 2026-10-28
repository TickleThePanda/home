# pocket-id-operator

Manages Pocket ID's OIDC clients from `PocketIDOIDCClient` resources in
`k8s/apps/internal/auth/pocketid-clients/`. Pocket ID itself stays the
hand-written StatefulSet in `k8s/apps/internal/auth/pocketid/`, adopted as an
external `PocketIDInstance`.

## The fork

Upstream (`aclerici38/pocket-id-operator`) grants a ClusterRole that can read
every Secret in the cluster. The fork at `TickleThePanda/pocket-id-operator`
adds `operator.watchNamespaces` and `operator.manageWorkloads`, so this
release gets a Role in `pocket-id-clients` only, with no rules on any
workload kind. It can't read Secrets in any other namespace.

- Charts publish from the fork's `push-chart` workflow to
  `oci://ghcr.io/ticklethepanda/charts`, signed with Cosign. Flux pulls the
  chart through `oci-repository.yaml` and rejects it unless that workflow
  signed it. The chart pins the image by digest, so the tag there is the only
  pin.
- To publish a new build, run the fork's CI on the branch, set the image
  digest and a new `-ticklethepanda.N` version in the chart, then run
  `push-chart` on that branch.
- Once upstream releases namespace-scoped RBAC, point the OCIRepository at
  `oci://ghcr.io/aclerici38/charts/pocket-id-operator`, update the `verify`
  identity and drop the fork.

## Access

- **Kubernetes:** a Role in `pocket-id-clients` (Secrets, `get` on
  ConfigMaps, the operator's CRDs) and the leader-election Role in
  `pocket-id-operator`. Nothing cluster-wide except the CRDs themselves.
- **Network:** `network-policy.yaml` allows only DNS, the API server and
  Traefik, which fronts Pocket ID. It calls Pocket ID at
  `https://auth.internal.ticklethepanda.co.uk`.
- **Pocket ID:** full admin, through Pocket ID's `STATIC_API_KEY`
  (`auth/pocketid/static-api-key.sops.yaml`). Reflector mirrors it into
  `pocket-id-clients`.

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
