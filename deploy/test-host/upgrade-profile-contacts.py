#!/usr/bin/env python3
"""One-time pre-release baseline consolidation; run with writers stopped.
Uses PG* environment, and requires the exact previous baseline checksum.
Backs up, verifies a restored copy against a fresh baseline and all business
row hashes, then applies the same DDL and synchronizes migration metadata.
"""
import argparse, datetime, difflib, hashlib, json, os, pathlib, re, subprocess
p=argparse.ArgumentParser();p.add_argument('--baseline',required=True,type=pathlib.Path);p.add_argument('--backup-dir',required=True,type=pathlib.Path);p.add_argument('--pg-bin',default='');p.add_argument('--rehearse-only',action='store_true');a=p.parse_args()
old='a65ec6f38bcfbc352d9bc50f4056f65c87438528cdec64a6139ec2ef6a77884c'
new=hashlib.sha256(a.baseline.read_bytes()).hexdigest()
assert new=='97abad7a577e86e79fa6293fd6b42e39ce573ec8da3e2c94b1063d17e9864124', 'unexpected target baseline'
source=os.environ['PGDATABASE'];assert re.fullmatch('[a-zA-Z0-9_]+',source)
stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%d%H%M%S')
clone='lottery_test_contacts_copy_'+stamp;fresh='lottery_test_contacts_fresh_'+stamp
out=a.backup_dir;out.mkdir(parents=True,exist_ok=False)
def command(tool,*args,db=source,stdin=None):
 return subprocess.check_output([str(pathlib.Path(a.pg_bin)/tool) if a.pg_bin else tool,*args],input=stdin,env={**os.environ,'PGDATABASE':db},text=True)
def sql(q,db=source):return command('psql','-XAtv','ON_ERROR_STOP=1','-c',q,db=db).strip()
assert sql("SELECT checksum FROM schema_migrations WHERE name='0001_baseline.up.sql'")==old,'unexpected baseline; refusing upgrade'
ddl=(pathlib.Path(__file__).with_suffix('.sql')).read_text()
def snapshot(db):
 return sql('''CREATE TEMP TABLE contacts_snapshot(name text, digest text);
DO $snapshot$ DECLARE t record; h text; BEGIN
FOR t IN SELECT tablename FROM pg_tables WHERE schemaname='public' AND tablename<>'schema_migrations' ORDER BY tablename LOOP
EXECUTE format('SELECT md5(coalesce(string_agg(row_hash, %L ORDER BY row_hash),%L)) FROM (SELECT md5((to_jsonb(x)%s)::text) row_hash FROM %I x) rows', '', '', CASE WHEN t.tablename='global_users' THEN $$-'wechat'-'telegram'-'whatsapp'$$ ELSE '' END, t.tablename) INTO h;
INSERT INTO contacts_snapshot VALUES(t.tablename,h);
END LOOP; END $snapshot$;
SELECT jsonb_object_agg(name,digest) FROM contacts_snapshot;''',db).splitlines()[-1]
def schema(db):
 raw=command('pg_dump','--schema-only','--no-owner','--no-privileges','--schema=public','--exclude-table=public.schema_migrations',db=db)
 return '\n'.join(x for x in raw.splitlines() if x and not x.startswith('--') and not x.startswith('\\'))
command('pg_dump','-Fc','-f',str(out/'before.dump'))
command('pg_restore','--list',str(out/'before.dump'))
created=[]
try:
 for db in [clone,fresh]:command('createdb',db);created.append(db)
 command('pg_restore','--no-owner','--no-privileges','-d',clone,str(out/'before.dump'),db=clone)
 before=snapshot(clone);sql('BEGIN;'+ddl+'COMMIT;',clone);assert snapshot(clone)==before,'business data changed on copy'
 command('psql','--single-transaction','-Xv','ON_ERROR_STOP=1','-f',str(a.baseline),db=fresh)
 diff='\n'.join(difflib.unified_diff(schema(fresh).splitlines(),schema(clone).splitlines(),fromfile='fresh',tofile='upgraded'))
 (out/'schema.diff').write_text(diff);assert not diff,'copy schema differs from fresh baseline'
 (out/'copy-data-hashes.json').write_text(before)
 if not a.rehearse_only:
  # Writers must remain stopped throughout snapshot, apply and comparison.
  live=snapshot(source);assert live==before,'source changed since backup; stop writers and repeat'
  sql("BEGIN;"+ddl+"""
ALTER VIEW betting_financial_facts OWNER TO lottery;
ALTER FUNCTION commission_correction_candidates(uuid,uuid,uuid) OWNER TO lottery;
ALTER FUNCTION commission_correction_candidates_v2(uuid,uuid,uuid) OWNER TO lottery;
"""+"UPDATE schema_migrations SET checksum='"+new+"' WHERE name='0001_baseline.up.sql' AND checksum='"+old+"';COMMIT;")
  assert snapshot(source)==before,'business data differs after apply'
  assert schema(source)==schema(fresh),'live schema differs after apply'
 (out/'verification.json').write_text(json.dumps({'old_checksum':old,'new_checksum':new,'all_business_rows_unchanged':True,'schema_matches_fresh_baseline':True,'applied':not a.rehearse_only},indent=2))
 print('Baseline upgrade '+('rehearsed' if a.rehearse_only else 'applied')+'; schema and every business table verified. Evidence: '+str(out))
finally:
 for db in created:command('dropdb',db)
