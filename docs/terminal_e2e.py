#!/usr/bin/env python3
"""End-to-end stability test for the workshop terminal.

Reads GUID, CLIENT_ID, PASSWORD, TENANT, SUBSCRIPTION, and RESOURCEGROUP from
the environment. The password is sent only to the terminal service and is
removed from anything this script prints.

Example:
  TERMINAL_URL=https://cloud-terminal.azurewebsites.net python3 docs/terminal_e2e.py
"""

import argparse
import asyncio
import http.cookiejar
import json
import os
import re
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

ANSI = re.compile(
    r"\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)"
    r"|\x1b\[[0-?]*[ -/]*[@-~]"
    r"|\x1b[@-_]"
)
PROMPT = re.compile(r"(?:\]#|az#)\s*$")


def redact(text):
    out = str(text)
    for key in ("PASSWORD", "CLIENT_ID"):
        secret = os.environ.get(key, "")
        if secret and len(secret) > 4:
            out = out.replace(secret, "«redacted»")
    return out


def require_env():
    missing = [
        name for name in ("GUID", "CLIENT_ID", "PASSWORD", "TENANT", "SUBSCRIPTION", "RESOURCEGROUP")
        if not os.environ.get(name, "").strip()
    ]
    if missing:
        sys.exit("Missing environment: " + ", ".join(missing))


class Client:
    def __init__(self, base):
        self.base = base.rstrip("/")
        parsed = urllib.parse.urlparse(self.base)
        self.origin = f"{parsed.scheme}://{parsed.netloc}"
        self.ws_base = ("wss" if parsed.scheme == "https" else "ws") + "://" + parsed.netloc
        self.jar = http.cookiejar.CookieJar()
        self.opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(self.jar))
        self.session_id = ""

    def cookies(self):
        return "; ".join(f"{cookie.name}={cookie.value}" for cookie in self.jar)

    def request(self, method, path, body=None, timeout=60):
        data = None if body is None else json.dumps(body).encode()
        request = urllib.request.Request(
            self.base + path,
            data=data,
            method=method,
            headers={
                "Accept": "application/json",
                "Origin": self.origin,
                "Content-Type": "application/json",
            },
        )
        try:
            with self.opener.open(request, timeout=timeout) as response:
                raw = response.read().decode()
                return response.status, json.loads(raw or "{}")
        except urllib.error.HTTPError as error:
            raw = error.read().decode("utf-8", "replace")
            try:
                payload = json.loads(raw) if raw else {}
            except json.JSONDecodeError:
                payload = {"message": raw[:300]}
            return error.code, payload

    def create(self):
        status, payload = self.request("POST", "/api/terminal", {
            "repo": os.environ.get("WORKSHOP_REPO", "fjcloud/aro-ai-app-platform"),
            "guid": os.environ["GUID"],
            "clientId": os.environ["CLIENT_ID"],
            "password": os.environ["PASSWORD"],
            "tenant": os.environ["TENANT"],
            "subscription": os.environ["SUBSCRIPTION"],
            "resourceGroup": os.environ["RESOURCEGROUP"],
            "cols": 100,
            "rows": 30,
        })
        if status != 200:
            raise RuntimeError(redact(payload.get("message") or payload))
        self.session_id = payload["id"]
        return payload

    def status(self):
        code, payload = self.request("GET", f"/api/terminal/{self.session_id}")
        if code != 200:
            raise RuntimeError(redact(payload.get("message") or payload))
        return payload


def visible(text):
    return ANSI.sub("", text).replace("\r", "")


