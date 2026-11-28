"""Credentials and dispatch for Barn's trusted GitHub publisher."""

import base64
from datetime import datetime
import json
import subprocess
import time
from urllib.request import Request, urlopen

from repair_authority import BARN


class PublisherToken:
    def __init__(self, config, repositories=("barn", "moo-conformance-tests"), permissions=None):
        self.config = config
        self.request_scope = {"repositories": list(repositories)}
        if permissions is not None:
            self.request_scope["permissions"] = permissions
        self.token = None
        self.expires = 0

    def get(self):
        now = int(time.time())
        if self.expires > now + 120:
            return self.token

        def encode(value):
            return base64.urlsafe_b64encode(value).rstrip(b"=")

        header = encode(b'{"alg":"RS256","typ":"JWT"}')
        claims = encode(json.dumps({"iat": now - 60, "exp": now + 480,
                                    "iss": self.config["client_id"]}).encode())
        unsigned = header + b"." + claims
        signed = subprocess.run(["openssl", "dgst", "-sha256", "-sign", self.config["key_file"]],
                                input=unsigned, capture_output=True, check=True, timeout=10).stdout
        jwt = (unsigned + b"." + encode(signed)).decode()
        installation = int(self.config["installation_id"])
        request = Request(f"https://api.github.com/app/installations/{installation}/access_tokens",
                          data=json.dumps(self.request_scope).encode(),
                          headers={"Authorization": f"Bearer {jwt}", "Accept": "application/vnd.github+json",
                                   "Content-Type": "application/json", "User-Agent": "Barn repair worker"})
        with urlopen(request, timeout=30) as response:
            result = json.load(response)
        self.token = result["token"]
        self.expires = datetime.fromisoformat(result["expires_at"].replace("Z", "+00:00")).timestamp()
        return self.token


def existing_dispatch(api, workflow, title, actor_id):
    page = 1
    while True:
        runs = api(f"repos/{BARN}/actions/workflows/{workflow}/runs"
                   f"?branch=master&event=workflow_dispatch&per_page=100&page={page}")["workflow_runs"]
        for run in runs:
            if run["display_title"] == title and run["actor"]["id"] == actor_id:
                return run["id"]
        if len(runs) < 100:
            return None
        page += 1


def dispatch(api, workflow, inputs):
    receipt = api(f"repos/{BARN}/actions/workflows/{workflow}/dispatches",
                  {"ref": "master", "inputs": inputs, "return_run_details": True})
    run_id = (receipt or {}).get("workflow_run_id")
    if type(run_id) is not int or run_id < 1:
        raise RuntimeError(f"GitHub did not return a run ID for {workflow}")
    return run_id
