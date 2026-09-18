"""Safety boundary for the unpublished adoption entry point."""
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

CLI = Path(__file__).resolve().parents[2] / "deploy/node-link-v1/cli.py"


class DisabledLinkScaffoldTests(unittest.TestCase):
    def test_every_action_is_unavailable_and_leaves_working_directory_untouched(self):
        for action in ("readiness", "apply", "status", "recover"):
            with self.subTest(action=action), tempfile.TemporaryDirectory() as directory:
                receipt = Path(directory) / "bootstrap.json"
                receipt.write_bytes(b'{"state":"installed-unlinked"}\n')
                before = receipt.read_bytes()
                result = subprocess.run([sys.executable, str(CLI), action],
                                        cwd=directory, capture_output=True, text=True)
                self.assertEqual(result.returncode, 78)
                response = json.loads(result.stdout)
                self.assertEqual(response["protocol"], "pod-node-link-v1")
                self.assertFalse(response["capability_enabled"])
                self.assertEqual(response["status"], "unavailable")
                self.assertEqual(response["mutation"], "none")
                self.assertEqual(response["error_code"], "link_contract_not_qualified")
                self.assertEqual(list(Path(directory).iterdir()), [receipt])
                self.assertEqual(receipt.read_bytes(), before)

    def test_no_operator_enable_override(self):
        result = subprocess.run([sys.executable, str(CLI), "apply", "--enable"],
                                capture_output=True, text=True)
        self.assertEqual(result.returncode, 2)
        self.assertEqual(result.stdout, "")


if __name__ == "__main__":
    unittest.main()
