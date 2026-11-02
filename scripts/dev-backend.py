#!/usr/bin/env python3
"""Start both DEV services from one explicit shell environment file."""
import argparse
import fcntl
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import time
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
RUNTIME = ROOT / '.local/dev-backend'
STATE = RUNTIME / 'runtime.json'


def process_command(pid):
    row = subprocess.run(['ps', '-p', str(pid), '-o', 'stat=', '-o', 'args='], capture_output=True, text=True, check=False).stdout.strip()
    if not row:
        return ''
    status, command = row.split(None, 1)
    return '' if status.startswith('Z') else command


def stop_processes(state):
    # Validate all identities before sending any signals. Never kill by port.
    for mode, entry in state['processes'].items():
        command = process_command(entry['pid'])
        if command and command != f"{entry['binary']} {mode}":
            raise RuntimeError(f'{mode} PID now belongs to another process; refusing to stop it')
    for entry in state['processes'].values():
        if process_command(entry['pid']):
            os.kill(entry['pid'], signal.SIGTERM)
    deadline = time.monotonic() + 45
    while any(process_command(e['pid']) for e in state['processes'].values()):
        if time.monotonic() >= deadline:
            raise RuntimeError('Services did not stop within 45 seconds')
        time.sleep(.2)


def load_environment(path):
    # Source once, export every assignment, and give both children this exact map.
    # The NUL-delimited environment stays in memory and is never printed or saved.
    raw = subprocess.check_output(['bash', '-c', 'set -ae; source "$1"; env -0', 'dev-backend', str(path)], cwd=ROOT)
    env = dict(part.decode().split('=', 1) for part in raw.split(b'\0') if part)
    if env.get('APP_ENV') != 'development':
        raise RuntimeError('This entry point requires APP_ENV=development')
    for name in ['DATABASE_URL', 'AUTH_KEY_FILE', 'HTTP_ADDR']:
        if not env.get(name):
            raise RuntimeError(f'{name} must be configured in the environment file')
    return env


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['start', 'restart', 'stop', 'status', 'check'])
    parser.add_argument('--env-file', required=True, type=Path)
    parser.add_argument('--adopt-pids', type=Path, help='One-time adoption of existing serve/worker PID JSON')
    args = parser.parse_args()
    env_path = args.env_file.resolve(strict=True)
    env = load_environment(env_path)
    RUNTIME.mkdir(parents=True, exist_ok=True)
    os.chmod(RUNTIME, 0o700)
    lock = (RUNTIME / 'control.lock').open('a')
    try:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    except BlockingIOError:
        raise RuntimeError('Another backend control command is running')
    state = json.loads(STATE.read_text()) if STATE.exists() else None
    if args.adopt_pids:
        if state or args.action != 'restart':
            raise RuntimeError('Adoption requires restart with no managed runtime')
        pids = json.loads(args.adopt_pids.read_text())
        if set(pids) != {'serve', 'worker'}:
            raise RuntimeError('Adoption requires exactly serve and worker PIDs')
        entries = {}
        for mode, pid in pids.items():
            command = process_command(pid)
            binary, actual_mode = command.rsplit(' ', 1)
            if actual_mode != mode or not Path(binary).is_relative_to(ROOT / '.local'):
                raise RuntimeError('Adopted process is not a local backend service')
            # Confirm the service identity before preserving its database and port.
            raw = subprocess.check_output(['ps', 'eww', '-p', str(pid), '-o', 'command='], text=True)
            import re
            for name in ['DATABASE_URL', 'AUTH_KEY_FILE', 'HTTP_ADDR', 'APP_ENV']:
                match = re.search(r'(?:^| )' + name + r'=(.*?)(?= [A-Za-z_][A-Za-z0-9_]*=|$)', raw)
                if not match or match.group(1) != env[name]:
                    raise RuntimeError(f'Adopted service {name} differs from the selected environment file')
            entries[mode] = {'pid': pid, 'binary': binary}
        state = {'env_file': str(env_path), 'processes': entries}
    if state and state['env_file'] != str(env_path):
        raise RuntimeError('Use the same environment file as the managed runtime')
    if args.action == 'status':
        if not state:
            raise RuntimeError('No managed backend runtime')
        for mode, entry in state['processes'].items():
            if process_command(entry['pid']) != f"{entry['binary']} {mode}":
                raise RuntimeError(f'{mode} is not running')
            print(f'{mode}: running (PID {entry["pid"]})')
        return
    if args.action == 'stop':
        if not state:
            raise RuntimeError('No managed backend runtime')
        stop_processes(state)
        STATE.unlink(missing_ok=True)
        return
    if args.action == 'start' and state:
        raise RuntimeError('A managed runtime exists; use restart')
    if args.action == 'restart' and not state:
        raise RuntimeError('No managed runtime; use start or explicitly adopt existing PIDs')
    # Build a new binary without overwriting the running executable.
    binary = RUNTIME / f'platform-{time.time_ns()}'
    subprocess.run(['go', 'build', '-o', str(binary), './cmd/platform'], cwd=ROOT / 'backend', env=env, check=True)
    subprocess.run([str(binary), 'check'], cwd=ROOT, env=env, check=True)
    subprocess.run([str(binary), 'check-draw-sources'], cwd=ROOT, env=env, check=True)
    if args.action == 'check':
        binary.unlink()
        return
    if state:
        stop_processes(state)
    new_state = {'env_file': str(env_path), 'processes': {}}
    try:
        for mode in ['serve', 'worker']:
            with (RUNTIME / f'{mode}.log').open('ab') as log:
                child = subprocess.Popen([str(binary), mode], cwd=ROOT, env=env, stdin=subprocess.DEVNULL, stdout=log, stderr=log, start_new_session=True)
            new_state['processes'][mode] = {'pid': child.pid, 'binary': str(binary)}
        STATE.write_text(json.dumps(new_state, indent=2) + '\n')
        deadline = time.monotonic() + 20
        while True:
            for mode, entry in new_state['processes'].items():
                if process_command(entry['pid']) != f"{entry['binary']} {mode}":
                    raise RuntimeError(f'{mode} stopped; inspect .local/dev-backend/{mode}.log')
            try:
                with urllib.request.urlopen('http://' + env['HTTP_ADDR'] + '/health/ready', timeout=2) as response:
                    if response.status == 200:
                        break
            except OSError:
                pass  # Poll the same child while its listener starts.
            if time.monotonic() >= deadline:
                raise RuntimeError('API did not become ready')
            time.sleep(.2)
        # Post-restart upstream verification is mandatory, not just API readiness.
        subprocess.run([str(binary), 'check-draw-sources'], cwd=ROOT, env=env, check=True)
        for mode, entry in new_state['processes'].items():
            if process_command(entry['pid']) != f"{entry['binary']} {mode}":
                raise RuntimeError(f'{mode} stopped during source verification')
        print('API and worker started; database schema, API readiness and source reads passed.')
    except BaseException:
        stop_processes(new_state)
        STATE.unlink(missing_ok=True)
        raise


if __name__ == '__main__':
    try:
        main()
    except (RuntimeError, subprocess.CalledProcessError) as error:
        print(str(error), file=sys.stderr)
        sys.exit(1)
