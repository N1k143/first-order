#!/usr/bin/env python3
"""Durable, bounded local 2GIS research batches. No production DB or publication."""
import argparse
import contextlib
import csv
import datetime as dt
import fcntl
import json
import os
from pathlib import Path
import re
import sqlite3
import time
import urllib.parse
import uuid

import dgis_pages as pages

APP_ID = 0x464F3247
STATES = {'unreviewed', 'keep', 'dismiss', 'has_site'}


def now():
    return time.time()


def valid_record(record):
    if not isinstance(record, dict):
        raise ValueError('invalid_candidate_record')
    identity = record.get('external_id', '')
    if not isinstance(identity, str) or not re.fullmatch(r'[0-9]{8,24}', identity) or record.get('source_url') != pages.BASE + '/astana/firm/' + identity:
        raise ValueError('invalid_candidate_identity')
    if record.get('source') != '2gis_public_html' or record.get('city') != 'Астана':
        raise ValueError('invalid_candidate_source')
    if record.get('website_listing') not in {'listed', 'not_listed', 'unknown'} or record.get('website_status') != 'needs_review':
        raise ValueError('invalid_candidate_status')
    if not isinstance(record.get('card_recognized'), bool) or not isinstance(record.get('html_complete'), bool):
        raise ValueError('invalid_card_evidence')
    for field in ['name', 'address', 'category']:
        if record.get(field) is not None and not isinstance(record[field], str):
            raise ValueError('invalid_candidate_text')
    if record.get('card_recognized') and (not isinstance(record.get('name'), str) or not record['name'].strip() or len(record['name']) > 500):
        raise ValueError('invalid_candidate_name')
    if record['website_listing'] == 'not_listed' and (not record.get('card_recognized') or not record.get('html_complete')):
        raise ValueError('unproven_missing_website')
    for key in ['phones', 'emails', 'platform_links', 'website_links']:
        if not isinstance(record.get(key), list) or len(record[key]) > 100 or any(not isinstance(v, str) or len(v) > 8192 for v in record[key]):
            raise ValueError('invalid_candidate_contacts')
    try:
        checked = dt.datetime.fromisoformat(record['checked_at'])
        if checked.tzinfo is None or checked.timestamp() > now() + 300:
            raise ValueError()
    except (ValueError, KeyError, TypeError):
        raise ValueError('invalid_candidate_date') from None
    if record['website_listing'] == 'listed' and not record['website_links']:
        raise ValueError('listed_candidate_requires_website_link')
    if record['website_listing'] == 'not_listed' and record['website_links']:
        raise ValueError('conflicting_candidate_links')
    return record


