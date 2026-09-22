import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]

class StopTests(unittest.TestCase):
    def run_stop(self, fail=False):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            docker = root / 'docker'
            docker.write_text('''#!/usr/bin/python3
import json, os, sys
args = sys.argv[1:]
with open(os.environ['STOP_TEST_LOG'], 'a') as f:
    f.write(json.dumps(args) + '\\n')
if args[:3] == ['compose', 'exec', '-T'] and 'SELECT name FROM exits ORDER BY id' in args:
    print('a\\nb\\nmissing')
elif args[:3] == ['container', 'ls', '-a']:
    print('relay-a\\ngluetun-a\\ngluetun-b\\nunrelated\\nrelay-other')
elif args[:2] == ['container', 'stop'] and os.environ.get('STOP_TEST_FAIL') == '1':
    sys.exit(1)
''')
            docker.chmod(0o755)
            log = root / 'calls'
            result = subprocess.run(['bash', str(ROOT / 'scripts/stop.sh')],
                env={**os.environ, 'PATH': tmp + ':' + os.environ['PATH'],
                     'STOP_TEST_LOG': str(log), 'STOP_TEST_FAIL': '1' if fail else '0'},
                text=True, capture_output=True)
            return result, [json.loads(line) for line in log.read_text().splitlines()]

    def test_stops_only_managed_pairs_before_stack(self):
        result, calls = self.run_stop()
        self.assertEqual(result.returncode, 0, result.stderr)
        stop = ['container', 'stop', '--time', '30', 'relay-a', 'gluetun-a', 'gluetun-b']
        self.assertIn(stop, calls)
        self.assertLess(calls.index(['compose', 'stop', 'backend']), calls.index(stop))
        self.assertEqual(calls[-1], ['compose', 'stop'])
        self.assertTrue(any('UPDATE exits' in ' '.join(call) for call in calls))

    def test_failure_still_stops_stack_without_claiming_success(self):
        result, calls = self.run_stop(fail=True)
        self.assertEqual(result.returncode, 1)
        self.assertEqual(calls[-1], ['compose', 'stop'])
        self.assertFalse(any('UPDATE exits' in ' '.join(call) for call in calls))
        self.assertIn('failed', result.stderr)

if __name__ == '__main__':
    unittest.main()
