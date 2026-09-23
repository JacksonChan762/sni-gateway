"""Test the installer without starting real containers."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


INSTALLER = Path(__file__).resolve().parents[2] / 'sni-gateway.sh'


class InstallTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.repo = self.root / 'deployment'
        self.repo.mkdir()
        shutil.copy(INSTALLER, self.repo / 'sni-gateway.sh')
        binaries = self.root / 'bin'
        binaries.mkdir()
        fake_docker = binaries / 'docker'
        fake_docker.write_text('''#!/usr/bin/env bash
printf '%s\\n' "$*" >> "$DOCKER_LOG"
if [[ "$*" == 'compose up -d --build --wait --wait-timeout 180' && "${FAIL_UP:-}" == 1 ]]; then exit 1; fi
''')
        fake_docker.chmod(0o755)
        self.log = self.root / 'docker.log'
        self.env = dict(os.environ, PATH=str(binaries) + os.pathsep + os.environ['PATH'],
                        DOCKER_LOG=str(self.log))

    def install(self, *args):
        return subprocess.run(['bash', 'sni-gateway.sh', 'install', *args], cwd=self.repo,
                              env=self.env, text=True, capture_output=True)

    def test_first_install_creates_private_credentials_and_waits(self):
        result = self.install()
        self.assertEqual(result.returncode, 0, result.stderr)
        env_file = self.repo / '.env'
        content = env_file.read_text()
        self.assertEqual(env_file.stat().st_mode & 0o777, 0o600)
        self.assertRegex(content, r'JWT_SECRET=[0-9a-f]{64}\n')
        self.assertRegex(content, r'ADMIN_PASSWORD=[0-9a-f]{48}\n')
        self.assertNotIn(content.split('ADMIN_PASSWORD=')[1].splitlines()[0], result.stdout)
        self.assertEqual(self.log.read_text().splitlines(), [
            'compose version', 'compose config --quiet',
            'compose up -d --build --wait --wait-timeout 180',
        ])

    def test_existing_credentials_are_preserved(self):
        content = 'JWT_SECRET=' + 'x' * 40 + '\nADMIN_PASSWORD=' + 'y' * 20 + '\n'
        (self.repo / '.env').write_text(content)
        self.assertEqual(self.install().returncode, 0)
        self.assertEqual((self.repo / '.env').read_text(), content)

    def test_example_credentials_are_rejected(self):
        (self.repo / '.env').write_text('JWT_SECRET=replace-with-a-long-random-secret\nADMIN_PASSWORD=admin\n')
        result = self.install()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('JWT_SECRET', result.stderr)
        self.assertNotIn('compose up', self.log.read_text())

    def test_start_failure_keeps_credentials_for_retry(self):
        self.env['FAIL_UP'] = '1'
        result = self.install()
        self.assertNotEqual(result.returncode, 0)
        content = (self.repo / '.env').read_text()
        self.assertIn('Installation did not complete', result.stderr)
        del self.env['FAIL_UP']
        self.assertEqual(self.install().returncode, 0)
        self.assertEqual((self.repo / '.env').read_text(), content)


if __name__ == '__main__':
    unittest.main()
