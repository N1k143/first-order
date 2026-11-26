import csv
import datetime as dt
import json
from pathlib import Path
import sqlite3
import tempfile
import unittest
from unittest import mock

import dgis_batch as batch
import dgis_pages as pages
import test_dgis_pages as fixture

A, B, C = ['1000000000000000' + str(i) for i in range(1, 4)]


def firm(identity, website=False):
    contacts = '<a href="https://business.example.com">Сайт</a>' if website else '<a href="https://instagram.com/example">Instagram</a>'
    return fixture.page(contacts).replace(fixture.ID, identity).replace('Пример кафе', 'Компания ' + identity[-1])


def search(*identities):
    return '<html><body>' + ''.join('<a href="/astana/firm/' + i + '">Компания ' + i[-1] + '</a>' for i in identities) + '</body></html>'


class FakePages:
    def __init__(self, searches, fail=None):
        self.searches, self.fail, self.calls = searches, fail, []

    def get(self, url):
        self.calls.append(url)
        if '/search/' in url:
            return self.searches[pages.urllib.parse.unquote(url.rsplit('/', 1)[-1])]
        identity = url.rsplit('/', 1)[-1]
        if identity == self.fail:
            raise pages.FetchError('http_503')
        return firm(identity, website=identity == C)


class BatchTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        self.store = batch.Store(self.root / 'state.sqlite')

    def tearDown(self):
        self.store.close()
        self.temp.cleanup()

    def test_balanced_queries_deduplicate_identity_not_branches(self):
        fake = FakePages({'Кафе': search(A, C), 'Салоны': search(B, A)})
        job = self.store.create(['Кафе', 'Салоны'], 3)
        done = self.store.run(job['id'], lambda: fake)
        self.assertEqual(done['status'], 'completed')
        self.assertEqual([item['id'] for item in done['queue']], [A, B, C])
        self.assertEqual(done['checked'], 3)
        self.assertEqual(len(fake.calls), 5)
        records = self.store.records()
        self.assertEqual(len(records), 3)
        first = next(r for r in records if r['external_id'] == A)
        self.assertEqual(first['discovery_queries'], ['Кафе', 'Салоны'])
        self.assertEqual(batch.export(self.store, self.root / 'prospects.json', 'prospects', 'json'), 2)
        # The factory must not be invoked on an already-completed job.
        self.assertEqual(self.store.run(job['id'], lambda: self.fail('completed job performed a request'))['checked'], 3)

    def test_resume_uses_committed_progress_and_retains_searches(self):
        fake = FakePages({'Кафе': search(A, B, C)}, fail=B)
        job = self.store.create(['Кафе'], 3)
        failed = self.store.run(job['id'], lambda: fake)
        self.assertEqual(failed['status'], 'failed')
        self.assertEqual(failed['position'], 1)
        self.assertEqual(len(self.store.records()), 1)
        with self.assertRaisesRegex(ValueError, 'source_cooldown'):
            self.store.run(job['id'], lambda: self.fail('cooldown performed a request'))
        retry = FakePages({})
        with mock.patch('dgis_batch.now', return_value=batch.now() + 301):
            done = self.store.run(job['id'], lambda: retry)
        self.assertEqual(done['status'], 'completed')
        self.assertEqual(done['attempts'], 2)
        self.assertEqual([url.rsplit('/', 1)[-1] for url in retry.calls], [B, C])
        self.assertEqual(len(self.store.records()), 3)

    def test_cooldown_applies_to_new_jobs_and_survives_reopen(self):
        failed = self.store.run(self.store.create(['Кафе'], 1)['id'], lambda: FakePages({'Кафе': search(A)}, fail=A))
        self.assertEqual(failed['status'], 'failed')
        self.store.close()
        self.store = batch.Store(self.root / 'state.sqlite')
        second = self.store.create(['Магазины'], 1)
        with self.assertRaisesRegex(ValueError, 'source_cooldown'):
            self.store.run(second['id'], lambda: self.fail('new job bypassed source cooldown'))

    def test_cache_avoids_refetch_and_notes_survive_refresh(self):
        self.store.run(self.store.create(['Кафе'], 1)['id'], lambda: FakePages({'Кафе': search(A)}))
        self.store.review(A, 'keep', 'Проверить компанию вручную')
        fake = FakePages({'Магазины': search(A)})
        done = self.store.run(self.store.create(['Магазины'], 1)['id'], lambda: fake)
        self.assertEqual(done['cached'], 1)
        self.assertEqual(len(fake.calls), 1)
        newer = pages.parse_firm(firm(A, website=True), pages.BASE + '/astana/firm/' + A)
        with self.store.db:
            self.store.upsert(newer)
        record = self.store.records()[0]
        self.assertEqual(record['review_state'], 'keep')
        self.assertEqual(record['notes'], 'Проверить компанию вручную')
        self.assertEqual(record['website_listing'], 'listed')
        self.assertEqual(batch.export(self.store, self.root / 'empty.csv', 'prospects', 'csv'), 0)

    def test_manual_known_site_excluded_and_stale_import_cannot_regress(self):
        current = pages.parse_firm(firm(A, website=True), pages.BASE + '/astana/firm/' + A)
        with self.store.db:
            self.store.upsert(current)
        old = pages.parse_firm(firm(A), pages.BASE + '/astana/firm/' + A)
        old['checked_at'] = (dt.datetime.now(dt.timezone.utc) - dt.timedelta(days=2)).isoformat()
        with self.store.db:
            self.store.upsert(old)
        self.assertEqual(self.store.records()[0]['website_listing'], 'listed')
        self.store.review(A, 'has_site', 'Сайт подтверждён оператором', 'https://business.example.com')
        self.assertEqual(batch.export(self.store, self.root / 'none.json', 'prospects', 'json'), 0)
        with self.assertRaises(ValueError):
            self.store.review(A, 'has_site', website='http://127.0.0.1/private')

    def test_atomic_candidate_checkpoint_on_failure(self):
        job = self.store.create(['Кафе'], 1)
        trigger = "CREATE TRIGGER fail_candidate BEFORE INSERT ON candidates BEGIN SELECT RAISE(ABORT,'test failure'); END"
        self.store.db.execute(trigger)
        with self.assertRaises(sqlite3.Error):
            self.store.run(job['id'], lambda: FakePages({'Кафе': search(A)}))
        self.assertEqual(self.store.job(job['id'])['position'], 0)
        self.assertEqual(len(self.store.records()), 0)
        self.store.db.execute('DROP TRIGGER fail_candidate')
        self.assertEqual(self.store.run(job['id'], lambda: FakePages({}))['status'], 'completed')

    def test_runner_lock_prevents_duplicate_workers(self):
        job = self.store.create(['Кафе'], 1)
        with self.store.runner():
            with self.assertRaisesRegex(ValueError, 'another_batch_is_running'):
                self.store.run(job['id'], lambda: self.fail('second worker performed a request'))

    def test_csv_formula_safety_and_unknown_exclusion(self):
        record = pages.parse_firm(firm(A), pages.BASE + '/astana/firm/' + A)
        record['name'] = '=HYPERLINK("https://evil.example")'
        with self.store.db:
            self.store.upsert(record)
        self.store.review(A, 'keep', '@formula')
        path = self.root / 'safe.csv'
        self.assertEqual(batch.export(self.store, path, 'prospects', 'csv'), 1)
        with path.open(encoding='utf-8-sig') as handle:
            row = next(csv.DictReader(handle))
        self.assertEqual(row['external_id'], "'" + A)
        self.assertTrue(row['name'].startswith("'="))
        self.assertTrue(row['phones'].startswith("'+"))
        self.assertTrue(row['notes'].startswith("'@"))
        self.assertEqual(path.stat().st_mode & 0o077, 0)
        with self.assertRaises(FileExistsError):
            batch.export(self.store, path, 'all', 'json')
        record.update(website_listing='unknown', reason='unresolved')
        record['checked_at'] = dt.datetime.now(dt.timezone.utc).isoformat()
        with self.store.db:
            self.store.upsert(record)
        self.assertEqual(batch.export(self.store, self.root / 'unknown.json', 'unknown', 'json'), 1)
        self.assertEqual(batch.export(self.store, self.root / 'no-prospects.json', 'prospects', 'json'), 0)

    def test_refuses_unrelated_state_file_and_invalid_missing_evidence(self):
        path = self.root / 'unrelated.sqlite'
        connection = sqlite3.connect(path)
        connection.execute('CREATE TABLE important_data(id INTEGER)')
        connection.close()
        with self.assertRaisesRegex(ValueError, 'not_a_2gis'):
            batch.Store(path)
        record = pages.parse_firm(firm(A), pages.BASE + '/astana/firm/' + A)
        record['html_complete'] = 'false'
        with self.assertRaises(ValueError):
            self.store.upsert(record)

    def test_possible_duplicate_is_a_hint_and_never_merges_distinct_ids(self):
        first = pages.parse_firm(firm(A), pages.BASE + '/astana/firm/' + A)
        second = dict(first, external_id=B, source_url=pages.BASE + '/astana/firm/' + B)
        with self.store.db:
            self.store.upsert(first)
            self.store.upsert(second)
        records = self.store.records()
        self.assertEqual(len(records), 2)
        self.assertEqual(records[0]['possible_duplicate_ids'], [B])
        second['address'] = 'Другой адрес, 12'
        self.assertFalse(batch.possible_duplicate(first, second))

    def test_interruption_keeps_progress_and_can_resume_without_refetch(self):
        fake = FakePages({'Кафе': search(A, B)})
        original = fake.get
        def interrupt(url):
            if url.endswith('/' + B):
                raise KeyboardInterrupt()
            return original(url)
        fake.get = interrupt
        job = self.store.create(['Кафе'], 2)
        paused = self.store.run(job['id'], lambda: fake)
        self.assertEqual(paused['status'], 'paused')
        self.assertEqual(paused['position'], 1)
        retry = FakePages({})
        done = self.store.run(job['id'], lambda: retry)
        self.assertEqual(done['status'], 'completed')
        self.assertEqual(retry.calls, [pages.BASE + '/astana/firm/' + B])
        events = self.store.db.execute("SELECT candidate_id FROM events WHERE job_id=? AND event='candidate_checked' ORDER BY id", (job['id'],)).fetchall()
        self.assertEqual([row['candidate_id'] for row in events], [A, B])
