# Release and operations handbook

This handbook describes how to release and operate the Lottery platform without letting a service start against an unknown database schema or turning a process restart into a financial retry. It records the behavior implemented in this repository and the decisions an owner must make before a customer production deployment. It is not evidence that production hosting, security, capacity, financial controls, or disaster recovery have been accepted.

## What runs

Build one `platform` executable from `backend/cmd/platform`. Run it as two separate long-lived processes against the same primary database:

- `platform serve` handles the API. `HTTP_ADDR` defaults to `127.0.0.1:8080`.
- `platform worker` runs period transitions and the durable draw, cancellation/refund, settlement, correction, notification, reconciliation, archive, and commission loops. It does not start with the API.

Both commands check the complete embedded migration set and SHA-256 checksums before doing work. The check uses a read-only transaction and refuses a missing migration, unknown migration, or changed checksum. Startup never migrates or repairs migration metadata. Run `platform check` for the same explicit validation, and run `platform migrate` only as a separately approved release step. Migrations execute in one database transaction under an advisory lock; applied SQL files and their checksums are immutable.

The repository's `backend/Dockerfile` builds a static executable and runs it as an unprivileged container user, with `platform serve` as its default command. Docker is not required for the systemd examples. The systemd units in [`deploy`](../deploy/README.md) use a release-installed executable and send stdout and stderr to the system journal.

## Release gates and sequence

Create the PostgreSQL database with UTF8 encoding before applying the baseline. Confirm `SHOW server_encoding` returns `UTF8`; when initializing a local cluster use `initdb --encoding=UTF8`, or explicitly create a UTF8 database from template0. SQL_ASCII is not a supported multilingual database: the default Chinese notification templates failed validation in a reproduced fresh-install test. Do not loosen template checks or silently convert an existing database. This is an operator prerequisite; the migration command does not automatically create or convert databases.

The release owner should have a reviewed build identity, the exact backend and frontend artifacts, a database backup and its verification evidence, and a rollback decision before touching a live environment. Customer-owned values such as cloud and region, TLS termination, DNS, trusted proxy ranges, backup service, alert receivers, RTO, RPO, frequency, and retention must be resolved by the customer and platform owner. No defaults in this repository settle those choices.

Use this sequence for a coordinated release:

1. Review the change scope, migration files, release notes, and operational impact with a human. Confirm the planned backend binary and matching frontend artifact and record the build commit. The source default build version is `development`; a source build should set `main.buildVersion` to the commit hash and write to a new release directory. Keep the version string free of secrets.
2. Run the repository's existing CI and release checks against isolated test resources. Do not point test commands at a production database. A passing build or test suite is not a production readiness or capacity claim.
3. Confirm the approved backup completed and can be read; confirm the associated key, roles, and external-object recovery arrangements below. Keep the production database and current artifacts intact.
4. Drain API traffic at the customer-owned ingress or proxy. Wait for in-flight requests to finish, then stop the worker and API processes. Confirm both are stopped before applying a schema change. The application does not configure ingress draining for you.
5. Install the immutable release binary and frontend into a new release directory. Keep the currently deployed release available for application rollback, subject to the forward-only schema rule below. Do not overwrite a running executable in place.
6. With the approved release environment and database credentials, run `platform migrate` once. If it fails, stop and investigate; do not start either service against a partial, incompatible, or unverified state.
7. Run `platform check` with the exact same binary and database configuration. It must exit successfully before either long-lived process starts.
8. Start the API and worker from that same release. Deploy the corresponding frontend and confirm it targets the intended API origin. Keep public traffic drained until checks below pass.
9. Verify API liveness and readiness, authenticated operations readiness for both processes when metrics listeners are enabled, startup logs, and read-only business state. Check queue and explicit-failure signals. Re-enable traffic only after the release owner accepts the evidence.

For a controlled source build, use an already-created, unique absolute release directory and verify the output does not exist before compiling:

```sh
set -eu
: "${LOTTERY_RELEASE_VERSION:?set to the reviewed commit hash}"
: "${LOTTERY_RELEASE_DIR:?set to a new release directory}"
test -d "$LOTTERY_RELEASE_DIR"
test ! -e "$LOTTERY_RELEASE_DIR/platform"
cd backend
go build -trimpath -ldflags "-s -w -X main.buildVersion=${LOTTERY_RELEASE_VERSION}" \
  -o "$LOTTERY_RELEASE_DIR/platform" ./cmd/platform
```

