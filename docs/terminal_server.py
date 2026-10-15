#!/usr/bin/env python3
"""Serve the workshop launcher and attach an Azure Container Instances shell.

The process serves only this directory's launcher files. A browser session
cookie is required to poll, attach, or delete a terminal. The service principal
secret is held in memory only until the container reports a successful sign-in.
"""

import hashlib
import hmac
import json
import os
import re
import secrets
import socket
import ssl
import struct
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

STATIC = os.path.dirname(os.path.abspath(__file__))
HOST = os.environ.get("HOST", "127.0.0.1")
PORT = int(os.environ.get("PORT", "8780"))
API = "2023-05-01"
IMAGE = "mcr.microsoft.com/azure-cli:latest"
WS_GUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
COOKIE = "terminal_secret"
READY_MARK = b"Azure CLI is ready."
FAILED_MARK = b"Sign-in failed."
SESSIONS = {}
SHELLS = {}
PROVISION_LOCKS = {}
SESSIONS_LOCK = threading.Lock()
STATE_LOCK = threading.Lock()


def _number(value, default):
    try:
        return int(value)
    except (TypeError, ValueError):
        return default


def redact(text, secrets):
    out = str(text)
    for secret in secrets:
        if secret and len(secret) > 4:
            out = out.replace(secret, "«redacted»")
    return out


