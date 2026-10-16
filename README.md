# minirail

A small Railway-style platform for DigitalOcean. Push to GitHub, and minirail builds the repo and runs it on your own Kubernetes cluster at `<app>.apps.<domain>`.

> Status: `make check` passes (Go tests plus Terraform validate and test). It has not yet run against a live DigitalOcean account; the known first-run risks are listed in [`tasks/`](tasks/).

## What it does

- **App types:** web (public HTTPS URL), worker (no URL) and cron (schedule, plus "run now").
- **Builds:** Railpack detects the language; a Dockerfile wins when present. Builds run as BuildKit Jobs in the cluster and push to DigitalOcean Container Registry.
- **Deploys:** a GitHub push webhook (HMAC-verified) or the Deploy button.
- **Databases:** DigitalOcean Managed Postgres, one role and database per app. Valkey is opt-in.
- **Dashboard:** create apps, set env vars, deploy, attach Postgres, stream build and runtime logs. One admin password.
- **State:** Kubernetes objects only (a ConfigMap and a Secret per app). The control plane has no database of its own.

```
GitHub push --webhook--> control plane (Go, on DOKS) --> BuildKit Job --> registry
                                   |                                        |
                                   +--> Deployment / CronJob <-- image -----+

Browser --> DO load balancer (wildcard TLS) --> Traefik --> <app>.apps.<domain>
```

## Try it locally

Runs the whole platform on your machine with [k3d](https://k3d.io) (k3s in Docker), at no cost. Needs Docker with buildx and `k3d`. Builds run inside the cluster, so give Docker at least 8 GB of memory.

```bash
make local-up
```

The dashboard is at `http://console.localhost`, apps at `http://<app>.localhost` (Chrome and curl resolve `*.localhost`; Safari may not). Postgres runs in the cluster. There are no push webhooks locally: use the Deploy button. Set `TF_VAR_github_token` before `make local-up` only if you deploy private repos. Read the admin password with:

```bash
docker run --rm --network k3d-minirail -v "$PWD":/work -w /work/infra/local hashicorp/terraform:1.16.4 output -raw admin_password
```

`SMOKE_REPO=https://github.com/<owner>/minirail make local-smoke` runs the same end-to-end check as `make smoke`. `make local-down` deletes the cluster, the registry and the local state.

## Requirements

- A DigitalOcean account and an API token with read and write scope.
- A domain you own, with `apps.<domain>` delegated to `ns1.digitalocean.com`, `ns2.digitalocean.com` and `ns3.digitalocean.com`. Check with `dig +short NS apps.<domain>`.
- A fine-grained GitHub token with read-only Contents access to the repos you deploy.
- Docker with buildx. Terraform runs in a container, so no local install is needed.
- Go 1.26 for `make check`.

## Set up

`make up` creates billable DigitalOcean resources (a Kubernetes cluster, a load balancer and a managed Postgres cluster). Check current DigitalOcean pricing first.

```bash
export DIGITALOCEAN_TOKEN=<your token>
export TF_VAR_domain=example.com
export TF_VAR_github_token=<your token>
make up
```

`make up` applies two Terraform stages: `infra/cloud` (VPC, cluster, registry, certificate, databases), then `infra/platform` (Traefik, RBAC, the control plane). The dashboard is at `https://console.apps.<domain>`. Read the admin password with:

```bash
docker run --rm -v "$PWD":/work -w /work/infra/platform hashicorp/terraform:1.16.4 output -raw admin_password
```

Check the install end to end with `SMOKE_REPO=https://github.com/<owner>/minirail make smoke`. It deploys `examples/hello` from that repo's `main` branch, attaches Postgres, then deletes the app.

## Deploy on push

In each app repo, open Settings, then Webhooks, and add:

- Payload URL: the `webhook_url` output
- Content type: `application/json`
- Secret: the `webhook_secret` output
- Events: push only

Read either output the same way as the admin password above.

## Tear down

```bash
make down
```

This deletes everything `make up` created. If it fails on the certificate, run it again once the load balancer is gone.

## Develop

```bash
make check
```

Runs `go vet`, `go test`, `terraform fmt`, then `init`, `validate` and `test` for every Terraform root and the shared module. The design is in [`docs/superpowers/specs`](docs/superpowers/specs/).

## License

[MIT](LICENSE)
