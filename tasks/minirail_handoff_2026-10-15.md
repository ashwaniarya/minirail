# Handoff: minirail (2026-10-15)

## Goal

Simple Railway clone: a thin Go control plane on DOKS, provisioned by Terraform, deployed on DigitalOcean. Personal hosting first, then sold as a setup service (option C).

## Decisions

- Spec: `docs/superpowers/specs/2026-10-15-railway-clone-design.md` (branch `spec`, ecaa25a). Plan: `docs/superpowers/plans/2026-10-15-minirail.md`.
- State lives in k8s objects: one ConfigMap per app, one Secret for its env.
- `apps.<domain>` delegated to DO DNS; wildcard Let's Encrypt cert on the DO load balancer.
- Railpack auto-detect; a Dockerfile wins.
- DO Managed Postgres by default; Valkey opt-in.
- GitHub push webhook with HMAC; private repos via one token.
- App types: web, worker, cron. App names are DNS-1035 labels (start with a letter).
- Terraform in two stages: `infra/cloud`, then `infra/platform`.
- Control image runs by digest, never by tag.
- Stacked branches, one concern each. Public repo, MIT license.

## State

| # | Task | Branch | Commit | Status |
|---|---|---|---|---|
| 0 | Public GitHub repo, `main` and PR #1 | spec | 63ce7fb | 🟢 done |
| 1 | Scaffold module, config loader and CI | feat/01-scaffold | 3738c58 | 🟢 done |
| 2 | App model, validation and ConfigMap codec | feat/02-app-model | 8b95c3d | 🟢 done |
| 3 | Workload manifests per app type | feat/03-manifests | 4d3a721 | 🟢 done |
| 4 | Build Job and git ls-remote | feat/04-build-job | 8987e60 | 🟢 done |
| 5 | ConfigMap-backed app store and env secrets | feat/05-store | b846287 | 🟢 done |
| 6 | Apply, restart, run-now and delete | feat/06-apply | c65d973 | 🟢 done |
| 7 | Trigger builds and reconcile deploys | feat/07-trigger | 6ad9ce4 | 🟢 done |
| 8 | Postgres role and database provisioner | feat/08-pg | b8b8326 | 🟢 done |
| 9 | Dashboard server core with auth | feat/09a-server | 1ee9cdc | 🟢 done |
| 10 | App list and create form | feat/09b-pages | 7a6e6af | 🟢 done |
| 11 | App detail page | feat/10a-detail | 9ffffdc | 🟢 done |
| 12 | Env, deploy, run, attach and delete actions | feat/10b-actions | 19579ae | 🟢 done |
| 13 | GitHub push webhook | feat/11a-webhook | 0dbfac3 | 🟢 done |
| 14 | Build and runtime log streaming | feat/11b-logs | eafc82a | 🟢 done |
| 15 | Control plane binary and image | feat/12-binary | 7eab20e | 🟢 done |
| 16 | Terraform cloud stage | feat/13-cloud | e321442 | 🟢 done |
| 17 | Platform providers, Traefik and DNS | feat/14a-platform-edge | ec472df | 🟢 done |
| 18 | Platform control plane | feat/14b-platform-control | 5359961 | 🟢 done |
| 19 | Operator commands, example app and smoke test | feat/15-operator | df94761 | 🟢 done |
| I1 | Selector overlap between builds and Services | feat/16-selector-kind | 080b653 | 🟢 done |
| I3 | DNS-1035 app names, restarted-at only on Restart | feat/17-dns1035-names | 2f528ae, 420e904 | 🟢 done |
| I4 | Wait for containers before streaming logs | feat/18-log-wait | bae1436 | 🟢 done |
| I2 | Control image by digest | feat/19-control-digest | 81cb032 | 🟢 done |
| 20 | Track `tasks/` in git | docs/20-track-tasks | bfe95f0 | 🟢 done |
| 21 | Open source: LICENSE (MIT) and README | docs/21-open-source | f90dac3 | 🟢 done |
| 23 | Control plane config: scheme and plain-http registry | feat/23-local-config | 3d095b2 | 🟢 done |
| 24 | Multi-arch railpack build | feat/24-multiarch-railpack | 00f4598 | 🟢 done |
| 25 | Platform stage as a reusable module | refactor/25-platform-module | 791de8c | 🟢 done |
| 26 | Module flags for local clusters | feat/26-module-local-flags | 23761af | 🟢 done |
| 27 | Local mode on k3d (`make local-up`) | feat/27-local-cluster | deed1ca, 4dd7f85 | 🟢 done, `make local-smoke` passed 2026-10-15 |

## Blockers / open questions

- `spec` merged into `main` through PR #1 (63ce7fb). Repo is public at github.com/ashwaniarya/minirail; default branch `main`.
- Every merge and `make up` (DO spend) need Arya's explicit OK. Arya exports tokens in their own shell.

## Next steps

Gated:

- [x] Task 0: create the GitHub repo and `main`
- [x] Push `spec` and open PR #1 (one PR for all 26 commits, merged as 63ce7fb)
- [ ] First `make up` on DigitalOcean

First `make up` risks (each is a small fix if it bites):

- [ ] Traefik `ports.websecure.targetPort: web` accepted by chart 41.6.1
- [ ] DO LB serves TLS with the cert annotation, http protocol and tls-ports 443
- [ ] Platform apply reads the Traefik LB IP only after it exists
- [ ] Rootless BuildKit runs on DOKS (Unconfined seccomp and AppArmor)
- [ ] DOCR accepts the registry cache with `mode=max`
- [ ] railpack-frontend image tag matches CLI v0.40.1
- [ ] railpack prepare downloads mise as UID 1000 with `HOME=/tmp`
- [ ] Postgres attach works on PG 17 (`GRANT role TO CURRENT_USER`, then `CREATE DATABASE ... OWNER`)
- [ ] `make down` deletes the LE cert after the LB is gone (may need a second run)
- [ ] `make image` writes a real digest into `infra/platform/image.auto.tfvars.json`

Deferred minors:

- [ ] M1 RBAC verbs `*` and no Pod Security Admission labels on `apps`
- [ ] M2 `store.Update` has no resourceVersion check
- [ ] M3 concurrent Trigger for one SHA returns 502, not 202
- [ ] M4 Trigger deletes a succeeded but unreconciled Job, so the SHA rebuilds
- [ ] M5 newest build picked by CreationTimestamp, not push order
- [ ] M6 log SSE has no heartbeat (send `: ping` every 30 s)
- [ ] M7 create form lacks the postgres and valkey checkboxes
- [ ] M8 no DOCR garbage collection (5 GiB starter quota)
- [ ] RF2 webhook test does not assert a non-JSON body builds nothing
- [ ] RF5 delete test does not assert Service, Ingress and env Secret removal

## Verify

`make check` exits 0: go vet, go test (71/71), terraform fmt, then init, validate and test for both stages (platform 2/2). Last green on f90dac3, and CI passed on PR #1.








## Git snapshot (auto, auto 17:34)

```
branch: feat/27-local-cluster
 M tasks/minirail_handoff_2026-10-15.md
?? infra/cloud/tasks/
 1 file changed, 15 insertions(+), 6 deletions(-)
deed1ca feat: run the whole platform locally on k3d
23761af feat: let the platform module serve http and push to a plain-http registry
791de8c refactor: move the platform stage into a reusable module
```
Full detail: /Users/arya/.claude/context-archive/minirail/2026-10-15_173440_auto.md
<!-- /snapshot -->