If any check fails, keep traffic drained and stop both processes. Restore the prior application binary only if it is compatible with the already-migrated schema. Database migrations are forward-only: never roll back migration history, edit an applied migration, reset the database, or restore a pre-release dump over the live database as an application rollback. Prepare a reviewed forward fix or recover into a separate new database and follow the recovery procedure below.

This project has not been released. It has one complete baseline migration for a fresh database, not an incremental upgrade chain. Old development databases are not upgraded or reset automatically. Offline commission history checkpoints and restoration commands have been removed; normal business-ledger history and result corrections remain.

## First deployment and platform-domain bootstrap

Do not run `seed` in production. The seed path is for local development and refuses the production environment. It also creates development brands and is not a production bootstrap plan. Do not use `create-admin --super` as an automatic approval or to bypass explicit brand-scoped authorization.

An empty migrated database does not trust a platform login host by default. README setup documents that a server owner must first add the real management hostname to `platform_domains`, with an explicit enabled value, before creating the operator account. After the owner confirms the exact lower-case hostname and that no existing row needs separate review, use the approved database client's parameter binding for the insert, for example `INSERT INTO platform_domains(domain, enabled) VALUES ($1, TRUE);`, binding the verified hostname as `$1`. Do not interpolate request text into SQL, enable a wildcard, or auto-enable an arbitrary domain. This bootstrap table has only `domain` and `enabled`; it does not configure DNS, TLS, a reverse proxy, or brand domains. The customer must first choose and establish those controls.

After the platform domain is confirmed, use the documented explicit account creation flow with a brand-specific operator where appropriate. Keep bootstrap passwords out of command arguments, shell history, environment dumps, repository files, and logs; inject them through the approved secret mechanism and remove the temporary value after use. Authentication keys are separate, persistent secrets: preserve the same 32-byte key across instances and recoveries. Replacing or losing it can make previously stored idempotent request evidence unreadable. The auth-key generator writes standard Base64 and is not the metrics-token generator.

## Back up before a release

`pg_dump` makes a consistent logical snapshot of one database. A custom-format dump does not include cluster-wide roles, the platform authentication key, or external object-store contents. PostgreSQL's backup documentation distinguishes logical dumps from physical backups and continuous WAL archiving; choose and operate a complete backup design with the database and cloud owners ([PostgreSQL 17 backup methods](https://www.postgresql.org/docs/17/backup.html), [`pg_dump`](https://www.postgresql.org/docs/17/app-pgdump.html), [continuous archiving and point-in-time recovery](https://www.postgresql.org/docs/17/continuous-archiving.html)).

The application repository does not define a production backup frequency, retention period, encryption and off-site location, restore service, or alert receiver. Agree those values with the customer. The backup set must account for:

- The database and a tested restoration path. For point-in-time recovery, the database owner must separately establish base backups, a complete archived WAL chain, and restore procedures.
- Database roles and grants needed by the application and recovery operators. A database dump alone is not a cluster-role backup.
- The unchanged `AUTH_KEY_FILE` material and its owner/mode, recovered from the approved secret store.
- Object-store data and configuration used by report archives or other external objects, plus any other customer-owned external dependency.
- Integrity metadata, timestamps, source database identity, and access-controlled restore instructions, stored with the backup but not in the application repository.

The following is a manual logical snapshot example for PostgreSQL-compatible tooling on the deployment host. Provision a private backup directory and service/password files first. `PGSERVICEFILE` defines a named connection with host, database, user, and TLS settings; `PGPASSFILE` supplies the password. Keep both files private and do not print their contents. The command deliberately uses a unique file and does not clean up previous backups.

```sh
set -eu
umask 077
: "${LOTTERY_BACKUP_DIR:?set to the approved private backup directory}"
: "${PGSERVICEFILE:?set to the protected libpq service file}"
: "${PGPASSFILE:?set to the protected libpq password file}"
export PGSERVICEFILE PGPASSFILE
backup_file="$(mktemp "${LOTTERY_BACKUP_DIR%/}/lottery-pre-release-XXXXXX.dump")"
pg_dump --no-password --format=custom --dbname="service=lottery-production" --file="$backup_file"
pg_restore --list "$backup_file" >/dev/null
sha256sum "$backup_file"
```

Here `lottery-production` is a service name in the protected service file, not a password or a network address. Protect the resulting path and checksum as backup records. A successful `pg_restore --list` checks that the archive can be read; only an isolated restore proves recoverability. Do not place secret values directly in command lines or print raw secret files to verify them.

