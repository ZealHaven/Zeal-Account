#!/usr/bin/env python3
"""Add a public hostname to the remotely-managed cloudflared tunnel.

The `zeal-pc` tunnel is managed from the Cloudflare dashboard (its ingress is
pushed to the process and overrides ~/.cloudflared/config.yml), so local edits
never take effect. This patches the remote tunnel configuration via the API,
using the API token embedded in the origin certificate written by
`cloudflared login` (cert-zealhaven-id.pem for zone zealhaven.id).

Usage: tunnel-add-hostname.py HOSTNAME SERVICE [CONFIG_PEM]
Example: tunnel-add-hostname.py account.zealhaven.id http://localhost:8082
"""
import base64
import json
import re
import sys
import urllib.error
import urllib.request

DEFAULT_PEM = "/home/ridzeal/.cloudflared/cert-zealhaven-id.pem"
TUNNEL_ID = "fa499c16-92dd-4346-a7b3-f0ffeac2aa71"


def load_token(pem_path: str) -> tuple[str, str]:
    text = open(pem_path).read()
    m = re.search(
        r"-----BEGIN ARGO TUNNEL TOKEN-----\n([A-Za-z0-9+/=\n]+?)-----END", text
    )
    if not m:
        sys.exit(f"no Argo Tunnel token found in {pem_path}")
    tok = m.group(1).replace("\n", "")
    payload = json.loads(base64.b64decode(tok + "=" * (-len(tok) % 4)))
    return payload["apiToken"], payload["accountID"]


def api(method: str, url: str, token: str, body: dict | None = None) -> dict:
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(
        url,
        data=data,
        method=method,
        headers={
            "Authorization": "Bearer " + token,
            "Content-Type": "application/json",
        },
    )
    try:
        return json.loads(urllib.request.urlopen(req, timeout=20).read())
    except urllib.error.HTTPError as e:
        sys.exit(f"{method} {url} -> HTTP {e.code}: {e.read()[:400].decode()}")


def main() -> None:
    if len(sys.argv) < 3:
        sys.exit(__doc__)
    hostname, service = sys.argv[1], sys.argv[2]
    pem = sys.argv[3] if len(sys.argv) > 3 else DEFAULT_PEM

    token, account_id = load_token(pem)
    base = (
        f"https://api.cloudflare.com/client/v4/accounts/{account_id}"
        f"/cfd_tunnel/{TUNNEL_ID}/configurations"
    )

    cur = api("GET", base, token)["result"]
    ingress = cur["config"]["ingress"]
    if any(r.get("hostname") == hostname for r in ingress):
        print(f"{hostname} already present; nothing to do")
        return

    # insert before the catch-all (the only rule without a hostname)
    idx = next(i for i, r in enumerate(ingress) if "hostname" not in r)
    ingress.insert(idx, {"hostname": hostname, "service": service})

    body = {"version": cur["version"], "config": cur["config"]}
    res = api("PUT", base, token, body)["result"]
    rules = res["config"]["ingress"]
    print(f"HTTP OK -> version {res['version']}, {len(rules)} rules")
    print(f"  rule {idx}: {rules[idx]}")
    print(f"  last:     {rules[-1]}")


if __name__ == "__main__":
    main()
