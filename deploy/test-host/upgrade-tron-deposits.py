#!/usr/bin/env python3
"""Consolidate the pre-release cash schema after verified backup and rehearsal.
Stop writers with the repository DEV service manager before running this script.
Credentials are sourced into memory from the exact service environment file.
"""
import argparse, difflib, hashlib, importlib.util, json, os, pathlib, re, subprocess, urllib.parse
root=pathlib.Path(__file__).resolve().parents[2]
p=argparse.ArgumentParser(description=__doc__)
p.add_argument('--env-file',required=True,type=pathlib.Path)
p.add_argument('--previous-baseline',required=True,type=pathlib.Path)
p.add_argument('--pg-bin',required=True,type=pathlib.Path)
p.add_argument('--backup-dir',required=True,type=pathlib.Path)
p.add_argument('--apply',action='store_true')
a=p.parse_args()
spec=importlib.util.spec_from_file_location('dev_backend',root/'scripts/dev-backend.py')
module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
env=module.load_environment(a.env_file.resolve());url=urllib.parse.urlsplit(env['DATABASE_URL'])
source=url.path.lstrip('/');assert re.fullmatch('[a-zA-Z0-9_]+',source)
pg_env={**os.environ,'PGHOST':url.hostname,'PGPORT':str(url.port or 5432),'PGUSER':urllib.parse.unquote(url.username),'PGPASSWORD':urllib.parse.unquote(url.password or ''),'PGDATABASE':source}
old=a.previous_baseline.read_bytes();target=(root/'backend/migrations/0001_baseline.up.sql').read_bytes()
assert target.startswith(old),'Target does not extend the supplied previous baseline'
ddl=target[len(old):].decode();assert ddl.strip(),'No new schema supplied'
assert ddl==(pathlib.Path(__file__).with_suffix('.sql')).read_text(),'DDL differs from consolidated baseline extension'
oldsum=hashlib.sha256(old).hexdigest();newsum=hashlib.sha256(target).hexdigest()
out=a.backup_dir.resolve();out.mkdir(parents=True,exist_ok=False);out.chmod(0o700)
from datetime import datetime,timezone
stamp=datetime.now(timezone.utc).strftime('%Y%m%d%H%M%S')
clone='lottery_test_tron_copy_'+stamp;fresh='lottery_test_tron_fresh_'+stamp

def command(tool,*args,db=source,stdin=None):
 return subprocess.check_output([str(a.pg_bin/tool),*args],env={**pg_env,'PGDATABASE':db},input=stdin,text=True)
def sql(query,db=source):return command('psql','-XAtv','ON_ERROR_STOP=1',db=db,stdin=query).strip()
def schema(db):
 raw=command('pg_dump','--schema-only','--no-owner','--no-privileges','--schema=public','--exclude-table=public.schema_migrations',db=db)
 return '\n'.join(line for line in raw.splitlines() if line and not line.startswith(('--','\\')))
def snapshot(db,names=None):
 if names is None:names=json.loads(sql("SELECT json_agg(tablename ORDER BY tablename) FROM pg_tables WHERE schemaname='public' AND tablename<>'schema_migrations'",db))
 result={}
 for name in names:
  assert re.fullmatch('[a-zA-Z0-9_]+',name)
  result[name]=sql('SELECT count(*)::text||\':\'||md5(coalesce(string_agg(to_jsonb(t)::text,E\'\\n\' ORDER BY to_jsonb(t)::text),\'\')) FROM public."'+name+'" t',db)
 return result
assert sql("SELECT checksum FROM schema_migrations WHERE name='0001_baseline.up.sql'")==oldsum,'Source migration differs from supplied previous baseline'
created=[]
try:
 backup=out/'before.dump';command('pg_dump','-Fc','-f',str(backup));backup.chmod(0o600)
 for db in [clone,fresh]:command('createdb',db);created.append(db)
 command('pg_restore','--exit-on-error','--no-owner','--no-privileges','-d',clone,str(backup),db=clone)
 before=snapshot(clone);sql('BEGIN;'+ddl+'COMMIT;',clone)
 assert snapshot(clone,before)==before,'Business data changed on restored copy'
 sql('BEGIN;'+target.decode()+'COMMIT;',fresh)
 diff='\n'.join(difflib.unified_diff(schema(fresh).splitlines(),schema(clone).splitlines()))
 (out/'schema.diff').write_text(diff);assert not diff,'Copy schema differs from fresh baseline'
 (out/'business-data-hashes.json').write_text(json.dumps(before,indent=2))
 if a.apply:
  assert snapshot(source,before)==before,'Source changed after backup; stop writers and repeat'
  sql('BEGIN;'+ddl+'COMMIT;')
  assert schema(source)==schema(fresh),'Source schema differs after apply'
  assert snapshot(source,before)==before,'Source business data differs after apply'
  # Explicitly synchronize metadata only after source and restored-copy checks.
  sql("BEGIN;LOCK TABLE schema_migrations IN ACCESS EXCLUSIVE MODE;UPDATE schema_migrations SET checksum='"+newsum+"' WHERE name='0001_baseline.up.sql' AND checksum='"+oldsum+"';COMMIT;")
 (out/'verification.json').write_text(json.dumps({'old_checksum':oldsum,'new_checksum':newsum,'business_tables_unchanged':len(before),'copy_and_source_match_fresh_schema':True,'applied':a.apply,'backup':str(backup)},indent=2))
 print('Cash baseline '+('applied' if a.apply else 'rehearsed')+'; '+str(len(before))+' business tables verified; evidence: '+str(out))
finally:
 for db in created:command('dropdb',db)