class Store:
    def __init__(self, path):
        self.path = Path(path)
        self.path.parent.mkdir(parents=True, mode=0o700, exist_ok=True)
        if self.path.is_symlink():
            raise ValueError('state_symlink_not_allowed')
        fresh = not self.path.exists()
        if fresh:
            os.close(os.open(self.path, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600))
        self.db = sqlite3.connect(self.path, timeout=5)
        self.db.row_factory = sqlite3.Row
        identity = self.db.execute('PRAGMA application_id').fetchone()[0]
        if not fresh and identity != APP_ID:
            self.db.close()
            raise ValueError('not_a_2gis_state_database')
        if self.db.execute('PRAGMA user_version').fetchone()[0] > 1:
            self.db.close()
            raise ValueError('state_database_version_too_new')
        self.path.chmod(0o600)
        self.db.executescript(f'''
          PRAGMA application_id={APP_ID};
          PRAGMA user_version=1;
          PRAGMA journal_mode=WAL;
          PRAGMA foreign_keys=ON;
          CREATE TABLE IF NOT EXISTS jobs(id TEXT PRIMARY KEY,data TEXT NOT NULL);
          CREATE TABLE IF NOT EXISTS candidates(id TEXT PRIMARY KEY,data TEXT NOT NULL,
            review_state TEXT NOT NULL DEFAULT 'unreviewed',notes TEXT NOT NULL DEFAULT '',known_website TEXT,
            updated_at REAL NOT NULL);
          CREATE TABLE IF NOT EXISTS events(id INTEGER PRIMARY KEY,job_id TEXT REFERENCES jobs(id),
            candidate_id TEXT,event TEXT NOT NULL,created_at REAL NOT NULL,operator_uid INTEGER NOT NULL);
          CREATE TABLE IF NOT EXISTS control(id INTEGER PRIMARY KEY CHECK(id=1),blocked_until REAL NOT NULL DEFAULT 0);
          INSERT OR IGNORE INTO control(id) VALUES(1);
        ''')

    def close(self):
        self.db.close()

    @contextlib.contextmanager
    def runner(self):
        descriptor = os.open(str(self.path) + '.runner.lock', os.O_CREAT | os.O_RDWR, 0o600)
        with os.fdopen(descriptor, 'a') as lock:
            try:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            except BlockingIOError:
                raise ValueError('another_batch_is_running') from None
            yield

    def event(self, event, job=None, candidate=None):
        self.db.execute('INSERT INTO events(job_id,candidate_id,event,created_at,operator_uid) VALUES(?,?,?,?,?)',
                        (job, candidate, event, now(), os.getuid()))

    def save_job(self, job, event, candidate=None):
        job['updated_at'] = now()
        self.db.execute('INSERT INTO jobs(id,data) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data',
                        (job['id'], json.dumps(job, ensure_ascii=False)))
        self.event(event, job['id'], candidate)

    def create(self, queries, limit):
        queries = list(dict.fromkeys(pages.clean(q) for q in queries))
        if not 1 <= len(queries) <= 5 or any(not q or len(q) > 100 or '/' in q for q in queries) or not 1 <= limit <= 10:
            raise ValueError('use_1_to_5_queries_and_limit_1_to_10')
        job = {'id': str(uuid.uuid4()), 'queries': queries, 'limit': limit, 'searches': {}, 'queue': [], 'position': 0,
               'status': 'pending', 'attempts': 0, 'checked': 0, 'cached': 0, 'error': None, 'created_at': now()}
        with self.db:
            self.save_job(job, 'created')
        return job

    def job(self, identity):
        row = self.db.execute('SELECT data FROM jobs WHERE id=?', (identity,)).fetchone()
        if not row:
            raise ValueError('job_not_found')
        return json.loads(row['data'])

    def upsert(self, record):
        valid_record(record)
        previous = self.db.execute('SELECT data FROM candidates WHERE id=?', (record['external_id'],)).fetchone()
        if previous:
            old = json.loads(previous['data'])
            queries = sorted(set(old.get('discovery_queries', [])) | set(record.get('discovery_queries', [])))
            if dt.datetime.fromisoformat(record['checked_at']) < dt.datetime.fromisoformat(old['checked_at']):
                record = old
            record = dict(record, discovery_queries=queries)
        self.db.execute('INSERT INTO candidates(id,data,updated_at) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data,updated_at=excluded.updated_at',
                        (record['external_id'], json.dumps(record, ensure_ascii=False), now()))

    def run(self, identity, factory=pages.PublicPages):
        with self.runner():
            job = self.job(identity)
            if job['status'] == 'completed':
                return job
            blocked = self.db.execute('SELECT blocked_until FROM control WHERE id=1').fetchone()[0]
            if blocked > now():
                raise ValueError('source_cooldown_until_' + dt.datetime.fromtimestamp(blocked, dt.timezone.utc).isoformat())
            if job['attempts'] >= 3:
                raise ValueError('attempt_limit_reached')
            job.update(status='running', attempts=job['attempts'] + 1, error=None)
            with self.db:
                self.save_job(job, 'started')
            try:
                client = factory()
                for query in job['queries']:
                    if query in job['searches']:
                        continue
                    url = pages.BASE + '/astana/search/' + urllib.parse.quote(query, safe='')
                    raw = client.get(url)
                    document = pages.Document(raw)
                    links = pages.search_links(raw)[:10]
                    if not document.complete or not links:
                        raise pages.FetchError('incomplete_search_or_challenge')
                    job['searches'][query] = links
                    with self.db:
                        self.save_job(job, 'search_completed')
                if not job['queue']:
                    seen, queue = {}, []
                    # Round-robin queries; one category must not consume the batch.
                    for index in range(10):
                        for query in job['queries']:
                            links = job['searches'][query]
                            if index >= len(links):
                                continue
                            link = links[index]
                            identity = link['url'].rsplit('/', 1)[-1]
                            if identity not in seen:
                                item = {**link, 'id': identity, 'queries': []}
                                queue.append(item)
                                seen[identity] = item
                            seen[identity]['queries'].append(query)
                    job['queue'] = queue[:job['limit']]
                    with self.db:
                        self.save_job(job, 'queue_prepared')
                while job['position'] < len(job['queue']):
                    item = job['queue'][job['position']]
                    row = self.db.execute('SELECT * FROM candidates WHERE id=?', (item['id'],)).fetchone()
                    cached = json.loads(row['data']) if row else None
                    fresh = cached and now() - dt.datetime.fromisoformat(cached['checked_at']).timestamp() < 86400
                    use_cache = bool(cached and cached.get('card_recognized') and (fresh or row['review_state'] in {'dismiss', 'has_site'}))
                    if use_cache:
                        record = cached
                    else:
                        record = pages.parse_firm(client.get(item['url']), item['url'])
                        if not record['card_recognized'] or not record['html_complete']:
                            raise pages.FetchError('incomplete_card_or_challenge')
                    record = dict(record)
                    record['discovery_queries'] = sorted(set(record.get('discovery_queries', [])) | set(item['queries']))
                    valid_record(record)
                    item.update(cache_hit=use_cache, result=record)
                    job['position'] += 1
                    job['cached' if use_cache else 'checked'] += 1
                    # Candidate refresh, evidence and progress are one checkpoint;
                    # reviewer notes/state are never part of the source update.
                    with self.db:
                        self.upsert(record)
                        self.save_job(job, 'candidate_cached' if use_cache else 'candidate_checked', item['id'])
                job['status'] = 'completed'
                with self.db:
                    self.save_job(job, 'completed')
            except (pages.FetchError, ValueError, OSError, KeyboardInterrupt) as error:
                code = 'interrupted' if isinstance(error, KeyboardInterrupt) else str(error) if isinstance(error, (pages.FetchError, ValueError)) else 'source_unavailable'
                job.update(status='paused' if code == 'interrupted' else 'failed', error=code)
                with self.db:
                    if code != 'interrupted':
                        seconds = 86400 if 'robots' in code else 3600 if any(s in code for s in ['403', '429', 'challenge', 'redirect']) else 300
                        self.db.execute('UPDATE control SET blocked_until=? WHERE id=1', (now() + seconds,))
                    self.save_job(job, job['status'])
            return job

    def review(self, identity, decision, notes='', website=None):
        if decision not in STATES or len(notes) > 2000:
            raise ValueError('invalid_review')
        if decision == 'has_site':
            target, kind = pages.external_link(website or '', identity)
            if kind != 'website':
                raise ValueError('own_website_url_required')
            website = target
        elif website:
            raise ValueError('website_only_for_has_site')
        with self.db:
            result = self.db.execute('UPDATE candidates SET review_state=?,notes=?,known_website=? WHERE id=?', (decision, notes, website, identity))
            if result.rowcount != 1:
                raise ValueError('candidate_not_found')
            self.event('review_' + decision, candidate=identity)

    def records(self, identity=None):
        rows = self.db.execute('SELECT * FROM candidates ORDER BY id').fetchall()
        selected = None
        if identity:
            selected = {item['id'] for item in self.job(identity)['queue'][:self.job(identity)['position']]}
        result = []
        for row in rows:
            if selected is not None and row['id'] not in selected:
                continue
            data = json.loads(row['data'])
            data.update(review_state=row['review_state'], notes=row['notes'], known_website=row['known_website'])
            data['priority_score'] = score(data)
            result.append(data)
        groups = {}
        for data in result:
            for key in duplicate_keys(data):
                groups.setdefault(key, set()).add(data['external_id'])
        for data in result:
            matches = set()
            for key in duplicate_keys(data):
                matches.update(groups[key])
            data['possible_duplicate_ids'] = sorted(matches - {data['external_id']})[:10]
        return sorted(result, key=lambda r: (-r['priority_score'], (r.get('name') or '').casefold(), r['external_id']))


