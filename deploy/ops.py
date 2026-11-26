#!/usr/bin/env python3
"""Local operations for the existing PM2/Supabase deployment; no external dependencies."""
import argparse
import contextlib
import datetime as dt
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import socket
import ssl
import subprocess
import tarfile
import time
import urllib.request
import uuid

UTC = dt.timezone.utc
DB = 'ingeneriya-supabase-db'
KIND = 'firstorder-backup-v1'


def execute(args, **kwargs):
    return subprocess.run(args, check=True, capture_output=True, timeout=300, **kwargs)


def sql(container, statement, database='postgres'):
    return execute(['docker', 'exec', '-i', container, 'psql', '-XqAt', '-v', 'ON_ERROR_STOP=1',
                    '-U', 'supabase_admin', '-d', database], input=statement.encode()).stdout.decode().strip()


def private_json(path, value):
    temp = path.with_name(path.name + '.next')
    with os.fdopen(os.open(temp, os.O_WRONLY | os.O_CREAT | os.O_TRUNC | os.O_NOFOLLOW, 0o600), 'w') as handle:
        json.dump(value, handle, indent=2)
        handle.write('\n')
    os.replace(temp, path)


def sha256(path):
    digest = hashlib.sha256()
    with path.open('rb') as handle:
        for block in iter(lambda: handle.read(1024 * 1024), b''):
            digest.update(block)
    return digest.hexdigest()


@contextlib.contextmanager
def lock(root, name):
    state = root / '.ops'
    state.mkdir(mode=0o700, exist_ok=True)
    with (state / (name + '.lock')).open('a') as handle:
        fcntl.flock(handle, fcntl.LOCK_EX | fcntl.LOCK_NB)
        yield


def inventory_query(tables):
    # Names come from pg_catalog, and identifiers are still quoted independently.
    def ident(value):
        return '"' + value.replace('"', '""') + '"'
    pieces = []
    for schema, table in tables:
        key = schema + '.' + table
        pieces.append("SELECT '" + key.replace("'", "''") + "' AS name,count(*) AS count FROM " + ident(schema) + '.' + ident(table))
    return 'SELECT jsonb_object_agg(name,count) FROM (' + ' UNION ALL '.join(pieces) + ') inventory'


def backups(root):
    result = []
    folder = root / '.backups'
    if folder.exists():
        for path in folder.iterdir():
            if path.is_symlink() or not path.is_dir() or not re.fullmatch(r'backup-\d{8}T\d{6}Z-[a-f0-9]{8}', path.name):
                continue
            try:
                manifest = json.loads((path / 'manifest.json').read_text())
                if manifest['kind'] == KIND and manifest['status'] == 'complete':
                    result.append((path, manifest))
            except (OSError, ValueError, KeyError):
                continue
    return sorted(result, key=lambda item: item[1]['created_at'])


def verified_manifest(path):
    manifest = json.loads((path / 'manifest.json').read_text())
    if manifest.get('kind') != KIND or manifest.get('status') != 'complete':
        raise ValueError('incomplete backup')
    if set(manifest['files']) != {'postgres.dump', 'globals.sql', 'config.tar.gz', 'roles.json'}:
        raise ValueError('invalid backup inventory')
    for name, evidence in manifest['files'].items():
        file = path / name
        if file.is_symlink() or not file.is_file() or file.stat().st_size != evidence['bytes'] or sha256(file) != evidence['sha256']:
            raise ValueError('backup checksum mismatch')
    return manifest


