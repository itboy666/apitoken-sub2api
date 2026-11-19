import json
from pathlib import Path
import subprocess
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


class UpstreamMergeTest(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name)
        root_patch = patch.object(sync_upstream, "ROOT", self.root)
        root_patch.start()
        self.addCleanup(root_patch.stop)
        sync_upstream.git("init", "-b", "apitoken")
        sync_upstream.git("config", "user.name", "Release test")
        sync_upstream.git("config", "user.email", "release-test@example.invalid")
        sync_upstream.git("config", "commit.gpgsign", "false")
        self.write(".github/workflows/release.yml", "official release\n")
        self.write(".github/workflows/check.yml", "official checks\n")
        self.write("backend/cmd/server/VERSION", "2.23.0\n")
        self.write("app.txt", "original app\n")
        self.commit("official base")
        sync_upstream.git("branch", "upstream")

    def write(self, name, content):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content)

    def commit(self, message):
        sync_upstream.git("add", ".")
        sync_upstream.git("commit", "-m", message)

    def workflow_tree(self, ref):
        return sync_upstream.git("ls-tree", "-r", ref, "--", ".github/workflows").stdout

    def test_clean_merge_preserves_workflows_and_merges_application_changes(self):
        self.write(".github/workflows/apitoken-release.yml", "distribution release\n")
        self.commit("custom release process")
        original = sync_upstream.git("rev-parse", "HEAD").stdout.strip()
        sync_upstream.git("checkout", "upstream")
        self.write(".github/workflows/release.yml", "changed official release\n")
        self.write(".github/workflows/new.yml", "new official workflow\n")
        (self.root / ".github/workflows/check.yml").unlink()
        self.write("app.txt", "updated official app\n")
        self.commit("next official version")
        sync_upstream.git("checkout", "apitoken")

        sync_upstream.merge_upstream("upstream", original)
        self.commit("combined release")

        self.assertEqual(self.workflow_tree("HEAD"), self.workflow_tree(original))
        self.assertFalse((self.root / ".github/workflows/new.yml").exists())
        self.assertEqual((self.root / "app.txt").read_text(), "updated official app\n")
        sync_upstream.git("merge-base", "--is-ancestor", "upstream", "HEAD")
        self.assertEqual(sync_upstream.git("status", "--porcelain").stdout, "")

    def test_workflow_and_version_conflicts_allow_the_combined_release(self):
        self.write(".github/workflows/release.yml", "guarded custom release\n")
        self.write("backend/cmd/server/VERSION", "2.23.0-apitoken.8\n")
        self.commit("custom version and guarded workflow")
        original = sync_upstream.git("rev-parse", "HEAD").stdout.strip()
        sync_upstream.git("checkout", "upstream")
        self.write(".github/workflows/release.yml", "changed official release\n")
        self.write("backend/cmd/server/VERSION", "2.24.0\n")
        self.write("app.txt", "updated official app\n")
        self.commit("next official version")
        sync_upstream.git("checkout", "apitoken")

        sync_upstream.merge_upstream("upstream", original)
        self.assertEqual(sync_upstream.git("diff", "--name-only", "--diff-filter=U").stdout.strip(), "backend/cmd/server/VERSION")
        self.write("backend/cmd/server/VERSION", "2.24.0-apitoken.1\n")
        self.commit("combined release")

        self.assertEqual(self.workflow_tree("HEAD"), self.workflow_tree(original))
        self.assertEqual((self.root / "app.txt").read_text(), "updated official app\n")
        sync_upstream.git("merge-base", "--is-ancestor", "upstream", "HEAD")

    def test_application_conflicts_stop_before_any_commit(self):
        self.write("app.txt", "custom app\n")
        self.commit("custom app")
        original = sync_upstream.git("rev-parse", "HEAD").stdout.strip()
        sync_upstream.git("checkout", "upstream")
        self.write("app.txt", "conflicting official app\n")
        self.commit("next official version")
        sync_upstream.git("checkout", "apitoken")

        with self.assertRaisesRegex(ValueError, "review conflicts.*app.txt"):
            sync_upstream.merge_upstream("upstream", original)

        self.assertEqual(sync_upstream.git("rev-parse", "HEAD").stdout.strip(), original)
        self.assertIn("app.txt", sync_upstream.git("diff", "--name-only", "--diff-filter=U").stdout)

    def test_fatal_merge_error_cannot_be_treated_as_a_resolved_merge(self):
        original = sync_upstream.git("rev-parse", "HEAD").stdout.strip()
        with self.assertRaisesRegex(ValueError, "Upstream merge stopped"):
            sync_upstream.merge_upstream("refs/heads/nonexistent", original)
        self.assertEqual(sync_upstream.git("rev-parse", "HEAD").stdout.strip(), original)
        self.assertEqual(sync_upstream.git("status", "--porcelain").stdout, "")

    def test_fatal_error_with_resolvable_paths_is_still_rejected(self):
        results = [
            subprocess.CompletedProcess([], 128, "", "fatal error"),
            subprocess.CompletedProcess([], 0, "backend/cmd/server/VERSION\0", ""),
        ]
        with patch.object(sync_upstream, "git", side_effect=results) as git:
            with self.assertRaisesRegex(ValueError, "Upstream merge stopped"):
                sync_upstream.merge_upstream("upstream", "original")
            self.assertEqual(git.call_count, 2)


if __name__ == "__main__":
    unittest.main()