def score(record):
    if record['website_listing'] != 'not_listed' or record.get('review_state') in {'dismiss', 'has_site'}:
        return 0
    return 50 + 20 * bool(record['phones']) + 15 * bool(record['platform_links']) + 10 * bool(record['emails']) + 5 * bool(record['address'])


def duplicate_keys(record):
    address = pages.clean(record.get('address') or '').casefold()
    if not address:
        return []
    keys = []
    name = pages.clean(record.get('name') or '').casefold()
    if name:
        keys.append(('name_address', address, name))
    for phone in record['phones']:
        digits = re.sub(r'\D', '', phone)
        if len(digits) == 11 and digits.startswith('8'):
            digits = '7' + digits[1:]
        if digits:
            keys.append(('phone_address', address, digits))
    return keys


def possible_duplicate(first, second):
    return bool(set(duplicate_keys(first)) & set(duplicate_keys(second)))


def csv_safe(value):
    text = '' if value is None else str(value)
    spreadsheet_number = text.isascii() and text.isdigit() and len(text) > 15
    return "'" + text if spreadsheet_number or text.lstrip().startswith(('=', '+', '-', '@', '\t', '\r')) else text


def export(store, path, kind, format, identity=None):
    records = store.records(identity)
    if kind == 'prospects':
        records = [r for r in records if score(r) > 0]
    elif kind == 'unknown':
        records = [r for r in records if r['website_listing'] == 'unknown']
    descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(descriptor, 'w', encoding='utf-8-sig' if format == 'csv' else 'utf-8', newline='') as handle:
        if format == 'json':
            json.dump({'source': '2gis_public_html', 'city': 'Астана', 'kind': kind, 'candidates': records,
                       'notice': 'No website listed is not proof of no website. Manual review required.'}, handle, ensure_ascii=False, indent=2)
        else:
            columns = ['external_id', 'name', 'address', 'phones', 'emails', 'platform_links', 'website_listing', 'priority_score', 'review_state', 'notes', 'known_website', 'source_url', 'discovery_queries', 'checked_at', 'possible_duplicate_ids']
            writer = csv.DictWriter(handle, fieldnames=columns)
            writer.writeheader()
            for record in records:
                writer.writerow({key: csv_safe('; '.join(record[key]) if isinstance(record.get(key), list) else record.get(key)) for key in columns})
    return len(records)