def backup(root):
    with lock(root, 'backup'):
        folder = root / '.backups'
        folder.mkdir(mode=0o700, exist_ok=True)
        created = dt.datetime.now(UTC)
        path = folder / ('backup-' + created.strftime('%Y%m%dT%H%M%SZ') + '-' + uuid.uuid4().hex[:8])
        path.mkdir(mode=0o700)
        image = execute(['docker', 'inspect', '--format', '{{.Image}}', DB]).stdout.decode().strip()
        manifest = {'kind': KIND, 'status': 'creating', 'created_at': created.isoformat(), 'image': image,
                    'commit': execute(['git', '-C', str(root), 'rev-parse', 'HEAD']).stdout.decode().strip()}
        manifest['image_tag'] = execute(['docker', 'inspect', '--format', '{{.Config.Image}}', DB]).stdout.decode().strip()
        manifest['image_repository_digests'] = json.loads(execute(['docker', 'image', 'inspect', '--format', '{{json .RepoDigests}}', image]).stdout)
        private_json(path / 'manifest.json', manifest)
        args = ['docker', 'exec', '-i', DB, 'psql', '-XqAt', '-v', 'ON_ERROR_STOP=1', '-U', 'supabase_admin', '-d', 'postgres']
        with (path / 'snapshot-errors.log').open('wb') as errors:
            session = subprocess.Popen(args, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=errors, text=True)
            def query(statement):
                session.stdin.write(statement + ';\n')
                session.stdin.flush()
                line = session.stdout.readline().strip()
                if not line:
                    raise RuntimeError('snapshot query failed')
                return line
            try:
                snapshot = query('BEGIN ISOLATION LEVEL REPEATABLE READ; SELECT pg_export_snapshot()')
                if not re.fullmatch(r'[0-9A-Fa-f]+-[0-9A-Fa-f]+-\d+', snapshot):
                    raise ValueError('invalid snapshot')
                tables = json.loads(query("SELECT jsonb_agg(ARRAY[schemaname,tablename] ORDER BY schemaname,tablename) FROM pg_tables WHERE schemaname IN('profiles','catalog','orders','applications','importer') OR (schemaname='auth' AND tablename='users') OR (schemaname='public' AND tablename='firstorder_schema_migrations')"))
                manifest['tables'] = tables
                manifest['row_counts'] = json.loads(query(inventory_query(tables)))
                with (path / 'postgres.dump').open('wb') as output:
                    subprocess.run(['docker', 'exec', DB, 'pg_dump', '-U', 'supabase_admin', '-d', 'postgres', '-Fc', '--snapshot=' + snapshot], stdout=output, stderr=errors, check=True, timeout=300)
            finally:
                session.stdin.close()
                session.wait(timeout=15)
                session.stdout.close()
        (path / 'globals.sql').write_bytes(execute(['docker', 'exec', DB, 'pg_dumpall', '-U', 'supabase_admin', '--globals-only']).stdout)
        roles = json.loads(sql(DB, "SELECT json_agg(rolname ORDER BY rolname) FROM pg_roles WHERE rolname NOT LIKE 'pg_%'"))
        private_json(path / 'roles.json', roles)
        with tarfile.open(path / 'config.tar.gz', 'w:gz') as archive:
            for relative in ['backend/.env', 'frontend/.env.local', 'supabase/.env', 'supabase/.smtp.local.env', 'supabase/docker-compose.yml']:
                file = root / relative
                if file.is_file() and not file.is_symlink():
                    archive.add(file, arcname=relative, recursive=False)
        with (path / 'postgres.dump').open('rb') as data:
            subprocess.run(['docker', 'exec', '-i', DB, 'pg_restore', '--list'], stdin=data, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, check=True, timeout=60)
        files = ['postgres.dump', 'globals.sql', 'config.tar.gz', 'roles.json']
        for name in files:
            (path / name).chmod(0o600)
        manifest.update(status='complete', files={name: {'sha256': sha256(path / name), 'bytes': (path / name).stat().st_size} for name in files})
        private_json(path / 'manifest.json', manifest)
        # Delete only this tool's complete backups, retaining the latest 14 and
        # every restored backup until a newer restored copy is available.
        existing = backups(root)
        restored = [p for p, m in existing if m.get('restored_at')]
        keep_restored = restored[-1] if restored else None
        for old, old_manifest in existing[:-14]:
            age = created - dt.datetime.fromisoformat(old_manifest['created_at'])
            if old != keep_restored and age > dt.timedelta(days=14):
                shutil.rmtree(old)
        return {'status': 'ok', 'operation': 'backup', 'backup': str(path), 'tables': len(tables)}


def restore_check(root, path):
    with lock(root, 'restore'):
        manifest = verified_manifest(path)
        if not re.fullmatch(r'sha256:[a-f0-9]{64}', manifest['image']):
            raise ValueError('invalid backup database image')
        name = 'firstorder-restore-' + uuid.uuid4().hex[:12]
        created = False
        try:
            execute(['docker', 'run', '--detach', '--name', name, '--network', 'none', '--label', 'firstorder.operation=restore-check', '--cpus', '1', '--memory', '2g', '--pids-limit', '256',
                     '--env', 'POSTGRES_PASSWORD=' + uuid.uuid4().hex, '--env', 'POSTGRES_USER=supabase_admin', manifest['image']])
            created = True
            for attempt in range(60):
                result = subprocess.run(['docker', 'exec', name, 'pg_isready', '-h', '127.0.0.1', '-U', 'supabase_admin', '-d', 'postgres'], capture_output=True, timeout=5)
                if result.returncode == 0:
                    break
                time.sleep(1)
            else:
                raise RuntimeError('isolated database not ready')
            # All SQL runs in a brand-new network-isolated container. No command
            # in this operation connects pg_restore to the production database.
            roles = json.loads((path / 'roles.json').read_text())
            for role in roles:
                identifier = '"' + role.replace('"', '""') + '"'
                literal = "'" + role.replace("'", "''") + "'"
                sql(name, 'DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=' + literal + ') THEN CREATE ROLE ' + identifier + ' NOLOGIN; END IF; END $$')
            sql(name, 'CREATE DATABASE firstorder_restore_test TEMPLATE template0')
            with (path / 'postgres.dump').open('rb') as data, (path / 'restore-errors.log').open('wb') as errors:
                subprocess.run(['docker', 'exec', '-i', name, 'pg_restore', '-U', 'supabase_admin', '-d', 'firstorder_restore_test', '--no-owner', '--no-privileges', '--exit-on-error'], stdin=data, stdout=subprocess.DEVNULL, stderr=errors, check=True, timeout=300)
            restored = json.loads(sql(name, inventory_query(manifest['tables']), 'firstorder_restore_test'))
            if restored != manifest['row_counts']:
                raise ValueError('restored row counts differ from exported snapshot')
            manifest['restored_at'] = dt.datetime.now(UTC).isoformat()
            manifest['restore_verified_tables'] = len(restored)
            private_json(path / 'manifest.json', manifest)
            return {'status': 'ok', 'operation': 'restore-check', 'tables': len(restored)}
        finally:
            if created:
                execute(['docker', 'rm', '-f', name])


