# minirail: a personal Railway clone on DigitalOcean Kubernetes

Date: 2026-10-15 · Status: design approved, spec awaiting review

## 1. Intent

Real personal hosting for Arya's own apps (Node, Go, Python services, workers, crons) on infrastructure Arya owns. Railway's core loop is the bar: connect a GitHub repo, push, get a running HTTPS URL, with env vars, logs and a Postgres database one click away.

Success criteria:

- `make up` on a fresh DigitalOcean account (plus one NS delegation) produces a working platform with HTTPS dashboard at `console.apps.<domain>`.
- Creating a `web` app from a private GitHub repo with no Dockerfile yields a live `https://<name>.apps.<domain>` within one build.
- A `git push` to the tracked branch redeploys automatically; a failed build leaves the previous version serving.
- `worker` and `cron` apps build and run the same way, without a URL.
- "Attach Postgres" gives the app its own database and `DATABASE_URL`.

Decisions taken in brainstorming (user-chosen):

| Topic | Decision |
|---|---|
| Purpose | Real personal hosting, single admin user |
| Domain / TLS | Subdomain `apps.<domain>` delegated to DO DNS; one wildcard Let's Encrypt cert on the DO load balancer |
| Builds | Railpack auto-detect; repo `Dockerfile` wins when present |
| Databases | DO Managed Postgres (on by default), DO Managed Valkey (opt-in) |
| Trigger | GitHub push webhook (HMAC) + Redeploy button; private repos via one GitHub token |
| Control plane | Thin Go service, app state stored as Kubernetes objects (no extra DB) |
| Service types | `web`, `worker`, `cron` |

## 2. Non-goals (v1)

Multi-user / teams / billing; per-tenant isolation; build-time secrets (app env vars are runtime only); rollback to an older build; custom domains per app; horizontal autoscaling; preview environments; GitLab/Bitbucket; remote Terraform state (local state files, documented); automatic GitHub webhook registration (user adds it per repo, dashboard shows URL).

## 3. Architecture

```
GitHub push --HMAC--> console.apps.<domain>/webhook/github
                               |
                     control plane (Go, ns minirail)
                               |  creates
                 build Job (ns apps): clone -> prepare -> buildctl
                               |  pushes
                registry.digitalocean.com/<reg>/<app>:<sha>
                               |  reconcile loop sets image
        Deployment (+Service+Ingress for web) | CronJob (cron)
                               |
 DO LB :443 (wildcard LE cert) -> Traefik :web -> pods -> DO Managed PG / Valkey
```

Repository layout:

```
infra/cloud/      Terraform stage 1: DO resources
infra/platform/   Terraform stage 2: in-cluster resources
cmd/minirail/     main.go
internal/apps/    app record, manifest builders, validation
internal/build/   build Job builder, git ls-remote
internal/web/     HTTP handlers, templates, webhook
internal/pg/      per-app database provisioning
build/            control-plane Dockerfile, build-step scripts
Makefile          up / down / image / smoke
scripts/smoke.sh
```

## 4. Infrastructure (Terraform, two stages)

Two root modules, because Kubernetes and Helm providers cannot be configured reliably from a cluster created in the same apply. Stage 2 reads stage 1 through `terraform_remote_state` (local backend, `../cloud/terraform.tfstate`).

**Prerequisite (manual, once):** at the current DNS provider of `<domain>`, add NS records for `apps.<domain>` → `ns1/ns2/ns3.digitalocean.com`. The Let's Encrypt cert in stage 1 fails until this resolves.

**Stage 1, `infra/cloud`:**

- VPC; DOKS cluster, version from `digitalocean_kubernetes_versions` (prefix variable); node pool `s-2vcpu-4gb`, autoscale 2–4.
- Container registry, tier variable (default `basic`, 5 repos = 5 apps); write-enabled docker credentials resource.
- `digitalocean_domain` for `apps.<domain>`; `digitalocean_certificate` type `lets_encrypt` for `*.apps.<domain>` and `apps.<domain>`.
- Managed Postgres (`db-s-1vcpu-1gb`, version variable) with firewall rule allowing only the DOKS cluster. Managed Valkey behind `enable_valkey` (default false), same firewall.
- Outputs: cluster name, registry endpoint, docker config (sensitive), cert name, PG admin URI (sensitive), Valkey URI (sensitive, nullable).

**Between stages, `make image`:** logs Docker into DOCR with the stage-1 docker config (temp `DOCKER_CONFIG`, no doctl), builds `build/Dockerfile` for `linux/amd64`, pushes `<registry>/minirail:<git-sha>`.

**Stage 2, `infra/platform`:**

