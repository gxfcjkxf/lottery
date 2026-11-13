#!/usr/bin/env python3
"""Pre-release brand presentation cleanup through the test-host deployment entry.
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
old = 'b1b5c380628902e4e116969b86a3b4bca2f26f1c9a79c84c3837bef3770bd159'
body = args.baseline.read_bytes()
ddl_bytes = Path(__file__).with_name('upgrade-brand-presentation.sql').read_bytes()
assert hashlib.sha256(body).hexdigest() == '98f695470f325bcd5efcf32d270bef821b59f8389f0a7306c46f8527671b53d7', 'Unexpected target baseline'
new = hashlib.sha256(body).hexdigest()
ddl = ddl_bytes.decode()
source = os.environ['PGDATABASE']
assert re.fullmatch('[a-zA-Z0-9_]+', source)
stamp = datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%d%H%M%S')
clone = 'lottery_test_presentation_copy_' + stamp
fresh = 'lottery_test_presentation_fresh_' + stamp
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
    result = {}
    for name in names:
        assert re.fullmatch('[a-zA-Z0-9_]+', name)
        row = 'to_jsonb(t)'
        removed = "ARRAY['primary_color','accent_color','success_color','warning_color','danger_color','font_family','font_scale','radius','shadow']::text[]"
        if name == 'brands':
            row = "(to_jsonb(t)-'theme')"
        if name in ['brand_presentations', 'brand_presentation_revisions']:
            row = "jsonb_set(to_jsonb(t),'{config}',t.config - " + removed + ")"
            if name == 'brand_presentation_revisions':
                row = "jsonb_set(" + row + ",'{effective}',t.effective - " + removed + ")"
        result[name] = sql("SELECT count(*)::text||':'||md5(coalesce(string_agg("
                           + row + "::text,E'\\n' ORDER BY " + row + "::text),'')) "
                           'FROM public."' + name + '" t', db)
    return result


assert sql("SELECT checksum FROM schema_migrations WHERE name='0001_baseline.up.sql'") == old, 'Unexpected source baseline'
backup = out / 'before.dump'
command('pg_dump', '-Fc', '-f', str(backup))
backup.chmod(0o600)
command('pg_restore', '--list', str(backup))
(out / 'before.dump.sha256').write_text(hashlib.sha256(backup.read_bytes()).hexdigest() + '  before.dump\n')
owner = sql("SELECT pg_get_userbyid(relowner) FROM pg_class WHERE oid='public.brands'::regclass")
assert re.fullmatch('[a-zA-Z0-9_]+', owner), 'Unsupported owner'
created = []
try:
    for db in [clone, fresh]:
        command('createdb', '--owner=' + owner, db)
        created.append(db)
    # Preserve original object ownership; SET ROLE covers newly added objects.
    command('pg_restore', '--exit-on-error', '-d', clone, str(backup), db=clone)
    before = snapshot(clone)
    sql('BEGIN; SET LOCAL ROLE ' + owner + '; ' + ddl + ' COMMIT;', clone)
    assert snapshot(clone, before) == before, 'Unrelated business data changed on restored copy'
    sql('BEGIN; SET LOCAL ROLE ' + owner + '; ' + body.decode() + ' COMMIT;', fresh)
    diff = '\n'.join(difflib.unified_diff(schema(fresh).splitlines(), schema(clone).splitlines()))
    (out / 'schema.diff').write_text(diff)
    assert not diff, 'Restored copy differs from the fresh baseline'
    (out / 'business-data-hashes.json').write_text(json.dumps(before, indent=2))
    if not args.rehearse_only:
        assert snapshot(source, before) == before, 'Source changed since backup; stop writers and repeat'
        sql('BEGIN; SET LOCAL ROLE ' + owner + '; ' + ddl + ' COMMIT;')
        assert schema(source) == schema(fresh), 'Live schema differs after application'
        assert snapshot(source, before) == before, 'Live business data changed'
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
    print('Brand presentation baseline ' + ('rehearsed' if args.rehearse_only else 'applied') +
          '; verified ' + str(len(before)) + ' original business tables; evidence: ' + str(out))
finally:
    for db in created:
        command('dropdb', db)
