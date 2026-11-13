#!/usr/bin/env bash
# Build a source snapshot locally, then activate a release over SSH.
set -Eeuo pipefail
[[ $# == 1 || ( $# == 2 && $2 == --upgrade-brand-presentation ) || ( $# == 2 && $2 == --upgrade-data-retention ) || ( $# == 2 && $2 == --web-only ) || ( $# == 2 && $2 == --upgrade-agent-wallet ) || ( $# == 2 && $2 == --upgrade-themes ) || ( $# == 2 && $2 == --upgrade-unique-usdt ) || ( $# == 2 && $2 == --remove-announcement-timezone ) || ( $# == 2 && $2 == --remove-auth-consent ) || ( $# == 2 && $2 == --remove-dedicated-wallet ) || ( $# == 2 && $2 == --upgrade-tron-key-pool ) || ( $# == 2 && $2 == --upgrade-tron-deposits ) || ( $# == 2 && $2 == --upgrade-release-20261111 ) || ( $# == 2 && $2 == --upgrade-cash-wallet ) || ( $# == 2 && $2 == --upgrade-brand-contacts ) || ( $# == 2 && $2 == --upgrade-profile-contacts ) || ( $# == 2 && $2 == --upgrade-fixed-entries ) || ( $# == 2 && $2 == --upgrade-manual-results ) || ( $# == 3 && $2 == --initial ) ]] || {
  echo "Usage: $0 SSH_HOST [--upgrade-brand-presentation|--upgrade-data-retention|--web-only|--upgrade-agent-wallet|--upgrade-themes|--upgrade-unique-usdt|--remove-announcement-timezone|--remove-auth-consent|--remove-dedicated-wallet|--upgrade-tron-key-pool|--upgrade-tron-deposits|--upgrade-release-20261111|--upgrade-cash-wallet|--upgrade-brand-contacts|--upgrade-fixed-entries|--upgrade-manual-results|--upgrade-profile-contacts]   (first deployment: 02-deploy.sh SSH_HOST CONFIG.json)" >&2; exit 1;
}
host=$1
[[ $host =~ ^[a-zA-Z0-9][a-zA-Z0-9_.@-]*$ ]] || { echo 'Invalid SSH host.' >&2; exit 1; }
mode=update
consolidate=false
web_only=false
api_release=none
if [[ $# == 3 ]]; then mode=initial; elif [[ $# == 2 ]]; then consolidate=true; fi
if [[ $# == 2 && $2 == --web-only ]]; then web_only=true; consolidate=false; fi
upgrade_script=upgrade-fixed-entries.py
if [[ $# == 2 && $2 == --upgrade-themes ]]; then upgrade_script=upgrade-themes.py; fi
if [[ $# == 2 && $2 == --upgrade-unique-usdt ]]; then upgrade_script=upgrade-unique-usdt-remote.py; fi
if [[ $# == 2 && $2 == --remove-announcement-timezone ]]; then upgrade_script=remove-announcement-timezone.py; fi
if [[ $# == 2 && $2 == --remove-auth-consent ]]; then upgrade_script=remove-auth-consent.py; fi
if [[ $# == 2 && $2 == --remove-dedicated-wallet ]]; then upgrade_script=remove-dedicated-wallet.py; fi
if [[ $# == 2 && $2 == --upgrade-manual-results ]]; then upgrade_script=upgrade-manual-results.py; fi
if [[ $# == 2 && $2 == --upgrade-profile-contacts ]]; then upgrade_script=upgrade-profile-contacts.py; fi
if [[ $# == 2 && $2 == --upgrade-brand-contacts ]]; then upgrade_script=upgrade-brand-contacts.py; fi
if [[ $# == 2 && $2 == --upgrade-cash-wallet ]]; then upgrade_script=upgrade-cash-wallet-remote.py; fi
if [[ $# == 2 && $2 == --upgrade-release-20261111 ]]; then upgrade_script=upgrade-release-20261111.py; fi
if [[ $# == 2 && $2 == --upgrade-tron-deposits ]]; then upgrade_script=upgrade-tron-deposits-remote.py; fi
if [[ $# == 2 && $2 == --upgrade-tron-key-pool ]]; then upgrade_script=upgrade-tron-key-pool-remote.py; fi
if [[ $# == 2 && $2 == --upgrade-agent-wallet ]]; then upgrade_script=upgrade-agent-wallet.py; fi
if [[ $# == 2 && $2 == --upgrade-data-retention ]]; then upgrade_script=upgrade-data-retention.py; fi
if [[ $# == 2 && $2 == --upgrade-brand-presentation ]]; then upgrade_script=upgrade-brand-presentation.py; fi
repo=$(cd "$(dirname "$0")/../.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/source" "$work/release/web"
if [[ $web_only == true ]]; then
  api_release=$(ssh -o BatchMode=yes "$host" 'readlink -f /opt/lottery/current')
  [[ $api_release =~ ^/opt/lottery/releases/[a-zA-Z0-9-]+$ ]] || { echo 'Invalid API source release.' >&2; exit 1; }
  scp -o BatchMode=yes "$host:$api_release/platform" "$work/current-api"
  scp -o BatchMode=yes "$host:$api_release/ops/0001_baseline.up.sql" "$work/current-baseline.sql"
  printf '%s\n' "$api_release" > "$work/release/API_SOURCE_RELEASE"
  echo "Explicit web-only release: retaining API and baseline from $api_release"
fi
if [[ $mode == initial ]]; then
  cp "$3" "$work/release/deploy.json"
else
  ssh -o BatchMode=yes "$host" 'cat /etc/lottery/deploy.json' > "$work/release/deploy.json"
fi
# This JSON is data, never sourced as shell or interpolated into SQL.
python3 - "$work/release/deploy.json" <<'PY'
import json,re,sys
p=sys.argv[1]
c=json.load(open(p))
assert set(c)=={'user_domains','admin_domain','platform_domain','agent_domain','certificate_email','admin_username','draw_allowed_origins'}, 'Unexpected config keys'
assert isinstance(c['user_domains'],list) and c['user_domains'], 'user_domains required'
domains=c['user_domains']+[c[k] for k in ('admin_domain','platform_domain','agent_domain')]
assert len(domains)==len(set(domains)), 'Use distinct hosts for each frontend'
for d in domains:
    assert isinstance(d,str) and len(d)<=253 and '.' in d and all(re.fullmatch(r'[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?',label) for label in d.split('.')), 'Invalid lowercase domain'
root=c['user_domains'][0]
assert all(d==root or d.endswith('.'+root) and len(d.split('.'))==len(root.split('.'))+1 for d in domains), 'All entry domains must be the root or its direct subdomains'
assert re.fullmatch(r'[^\s@]+@[^\s@]+\.[^\s@]+',c['certificate_email']), 'Set certificate_email before deploying'
assert re.fullmatch(r'[a-z][a-z0-9_]{2,31}',c['admin_username']), 'Invalid admin_username'
from urllib.parse import urlsplit
assert isinstance(c['draw_allowed_origins'],list)
for origin in c['draw_allowed_origins']:
    u=urlsplit(origin)
    assert u.scheme=='https' and u.hostname and not u.username and not u.password and not u.path and not u.query and not u.fragment and not any(x.isspace() for x in origin), 'Invalid HTTPS draw origin'
json.dump(c,open(p,'w'),indent=2)
PY
ssh -o BatchMode=yes "$host" "bash -s -- $mode" <<'REMOTE'
set -Eeuo pipefail
[[ $EUID == 0 && $(uname -m) == x86_64 ]]
command -v nginx psql certbot python3 systemd-run >/dev/null
[[ -d /opt/lottery/incoming ]]
if [[ $1 == initial ]]; then
  [[ ! -e /opt/lottery/current && ! -e /etc/lottery/platform.env ]]
  [[ $(runuser -u postgres -- psql -XAtc "SELECT count(*) FROM pg_database WHERE datname='lottery'") == 0 ]]
  [[ $(runuser -u postgres -- psql -XAtc "SELECT count(*) FROM pg_roles WHERE rolname='lottery'") == 0 ]]
else
  [[ -L /opt/lottery/current && -s /etc/lottery/platform.env ]]
fi
REMOTE
root_domain=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["user_domains"][0])' "$work/release/deploy.json")
ssh -o BatchMode=yes "$host" "python3 - check $root_domain" < "$repo/deploy/test-host/namecom-dns.py"
# Include working-tree changes and untracked source, never local credentials/builds.
# Building a private snapshot avoids touching local DEV processes or dist output.
COPYFILE_DISABLE=1 tar --no-xattrs -C "$repo" --exclude=node_modules --exclude=dist --exclude=.local --exclude=__pycache__ \
  --exclude='.env*' --exclude='*.tsbuildinfo' --exclude='*_test.go' --exclude=bin \
  -cf - backend shared user-web admin-web platform-web agent-web \
  package.json pnpm-lock.yaml pnpm-workspace.yaml deploy/systemd deploy/test-host | tar -C "$work/source" -xf -
release="$(date -u +%Y%m%dT%H%M%SZ)-$(openssl rand -hex 4)"
printf '%s\n' "$release" > "$work/release/VERSION"
# Use the packageManager version declared by this snapshot.
pnpm_version=$(node -p "require('$work/source/package.json').packageManager")
[[ $pnpm_version =~ ^pnpm@[0-9]+\.[0-9]+\.[0-9]+$ ]]
cd "$work/source"
pnpm dlx "$pnpm_version" install --frozen-lockfile
user_origin=$(python3 -c 'import json,sys; print("https://"+json.load(open(sys.argv[1]))["user_domains"][0])' "$work/release/deploy.json")
VITE_USER_APP_ORIGIN="$user_origin" pnpm dlx "$pnpm_version" -r --workspace-concurrency=1 build
for app in user admin platform agent; do
  cp -R "$app-web/dist" "$work/release/web/$app"
done
if [[ $web_only == true ]]; then
  cp "$work/current-api" "$work/release/platform"
else
(cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags "-s -w -X main.buildVersion=$release" -o "$work/release/platform" ./cmd/platform)
fi
cp -R deploy/systemd "$work/release/systemd"
mkdir "$work/release/ops"
cp deploy/test-host/namecom-dns.py deploy/test-host/render-nginx.py deploy/test-host/upgrade-fixed-entries.py deploy/test-host/upgrade-manual-results.py "$work/release/ops/"
cp deploy/test-host/upgrade-profile-contacts.py deploy/test-host/upgrade-profile-contacts.sql "$work/release/ops/"
cp deploy/test-host/upgrade-brand-contacts.py deploy/test-host/upgrade-brand-contacts.sql "$work/release/ops/"
cp deploy/test-host/upgrade-cash-wallet-remote.py deploy/test-host/upgrade-cash-wallet.sql "$work/release/ops/"
cp deploy/test-host/upgrade-unique-usdt-remote.py deploy/test-host/upgrade-unique-usdt.sql "$work/release/ops/"
cp deploy/test-host/upgrade-release-20261111.py deploy/test-host/upgrade-release-20261111.sql "$work/release/ops/"
cp deploy/test-host/upgrade-tron-deposits-remote.py deploy/test-host/upgrade-tron-deposits.sql "$work/release/ops/"
cp deploy/test-host/upgrade-tron-key-pool-remote.py deploy/test-host/upgrade-tron-key-pool.sql "$work/release/ops/"
cp deploy/test-host/upgrade-brand-presentation.py deploy/test-host/upgrade-brand-presentation.sql "$work/release/ops/"
cp deploy/test-host/upgrade-data-retention.py deploy/test-host/upgrade-data-retention.sql "$work/release/ops/"
cp deploy/test-host/upgrade-agent-wallet.py deploy/test-host/upgrade-agent-wallet.sql "$work/release/ops/"
cp deploy/test-host/remove-announcement-timezone.py deploy/test-host/remove-announcement-timezone.sql "$work/release/ops/"
cp deploy/test-host/remove-auth-consent.py deploy/test-host/remove-auth-consent.sql "$work/release/ops/"
cp deploy/test-host/remove-dedicated-wallet.py "$work/release/ops/"
cp deploy/test-host/upgrade-themes.py "$work/release/ops/"
if [[ $web_only == true ]]; then
  cp "$work/current-baseline.sql" "$work/release/ops/0001_baseline.up.sql"
else
  cp backend/migrations/0001_baseline.up.sql "$work/release/ops/"
fi
(cd "$work/release" && COPYFILE_DISABLE=1 tar --no-xattrs -czf "$work/release.tgz" .)
shasum -a 256 "$work/release.tgz" | awk '{print $1}' > "$work/release.sha256"
scp -o IPQoS=none "$work/release.tgz" "$host:/opt/lottery/incoming/$release.tgz"
scp -o IPQoS=none "$work/release.sha256" "$host:/opt/lottery/incoming/$release.sha256"
ssh -o BatchMode=yes "$host" "bash -s -- $release $mode $consolidate $upgrade_script $api_release" <<'REMOTE'
set -Eeuo pipefail
umask 077
release=$1
mode=$2
consolidate=$3
upgrade_script=$4
api_release=$5
exec 9>/run/lock/lottery-deploy.lock
flock -n 9 || { echo 'Another deployment is running.' >&2; exit 1; }
candidate=/opt/lottery/releases/$release
stopped=false
complete=false
finish() {
  local exit_code=$?
  if [[ $complete != true ]]; then
    echo "Deployment failed; candidate: $candidate. No automatic rollback or database reset." >&2
    if [[ $stopped == true ]]; then systemctl stop lottery-api lottery-worker; fi
  fi
  exit "$exit_code"
}
trap finish EXIT
cd /opt/lottery/incoming
[[ $(sha256sum "$release.tgz" | cut -d ' ' -f1) == "$(cat "$release.sha256")" ]]
mkdir "$candidate"
tar -xzf "$release.tgz" -C "$candidate"
chown -R root:root "$candidate"
chmod -R u=rwX,go=rX "$candidate"
chmod 755 "$candidate/platform"
if [[ $api_release != none ]]; then
  [[ $(readlink -f /opt/lottery/current) == "$api_release" ]] || { echo 'API release changed during the web build; refusing activation.' >&2; exit 1; }
  cmp "$candidate/platform" "$api_release/platform"
  cmp "$candidate/ops/0001_baseline.up.sql" "$api_release/ops/0001_baseline.up.sql"
fi
if [[ $mode == update ]]; then
  python3 - "$candidate/deploy.json" /etc/lottery/deploy.json <<'PY_CONFIG'
import json,sys
assert json.load(open(sys.argv[1])) == json.load(open(sys.argv[2])), 'Deployment configuration changed'
PY_CONFIG
fi
# Require all concrete DNS names, not a wildcard claim, to reach this host.
python3 - "$candidate/deploy.json" <<'PY'
import json,socket,subprocess,sys
c=json.load(open(sys.argv[1]))
local=json.loads(subprocess.check_output(['ip','-j','address','show','scope','global']))
# iproute2 on this host includes an empty addr_info record.
addresses={a['local'] for link in local for a in link['addr_info'] if a}
for host in c['user_domains']+[c[k] for k in ('admin_domain','platform_domain','agent_domain')]:
    resolved={r[4][0] for r in socket.getaddrinfo(host,443,type=socket.SOCK_STREAM)}
    assert resolved and resolved<=addresses, f'{host}: DNS {resolved} does not point exclusively to this host {addresses}'
PY
app() {
  systemd-run --quiet --wait --pipe --collect --service-type=exec \
    --property=User=lottery --property=Group=lottery \
    --property=EnvironmentFile=/etc/lottery/platform.env \
    "$candidate/platform" "$@"
}
if [[ $mode == initial ]]; then
  [[ ! -e /opt/lottery/current && ! -e /etc/lottery/platform.env ]]
  # Persistent auth material is created once and never regenerated by updates.
  "$candidate/platform" generate-auth-key /etc/lottery/auth.key
  chown lottery:lottery /etc/lottery/auth.key
  password=$(openssl rand -hex 24)
  LOTTERY_DB_PASSWORD="$password" runuser -u postgres -- psql -Xv ON_ERROR_STOP=1 <<'SQL'
\getenv password LOTTERY_DB_PASSWORD
CREATE ROLE lottery LOGIN PASSWORD :'password';
CREATE DATABASE lottery OWNER lottery TEMPLATE template0 ENCODING 'UTF8';
SQL
  # These generated values have no shell syntax; EnvironmentFile is never sourced.
  {
    echo 'APP_ENV=production'
    echo 'HTTP_ADDR=127.0.0.1:8080'
    echo "DATABASE_URL=postgres://lottery:$password@127.0.0.1:5432/lottery?sslmode=disable"
    echo 'DATABASE_READ_URLS=[]'
    echo 'DB_MAX_CONNS=6'
    echo 'AUTH_KEY_FILE=/etc/lottery/auth.key'
    echo 'TRUSTED_PROXY_CIDRS=127.0.0.1/32'
    python3 -c 'import json,sys; print("PLATFORM_DRAW_ALLOWED_ORIGINS="+json.dumps(json.load(open(sys.argv[1]))["draw_allowed_origins"],separators=(",",":")))' "$candidate/deploy.json"
  } > /etc/lottery/platform.env
  unset password
  chmod 600 /etc/lottery/platform.env
  app migrate
  app check
  admin_username=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["admin_username"])' "$candidate/deploy.json")
  echo "BOOTSTRAP_ADMIN_PASSWORD=$(openssl rand -hex 12)" > /etc/lottery/bootstrap.env
  chmod 600 /etc/lottery/bootstrap.env
  BOOTSTRAP_ADMIN_PASSWORD=$(cut -d = -f2 /etc/lottery/bootstrap.env)
  export BOOTSTRAP_ADMIN_PASSWORD
  systemd-run --quiet --wait --pipe --collect --service-type=exec \
    --property=User=lottery --property=EnvironmentFile=/etc/lottery/platform.env \
    --setenv=BOOTSTRAP_ADMIN_PASSWORD \
    "$candidate/platform" create-admin --username "$admin_username" --super
  unset BOOTSTRAP_ADMIN_PASSWORD
  install -m 600 "$candidate/deploy.json" /etc/lottery/deploy.json
else
  if [[ $consolidate == true ]]; then
    stopped=true
    systemctl stop lottery-api lottery-worker
    install -d -m 700 -o postgres -g postgres /var/lib/postgresql/lottery-fixed-entry-backups
    install -d -m 700 /var/backups/lottery/fixed-entries
    runuser -u postgres -- env PGDATABASE=lottery python3 "$candidate/ops/$upgrade_script" \
      --baseline "$candidate/ops/0001_baseline.up.sql" \
      --backup-dir "/var/lib/postgresql/lottery-fixed-entry-backups/$release"
    mv "/var/lib/postgresql/lottery-fixed-entry-backups/$release" "/var/backups/lottery/fixed-entries/$release"
    chown -R root:root "/var/backups/lottery/fixed-entries/$release"
  fi
  # Same-baseline updates only. Changed checksums fail here before any downtime.
  # Baseline consolidation must follow the reviewed copy/verify procedure.
  app check
fi
# Baseline objects must remain owned by the application role after privileged migrations.
runuser -u postgres -- psql -X -d lottery -v ON_ERROR_STOP=1 <<'SQL'
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
             WHERE n.nspname='public' AND c.relkind IN ('r','p','v','m','S')
               AND pg_get_userbyid(c.relowner) <> 'lottery')
     OR EXISTS (SELECT 1 FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace
                WHERE n.nspname='public' AND pg_get_userbyid(p.proowner) <> 'lottery') THEN
    RAISE EXCEPTION 'public schema objects must be owned by lottery';
  END IF;
END $$;
SQL
# The three management applications share fixed, explicitly trusted entries.
mapfile -t management_domains < <(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print("\n".join(c[k] for k in ("platform_domain","admin_domain","agent_domain")))' "$candidate/deploy.json")
runuser -u postgres -- psql -X -d lottery -v ON_ERROR_STOP=1 \
 -v platform_domain="${management_domains[0]}" -v admin_domain="${management_domains[1]}" -v agent_domain="${management_domains[2]}" <<'SQL'
INSERT INTO platform_domains(domain,enabled) VALUES (:'platform_domain',true),(:'admin_domain',true),(:'agent_domain',true)
ON CONFLICT(domain) DO UPDATE SET enabled=excluded.enabled;
SQL
app check-draw-sources
# Prepare TLS and nginx before interrupting the application.
mapfile -t domains < <(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print("\n".join(c["user_domains"]+[c[k] for k in ("admin_domain","platform_domain","agent_domain")]))' "$candidate/deploy.json")
email=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["certificate_email"])' "$candidate/deploy.json")
root_domain=${domains[0]}
install -d -m 755 /usr/local/lib/lottery
install -m 755 "$candidate/ops/namecom-dns.py" /usr/local/lib/lottery/namecom-dns.py
python3 /usr/local/lib/lottery/namecom-dns.py check "$root_domain"
certbot certonly --manual --preferred-challenges dns --non-interactive --agree-tos \
  --manual-auth-hook "/usr/local/lib/lottery/namecom-dns.py auth $root_domain" \
  --manual-cleanup-hook "/usr/local/lib/lottery/namecom-dns.py cleanup $root_domain" \
  --email "$email" --cert-name lottery-wildcard -d "$root_domain" -d "*.$root_domain" --keep-until-expiring
python3 "$candidate/ops/render-nginx.py" "$candidate/deploy.json" "$candidate/nginx.conf"
if [[ $mode == update ]]; then
  # Validate the new nginx config without replacing the active one.
  sed "s|/etc/nginx/sites-enabled/\*|$candidate/nginx.conf|" /etc/nginx/nginx.conf > "$candidate/nginx-check.conf"
  nginx -t -c "$candidate/nginx-check.conf"
  # Stop both processes so no new writes occur during the release snapshot.
  stopped=true
  systemctl stop lottery-api lottery-worker
  runuser -u postgres -- pg_dump -Fc lottery > "/var/backups/lottery/$release.dump"
  pg_restore --list "/var/backups/lottery/$release.dump" >/dev/null
  sha256sum "/var/backups/lottery/$release.dump" > "/var/backups/lottery/$release.dump.sha256"
fi
for service in api worker; do
  install -m 644 "$candidate/systemd/lottery-$service.service.example" "/etc/systemd/system/lottery-$service.service"
done
systemctl daemon-reload
ln -s "$candidate" /opt/lottery/current.next
mv -Tf /opt/lottery/current.next /opt/lottery/current
install -m 644 "$candidate/nginx.conf" /etc/nginx/sites-available/lottery
nginx -t
stopped=true
systemctl enable --now lottery-api lottery-worker
systemctl restart lottery-api lottery-worker
curl --fail --silent --show-error --retry 20 --retry-connrefused --retry-delay 1 \
  --retry-max-time 45 http://127.0.0.1:8080/health/ready >/dev/null
systemctl reload nginx
for domain in "${domains[@]}"; do
  curl --fail --silent --show-error --resolve "$domain:443:127.0.0.1" "https://$domain/" -o /dev/null
  curl --fail --silent --show-error --resolve "$domain:443:127.0.0.1" "https://$domain/health/ready" -o /dev/null
done
sleep 3
for service in api worker; do systemctl is-active --quiet "lottery-$service"; done
rm "/opt/lottery/incoming/$release.tgz" "/opt/lottery/incoming/$release.sha256"
# The healthy current release is the only served application version.
python3 - "$release" <<'PY_CLEANUP'
from pathlib import Path
import shutil, sys
releases = Path('/opt/lottery/releases')
current = Path('/opt/lottery/current').resolve(strict=True)
assert current == releases / sys.argv[1]
for directory in releases.iterdir():
    if directory != current:
        shutil.rmtree(directory)
static = Path('/opt/lottery/static')
if static.exists():
    shutil.rmtree(static)
PY_CLEANUP
complete=true
echo "Release $release deployed."
if [[ $mode == initial ]]; then
  echo 'Bootstrap password: root-only /etc/lottery/bootstrap.env. Read privately, change password, then delete this file.'
fi
REMOTE
