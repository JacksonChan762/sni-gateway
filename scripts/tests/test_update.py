"""Exercise update control flow with real local Git repos and a fake Docker CLI."""
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().parents[2] / 'sni-gateway.sh'
SINGBOX_SOURCE = Path(__file__).resolve().parents[2] / 'backend/internal/docker/singbox.go'
SINGBOX_IMAGE = re.search(r'const SingBoxImage = "([^"]+)"', SINGBOX_SOURCE.read_text()).group(1)


class UpdateTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.env = dict(os.environ, GIT_CONFIG_NOSYSTEM='1',
                        GIT_AUTHOR_NAME='Test', GIT_AUTHOR_EMAIL='test@example.com',
                        GIT_COMMITTER_NAME='Test', GIT_COMMITTER_EMAIL='test@example.com')
        self.remote = self.root / 'remote'
        self.repo = self.root / 'deployment'
        self.run_cmd('git', 'init', '--initial-branch=main', str(self.remote))
        shutil.copy(SCRIPT, self.remote / 'sni-gateway.sh')
        image_source = self.remote / 'backend/internal/docker/singbox.go'
        image_source.parent.mkdir(parents=True)
        shutil.copy(SINGBOX_SOURCE, image_source)
        (self.remote / '.gitignore').write_text('/backups/\n')
        self.run_cmd('git', '-C', str(self.remote), 'add', '.')
        self.run_cmd('git', '-C', str(self.remote), 'commit', '-m', 'initial')
        self.run_cmd('git', 'clone', str(self.remote), str(self.repo))
        binaries = self.root / 'bin'
        binaries.mkdir()
        fake = binaries / 'docker'
        fake.write_text('''#!/usr/bin/env bash
printf '%s\\n' "$*" >> "$DOCKER_LOG"
if [[ "$*" == *pg_dump* ]]; then
  [[ "${FAIL_STEP:-}" != backup ]] || exit 1
  echo database-backup
fi
if [[ "$*" == 'compose build --pull' && "${FAIL_STEP:-}" == build ]]; then exit 1; fi
if [[ "$*" == *'up -d'* && "${FAIL_STEP:-}" == health ]]; then exit 1; fi
''')
        fake.chmod(0o755)
        self.log = self.root / 'docker.log'
        self.env.update(PATH=str(binaries) + os.pathsep + os.environ['PATH'], DOCKER_LOG=str(self.log))

    def run_cmd(self, *args):
        return subprocess.run(args, env=self.env, text=True, capture_output=True, check=True)

    def update(self, mode='--apply'):
        return subprocess.run(['bash', 'sni-gateway.sh', 'update', mode], cwd=self.repo,
                              env=self.env, text=True, capture_output=True)

    def advance(self):
        (self.remote / 'new-file').write_text('new revision')
        self.run_cmd('git', '-C', str(self.remote), 'add', '.')
        self.run_cmd('git', '-C', str(self.remote), 'commit', '-m', 'update')

    def test_check_does_not_change_head_or_call_docker(self):
        self.advance()
        before = self.run_cmd('git', '-C', str(self.repo), 'rev-parse', 'HEAD').stdout
        result = self.update('--check')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('Update available', result.stdout)
        self.assertEqual(before, self.run_cmd('git', '-C', str(self.repo), 'rev-parse', 'HEAD').stdout)
        self.assertFalse(self.log.exists())

    def test_dirty_checkout_refused(self):
        (self.repo / 'local-work').write_text('keep me')
        self.assertNotEqual(self.update().returncode, 0)
        self.assertEqual((self.repo / 'local-work').read_text(), 'keep me')
        self.assertFalse(self.log.exists())

    def test_success_backup_and_idempotence(self):
        self.advance()
        result = self.update()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue((self.repo / 'new-file').exists())
        self.assertTrue((self.repo / '.git/sni-deployed-revision').exists())
        backups = list((self.repo / 'backups').glob('*/database.dump'))
        self.assertEqual(len(backups), 1)
        self.assertEqual(backups[0].read_text(), 'database-backup\n')
        self.assertEqual(backups[0].stat().st_mode & 0o777, 0o600)
        calls = self.log.read_text()
        self.assertLess(calls.index('pg_dump'), calls.index('compose build'))
        self.assertEqual(self.update().returncode, 0)
        self.assertEqual(self.log.read_text().count('compose build'), 1)

    def test_update_uses_image_from_new_revision(self):
        image_source = self.remote / 'backend/internal/docker/singbox.go'
        image_source.write_text(image_source.read_text().replace(
            SINGBOX_IMAGE, 'registry.example/sing-box:next'))
        self.run_cmd('git', '-C', str(self.remote), 'add', '.')
        self.run_cmd('git', '-C', str(self.remote), 'commit', '-m', 'update image')
        result = self.update()
        self.assertEqual(result.returncode, 0, result.stderr)
        calls = self.log.read_text()
        self.assertIn('image inspect registry.example/sing-box:next', calls)
        self.assertNotIn(f'image inspect {SINGBOX_IMAGE}', calls)

    def test_backup_failure_does_not_advance_source(self):
        self.advance()
        self.env['FAIL_STEP'] = 'backup'
        self.assertNotEqual(self.update().returncode, 0)
        self.assertFalse((self.repo / 'new-file').exists())
        self.assertNotIn('compose build', self.log.read_text())

    def test_failed_build_and_health_can_retry(self):
        for step in ('build', 'health'):
            with self.subTest(step=step):
                self.env['FAIL_STEP'] = step
                self.assertNotEqual(self.update().returncode, 0)
                self.assertFalse((self.repo / '.git/sni-deployed-revision').exists())
        del self.env['FAIL_STEP']
        self.assertEqual(self.update().returncode, 0)
        self.assertTrue((self.repo / '.git/sni-deployed-revision').exists())

    def test_local_commits_refused(self):
        self.run_cmd('git', '-C', str(self.repo), 'commit', '--allow-empty', '-m', 'local')
        self.assertNotEqual(self.update().returncode, 0)
        self.assertFalse(self.log.exists())


if __name__ == '__main__':
    unittest.main()
