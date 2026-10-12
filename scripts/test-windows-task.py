#!/usr/bin/env python3
import pathlib
import unittest
import xml.etree.ElementTree as ET
from xml.sax.saxutils import escape

ROOT = pathlib.Path(__file__).resolve().parent.parent
NS = {"t": "http://schemas.microsoft.com/windows/2004/02/mit/task"}


class WindowsTaskTests(unittest.TestCase):
    def test_task_settings_and_escaped_launcher_survive_utf16_rendering(self):
        launcher = 'C:\\Program Files\\Media & Télévision\\start-portico.cmd'
        text = (ROOT / "packaging/windows/startup-task.xml").read_text()
        rendered = text.replace("UTF-8", "UTF-16").replace("__PORTICO_LAUNCHER__", escape(launcher))
        task = ET.fromstring(rendered.encode("utf-16"))
        settings = task.find("t:Settings", NS)
        self.assertEqual(settings.find("t:ExecutionTimeLimit", NS).text, "PT0S")
        self.assertEqual(settings.find("t:DisallowStartIfOnBatteries", NS).text, "false")
        self.assertEqual(settings.find("t:StopIfGoingOnBatteries", NS).text, "false")
        self.assertIn(launcher, task.find("t:Actions/t:Exec/t:Arguments", NS).text)
        self.assertEqual(task.find("t:Principals/t:Principal/t:UserId", NS).text, "S-1-5-18")
        self.assertEqual(task.find("t:Principals/t:Principal/t:RunLevel", NS).text, "HighestAvailable")


if __name__ == "__main__":
    unittest.main()
