#!/usr/bin/env python3
"""Smoke-test a built binary using temporary config and local AWS/Kubernetes APIs.

No real cloud credentials, kubeconfig, or infrastructure are used.
"""
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs


class FixtureAPI(BaseHTTPRequestHandler):
    deny = False
    discovery_error_code = ""
    aws_actions = []

    def do_POST(self):
        fields = parse_qs(self.rfile.read(int(self.headers.get("Content-Length", "0"))).decode())
        action = fields.get("Action", [""])[0]
        version = fields.get("Version", [""])[0]
        signed = re.search(r"Credential=[^/]+/\d{8}/([^/]+)/", self.headers.get("Authorization", ""))
        self.aws_actions.append((action, signed.group(1) if signed else ""))
        status = 200
        if action == "DescribeRegions" and self.discovery_error_code:
            status = 403
            body = (f'<Response><Errors><Error><Code>{self.discovery_error_code}</Code>'
                    '<Message>private-pvc-secret at https://private-cluster.invalid</Message>'
                    '</Error></Errors><RequestID>fixture</RequestID></Response>')
        elif action in ("DescribeRegions", "DescribeVolumes", "DescribeAddresses"):
            content = {"DescribeRegions": '<regionInfo><item><regionName>us-west-2</regionName>'
                       '<optInStatus>opt-in-not-required</optInStatus></item>'
                       '<item><regionName>ap-south-1</regionName><optInStatus>opted-in</optInStatus></item></regionInfo>',
                       "DescribeVolumes": "<volumeSet/>", "DescribeAddresses": "<addressesSet/>"}[action]
            body = f'<{action}Response xmlns="http://ec2.amazonaws.com/doc/2016-11-15/"><requestId>fixture</requestId>{content}</{action}Response>'
        elif action == "DescribeLoadBalancers" and version in ("2012-06-01", "2015-12-01"):
            collection = "LoadBalancerDescriptions" if version == "2012-06-01" else "LoadBalancers"
            body = (f'<DescribeLoadBalancersResponse xmlns="http://elasticloadbalancing.amazonaws.com/doc/{version}/">'
                    f'<DescribeLoadBalancersResult><{collection}/></DescribeLoadBalancersResult>'
                    '<ResponseMetadata><RequestId>fixture</RequestId></ResponseMetadata></DescribeLoadBalancersResponse>')
        else:
            self.send_error(400, "Unexpected fixture operation")
            return
        encoded = body.encode()
        self.send_response(status)
        self.send_header("Content-Type", "text/xml")
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def do_GET(self):
        if self.deny:
            body = {"kind": "Status", "apiVersion": "v1", "status": "Failure", "reason": "Forbidden", "code": 403,
                    "message": "private-pvc-secret at https://private-cluster.invalid"}
            status = 403
        else:
            kind = "PodList" if self.path.startswith("/api/v1/pods") else "PersistentVolumeClaimList"
            body = {"kind": kind, "apiVersion": "v1", "metadata": {}, "items": []}
            status = 200
        encoded = json.dumps(body).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Warning", '299 fixture "private-pvc-secret at https://private-cluster.invalid"')
        self.send_header("Content-Length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def log_message(self, *_):
        pass


def main():
    binary = str(Path(sys.argv[1]).resolve())
    with tempfile.TemporaryDirectory(prefix="orphanscan-smoke-") as temporary:
        directory = Path(temporary)
        empty = directory / "empty-aws-config"
        empty.write_text("")
        env = {key: value for key, value in os.environ.items() if not key.startswith("AWS_")}
        env.update(AWS_CONFIG_FILE=str(empty), AWS_SHARED_CREDENTIALS_FILE=str(empty),
                   AWS_EC2_METADATA_DISABLED="true", DO_NOT_TRACK="1", CI="true")
        server = ThreadingHTTPServer(("127.0.0.1", 0), FixtureAPI)
        endpoint = f"http://127.0.0.1:{server.server_port}"
        # Unexpected external requests must fail at this local server rather
        # than reaching real infrastructure. Loopback fixture calls bypass it.
        env.update(HTTP_PROXY=endpoint, HTTPS_PROXY=endpoint, NO_PROXY="127.0.0.1,localhost",
                   http_proxy=endpoint, https_proxy=endpoint, no_proxy="127.0.0.1,localhost")
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        kubeconfig = directory / "kubeconfig.json"
        kubeconfig.write_text(json.dumps({
            "apiVersion": "v1", "kind": "Config",
            "clusters": [{"name": "fixture", "cluster": {"server": f"http://127.0.0.1:{server.server_port}"}}],
            "contexts": [{"name": "fixture", "context": {"cluster": "fixture", "user": "fixture"}}],
            "users": [{"name": "fixture", "user": {}}], "current-context": "fixture",
        }))
        env["KUBECONFIG"] = str(kubeconfig)

        def run(arguments, code, status=None):
            result = subprocess.run([binary, *arguments], env=env, capture_output=True, text=True, timeout=15)
            assert result.returncode == code, (arguments, result.returncode, result.stdout, result.stderr)
            assert "private-pvc-secret" not in result.stdout + result.stderr
            assert "private-cluster.invalid" not in result.stdout + result.stderr
            if status:
                report = json.loads(result.stdout)
                assert report["schema_version"] == 1 and report["status"] == status, report
                assert "total_monthly_waste" not in report
            return result

        try:
            assert "OrphanScan " in run(["--version"], 0).stdout
            run(["--help"], 0)
            run(["unexpected", "--output=json"], 2)
            run(["--timeout=0"], 2)
            run(["--sources=aws", "--output=json"], 1, "failed")
            run(["--sources=aws", "--regions=us-east-1", "--output=json"], 1, "failed")
            run(["--sources=kubernetes", "--output=json"], 0, "complete")
            run(["--sources=aws,kubernetes", "--output=json"], 3, "partial")
            FixtureAPI.deny = True
            run(["--sources=kubernetes", "--output=json"], 1, "failed")
            FixtureAPI.deny = False
            env.update(AWS_ACCESS_KEY_ID="fixture", AWS_SECRET_ACCESS_KEY="fixture", AWS_ENDPOINT_URL=endpoint)

            def scan_regions(arguments, expected, discovery_endpoint=None):
                start = len(FixtureAPI.aws_actions)
                result = run(["--sources=aws", "--output=json", *arguments], 0, "complete")
                checks = json.loads(result.stdout)["checks"]
                assert {check["region"] for check in checks} == set(expected), checks
                assert len(checks) == 4 * len(expected), checks
                discoveries = [region for action, region in FixtureAPI.aws_actions[start:] if action == "DescribeRegions"]
                assert discoveries == ([] if discovery_endpoint is None else [discovery_endpoint]), discoveries

            scan_regions([], ["ap-south-1", "us-west-2"], "us-east-1")
            env["AWS_REGION"] = "eu-west-1"
            scan_regions([], ["eu-west-1"])
            scan_regions(["--all-regions"], ["ap-south-1", "us-west-2"], "eu-west-1")
            scan_regions(["--regions=ap-south-1"], ["ap-south-1"])
            run(["--all-regions", "--regions=us-east-1"], 2)
            complete = run([], 0)
            verbose = run(["--verbose"], 0)
            assert "Status: Complete" in complete.stdout and not complete.stderr
            assert "\nChecks\n" not in complete.stdout and "prices checked" not in complete.stdout
            assert "\nChecks\n" in verbose.stdout and "prices checked" in verbose.stdout
            plain_json = run(["--output=json"], 0, "complete")
            verbose_json = run(["--output=json", "--verbose"], 0, "complete")
            assert plain_json.stdout == verbose_json.stdout

            # Reproduce a no-argument partial scan with failed AWS authentication.
            env.pop("AWS_REGION")
            FixtureAPI.discovery_error_code = "AuthFailure"
            auth = run([], 3)
            assert "Status: Partial" in auth.stdout and not auth.stderr
            assert "AWS          Unavailable" in auth.stdout and "AWS          $0.00" not in auth.stdout
            assert "Kubernetes   $0.00/month" in auth.stdout
            assert auth.stdout.count("Authentication failed while discovering regions.") == 1
            assert "ec2:DescribeRegions" not in auth.stdout and "--regions" not in auth.stdout
            auth_json = run(["--output=json"], 3, "partial")
            assert auth_json.stderr.count("Authentication failed while discovering regions.") == 1
            assert "[ERROR]" not in auth_json.stderr and "[WARN]" not in auth_json.stderr

            FixtureAPI.discovery_error_code = "UnauthorizedOperation"
            failed = run(["--sources=aws", "--all-regions", "--output=json"], 1, "failed")
            assert json.loads(failed.stdout)["checks"][0]["resource"] == "configuration"
            denied_text = run(["--sources=aws", "--all-regions"], 1)
            assert "ec2:DescribeRegions" in denied_text.stdout and not denied_text.stderr
            assert "AWS          Unavailable" in denied_text.stdout and "$0.00" not in denied_text.stdout
            run(["--sources=aws,kubernetes", "--all-regions", "--output=json"], 3, "partial")
        finally:
            server.shutdown()
            server.server_close()
            thread.join()
    print("Binary smoke tests passed (isolated configuration and local API fixtures).")


if __name__ == "__main__":
    main()