- Namespaces `minirail` (control plane) and `apps` (user workloads).
- Traefik Helm release (chart version pinned at implementation), 2 replicas, IngressClass `traefik`. Service annotations: `do-loadbalancer-certificate-name` (name survives LE renewals, ID does not), `do-loadbalancer-tls-ports: "443"`, `do-loadbalancer-protocol: http`, `do-loadbalancer-redirect-http-to-https: "true"`. Port 443 targets Traefik's `web` entrypoint, since TLS ends at the LB. Forwarded headers trusted from the VPC CIDR only.
- After Helm waits for the LB IP: `digitalocean_record` `*` and `@` A records in the apps zone.
- Secrets in `apps`: `registry` (dockerconfigjson, used for push and pull), `github-token` (read by the clone step), `valkey` (`REDIS_URL`, only when enabled). Secrets in `minirail`: `github-token` (for `git ls-remote`), `webhook-secret` (random), `admin-password` (random, output sensitive), `pg-admin`.
- Control plane: ServiceAccount, Role + RoleBinding scoped to namespace `apps` only (configmaps, secrets, deployments, services, ingresses, jobs, cronjobs, pods, pods/log), Deployment (1 replica), Service, Ingress `console.apps.<domain>`.

`make up` = stage 1 apply → image → stage 2 apply. `make down` = reverse order.

## 5. Control plane

### 5.1 App record

An app is a ConfigMap `<name>-app` in namespace `apps`, label `paas/app=<name>`, keys:

| Key | Values |
|---|---|
| `repo` | `https://github.com/<owner>/<repo>` |
| `branch` | default `main` |
| `type` | `web` \| `worker` \| `cron` |
| `port` | `web` only, default `8080`, injected as `PORT` |
| `schedule` | `cron` only, 5-field cron expression |
| `replicas` | `web`/`worker`, 1–5, default 1 |
| `root` | optional subdirectory (monorepos), default `.` |
| `postgres`, `valkey` | `true` \| `false` |
| `deployed-sha` | last rolled-out commit |

Env vars: Secret `<name>-env`. Database URL: Secret `<name>-db`, kept separate so editing env never clobbers it. Workloads load both via `envFrom`, plus `valkey` when attached.

Name rule: DNS-1123 label, 1–30 chars, not `console`, `www` or `minirail`. Validated server-side before any object is created.

### 5.2 Workloads per type

| Type | Objects (all labelled `paas/app`) | URL | Rollout |
|---|---|---|---|
| `web` | Deployment + Service + Ingress `<name>.apps.<domain>`; TCP readiness probe on `port` | yes | set Deployment image |
| `worker` | Deployment | no | set Deployment image |
| `cron` | CronJob, `concurrencyPolicy: Forbid` | no | set CronJob image; next run uses it |

Workloads are created on the first successful build and updated by server-side apply afterwards. Fixed defaults: requests 100m / 128Mi, memory limit 512Mi, `imagePullSecrets: registry`.

### 5.3 Build pipeline

1. Trigger resolves the commit SHA: webhook payload `after`, or `git ls-remote <repo> <branch>` for Redeploy.
2. Control plane creates Job `<name>-build-<sha7>` in `apps`: `backoffLimit: 0`, `activeDeadlineSeconds: 1800`, `ttlSecondsAfterFinished: 86400`, labels `paas/app`, `paas/kind=build`, annotation `paas/sha`. An existing Job with that name (same SHA) makes the trigger a no-op.
3. Init `clone` (control-plane image, which ships `git` and the `railpack` binary): shallow fetch of the exact SHA into an `emptyDir`. Token supplied by a git credential helper reading `GITHUB_TOKEN` from Secret `github-token` in `apps`; the token never appears in args or the pod spec.
4. Init `prepare`: if `<root>/Dockerfile` exists, skip; else `railpack prepare <root> --plan-out /work/plan/railpack-plan.json --info-out /work/plan/railpack-info.json`. Exit code 1 fails the build with Railpack's logs.
5. Main `build`: `moby/buildkit:v0.34.0-rootless` with `buildctl-daemonless.sh`, seccomp and AppArmor `Unconfined`, UID 1000, `BUILDKITD_FLAGS=--oci-worker-no-process-sandbox`. Frontend `dockerfile.v0`, or `gateway.v0` with `source=ghcr.io/railwayapp/railpack-frontend:<pinned>` when a plan exists. Output `type=image,name=<registry>/<name>:<sha>,push=true`; cache `type=registry,ref=<registry>/<name>:buildcache,mode=max`. Registry auth from Secret `registry` mounted as `/home/user/.docker/config.json`. Limits 2 CPU / 3Gi.

Railpack CLI and frontend versions are pinned to the same release (the frontend must match the CLI that wrote the plan). Pick the newest release at least two weeks old at implementation time.

### 5.4 Reconcile loop

Every 10 s: list Jobs `paas/kind=build`; per app, take the newest succeeded Job; if its `paas/sha` differs from the app's `deployed-sha`, apply the workload with the new image, then write `deployed-sha`. State is only in Kubernetes, so a control-plane restart loses nothing. A failed Job changes nothing; the live version keeps serving.