## Restore into an isolated database

Treat restoration as a separate recovery exercise. Obtain incident authority, identify the approved source backup and recovery point, verify its checksum, and prepare an isolated PostgreSQL cluster or customer-approved recovery environment. Create a **new, uniquely named database** that does not exist, using an explicitly reviewed restore owner and the approved maintenance service. Configure a separate `PGSERVICEFILE` entry for that new database. Never target the production database, never use `pg_restore --clean`, and never overwrite the original database or backup.

```sh
set -eu
: "${LOTTERY_BACKUP_FILE:?set to the verified backup file}"
: "${LOTTERY_RESTORE_DATABASE:?set to a new, unique database name}"
: "${LOTTERY_RESTORE_OWNER:?set to the reviewed restore owner role}"
: "${LOTTERY_MAINTENANCE_SERVICE:?set to the approved maintenance service name}"
: "${LOTTERY_RESTORE_SERVICE:?set to the service name configured for the new database}"
: "${PGSERVICEFILE:?set to the protected libpq service file}"
: "${PGPASSFILE:?set to the protected libpq password file}"
export PGSERVICEFILE PGPASSFILE
pg_restore --list "$LOTTERY_BACKUP_FILE" >/dev/null
createdb --maintenance-db="service=${LOTTERY_MAINTENANCE_SERVICE}" \
  --owner="$LOTTERY_RESTORE_OWNER" --template=template0 "$LOTTERY_RESTORE_DATABASE"
pg_restore --no-password --single-transaction --exit-on-error \
  --dbname="service=${LOTTERY_RESTORE_SERVICE}" "$LOTTERY_BACKUP_FILE"
```

