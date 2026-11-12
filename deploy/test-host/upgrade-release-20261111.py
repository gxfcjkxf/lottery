#!/usr/bin/env python3
"""Pre-release announcements, agent wallet and compliance removal upgrade through the test-host deployment entry.
Run as postgres with PGDATABASE and application writers stopped for application.
A rehearsal restores a backup into an owned temporary database and compares it
with the consolidated baseline before any live schema or migration change.
"""
import argparse
import datetime
import difflib
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--baseline', required=True, type=Path)
parser.add_argument('--backup-dir', required=True, type=Path)
parser.add_argument('--rehearse-only', action='store_true')
args = parser.parse_args()
old = 'af2088754e381f850a34e62c7195a24016c6e1965e5b935f2241e60920651a25'
body = args.baseline.read_bytes()
ddl_bytes = Path(__file__).with_name('upgrade-release-20261111.sql').read_bytes()
new = hashlib.sha256(body).hexdigest()
assert new == 'f8973251c87a7f482385632c883debcf8c2318f7b22981796fdb82b86261ebda', 'Unexpected target baseline'
ddl = ddl_bytes.decode()
source = os.environ['PGDATABASE']
assert re.fullmatch('[a-zA-Z0-9_]+', source)
stamp = datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%d%H%M%S')
clone = 'lottery_test_release_copy_' + stamp
fresh = 'lottery_test_release_fresh_' + stamp
out = args.backup_dir
out.mkdir(parents=True, exist_ok=False)
out.chmod(0o700)


def command(tool, *values, db=source, stdin=None):
    return subprocess.check_output([tool, *values], input=stdin,
                                   env={**os.environ, 'PGDATABASE': db}, text=True)


def sql(query, db=source):
    return command('psql', '-XAtv', 'ON_ERROR_STOP=1', db=db, stdin=query).strip()


def schema(db):
    raw = command('pg_dump', '--schema-only', '--no-owner', '--no-privileges',
                  '--schema=public', '--exclude-table=public.schema_migrations', db=db)
    return '\n'.join(line for line in raw.splitlines()
                     if line and not line.startswith(('--', '\\')))


def snapshot(db, names=None):
    if names is None:
        names = json.loads(sql("SELECT json_agg(tablename ORDER BY tablename) FROM pg_tables "
                               "WHERE schemaname='public' AND tablename<>'schema_migrations'", db))
    retired = {'brand_compliance_policies','compliance_decisions','compliance_gate_rejections','compliance_policy_revisions','platform_compliance_settings'}
    result = {}
    for name in names:
        if name in retired:
            continue
        assert re.fullmatch('[a-zA-Z0-9_]+', name)
        where = " WHERE key !~ '^compliance_' AND key NOT LIKE 'announcement.%'" if name == 'permissions' else " WHERE permission_key !~ '^compliance_'" if name == 'role_permissions' else ''
        result[name] = sql("SELECT count(*)::text||':'||md5(coalesce(string_agg("
                           "to_jsonb(t)::text,E'\\n' ORDER BY to_jsonb(t)::text),'')) "
                           'FROM public."' + name + '" t' + where, db)
    return result


assert sql("SELECT checksum FROM schema_migrations WHERE name='0001_baseline.up.sql'") == old, 'Unexpected source baseline'
backup = out / 'before.dump'
command('pg_dump', '-Fc', '-f', str(backup))
backup.chmod(0o600)
command('pg_restore', '--list', str(backup))
(out / 'before.dump.sha256').write_text(hashlib.sha256(backup.read_bytes()).hexdigest() + '  before.dump\n')
created = []
try:
    for db in [clone, fresh]:
        command('createdb', '--owner=lottery', db)
        created.append(db)
    # Preserve original object ownership; SET ROLE covers newly added objects.
    command('pg_restore', '--exit-on-error', '-d', clone, str(backup), db=clone)
    before = snapshot(clone)
    permission_before = json.loads(sql("SELECT json_agg(key ORDER BY key) FROM permissions", clone))
    expected_permissions = sorted([key for key in permission_before if not key.startswith('compliance_')] + ['announcement.'+action+'.'+scope for action in ['view','write','publish'] for scope in ['brand','platform']])
    sql('BEGIN; SET LOCAL ROLE lottery; ' + ddl + ' COMMIT;', clone)
    after = snapshot(clone, before)
    (out / 'business-data-before.json').write_text(json.dumps(before, indent=2))
    (out / 'business-data-after.json').write_text(json.dumps(after, indent=2))
    assert after == before, 'Business data changed on restored copy: ' + ', '.join(name for name in before if before[name] != after[name])
    assert sorted(json.loads(sql("SELECT json_agg(key ORDER BY key) FROM permissions",clone))) == expected_permissions, 'Unexpected permission changes'
    sql('BEGIN; SET LOCAL ROLE lottery; ' + body.decode() + ' COMMIT;', fresh)
    diff = '\n'.join(difflib.unified_diff(schema(fresh).splitlines(), schema(clone).splitlines()))
    (out / 'schema.diff').write_text(diff)
    assert not diff, 'Restored copy differs from the fresh baseline'
    (out / 'business-data-hashes.json').write_text(json.dumps(before, indent=2))
    if not args.rehearse_only:
        assert snapshot(source, before) == before, 'Source changed since backup; stop writers and repeat'
        sql('BEGIN; SET LOCAL ROLE lottery; ' + ddl + ' COMMIT;')
        assert schema(source) == schema(fresh), 'Live schema differs after application'
        assert snapshot(source, before) == before, 'Live business data changed'
        assert sorted(json.loads(sql("SELECT json_agg(key ORDER BY key) FROM permissions"))) == expected_permissions, 'Unexpected live permission changes'
        sql("BEGIN; LOCK TABLE schema_migrations IN ACCESS EXCLUSIVE MODE; "
            "UPDATE schema_migrations SET checksum='" + new + "' "
            "WHERE name='0001_baseline.up.sql' AND checksum='" + old + "'; COMMIT;")
    (out / 'verification.json').write_text(json.dumps({
        'old_checksum': old, 'new_checksum': new,
        'business_tables_unchanged': len(before),
        'schema_matches_fresh_baseline': True,
        'applied': not args.rehearse_only,
        'backup': str(backup),
    }, indent=2))
    print('Release baseline ' + ('rehearsed' if args.rehearse_only else 'applied') +
          '; verified ' + str(len(before)) + ' original business tables; evidence: ' + str(out))
finally:
    for db in created:
        command('dropdb', db)