### 5.5 HTTP surface

Dashboard (server-rendered `html/template`, HTTP basic auth, user `admin`):

| Route | Purpose |
|---|---|
| `GET /` | app list: type, URL, deployed SHA, last build status |
| `GET/POST /apps/new` | create app (form = 5.1 keys) |
| `GET /apps/{name}` | detail: config, env editor, attach toggles, builds, webhook URL |
| `POST /apps/{name}/env` | replace `<name>-env` (KEY=VALUE lines) and restart the workload |
| `POST /apps/{name}/deploy` | Redeploy current branch head |
| `POST /apps/{name}/run` | `cron` only: one-off Job from the CronJob template |
| `POST /apps/{name}/attach/{postgres,valkey}` | attach a database |
| `POST /apps/{name}/delete` | delete every object labelled `paas/app=<name>`; the database is kept |
| `GET /apps/{name}/logs?source=build\|runtime` | Server-Sent Events stream: build = Job pod containers in order; runtime = newest pod (web/worker) or last Job (cron), `tailLines=200`, follow |
| `POST /webhook/github` | no basic auth; `X-Hub-Signature-256` HMAC check against `webhook-secret` (constant-time); `push` events only; builds every app whose repo + branch match (monorepo services share one repo) |
| `GET /healthz` | liveness |

### 5.6 Postgres attach

The control plane connects with `pg-admin` (`sslmode=require`) and runs: create role `app_<name>` with a random password, create database `app_<name>` owned by it, revoke `CONNECT` from `PUBLIC`. It then writes `DATABASE_URL` into `<name>-db` and restarts the workload. Idempotent: an existing role gets its password reset and the Secret rewritten. Deleting an app never drops its database.

Valkey attach adds the shared `valkey` Secret (`REDIS_URL`) to `envFrom`. Apps share one Valkey and must namespace their keys; this is shown in the UI.

## 6. Security

- Dashboard over HTTPS only (LB redirect), basic auth with a 32-char random password from Terraform.
- Webhook: HMAC SHA-256, constant-time compare, body capped at 1 MiB.
- Control-plane RBAC is namespaced to `apps`; it cannot touch `minirail`, `kube-system` or Traefik.
- GitHub token: fine-grained, read-only `contents` on chosen repos; only the clone init container reads it.
- Postgres and Valkey are reachable only from the cluster (DO database firewall).
- User build steps run in rootless BuildKit; seccomp/AppArmor are unconfined for that container only (required by rootless BuildKit).
- Known limit: user apps share namespace `apps` with no NetworkPolicy, acceptable for a single owner.

## 7. Error handling

| Failure | Behaviour |
|---|---|
| Invalid form input | 400 with message; nothing created |
| Webhook bad signature | 401, logged |
| Webhook for unknown repo/branch | 204, no-op |
| `git ls-remote` fails (bad token, repo) | Redeploy shows the error; no Job |
| Railpack cannot plan | Job fails at `prepare`; logs show Railpack output |
| Build or push fails | Job failed; dashboard shows it; live version untouched |
| New version crash-loops | Deployment rollout stalls with old ReplicaSet still serving (`maxUnavailable: 0`); dashboard shows pod status |
| PG attach fails | Error shown; no Secret written |
| Kubernetes API errors | Logged with app name; HTTP 502 to the dashboard |

## 8. Testing

| Tier | What |
|---|---|
| 🔬 Unit | name validation; manifest builders for each type (golden comparisons); build Job builder for Dockerfile vs Railpack branches; HMAC verify; cron expression validation; SHA → Job name |
| 🎭 Behavioural | HTTP handlers and the reconcile loop against `client-go` fake clientset: create app → trigger → mark Job succeeded → reconcile → Deployment image set; failed Job leaves image unchanged; webhook matching across two apps on one repo |
| 💨 Smoke | `go test ./...`; `terraform fmt -check` + `validate` for both stages (via `hashicorp/terraform` Docker image, since terraform is not installed locally); `scripts/smoke.sh` after `make up`: create a sample web app, wait for build, `curl` its HTTPS URL for 200 |

Postgres provisioning SQL is covered by the smoke test against the real managed cluster, not mocked.

## 9. Defaults and cost

Defaults: region `blr1` (variable), 2× `s-2vcpu-4gb` nodes, LB small, PG 1 GB, registry basic, Valkey off. Estimated ~$80/month at DO list prices (from memory, to verify before first apply); +$15 with Valkey, +$15 for the professional registry tier (unlimited repos).

## 10. Risks

- Rootless BuildKit on DOKS node kernels is unverified until the first real build; the smoke test proves it.
- Builds share nodes with apps; a heavy build can starve small apps. Mitigated by build limits and autoscaling to 4 nodes.
- Local Terraform state holds secrets (DB admin URI, registry creds). It lives only on the operator's disk and is git-ignored.
- Railpack is young and ships often; pinned versions need deliberate bumps.
