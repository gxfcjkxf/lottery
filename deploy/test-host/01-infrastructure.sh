#!/usr/bin/env bash
# Run on the target: ssh lottery 'bash -s' < deploy/test-host/01-infrastructure.sh
set -Eeuo pipefail
trap 'echo "Infrastructure failed at line $LINENO; inspect the error before rerunning." >&2' ERR
[[ $EUID == 0 ]] || { echo 'Run as root.' >&2; exit 1; }
# shellcheck source=/dev/null
. /etc/os-release
[[ $ID == ubuntu && $VERSION_ID == 26.04 && $(uname -m) == x86_64 ]] || {
  echo 'This script requires Ubuntu 26.04 x86_64.' >&2; exit 1;
}
[[ ! -e /opt/lottery/current ]] || { echo 'Infrastructure initialization refuses an installed application.' >&2; exit 1; }
# Verify key access before disabling passwords. Keep the current root/key entry.
[[ -s /root/.ssh/authorized_keys && -n ${SSH_CONNECTION:-} ]] || {
  echo 'Run over an established root SSH key session.' >&2; exit 1;
}
ssh_port=$(awk '{print $4}' <<< "$SSH_CONNECTION")
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y postgresql-18 postgresql-client-18 nginx certbot ca-certificates curl \
  openssl python3 rsync ufw logrotate unattended-upgrades bind9-dnsutils

# This 1 GB host needs swap for transient memory pressure, not compilation.
if [[ ! -e /swapfile-lottery ]]; then
  fallocate -l 1536M /swapfile-lottery
  chmod 600 /swapfile-lottery
  mkswap /swapfile-lottery
fi
if ! swapon --show=NAME --noheadings | grep -Fxq /swapfile-lottery; then
  swapon /swapfile-lottery
fi
if ! grep -q '^/swapfile-lottery ' /etc/fstab; then
  echo '/swapfile-lottery none swap sw 0 0' >> /etc/fstab
fi
cat > /etc/sysctl.d/60-lottery.conf <<'CONF'
vm.swappiness = 10
CONF
sysctl -p /etc/sysctl.d/60-lottery.conf
install -d /etc/systemd/journald.conf.d
cat > /etc/systemd/journald.conf.d/lottery.conf <<'CONF'
[Journal]
SystemMaxUse=100M
RuntimeMaxUse=32M
MaxRetentionSec=7day
CONF
systemctl restart systemd-journald

# Ubuntu's single, package-created PostgreSQL 18/main cluster.
[[ $(pg_lsclusters --no-header | wc -l) == 1 ]]
pg_lsclusters --no-header | grep -Eq '^18[[:space:]]+main[[:space:]]+5432[[:space:]]'
install -d /etc/postgresql/18/main/conf.d
cat > /etc/postgresql/18/main/conf.d/lottery.conf <<'CONF'
listen_addresses = '127.0.0.1'
max_connections = 40
shared_buffers = '128MB'
effective_cache_size = '384MB'
work_mem = '2MB'
maintenance_work_mem = '32MB'
max_parallel_workers_per_gather = 0
jit = off
min_wal_size = '80MB'
max_wal_size = '512MB'
password_encryption = 'scram-sha-256'
CONF
# Keep fsync, full_page_writes, synchronous_commit and autovacuum enabled.
systemctl enable postgresql nginx certbot.timer unattended-upgrades
systemctl restart postgresql@18-main
runuser -u postgres -- pg_isready -h 127.0.0.1 -p 5432
if ! id lottery >/dev/null 2>&1; then
  useradd --system --home-dir /opt/lottery --shell /usr/sbin/nologin lottery
fi
install -d -m 755 /opt/lottery /opt/lottery/releases
install -d -m 700 /opt/lottery/incoming /var/backups/lottery
install -d -o root -g lottery -m 750 /etc/lottery
install -d -m 755 /var/www/letsencrypt
# Ubuntu 26.04 already declares server_tokens and gzip in nginx.conf.
sed -i -E 's/^([[:space:]]*)server_tokens[[:space:]]+[^;]+;/\1server_tokens off;/' /etc/nginx/nginx.conf
cat > /etc/nginx/conf.d/lottery-runtime.conf <<'CONF'
gzip_types text/css application/javascript application/json image/svg+xml;
CONF
cat > /etc/nginx/sites-available/lottery <<'CONF'
server {
    listen 80 default_server;
    listen [::]:80 default_server;
    server_name _;
    location /.well-known/acme-challenge/ { root /var/www/letsencrypt; }
    location / { return 404; }
}
CONF
rm -f /etc/nginx/sites-enabled/default
ln -sfn /etc/nginx/sites-available/lottery /etc/nginx/sites-enabled/lottery
nginx -t
systemctl restart nginx
install -d -m 755 /etc/letsencrypt
if [[ ! -e /etc/letsencrypt/namecom.json ]]; then
  (umask 077; printf '{"username":"","token":""}\n' > /etc/letsencrypt/namecom.json)
fi
install -d /etc/letsencrypt/renewal-hooks/deploy
cat > /etc/letsencrypt/renewal-hooks/deploy/lottery-nginx <<'HOOK'
#!/bin/sh
set -eu
nginx -t
systemctl reload nginx
HOOK
chmod 755 /etc/letsencrypt/renewal-hooks/deploy/lottery-nginx
systemctl start certbot.timer

# SSH remains reachable; only the proxy is public. No database/API public ports.
ufw allow "$ssh_port/tcp"
ufw allow 80/tcp
ufw allow 443/tcp
ufw default deny incoming
ufw default allow outgoing
ufw --force enable
cat > /etc/ssh/sshd_config.d/00-lottery.conf <<'CONF'
PasswordAuthentication no
KbdInteractiveAuthentication no
PermitRootLogin prohibit-password
X11Forwarding no
CONF
sshd -t
systemctl reload ssh
nginx -t
systemctl is-active postgresql@18-main nginx
ufw status
free -h
echo 'Infrastructure ready. Verify a NEW ssh lottery connection before closing this session.'
