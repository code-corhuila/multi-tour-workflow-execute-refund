# multi-tour-workflow-execute-refund

> Workflow saga execute-refund (commercial-reservation), with compensations

Part of the **Multi Tour** distributed system — team `multi-tour`, Grupo 2.
Governance and documentation live in [`multi-tour-docs`](https://github.com/code-corhuila/multi-tour-docs).

## What it is

The saga of HU-WFL-003 (ADR-004 D3): it marks the refund as executing in the reservation, records the outflow in the cash, and marks the refund executed with the cash movement; if a step fails it undoes the ones before it, and a recorded outflow is reversed with a compensating movement. Its state is saved after every step. Spec (APPROVED, signed 2026-11-14): `07-api/specs/commercial-reservation/multi-tour-workflow-execute-refund/` in `multi-tour-docs`.

It is implemented: the steps, the participant clients (cash-reporting and commercial-reservation) and the entry route.

- Route: `POST /api/v1/sagas/execute-refund` starts a saga (Bearer token of a Tenant Administrator or a Collaborator with the right, header `Idempotency-Key` of 8 to 128 characters, JSON body; `201` with `Location`, or `200` for a repeated key); `GET /api/v1/sagas/execute-refund/{id}` reads its state; `GET /health`.
- Idempotency key per tenant: the key is unique per tenant (`uq_execute_refund_saga_tenant_idempotency_key`), so a repeated key returns the saga already started and never reveals another tenant's.
- Retries: each participant call is retried with full jitter after a network failure, 429 or 5xx, up to `STEP_ATTEMPTS`; every write carries the step key as `Idempotency-Key`. Any other 4xx, 408 and 425 included, is a definitive rejection.
- Option B: when step 3 (complete the execution) ends with an unknown outcome, the saga reads the refund once: EXECUTED with the movement completes, EXECUTING or AUTHORIZED compensates, a failed read or another state ends FAILED for a person. An unknown outflow also ends FAILED, because the money may have moved.
- Resume: a RUNNING saga not moved for longer than the saga bound is claimed and finished from its saved state; the sweep repeats every bound.

## How to start it

Run the migrator once per environment (see `deploy/compose.yml`), then `docker compose -f deploy/compose.yml up workflow-execute-refund`, with the variables of `.env.example` and the `SERVICE_TOKEN` issued by `multi-tour-security`. The service listens on 8080 inside the platform network and publishes no port.

## Where the data is

In PostgreSQL, schema `workflow_execute_refund` of the single instance (ADR-005 E4); its migrations live in `db/` of this repository.

## How to test it

`gofmt -l .` must print nothing, then `go vet ./...` and `go test ./...` (Go 1.26). The same three run in `ci.yml` on pull requests to `develop` and `qa`, with a PostgreSQL service for `TEST_DATABASE_URL` and the `db-ci` script for the migrations.

The tests that touch the state store need a PostgreSQL: set `TEST_DATABASE_URL` to an instance whose user may create a database (they create a throwaway one and apply `db/01_ddl`); without it they are skipped locally and fail in the CI. `sh db/tests/db-ci.sh` builds the schema from an empty PostgreSQL, reruns it, rolls it back and rebuilds it.

To try it by hand: start the saga as in "How to start it", then `curl -X POST http://<host>:8080/api/v1/sagas/execute-refund -H "Authorization: Bearer <token>" -H "Idempotency-Key: <8 to 128 characters>" -H "Content-Type: application/json" -d @refund.json` from inside the platform network (the service publishes no port) and read the result with the `GET` of the returned id. The body fields are in `openapi/`.

## Branching

Three permanent branches. **None of them accepts a direct commit** — you enter through a child
branch and leave through a Pull Request.

```
develop  <--PR--  feat/... fix/... chore/...
qa       <--PR--  qa/...
main     <--PR--  release/...  hotfix/...
```

Promotion happens **by re-application** (`git cherry-pick -x`), never by merging one permanent
branch into another: `merge develop -> qa` and `merge qa -> main` do not exist in this model.

`main` requires **1 approval from `ariel5253`**. On `develop` and `qa` the team sets its own review
rule.

Full policy: `00-governance/branching-policy.md` in `multi-tour-docs`.
