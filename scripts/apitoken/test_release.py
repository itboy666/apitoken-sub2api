import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parent))
import sync_upstream
import release


class ReleaseTest(unittest.TestCase):
    def test_revision_progresses_numerically_and_resets_for_new_official_base(self):
        self.assertEqual(sync_upstream.choose_version("v2.23.0", "2.23.0-apitoken.8", []), "2.23.0-apitoken.8")
        self.assertEqual(sync_upstream.choose_version("v2.23.0", "2.23.0-apitoken.8", ["v2.23.0-apitoken.9", "v2.23.0-apitoken.10"]), "2.23.0-apitoken.11")
        self.assertEqual(sync_upstream.choose_version("v2.24.0", "2.23.0-apitoken.10", ["v2.23.0-apitoken.10"]), "2.24.0-apitoken.1")

    def test_does_not_rewrite_applied_payment_migrations(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / "backend/migrations").mkdir(parents=True)
            migration = root / "backend/migrations/389_example.sql"
            migration.write_text("original migration")
            metadata = {"protected_migrations": {migration.name: release.sha256(migration)}}
            with patch.object(sync_upstream, "ROOT", root):
                sync_upstream.assert_protected_migrations(metadata)
                migration.write_text("changed migration")
                with self.assertRaisesRegex(ValueError, "must not be rewritten"):
                    sync_upstream.assert_protected_migrations(metadata)

    def test_non_stable_tag_is_rejected_before_fetching(self):
        for tag in ["production", "v2.24.0-rc1", "v2.23.0-apitoken.8", "../v2.23.0", "v2.23.0\nextra"]:
            with self.assertRaises(ValueError):
                sync_upstream.stable_release(tag)

    def test_prerelease_cannot_be_mistaken_for_stable(self):
        from io import BytesIO
        with patch.object(sync_upstream, "urlopen", return_value=BytesIO(json.dumps({"tag_name": "v2.24.0", "prerelease": True}).encode())):
            with self.assertRaisesRegex(ValueError, "published stable"):
                sync_upstream.stable_release()


if __name__ == "__main__":
    unittest.main()