def environment(root):
    file = root / 'backend/.env'
    if file.stat().st_mode & 0o077:
        raise ValueError('backend environment file permissions must be private')
    settings = {}
    for line in file.read_text().splitlines():
        if line.strip() and not line.lstrip().startswith('#') and '=' in line:
            key, value = line.split('=', 1)
            parts = shlex.split(value, comments=True)
            settings[key.strip()] = parts[0] if parts else ''
    return settings


def check(root):
    settings = environment(root)
    checks = {}
    def http(name, url, token=None, apikey=None):
        try:
            headers = {'User-Agent': 'FirstOrder-Operations/1.0'}
            if token:
                headers['Authorization'] = 'Bearer ' + token
            if apikey:
                headers['apikey'] = apikey
            with urllib.request.urlopen(urllib.request.Request(url, headers=headers), timeout=10) as response:
                checks[name] = response.status == 200
        except Exception:
            checks[name] = False
    http('public_health', 'https://e1.iq-home.kz/health')
    http('catalog', 'https://e1.iq-home.kz/api/v1/companies?limit=1')
    http('auth', settings.get('SUPABASE_URL', '') + '/auth/v1/health', apikey=settings.get('SUPABASE_ANON_KEY'))
    http('database_ready', 'http://127.0.0.1:18082/ready', settings.get('READY_TOKEN'))
    try:
        with socket.create_connection(('e1.iq-home.kz', 443), timeout=10) as connection:
            with ssl.create_default_context().wrap_socket(connection, server_hostname='e1.iq-home.kz') as tls:
                expiry = ssl.cert_time_to_seconds(tls.getpeercert()['notAfter'])
                checks['tls_certificate'] = expiry - time.time() > 7 * 86400
    except Exception:
        checks['tls_certificate'] = False
    checks['production_mode'] = settings.get('APP_ENV') == 'production'
    try:
        processes = json.loads(execute(['pm2', 'jlist']).stdout)
        checks['processes'] = all(any(p.get('name') == name and p.get('pm2_env', {}).get('status') == 'online' for p in processes) for name in ['ingeneriya-catalog', 'ingeneriya-frontend', 'pm2-logrotate'])
    except Exception:
        checks['processes'] = False
    usage = shutil.disk_usage(root)
    checks['disk_space'] = usage.free > 10 * 1024**3 and usage.free / usage.total > 0.10
    existing = backups(root)
    now = dt.datetime.now(UTC)
    checks['recent_backup'] = bool(existing) and now - dt.datetime.fromisoformat(existing[-1][1]['created_at']) < dt.timedelta(hours=30)
    checks['recent_restore_check'] = any(m.get('restored_at') and now - dt.datetime.fromisoformat(m['restored_at']) < dt.timedelta(days=8) for _, m in existing)
    failed_jobs = int(sql(DB, "SELECT count(*) FROM importer.collection_jobs WHERE status='failed' AND updated_at>now()-interval '24 hours'"))
    stale_jobs = int(sql(DB, "SELECT count(*) FROM importer.collection_jobs WHERE status='running' AND lease_until<now()-interval '5 minutes'"))
    checks['collection_worker'] = stale_jobs == 0
    result = {'status': 'ok' if all(checks.values()) else 'failed', 'operation': 'check', 'checked_at': now.isoformat(), 'checks': checks,
              'warnings': {'recent_failed_collection_jobs': failed_jobs}, 'external_alert_delivery': 'not_configured'}
    private_json(root / '.ops/status.json', result)
    return result


def main():
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('operation', choices=['backup', 'restore-check', 'check'])
    parser.add_argument('--root', type=Path, default=Path(__file__).resolve().parent.parent)
    parser.add_argument('--backup', type=Path)
    args = parser.parse_args()
    root = args.root.resolve()
    (root / '.ops').mkdir(mode=0o700, exist_ok=True)
    try:
        if args.operation == 'backup':
            result = backup(root)
        elif args.operation == 'restore-check':
            entries = backups(root)
            if args.backup is None and not entries:
                raise ValueError('no complete backup')
            result = restore_check(root, args.backup.resolve() if args.backup else entries[-1][0])
        else:
            with lock(root, 'check'):
                result = check(root)
    except Exception as error:
        # Never print subprocess arguments, database stderr or secret settings.
        result = {'status': 'failed', 'operation': args.operation, 'error_type': type(error).__name__}
    print(json.dumps(result))
    raise SystemExit(0 if result['status'] == 'ok' else 1)


if __name__ == '__main__':
    main()