The `createdb` command must fail if the selected name already exists; resolve that collision by choosing a new name, never by dropping or clearing the existing database. Before running the restore, independently confirm that `LOTTERY_RESTORE_SERVICE` resolves to exactly the new database and isolated cluster. Then run `platform check` against that database with the candidate binary, and verify migration state, representative business records, ledger and audit immutability, durable task state, authentication-key availability, and external objects. Do not start API or worker services until an incident owner approves the recovered instance and its network isolation. Rehearse customer-specific roles, object storage, WAL/PITR, retention, and cutover separately; the local synthetic recovery evidence in [the replication and recovery runbook](10-replication-and-recovery.md) is not production disaster-recovery acceptance. PostgreSQL documents the archive restore options and their effects in [`pg_restore`](https://www.postgresql.org/docs/17/app-pgrestore.html).

## Process management with systemd

The example units are `Type=simple`, use the fixed `lottery` account, run from `/opt/lottery/current`, read common settings from `/etc/lottery/platform.env`, and start only the `serve` or `worker` command. They stop on `SIGTERM`, allow 45 seconds for shutdown, and restart on process failure. Filesystem writes are disabled for the process; stdout and stderr go to the journal. The application key and environment file must be readable only by their intended owners. Have the service owner install and review the units and directories before enabling them.

`platform.env` is an `EnvironmentFile`, not a shell script. It should contain only approved key/value settings such as `APP_ENV=production`, `DATABASE_URL`, `AUTH_KEY_FILE`, `DB_MAX_CONNS`, and optional read-node and proxy settings. Do not include shell expansions, `source` it, or log its contents. Keep the file root-owned and mode `0600`; make the referenced auth key readable by `lottery` and no other untrusted account. `DATABASE_URL` may contain a password, so protect the environment file as a secret. Select `HTTP_ADDR` and `TRUSTED_PROXY_CIDRS` only after the customer chooses the ingress topology. The sample binds the API to loopback for a same-host proxy and makes no claim about an external TLS or DNS design.

Metrics listeners are off unless their service-specific address is nonempty. The units explicitly leave `API_METRICS_ADDR` and `WORKER_METRICS_ADDR` empty. If metrics are approved, set only the respective service variable to a literal loopback IP and port. Non-loopback binds, including private LAN addresses, are rejected because these listeners do not provide TLS. Remote scraping requires a separately secured proxy or local forwarding. Configure `METRICS_TOKEN_FILE` to a protected token file readable by `lottery`, mode `0400` or `0600`, with no group or other permissions. When either listener is enabled, this file is required. Generate a token with the standalone command `platform generate-metrics-token /etc/lottery/metrics.token`; it creates a 256-bit URL-safe token using exclusive file creation and mode `0600`, and needs no database configuration. This token is independent of `AUTH_KEY_FILE`. Never put the token in a unit, shell argument, or log.

The exact metrics listener depends on which unit starts the process: the API reads `API_METRICS_ADDR`; the worker reads `WORKER_METRICS_ADDR`. Both expose `/metrics`, `/health/live`, and `/health/ready` only on that listener. Every request to all three operations routes requires an explicit `Authorization: Bearer …` header. Missing or incorrect credentials receive 401. Readiness also checks database availability and migration checksums; worker readiness requires fresh period-tick and notification loops. The ordinary API listener separately has public `/health/live` and `/health/ready` routes for ingress checks; do not confuse those with the token-protected operations listener.

The runtime metrics include bounded HTTP method/route/status counters and latency, worker component runs/committed item counts/duration/freshness, database and snapshot status, pending/oldest/failed work, reconciliation discrepancy count, replication observability/byte lag, and connection-pool counts. Operational snapshots are read-only, refreshed every 30 seconds with a two-second database timeout, and stop being emitted as fresh after 75 seconds. Treat a missing or stale sample as unknown, not zero. No user, wallet, account, credential, or database identifier is used as a label. The API and worker listeners are separate; collect and alert on both if both are enabled. The code does not define alert thresholds or receivers.

Tracing is off unless `TRACE_OTLP_ENDPOINT` is set. HTTPS is accepted; HTTP is accepted only for a literal loopback address. The endpoint cannot include URL credentials, a query, or a fragment. `TRACE_SAMPLE_RATIO` accepts a finite value from `0` through `1` and defaults to `0.1`. Export uses bounded timeouts and does not read generic OTEL exporter environment variables. Span attributes are restricted to service identity and bounded route, status, or worker-component fields; do not add credentials, database IDs, user IDs, wallet values, or customer data. Validated W3C parent trace and span IDs retain upstream continuity; vendor tracestate strings are discarded and baggage is never extracted. The OpenTelemetry guidance explains [context propagation](https://opentelemetry.io/docs/concepts/context-propagation/) and its security considerations.

## Health, failures, and financial work

Use the API's public liveness route only to determine whether its HTTP process responds. Public readiness checks the database and exact migration state. For metrics-enabled processes, use the authenticated operations readiness route to check listener-specific readiness; a worker is not ready until its critical period and notification loops have completed recently. A listener bind failure or invalid token configuration prevents the process from starting. A metrics listener failure cancels the process so the service manager can restart the process, not replay an operator-approved financial action.

Worker errors and panic remain errors; observation does not convert them to success. Durable work may be in a failed or blocked state and requires the relevant business/operator procedure. A process restart is not a retry decision. Do not use a generic restart, replay, seed, or request with a new idempotency key to retry a failed financial task. Inspect the original task and its committed evidence, verify the named recovery action and authorization, and use the task-specific explicit recovery route only when its contract permits it. Unknown client results must be resolved by querying or replaying the original request with its original idempotency key. Never retry by creating a second financial request.

The application does not make financial authorization automatic. Production startup must not auto-enable payouts, commissions, report archives, or other financial gates; do not treat `--super` as an approval role. Keep production seeds disabled, review each brand's explicit financial settings, and preserve any unresolved policy decision. Operational metrics can reveal queues and discrepancies; they do not approve retries, release blocked work, or establish that a financial process is complete.

## Customer decisions before production operation

Resolve and record these customer-owned settings before accepting production traffic:

- Cloud provider, account, region, network boundary, database ownership, and approved service endpoint.
- DNS names, TLS certificate issuance/renewal, TLS termination point, ingress/proxy topology, exact trusted proxy CIDRs, and the API bind address.
- Database encryption, credentials, role/grant model, connection budget, TLS verification, backup location, encryption, frequency, retention, restore authority, WAL/PITR design, and alert receiver.
- Recovery point and time objectives, business approval for the accepted data-loss window, incident authority, and tested restore/cutover process.
- Metrics scrape identity, private listener IP and ports for API and worker, token rotation and distribution, trace collector ownership and endpoint, and alert policies/receivers.
- Customer-approved production domains, brand and legal configuration, identity/compliance adapters, financial modes, operator roles, and go-live acceptance evidence.

Until owners resolve these items and perform customer-environment exercises, describe the system as an application release with documented operator procedures, not as a production-ready deployment or completed disaster-recovery service.
