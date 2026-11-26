import json
from pathlib import Path
import tempfile
import unittest
import ops


class BackupManifestTests(unittest.TestCase):
    def test_tampered_or_incomplete_backup_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            names = ['postgres.dump', 'globals.sql', 'config.tar.gz', 'roles.json']
            for name in names:
                (root / name).write_bytes(b'backup fixture')
            manifest = {'kind': ops.KIND, 'status': 'complete', 'files': {
                name: {'bytes': (root / name).stat().st_size, 'sha256': ops.sha256(root / name)} for name in names}}
            ops.private_json(root / 'manifest.json', manifest)
            self.assertEqual(ops.verified_manifest(root)['status'], 'complete')
            (root / 'postgres.dump').write_bytes(b'changed backup')
            with self.assertRaises(ValueError):
                ops.verified_manifest(root)
            manifest['status'] = 'creating'
            ops.private_json(root / 'manifest.json', manifest)
            with self.assertRaises(ValueError):
                ops.verified_manifest(root)

    def test_inventory_quotes_identifiers_and_private_metadata(self):
        query = ops.inventory_query([['profiles', 'unsafe"table']])
        self.assertIn('"unsafe""table"', query)
        with tempfile.TemporaryDirectory() as directory:
            file = Path(directory) / 'status.json'
            ops.private_json(file, {'status': 'ok'})
            self.assertEqual(file.stat().st_mode & 0o077, 0)
            self.assertEqual(json.loads(file.read_text())['status'], 'ok')

    def test_backup_discovery_ignores_unrelated_and_symlinked_directories(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            folder = root / '.backups'
            folder.mkdir()
            unrelated = folder / 'important-other-backup'
            unrelated.mkdir()
            (folder / 'backup-20260101T000000Z-01234567').symlink_to(unrelated, target_is_directory=True)
            self.assertEqual(ops.backups(root), [])
            self.assertTrue(unrelated.exists())
