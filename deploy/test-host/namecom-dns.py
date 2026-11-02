#!/usr/bin/env python3
"""Certbot DNS-01 hooks for the Name.com production Core API."""
import base64
import http.client
import json
import os
from pathlib import Path
import re
import ssl
import stat
import subprocess
import sys
import time

CREDENTIALS = Path('/etc/letsencrypt/namecom.json')


def credentials(path=CREDENTIALS):
    info = path.stat()
    if info.st_uid != 0 or stat.S_IMODE(info.st_mode) != 0o600:
        raise RuntimeError('Name.com credentials must be root-owned mode 0600')
    config = json.loads(path.read_text())
    if set(config) != {'username', 'token'} or not all(isinstance(v, str) and v.strip() for v in config.values()):
        raise RuntimeError('Set Name.com production username and token in the credentials file')
    if config['username'].endswith('-test'):
        raise RuntimeError('Name.com DEV credentials cannot validate live DNS; production credentials required')
    return config


def request(config, zone, method, suffix='', body=None):
    auth = base64.b64encode((config['username'] + ':' + config['token']).encode()).decode()
    connection = http.client.HTTPSConnection('api.name.com', timeout=20, context=ssl.create_default_context())
    try:
        connection.request(method, f'/core/v1/domains/{zone}/records{suffix}',
                           body=None if body is None else json.dumps(body),
                           headers={'Authorization': 'Basic ' + auth, 'Content-Type': 'application/json'})
        response = connection.getresponse()
        raw = response.read()
        accepted = (200, 204) if method == 'DELETE' else (200,)
        if response.status not in accepted:
            raise RuntimeError(f'Name.com Core API {method} failed: HTTP {response.status}')
        return json.loads(raw) if raw else None
    finally:
        connection.close()


def dig(name, record, server=None):
    command = ['dig', '+short', '+time=2', '+tries=1']
    if server:
        command.append('@' + server)
    result = subprocess.run(command + [name, record], capture_output=True, text=True, check=True)
    return result.stdout.splitlines()


def remove(config, zone, record_id, validation):
    record = request(config, zone, 'GET', '/' + str(record_id))
    if record['type'] != 'TXT' or record['host'] != '_acme-challenge' or record['answer'] != validation:
        raise RuntimeError('DNS cleanup refused: record does not match this ACME challenge')
    request(config, zone, 'DELETE', '/' + str(record_id))


def authorize(config, zone, validation):
    record = request(config, zone, 'POST', body={'host': '_acme-challenge', 'type': 'TXT', 'answer': validation, 'ttl': 300})
    record_id = record['id']
    if not isinstance(record_id, int) or record_id <= 0:
        raise RuntimeError('Name.com returned an invalid DNS record id')
    try:
        servers = dig(zone, 'NS')
        if not servers or not all(server.endswith('.name.com.') for server in servers):
            raise RuntimeError('Domain is not delegated to Name.com nameservers')
        servers += ['1.1.1.1', '8.8.8.8']
        deadline = time.monotonic() + 600
        while time.monotonic() < deadline:
            if all('"' + validation + '"' in dig('_acme-challenge.' + zone, 'TXT', server) for server in servers):
                print(record_id, flush=True)  # Certbot passes only this id to the cleanup hook.
                return
            time.sleep(5)
        raise RuntimeError('ACME TXT record did not propagate within 600 seconds')
    except BaseException:
        remove(config, zone, record_id, validation)
        raise


def main():
    if len(sys.argv) != 3 or sys.argv[1] not in ('check', 'auth', 'cleanup'):
        raise RuntimeError('Usage: namecom-dns.py check|auth|cleanup ZONE')
    action, zone = sys.argv[1:]
    if not re.fullmatch(r'[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?', zone) or '.' not in zone:
        raise RuntimeError('Invalid DNS zone')
    config = credentials()
    if action == 'check':
        request(config, zone, 'GET')
        print('Name.com production DNS access verified')
        return
    if os.environ.get('CERTBOT_DOMAIN', '').removeprefix('*.') != zone:
        raise RuntimeError('Certificate domain differs from the configured DNS zone')
    validation = os.environ['CERTBOT_VALIDATION']
    if not re.fullmatch(r'[A-Za-z0-9_-]{43}', validation):
        raise RuntimeError('Invalid ACME DNS challenge')
    if action == 'auth':
        authorize(config, zone, validation)
    else:
        record_id = os.environ['CERTBOT_AUTH_OUTPUT'].strip()
        if not record_id.isdecimal():
            raise RuntimeError('Missing or invalid ACME DNS cleanup record id')
        remove(config, zone, int(record_id), validation)


if __name__ == '__main__':
    try:
        main()
    except (RuntimeError, OSError, ValueError, KeyError, http.client.HTTPException) as error:
        if isinstance(error, RuntimeError):
            print(str(error), file=sys.stderr)
        else:
            print('Name.com DNS hook failed; inspect credentials, API access and DNS connectivity', file=sys.stderr)
        sys.exit(1)