def summary(job):
    result = {key: job[key] for key in ['id', 'status', 'queries', 'limit', 'position', 'checked', 'cached', 'attempts', 'error']}
    result.update(processed=job['position'], total=len(job['queue']), searches_completed=len(job['searches']))
    return result


def main():
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--state', type=Path, default=Path(__file__).resolve().parents[2] / '.ops/2gis.sqlite')
    commands = parser.add_subparsers(dest='command', required=True)
    start = commands.add_parser('start')
    start.add_argument('--query', action='append', required=True)
    start.add_argument('--limit', type=int, default=6)
    for name in ['resume', 'status', 'history']:
        commands.add_parser(name).add_argument('--job', required=True)
    output = commands.add_parser('export')
    output.add_argument('--job')
    output.add_argument('--kind', choices=['prospects', 'all', 'unknown'], default='prospects')
    output.add_argument('--format', choices=['csv', 'json'], default='csv')
    output.add_argument('--output', type=Path, required=True)
    review = commands.add_parser('review')
    review.add_argument('--id', required=True)
    review.add_argument('--decision', choices=sorted(STATES), required=True)
    review.add_argument('--notes', default='')
    review.add_argument('--website')
    seed = commands.add_parser('import-report')
    seed.add_argument('--input', type=Path, required=True)
    args = parser.parse_args()
    store = None
    try:
        store = Store(args.state)
        if args.command == 'start':
            result = summary(store.run(store.create(args.query, args.limit)['id']))
        elif args.command == 'resume':
            result = summary(store.run(args.job))
        elif args.command == 'status':
            result = summary(store.job(args.job))
        elif args.command == 'history':
            store.job(args.job)
            result = {'job': args.job, 'events': [dict(r) for r in store.db.execute('SELECT event,candidate_id,created_at,operator_uid FROM events WHERE job_id=? ORDER BY id', (args.job,))]}
        elif args.command == 'review':
            store.review(args.id, args.decision, args.notes, args.website)
            result = {'reviewed': args.id, 'decision': args.decision}
        elif args.command == 'import-report':
            if args.input.stat().st_size > pages.MAX_BYTES:
                raise ValueError('report_too_large')
            report = json.loads(args.input.read_text())
            records = report['candidates']
            if report.get('source') != '2gis_public_html' or len(records) > 100:
                raise ValueError('invalid_report')
            with store.db:
                for record in records:
                    store.upsert(record)
                    store.event('report_imported', candidate=record['external_id'])
            result = {'imported': len(records)}
        else:
            result = {'exported': export(store, args.output, args.kind, args.format, args.job), 'output': str(args.output)}
        print(json.dumps(result, ensure_ascii=False))
        return 1 if result.get('status') == 'failed' else 0
    except (ValueError, OSError, sqlite3.Error, KeyError) as error:
        print(json.dumps({'error': str(error) if isinstance(error, ValueError) else type(error).__name__}))
        return 1
    finally:
        if store:
            store.close()


if __name__ == '__main__':
    raise SystemExit(main())