class Shell:
    def __init__(self, client):
        self.client = client
        self.ws = None
        self.raw = ""
        self.closes = []

    async def connect(self, websockets):
        headers = {"Cookie": self.client.cookies()}
        self.ws = await websockets.connect(
            f"{self.client.ws_base}/ws/{self.client.session_id}",
            origin=self.client.origin,
            additional_headers=headers,
            proxy=None,
            compression=None,
            open_timeout=30,
            ping_interval=None,
            ping_timeout=None,
            close_timeout=5,
            max_size=4_000_000,
        )
        self.raw = ""

    async def close(self):
        if self.ws is not None:
            try:
                await self.ws.close()
            except Exception:
                pass
            self.ws = None

    async def drop(self):
        """Close the TCP socket the way a vanished browser tab does."""
        if self.ws is None:
            return
        transport = getattr(self.ws, "transport", None)
        if transport is not None:
            transport.abort()
        else:
            await self.close()
        self.ws = None

    def screen(self):
        return visible(self.raw)[-4000:]

    async def pump(self, timeout):
        if self.ws is None:
            return False
        try:
            message = await asyncio.wait_for(self.ws.recv(), timeout)
        except asyncio.TimeoutError:
            return False
        except Exception as error:
            code = getattr(error, "code", None) or getattr(error, "rcvd", None)
            reason = getattr(error, "reason", "") or str(error)
            self.closes.append(redact(f"code={code} {reason}")[:240])
            self.ws = None
            return False
        if isinstance(message, bytes):
            message = message.decode("utf-8", "replace")
        self.raw = (self.raw + message)[-50000:]
        return True

    async def wait_for(self, predicate, timeout, label):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            if predicate():
                return True
            remaining = deadline - time.monotonic()
            if not await self.pump(min(2, max(0.1, remaining))):
                if self.ws is None:
                    return False
        return predicate()

    async def send(self, text):
        if self.ws is None:
            raise RuntimeError("socket is closed")
        await self.ws.send(text.encode())


