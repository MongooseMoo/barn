"""Preserve the managed harness's final read, including successful observations."""

import json
import os
import re
from dataclasses import asdict
from pathlib import Path

import pytest
from moo_conformance.transport import SocketTransport


@pytest.fixture(autouse=True)
def capture_gunray_read(monkeypatch):
    execute = SocketTransport.execute

    def record(self, code):
        result = execute(self, code)
        case = re.search(r'case_id = "(gunray_\w+)"', code)
        if case:
            observation = {"case": case[1], "code": code, "result": asdict(result)}
            with Path(os.environ["GUNRAY_OBSERVATIONS"]).open("a") as stream:
                stream.write(json.dumps(observation, default=str) + "\n")
        return result

    monkeypatch.setattr(SocketTransport, "execute", record)
