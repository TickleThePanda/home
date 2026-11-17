# cloudflare-zone

Security rules for `home.ticklethepanda.co.uk` in the `ticklethepanda.co.uk`
zone (Free plan). Every rule is scoped to that host, so `ha.` and the
DNS-only records are unaffected. DNS records and zone settings aren't managed
here. State lives in `ttp-home-tfstate`, prefix `cloudflare-zone`.

- `waf.tf`: custom rules. One rule per action. Each check is a named
  entry in `block_checks`, ORed together.
- `ratelimit.tf`: the single rate-limit rule.

## Free-plan limits

- 5 custom rules and 1 rate-limit rule (10s period, 10s block).
- The Free Managed Ruleset is always active for the whole zone. There's no
  managed-rules entrypoint, so don't add one.
- No log-only action. Test a new check by adding it on its own and watching
  Security → Events for the host.
- Keep Bot Fight Mode off. It applies to the whole zone, including `ha.`, and
  can't be skipped.

## Applying

CI applies this in the `terraform` job (`.github/workflows/deploy-terraform.yaml`),
with the `CLOUDFLARE_ZONE_API_TOKEN` secret from the `prod` environment: an
account-owned token with zone read and write access for this zone.

For a local `plan`, put that token in `CLOUDFLARE_API_TOKEN`:

```
export CLOUDFLARE_API_TOKEN=...
terraform init
terraform plan
```