async def run(args):
    import websockets

    require_env()
    client = Client(args.url)
    report = []

    def step(name, ok, detail):
        report.append((name, ok, redact(detail)))
        print(f"{'PASS' if ok else 'FAIL'}  {name}: {redact(detail)}", flush=True)
        return ok

    started = time.monotonic()
    try:
        created = client.create()
    except Exception as error:
        step("create", False, error)
        return 1
    step("create", True, f"id={created['id']} group={created.get('group')} status={created.get('status')}")

    ready = None
    deadline = time.monotonic() + args.ready_timeout
    last = ""
    while time.monotonic() < deadline:
        try:
            ready = client.status()
        except Exception as error:
            step("ready", False, error)
            return 1
        message = f"{ready.get('status')}: {ready.get('message')}"
        if message != last:
            print(f"      {redact(message)}", flush=True)
            last = message
        if ready.get("status") == "ready":
            break
        if ready.get("status") == "error":
            step("ready", False, ready.get("message"))
            return 1
        await asyncio.sleep(2)
    else:
        step("ready", False, "container was not ready in time")
        return 1
    step("ready", True, f"{time.monotonic() - started:.0f}s {ready.get('message')}")

    shell = Shell(client)
    try:
        await shell.connect(websockets)
    except Exception as error:
        response = getattr(error, "response", None)
        body = getattr(response, "body", b"") if response is not None else b""
        if isinstance(body, bytes):
            body = body.decode("utf-8", "replace")
        step("attach", False, f"{error} {body[:240]}")
        return 1

    def signed_in():
        screen = shell.screen()
        return "Azure CLI is ready." in screen or "Sign-in failed." in screen

    if not await shell.wait_for(signed_in, args.login_timeout, "login"):
        tail = shell.screen()[-500:].replace("\n", " ")
        step("login", False, f"closed={shell.closes} screen={tail!r}")
        await shell.close()
        return 1
    if "Sign-in failed." in shell.screen():
        step("login", False, "Azure CLI sign-in failed")
        await shell.close()
        return 1
    step("login", True, "Azure CLI is ready")

    async def command(marker, timeout):
        token = f"E2E_{marker}_{int(time.time())}"
        await shell.send(f"printf '%s\\n' {token}\n")

        def appeared():
            return token in shell.screen()

        ok = await shell.wait_for(appeared, timeout, marker)
        return ok, token

    ok, token = await command("ROUND", 30)
    if not step("roundtrip", ok, token if ok else f"closed={shell.closes} screen={shell.screen()[-400:]!r}"):
        await shell.close()
        return 1

    # Persist a marker, drop the socket, and expect tmux to redraw it.
    held = token
    failures = 0
    for index in range(1, args.cycles + 1):
        await shell.drop()
        await asyncio.sleep(args.pause)
        try:
            await shell.connect(websockets)
        except Exception as error:
            failures += 1
            step(f"reconnect-{index}", False, error)
            continue
        def still_there(expected=held):
            return expected in shell.screen()
        kept = await shell.wait_for(still_there, 25, "redraw")
        if not kept:
            failures += 1
            step(f"reconnect-{index}", False, f"previous output missing closed={shell.closes} screen={shell.screen()[-400:]!r}")
            continue
        ok, held = await command(f"R{index}", 30)
        if not ok:
            failures += 1
            step(f"reconnect-{index}", False, f"command lost closed={shell.closes}")
            continue
        step(f"reconnect-{index}", True, "same tmux screen, command answered")

    if failures:
        step("reconnects", False, f"{failures}/{args.cycles} failed")
        await shell.close()
        return 1
    step("reconnects", True, f"{args.cycles} attach cycles kept the session")

    # A command that outlives the browser socket must still finish inside tmux.
    long_token = f"E2E_LONG_{int(time.time())}"
    await shell.send(f"sleep {args.long_sleep}; printf '%s\\n' {long_token}\n")
    await asyncio.sleep(1)
    await shell.drop()
    await asyncio.sleep(args.pause)
    await shell.connect(websockets)
    def long_done():
        return long_token in shell.screen()
    ok = await shell.wait_for(long_done, args.long_sleep + 30, "long")
    step("survives-disconnect", ok, long_token if ok else f"closed={shell.closes} screen={shell.screen()[-400:]!r}")
    if not ok:
        await shell.close()
        return 1

    # Stay quiet and make sure the proxy does not reap the socket.
    idle_started = time.monotonic()
    idle_deadline = idle_started + args.idle
    idle_closed = False
    while time.monotonic() < idle_deadline:
        if not await shell.pump(min(5, idle_deadline - time.monotonic())) and shell.ws is None:
            idle_closed = True
            break
    quiet = time.monotonic() - idle_started
    if idle_closed:
        step("idle", False, f"died after {quiet:.0f}s of {args.idle:.0f}s {shell.closes} screen={shell.screen()[-400:]!r}")
        return 1
    ok, idle_token = await command("IDLE", 30)
    step("idle", ok, f"quiet {args.idle}s then {idle_token}" if ok else f"closed={shell.closes}")
    if not ok:
        await shell.close()
        return 1

    # Rapid input should not desynchronize the websocket.
    burst = [f"E2E_BURST_{index}_{int(time.time())}" for index in range(8)]
    await shell.send("".join(f"printf '%s\\n' {item}\n" for item in burst))
    def burst_done():
        screen = shell.screen()
        return all(item in screen for item in burst)
    ok = await shell.wait_for(burst_done, 30, "burst")
    step("burst", ok, f"{len(burst)} lines" if ok else f"closed={shell.closes} screen={shell.screen()[-400:]!r}")
    await shell.close()
    passed = all(item[1] for item in report)
    print(("STABLE" if passed else "UNSTABLE") + f"  {time.monotonic() - started:.0f}s", flush=True)
    return 0 if passed else 1


def main():
    parser = argparse.ArgumentParser(description="Exercise the workshop terminal until it stays attached.")
    parser.add_argument("--url", default=os.environ.get("TERMINAL_URL", "https://cloud-terminal.azurewebsites.net"))
    parser.add_argument("--cycles", type=int, default=5)
    parser.add_argument("--pause", type=float, default=1.5)
    parser.add_argument("--idle", type=float, default=75)
    parser.add_argument("--long-sleep", type=float, default=12)
    parser.add_argument("--ready-timeout", type=float, default=600)
    parser.add_argument("--login-timeout", type=float, default=180)
    args = parser.parse_args()
    try:
        code = asyncio.run(run(args))
    except KeyboardInterrupt:
        return 130
    return code


if __name__ == "__main__":
    sys.exit(main())