def http_json(method, url, token=None, body=None, timeout=60):
    data = None if body is None else json.dumps(body).encode()
    headers = {"Accept": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    if data is not None:
        headers["Content-Type"] = "application/json"
    request = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            raw = response.read()
            return response.status, json.loads(raw.decode() or "{}")
    except urllib.error.HTTPError as error:
        raw = error.read().decode("utf-8", "replace")
        try:
            payload = json.loads(raw) if raw else {}
        except json.JSONDecodeError:
            payload = {"error": {"message": raw[:500]}}
        return error.code, payload


def azure_error(payload):
    error = payload.get("error") if isinstance(payload, dict) else None
    if isinstance(error, dict):
        return error.get("message") or error.get("code") or "Azure request failed"
    if isinstance(payload, dict) and payload.get("message"):
        return str(payload["message"])
    return "Azure request failed"


def access_token(session):
    now = time.time()
    if session.get("token") and session.get("token_expires", 0) - 60 > now:
        return session["token"]
    form = urllib.parse.urlencode({
        "client_id": session["clientId"],
        "client_secret": session["password"],
        "scope": "https://management.azure.com/.default",
        "grant_type": "client_credentials",
    }).encode()
    request = urllib.request.Request(
        f"https://login.microsoftonline.com/{session['tenant']}/oauth2/v2.0/token",
        data=form,
        headers={"Content-Type": "application/x-www-form-urlencoded"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(request, timeout=30) as response:
            payload = json.loads(response.read().decode())
    except urllib.error.HTTPError as error:
        raw = error.read().decode("utf-8", "replace")
        try:
            payload = json.loads(raw) if raw else {}
        except json.JSONDecodeError:
            payload = {}
        detail = ""
        if isinstance(payload, dict):
            detail = str(payload.get("error_description") or payload.get("error") or "")
        detail = detail.split("\r\n", 1)[0][:300]
        raise RuntimeError(detail or "Azure rejected the service principal sign-in.") from None
    session["token"] = payload["access_token"]
    session["token_expires"] = now + int(payload.get("expires_in", 3600))
    return session["token"]


def group_name(session):
    guid = str(session.get("guid") or "")
    resource_group = str(session.get("resourceGroup") or "")
    slug = re.sub(r"[^a-z0-9]", "", guid.lower()) or re.sub(r"[^a-z0-9]", "", resource_group.lower())
    if not slug:
        raise ValueError("GUID must include a letter or number")
    digest = hashlib.sha256(f"{resource_group.lower()}\n{guid}".encode()).hexdigest()[:20]
    return f"azsh-{digest}"


def group_url(session):
    name = group_name(session)
    return (
        "https://management.azure.com/subscriptions/"
        f"{session['subscription']}/resourceGroups/{session['resourceGroup']}"
        f"/providers/Microsoft.ContainerInstance/containerGroups/{name}?api-version={API}"
    )


def set_status(session, status, message):
    session["status"] = status
    session["message"] = message


def container_state(payload):
    props = payload.get("properties") or {}
    view = props.get("instanceView") or {}
    events = list(view.get("events") or [])
    for container in props.get("containers") or []:
        events.extend(((container.get("properties") or {}).get("instanceView") or {}).get("events") or [])
    detail = ""
    if events:
        last = events[-1]
        detail = last.get("message") or last.get("name") or ""
    state = view.get("state") or props.get("provisioningState") or "Pending"
    return state, detail


def ensure_container(session):
    token = access_token(session)
    status, group = http_json(
        "GET",
        f"https://management.azure.com/subscriptions/{session['subscription']}/resourcegroups/{session['resourceGroup']}?api-version=2021-04-01",
        token,
    )
    if status != 200:
        raise RuntimeError(azure_error(group))
    location = group.get("location") or "eastus"
    session["location"] = location
    name = group_name(session)
    url = group_url(session)
    status, current = http_json("GET", url, token)
    if status == 200:
        state, detail = container_state(current)
        if state == "Running":
            set_status(session, "ready", f"Container {name} is already running in {location}.")
            return
        if state not in ("Failed", "Stopped", "Terminated"):
            set_status(session, "starting", detail or f"Container {name} is {state}.")
            wait_until_running(session, url)
            return
        set_status(session, "starting", f"Replacing the previous {state.lower()} container.")
        reset_shell(session)
        http_json("DELETE", url, token)
        deadline = time.time() + 120
        while time.time() < deadline:
            status, _ = http_json("GET", url, access_token(session))
            if status == 404:
                break
            time.sleep(2)
    set_status(session, "starting", f"Creating container {name} in {location}. The Azure CLI image can take a few minutes.")
    body = {
        "location": location,
        "properties": {
            "osType": "Linux",
            "restartPolicy": "Always",
            "containers": [{
                "name": "az",
                "properties": {
                    "image": IMAGE,
                    "command": ["/bin/sh", "-c", "sleep infinity"],
                    "resources": {"requests": {"cpu": 1, "memoryInGB": 2}},
                },
            }],
        },
    }
    status, created = http_json("PUT", url, access_token(session), body)
    if status not in (200, 201):
        raise RuntimeError(azure_error(created))
    wait_until_running(session, url)


def wait_until_running(session, url):
    deadline = time.time() + 480
    last = ""
    while time.time() < deadline:
        status, payload = http_json("GET", url, access_token(session))
        if status == 200:
            state, detail = container_state(payload)
            message = detail or f"Container is {state}."
            if message != last:
                set_status(session, "starting", message)
                last = message
            if state == "Running":
                set_status(session, "ready", "Container is running. Attaching the terminal.")
                return
            if state in ("Failed", "Stopped", "Terminated"):
                raise RuntimeError(detail or f"Container entered {state}.")
        elif status not in (404, 409):
            raise RuntimeError(azure_error(payload))
        time.sleep(3)
    raise RuntimeError("The container did not start within 8 minutes.")


def delete_container(session):
    status, payload = http_json("DELETE", group_url(session), access_token(session))
    if status not in (200, 202, 204, 404):
        raise RuntimeError(azure_error(payload))


def provision(session):
    try:
        with provision_lock(session):
            ensure_container(session)
    except Exception as error:
        set_status(session, "error", redact(error, [session.get("password", "")]))
        session["password"] = ""


def quote(value):
    return "'" + str(value).replace("'", "'\\''") + "'"


def shell_key(session):
    return session["resourceGroup"] + "/" + group_name(session)


def shell_state(session):
    key = shell_key(session)
    with STATE_LOCK:
        state = SHELLS.get(key)
        if state is None:
            state = {"logged_in": False, "login_sent": False, "lock": threading.Lock()}
            SHELLS[key] = state
        return state


def reset_shell(session):
    state = shell_state(session)
    with state["lock"]:
        state["logged_in"] = False
        state["login_sent"] = False


def provision_lock(session):
    key = shell_key(session)
    with STATE_LOCK:
        lock = PROVISION_LOCKS.get(key)
        if lock is None:
            lock = threading.Lock()
            PROVISION_LOCKS[key] = lock
        return lock


def clear_passwords(session):
    with SESSIONS_LOCK:
        for item in SESSIONS.values():
            if item.get("resourceGroup") == session["resourceGroup"] and item.get("guid") == session["guid"]:
                item["password"] = ""


def shell_prompt(data):
    visible = re.sub(rb"\x1b\[[0-9;?]*[A-Za-z]", b"", data.replace(b"\r", b""))
    return bool(re.search(rb"\]#\s*$", visible))


def login_script(session):
    # The secret arrives on stdin of `read`, after history is disabled.
    # It is not part of this command text.
    return (
        "az login --service-principal --username "
        f"{quote(session['clientId'])} --password \"$AZURE_CLIENT_SECRET\" "
        f"--tenant {quote(session['tenant'])} >/tmp/az-login.txt 2>&1\n"
        "unset AZURE_CLIENT_SECRET\n"
        f"az account set --subscription {quote(session['subscription'])}\n"
        "if az account show >/dev/null 2>&1; then rm -f /tmp/az-login.txt; clear; echo 'Azure CLI is ready.'; "
        "else echo 'Sign-in failed.'; rm -f /tmp/az-login.txt; fi\n"
        "stty echo\n"
    )


def encode_frame(payload, opcode, mask):
    first = bytes([0x80 | opcode])
    length = len(payload)
    if mask:
        mask_bit = 0x80
        key = os.urandom(4)
    else:
        mask_bit = 0
        key = b""
    if length < 126:
        header = first + bytes([mask_bit | length])
    elif length < 65536:
        header = first + bytes([mask_bit | 126]) + struct.pack("!H", length)
    else:
        header = first + bytes([mask_bit | 127]) + struct.pack("!Q", length)
    if mask:
        payload = bytes(byte ^ key[index % 4] for index, byte in enumerate(payload))
        header += key
    return header + payload


class FrameReader:
    def __init__(self, sock):
        self.sock = sock
        self.buffer = b""

    def _read(self, size):
        while len(self.buffer) < size:
            chunk = self.sock.recv(65536)
            if not chunk:
                raise ConnectionError("socket closed")
            self.buffer += chunk
        data = self.buffer[:size]
        self.buffer = self.buffer[size:]
        return data

    def read_frame(self):
        header = self._read(2)
        opcode = header[0] & 0x0F
        masked = bool(header[1] & 0x80)
        length = header[1] & 0x7F
        if length == 126:
            length = struct.unpack("!H", self._read(2))[0]
        elif length == 127:
            length = struct.unpack("!Q", self._read(8))[0]
        if length > 4_000_000:
            raise ConnectionError("websocket frame is too large")
        key = self._read(4) if masked else b""
        payload = self._read(length)
        if masked:
            payload = bytes(byte ^ key[index % 4] for index, byte in enumerate(payload))
        return opcode, payload


def open_azure_socket(uri):
    parsed = urllib.parse.urlparse(uri)
    if parsed.scheme not in ("wss", "ws"):
        raise RuntimeError("Azure returned an unexpected terminal address")
    port = parsed.port or (443 if parsed.scheme == "wss" else 80)
    raw = socket.create_connection((parsed.hostname, port), timeout=30)
    try:
        raw.setsockopt(socket.IPPROTO_TCP, socket.TCP_NODELAY, 1)
    except OSError:
        pass
    if parsed.scheme == "wss":
        context = ssl.create_default_context()
        raw = context.wrap_socket(raw, server_hostname=parsed.hostname)
    key = os.urandom(16)
    key_b64 = __import__("base64").b64encode(key).decode()
    path = parsed.path or "/"
    if parsed.query:
        path += "?" + parsed.query
    handshake = (
        f"GET {path} HTTP/1.1\r\n"
        f"Host: {parsed.hostname}\r\n"
        "Upgrade: websocket\r\n"
        "Connection: Upgrade\r\n"
        f"Sec-WebSocket-Key: {key_b64}\r\n"
        "Sec-WebSocket-Version: 13\r\n"
        f"Origin: https://{parsed.hostname}\r\n"
        "\r\n"
    )
    raw.sendall(handshake.encode())
    data = b""
    while b"\r\n\r\n" not in data:
        chunk = raw.recv(4096)
        if not chunk:
            raise ConnectionError("Azure closed the terminal handshake")
        data += chunk
    head, rest = data.split(b"\r\n\r\n", 1)
    if b" 101 " not in head.split(b"\r\n", 1)[0]:
        raise ConnectionError(head.decode("utf-8", "replace")[:300])
    raw.settimeout(None)
    reader = FrameReader(raw)
    reader.buffer = rest
    return raw, reader


def exec_terminal(session, columns, rows):
    name = group_name(session)
    url = (
        "https://management.azure.com/subscriptions/"
        f"{session['subscription']}/resourceGroups/{session['resourceGroup']}"
        "/providers/Microsoft.ContainerInstance/containerGroups/"
        f"{name}/containers/az/exec?api-version={API}"
    )
    status, payload = http_json(
        "POST",
        url,
        access_token(session),
        {"command": "/bin/bash", "terminalSize": {"rows": rows, "cols": columns}},
    )
    if status != 200:
        raise RuntimeError(azure_error(payload))
    uri = payload.get("webSocketUri")
    password = payload.get("password")
    if not uri or not password:
        raise RuntimeError("Azure did not return a terminal socket")
    return uri, password


BROWSER_READERS = {}
BROWSER_LOCK = threading.Lock()


def _threads_available():
    try:
        threading.Thread(target=lambda: None, daemon=True).start()
    except RuntimeError:
        return False
    return True


def start_background(target, args=()):
    # Python Workers cannot start a thread. Run the container setup on the
    # request instead of failing the terminal create.
    try:
        threading.Thread(target=target, args=args, daemon=True).start()
    except RuntimeError:
        target(*args)


def bridge(browser, session, columns, rows):
    azure_sock = None
    stop = threading.Event()
    try:
        uri, shell_password = exec_terminal(session, columns, rows)
        azure_sock, reader = open_azure_socket(uri)
        azure_sock.sendall(encode_frame(shell_password.encode(), 0x1, True))
        for sock in (azure_sock, browser):
            try:
                sock.settimeout(0.05)
            except OSError:
                pass

        recent = [b""]
        generation = [0]

        def handle_azure(opcode, payload):
            if opcode == 0x8:
                stop.set()
                return
            if opcode == 0x9:
                azure_sock.sendall(encode_frame(payload, 0xA, True))
                return
            if opcode in (0x1, 0x2) and payload:
                recent[0] = (recent[0] + payload)[-800:]
                generation[0] += 1
                if READY_MARK in recent[0]:
                    state = shell_state(session)
                    with state["lock"]:
                        state["logged_in"] = True
                    clear_passwords(session)
                elif FAILED_MARK in recent[0]:
                    state = shell_state(session)
                    with state["lock"]:
                        state["login_sent"] = False
                browser.sendall(encode_frame(payload, 0x2, False))

        def pump_azure():
            try:
                frame = reader.read_frame()
            except TimeoutError:
                return False
            except OSError:
                stop.set()
                return False
            handle_azure(*frame)
            return True

        threaded = _threads_available()

        def wait_for_prompt(previous, timeout):
            deadline = time.time() + timeout
            while time.time() < deadline and not stop.is_set():
                if not threaded:
                    pump_azure()
                if generation[0] != previous and shell_prompt(recent[0]):
                    return True
                if threaded:
                    time.sleep(0.05)
            return False

        def sign_in():
            # The PTY echoes input before bash runs it. Turn echo off and wait
            # for the next prompt before the secret is written.
            if not wait_for_prompt(-1, 30) or stop.is_set():
                return
            state = shell_state(session)
            password = ""
            script = ""
            with state["lock"]:
                if state["logged_in"]:
                    session["password"] = ""
                    announce = True
                else:
                    announce = False
                if announce or state["login_sent"] or stop.is_set():
                    password = ""
                else:
                    password = session.get("password") or ""
                    if password and "\n" not in password and "\r" not in password:
                        state["login_sent"] = True
                        script = login_script(session)
                    else:
                        password = ""
            try:
                if announce:
                    azure_sock.sendall(encode_frame(b"echo 'Azure CLI is ready.'\n", 0x2, True))
                    return
                if not password:
                    return
                stamp = generation[0]
                azure_sock.sendall(encode_frame(
                    b"set +o history\nHISTFILE=/dev/null\nunset HISTFILE\nhistory -c\nrm -f ~/.bash_history\n"
                    b"stty -echo\nbind 'set enable-bracketed-paste off'\n",
                    0x2,
                    True,
                ))
                if not wait_for_prompt(stamp, 8) or stop.is_set():
                    with state["lock"]:
                        if not state["logged_in"]:
                            state["login_sent"] = False
                    return
                azure_sock.sendall(encode_frame(b"read -r AZURE_CLIENT_SECRET\n", 0x2, True))
                deadline = time.time() + 0.6
                while time.time() < deadline and not stop.is_set():
                    if threaded:
                        time.sleep(0.05)
                    else:
                        pump_azure()
                if stop.is_set():
                    with state["lock"]:
                        if not state["logged_in"]:
                            state["login_sent"] = False
                    return
                secret = password
                password = ""
                azure_sock.sendall(encode_frame((secret + "\n").encode(), 0x2, True))
                secret = ""
                azure_sock.sendall(encode_frame(script.encode(), 0x2, True))
                script = ""
            except OSError:
                with state["lock"]:
                    if not state["logged_in"]:
                        state["login_sent"] = False

        with BROWSER_LOCK:
            browser_reader = BROWSER_READERS[id(browser)]
        if threaded:
            def pump_azure_forever():
                try:
                    while not stop.is_set():
                        if not pump_azure():
                            continue
                finally:
                    stop.set()
                    try:
                        browser.shutdown(socket.SHUT_RDWR)
                    except OSError:
                        pass

            threading.Thread(target=pump_azure_forever, daemon=True).start()
            threading.Thread(target=sign_in, daemon=True).start()
            while not stop.is_set():
                try:
                    opcode, payload = browser_reader.read_frame()
                except TimeoutError:
                    continue
                except OSError:
                    break
                if opcode == 0x8:
                    break
                if opcode == 0x9:
                    browser.sendall(encode_frame(payload, 0xA, False))
                    continue
                if opcode in (0x1, 0x2) and payload:
                    azure_sock.sendall(encode_frame(payload, 0x2, True))
        else:
            sign_in()
            while not stop.is_set():
                pump_azure()
                try:
                    opcode, payload = browser_reader.read_frame()
                except TimeoutError:
                    continue
                except OSError:
                    break
                if opcode == 0x8:
                    break
                if opcode == 0x9:
                    browser.sendall(encode_frame(payload, 0xA, False))
                    continue
                if opcode in (0x1, 0x2) and payload:
                    azure_sock.sendall(encode_frame(payload, 0x2, True))
    except Exception as error:
        message = redact(error, [session.get("password", "")]).encode()
        try:
            browser.sendall(encode_frame(b"\r\n" + message + b"\r\n", 0x1, False))
        except OSError:
            pass
    finally:
        stop.set()
        if azure_sock is not None:
            try:
                azure_sock.close()
            except OSError:
                pass


ID_RE = re.compile(r"[0-9a-fA-F-]{36}\Z")
TENANT_RE = re.compile(r"[A-Za-z0-9][A-Za-z0-9.-]{0,127}\Z")
RG_RE = re.compile(r"[A-Za-z0-9][A-Za-z0-9._()-]{0,89}\Z")
GUID_RE = re.compile(r"[A-Za-z0-9-]{0,64}\Z")
VENDOR_TYPES = {
    ".js": "text/javascript; charset=utf-8",
    ".css": "text/css; charset=utf-8",
}


def validate_payload(payload):
    required = ("clientId", "password", "tenant", "subscription", "resourceGroup")
    if any(not str(payload.get(key) or "").strip() for key in required):
        return "Paste GUID, CLIENT_ID, PASSWORD, TENANT, SUBSCRIPTION, and RESOURCEGROUP."
    if not ID_RE.fullmatch(str(payload["clientId"]).strip()) or not ID_RE.fullmatch(str(payload["subscription"]).strip()):
        return "CLIENT_ID and SUBSCRIPTION must be ids."
    if not TENANT_RE.fullmatch(str(payload["tenant"]).strip()):
        return "TENANT must be a domain or tenant id."
    if not RG_RE.fullmatch(str(payload["resourceGroup"]).strip()):
        return "RESOURCEGROUP is not a resource group name."
    password = str(payload["password"])
    if not password or len(password) > 256 or re.search(r"[\r\n\x00]", password):
        return "PASSWORD cannot contain a line break."
    if not GUID_RE.fullmatch(str(payload.get("guid") or "")):
        return "GUID contains unsupported characters."
    return ""


def session_cookie(secret, secure):
    parts = [f"{COOKIE}={secret}", "HttpOnly", "SameSite=Strict", "Path=/", "Max-Age=28800"]
    if secure:
        parts.append("Secure")
    return "; ".join(parts)


HITS = {}
HITS_LOCK = threading.Lock()


def too_many(ip):
    now = time.time()
    with HITS_LOCK:
        recent = [stamp for stamp in HITS.get(ip, []) if now - stamp < 600]
        if len(recent) >= 8:
            HITS[ip] = recent
            return True
        recent.append(now)
        HITS[ip] = recent
        return False


def scrub_log(message):
    if re.search(r"(?i)password|terminal_secret", message):
        return None
    return message


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def version_string(self):
        return "terminal"

    def log_message(self, fmt, *args):
        message = scrub_log(fmt % args)
        if message is None:
            return
        super().log_message("%s", message)

    def log_error(self, fmt, *args):
        try:
            message = fmt % args
        except Exception:
            message = str(fmt)
        message = scrub_log(message)
        if message is None:
            super().log_error("%s", "rejected request")
            return
        super().log_error("%s", message)

    def end_headers(self):
        self.send_header("Cache-Control", "no-store")
        self.send_header("X-Content-Type-Options", "nosniff")
        self.send_header("Referrer-Policy", "no-referrer")
        self.send_header("X-Frame-Options", "DENY")
        self.send_header(
            "Content-Security-Policy",
            "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; "
            "connect-src 'self'; img-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'",
        )
        if self.https_request():
            self.send_header("Strict-Transport-Security", "max-age=31536000")
        super().end_headers()

    def _json(self, status, payload, cookie=None):
        raw = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        if cookie:
            self.send_header("Set-Cookie", cookie)
        self.end_headers()
        if not getattr(self, "_head", False):
            self.wfile.write(raw)

    def cookie_value(self):
        for part in self.headers.get("Cookie", "").split(";"):
            name, sep, value = part.strip().partition("=")
            if sep and name == COOKIE:
                return value
        return ""

    def https_request(self):
        return self.headers.get("X-Forwarded-Proto", "").split(",")[0].strip().lower() == "https"

    def public_host(self):
        forwarded = self.headers.get("X-Forwarded-Host", "").split(",")[0].strip()
        return forwarded or self.headers.get("Host", "")

    def origin_allowed(self):
        origin = self.headers.get("Origin", "")
        host = self.public_host()
        if not origin or not host:
            return False
        parsed = urllib.parse.urlparse(origin)
        return parsed.scheme in ("http", "https") and parsed.netloc == host

    def authorized(self, session_id):
        supplied = self.cookie_value()
        with SESSIONS_LOCK:
            session = SESSIONS.get(session_id)
            if session is None or not supplied:
                return None
            if not hmac.compare_digest(supplied, session["secret"]):
                return None
            return session

    def redirect(self, location):
        self.send_response(302)
        self.send_header("Location", location)
        self.send_header("Content-Length", "0")
        self.end_headers()

    def serve_static(self, path):
        if path in ("/", "/index.html"):
            target = os.path.join(STATIC, "index.html")
            content_type = "text/html; charset=utf-8"
        elif path.startswith("/vendor/"):
            name = path[len("/vendor/"):]
            if not re.fullmatch(r"[A-Za-z0-9._-]+", name):
                self.send_error(404)
                return
            target = os.path.join(STATIC, "vendor", name)
            content_type = VENDOR_TYPES.get(os.path.splitext(name)[1], "")
            if not content_type:
                self.send_error(404)
                return
        else:
            self.send_error(404)
            return
        try:
            with open(target, "rb") as handle:
                raw = handle.read()
        except OSError:
            self.send_error(404)
            return
        self.send_response(200)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        if not getattr(self, "_head", False):
            self.wfile.write(raw)

    def do_HEAD(self):
        path = urllib.parse.urlparse(self.path).path
        if path.startswith("/ws/"):
            self.send_error(405)
            return
        self._head = True
        self.do_GET()

    def read_body(self, limit=16384):
        try:
            length = int(self.headers.get("Content-Length", "0"))
        except ValueError:
            self.close_connection = True
            return None
        if length < 0 or length > limit:
            self.close_connection = True
            return None
        if length == 0:
            return b""
        return self.rfile.read(length)

    def client_ip(self):
        forwarded = self.headers.get("X-Forwarded-For", "")
        if forwarded:
            return forwarded.split(",")[-1].strip()
        return self.client_address[0]

    def do_GET(self):
        path = urllib.parse.urlparse(self.path).path
        if path == "/api/health":
            self._json(200, {"ok": True})
            return
        if path.startswith("/api/terminal/"):
            self.get_session(path.rsplit("/", 1)[-1])
            return
        if path.startswith("/ws/"):
            self.attach(path.rsplit("/", 1)[-1])
            return
        if path in ("/docs", "/docs/", "/docs/index.html"):
            self.redirect("/index.html")
            return
        self.serve_static(path)

    def do_POST(self):
        path = urllib.parse.urlparse(self.path).path
        body = self.read_body()
        if path != "/api/terminal":
            self._json(404, {"message": "Not found"})
            return
        if body is None or not body:
            self._json(400, {"message": "Expected a small JSON body."})
            return
        if not self.origin_allowed():
            self._json(403, {"message": "Open the terminal from this site."})
            return
        if too_many(self.client_ip()):
            self._json(429, {"message": "Too many terminal requests. Wait a few minutes."})
            return
        try:
            payload = json.loads(body.decode())
        except json.JSONDecodeError:
            self._json(400, {"message": "The request was not JSON."})
            return
        self.create_session(payload)

    def do_DELETE(self):
        path = urllib.parse.urlparse(self.path).path
        if not path.startswith("/api/terminal/"):
            self._json(404, {"message": "Not found"})
            return
        session_id = path.rsplit("/", 1)[-1]
        session = self.authorized(session_id)
        if not session:
            self._json(401, {"message": "That terminal session is gone. Choose Create again."})
            return
        try:
            delete_container(session)
            reset_shell(session)
        except Exception as error:
            self._json(502, {"message": redact(error, [session.get("password", "")])})
            return
        set_status(session, "deleted", f"Removed container {group_name(session)}.")
        self._json(200, {"status": "deleted"})

    def create_session(self, payload):
        problem = validate_payload(payload)
        if problem:
            self._json(400, {"message": problem})
            return
        session = {
            "id": uuid.uuid4().hex,
            "secret": secrets.token_urlsafe(32),
            "guid": str(payload.get("guid") or ""),
            "clientId": str(payload["clientId"]).strip(),
            "password": str(payload["password"]),
            "tenant": str(payload["tenant"]).strip(),
            "subscription": str(payload["subscription"]).strip(),
            "resourceGroup": str(payload["resourceGroup"]).strip(),
            "columns": max(40, min(_number(payload.get("cols"), 120), 220)),
            "rows": max(12, min(_number(payload.get("rows"), 32), 80)),
            "status": "starting",
            "message": "Requesting an Azure token.",
            "token": "",
            "token_expires": 0,
        }
        try:
            name = group_name(session)
        except ValueError as error:
            self._json(400, {"message": str(error)})
            return
        with SESSIONS_LOCK:
            SESSIONS[session["id"]] = session
        start_background(provision, (session,))
        self._json(200, {"id": session["id"], "group": name}, session_cookie(session["secret"], self.https_request()))

    def get_session(self, session_id):
        session = self.authorized(session_id)
        if not session:
            self._json(401, {"message": "That terminal session is gone. Choose Create again."})
            return
        self._json(200, {
            "status": session["status"],
            "message": session["message"],
            "group": group_name(session) if session.get("resourceGroup") else "",
            "location": session.get("location", ""),
        })

    def attach(self, session_id):
        session = self.authorized(session_id)
        if not session:
            self.send_error(401, "That terminal session is gone")
            return
        if not self.origin_allowed():
            self.send_error(403, "Open the terminal from this site")
            return
        if session["status"] != "ready":
            self.send_error(409, "Terminal is not ready")
            return
        key = self.headers.get("Sec-WebSocket-Key")
        if not key or self.headers.get("Upgrade", "").lower() != "websocket":
            self.send_error(400, "Expected a websocket")
            return
        accept = __import__("base64").b64encode(
            __import__("hashlib").sha1((key + WS_GUID).encode()).digest()
        ).decode()
        self.send_response(101, "Switching Protocols")
        self.send_header("Upgrade", "websocket")
        self.send_header("Connection", "Upgrade")
        self.send_header("Sec-WebSocket-Accept", accept)
        self.end_headers()
        browser = self.connection
        reader = FrameReader(browser)
        with BROWSER_LOCK:
            BROWSER_READERS[id(browser)] = reader
        try:
            bridge(browser, session, session["columns"], session["rows"])
        finally:
            with BROWSER_LOCK:
                BROWSER_READERS.pop(id(browser), None)
            self.close_connection = True


def main():
    server = ThreadingHTTPServer((HOST, PORT), Handler)
    shown = "127.0.0.1" if HOST in ("0.0.0.0", "::") else HOST
    print(f"http://{shown}:{PORT}/index.html", flush=True)
    server.serve_forever()


if __name__ == "__main__":
    main()
