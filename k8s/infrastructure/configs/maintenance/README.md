# maintenance

Scheduled jobs that take things down on purpose, under kured's lock
([`kured/README.md`](../../controllers/kured/README.md#the-lock)).

## router-firmware

Flashes the router's firmware early on Saturday, when feed updates have
changed the package set within the pinned release (`router/README.md`). The
CronJob runs at 01:00 Europe/London, before kured's 03:00–05:00 window:

1. Waits up to an hour until every node is Ready and schedulable, no
   blocking alert is firing, and it can take kured's lock. Otherwise it
   gives up until next week.
2. Takes the lock as `router-firmware/<uuid>`, with a 4.5h TTL.
3. Dispatches Deploy (`deploy.yaml`) with `layers: firmware` and
   `lock: <uuid>`. The run shows as "Deploy router firmware". It rebuilds
   the image, compares it with the router, and flashes only if they differ.
   It checks that the lock is held for it, but never takes or releases it.
4. Polls the run every 30s until it finishes. If it is still queued after
   90 minutes, behind another Deploy, it cancels the run.
5. Releases the lock, or keeps it (below).

| Outcome | Lock | Job |
|---|---|---|
| Run succeeded, flashed or not | released | succeeds |
| Run cancelled while queued | released | succeeds |
| Run failed before sysupgrade started | released | fails |
| Dispatch failed (e.g. revoked token) | released | fails |
| Flash failed after sysupgrade started | kept, TTL removed | fails |
| Pod died mid-run | expires after its TTL | fails |

A failed Job fires `RouterFirmwareFailed`. A kept lock pauses kured until
someone releases it. Check the router (`router/README.md`), release the lock
([`kured/README.md`](../../controllers/kured/README.md#the-lock)), then
delete the Job to clear the alert:

```sh
kubectl -n maintenance delete job <job-name>
```

### The GitHub token

`secret.sops.yaml` holds a fine-grained personal access token, with no
expiry:

- **Repository access:** `TickleThePanda/home` only.
- **Permissions:** Actions read and write (dispatch, read and cancel runs).

GitHub has no API to create one, so create it in the web UI. This link
fills in most of the form; pick the repository and check the expiry:

```
https://github.com/settings/personal-access-tokens/new?name=router-firmware-cronjob&target_name=TickleThePanda&expires_in=none&actions=write
```

Then encrypt it from the repo root. The token stays out of shell history:

```sh
read -rs 'TOKEN?GitHub token: ' && echo   # bash: read -rsp 'GitHub token: ' TOKEN
printf '%s\n' 'apiVersion: v1' 'kind: Secret' 'metadata:' \
  '  name: router-firmware-github' '  namespace: maintenance' \
  'type: Opaque' 'stringData:' "  token: $TOKEN" \
| sops encrypt --filename-override k8s/infrastructure/configs/maintenance/secret.sops.yaml \
    --input-type yaml --output-type yaml /dev/stdin \
  > k8s/infrastructure/configs/maintenance/secret.sops.yaml
unset TOKEN
```

To rotate it, revoke the old token, repeat the steps above, and push. The
next Job reads the new value.

### Run it now

```sh
kubectl -n maintenance create job rf-test --from=cronjob/router-firmware
kubectl -n maintenance logs -f job/rf-test
kubectl -n maintenance delete job rf-test
```

This flashes the router if it doesn't run the current build, cutting the
internet for a few minutes.
