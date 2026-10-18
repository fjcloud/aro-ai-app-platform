#!/usr/bin/env python3
"""Serve the workshop launcher and attach an Azure Container Instances shell.

The process serves only this directory's launcher files. A browser session
cookie is required to poll, attach, or delete a terminal. The same lab export
reattaches to the container already running for that client. The interactive
shell is one tmux session, so a reconnect shows the same screen and keeps
commands that were already running. The service principal secret stays in
memory until the terminal is removed, so a later reconnect can refresh the
Azure token. Home files live on an Azure file share.
"""

import datetime
import hashlib
import hmac
import json
import os
import re
import secrets
import select
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
STORAGE_API = "2023-01-01"
IMAGE = os.environ.get("CLI_IMAGE", "fjaazcli.azurecr.io/az:4")
HOME_MOUNT = "/home/az"
EXEC_COMMAND = "/usr/local/bin/workshop-shell"
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


def group_url(session, name=None):
    name = name or group_name(session)
    return (
        "https://management.azure.com/subscriptions/"
        f"{session['subscription']}/resourceGroups/{session['resourceGroup']}"
        f"/providers/Microsoft.ContainerInstance/containerGroups/{name}?api-version={API}"
    )


def client_key(session):
    return "|".join((
        str(session.get("subscription") or ""),
        str(session.get("resourceGroup") or ""),
        str(session.get("guid") or ""),
        str(session.get("clientId") or ""),
    ))


def storage_account_name(session):
    digest = hashlib.sha256(
        f"{str(session.get('resourceGroup') or '').lower()}\n{session.get('guid') or ''}".encode()
    ).hexdigest()[:20]
    return f"azsh{digest}"


def container_names(session):
    names = [group_name(session)]
    guid = str(session.get("guid") or "")
    slug = re.sub(r"[^a-z0-9]", "", guid.lower())[:10]
    if slug:
        legacy = f"azsh-{slug}"
        if legacy not in names:
            names.append(legacy)
    return names


def set_status(session, status, message):
    session["status"] = status
    session["message"] = message


def same_terminal(payload):
    containers = (payload.get("properties") or {}).get("containers") or []
    if not containers:
        return False
    props = containers[0].get("properties") or {}
    mounts = props.get("volumeMounts") or []
    return props.get("image") == IMAGE and any(item.get("mountPath") == HOME_MOUNT for item in mounts)


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


def ensure_home(session, location):
    name = storage_account_name(session)
    account_url = (
        "https://management.azure.com/subscriptions/"
        f"{session['subscription']}/resourceGroups/{session['resourceGroup']}"
        f"/providers/Microsoft.Storage/storageAccounts/{name}?api-version={STORAGE_API}"
    )
    status, current = http_json("GET", account_url, access_token(session))
    if status == 404:
        set_status(session, "starting", "Creating the persistent home directory.")
        created_status, created = http_json("PUT", account_url, access_token(session), {
            "location": location,
            "sku": {"name": "Standard_LRS"},
            "kind": "StorageV2",
            "properties": {
                "allowBlobPublicAccess": False,
                "minimumTlsVersion": "TLS1_2",
                "supportsHttpsTrafficOnly": True,
                "allowSharedKeyAccess": True,
            },
        })
        if created_status not in (200, 202):
            raise RuntimeError(azure_error(created))
    elif status != 200:
        raise RuntimeError(azure_error(current))
    deadline = time.time() + 180
    while time.time() < deadline:
        status, current = http_json("GET", account_url, access_token(session))
        if status == 200 and (current.get("properties") or {}).get("provisioningState") == "Succeeded":
            break
        if status not in (200, 202, 409):
            raise RuntimeError(azure_error(current))
        time.sleep(3)
    else:
        raise RuntimeError("The home directory storage did not become ready.")
    share_url = account_url.split("?", 1)[0] + f"/fileServices/default/shares/home?api-version={STORAGE_API}"
    status, share = http_json("PUT", share_url, access_token(session), {"properties": {"shareQuota": 5}})
    if status not in (200, 201):
        raise RuntimeError(azure_error(share))
    status, keys = http_json("POST", account_url.split("?", 1)[0] + f"/listKeys?api-version={STORAGE_API}", access_token(session))
    if status != 200:
        raise RuntimeError(azure_error(keys))
    values = [item.get("value") for item in (keys.get("keys") or []) if item.get("value")]
    if not values:
        raise RuntimeError("Azure did not return a key for the home directory.")
    session["storageKey"] = values[0]
    return name


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
        if state == "Running" and same_terminal(current):
            set_status(session, "ready", f"Reattached to container {name} in {location}. Home files were kept.")
            return
        if state.lower() == "deleting" or state in ("Running", "Failed", "Stopped", "Terminated"):
            if state == "Running":
                set_status(session, "starting", "Switching the running terminal onto the persistent home directory.")
            elif state.lower() == "deleting":
                set_status(session, "starting", "Waiting for the previous container to finish removing.")
            else:
                set_status(session, "starting", f"Replacing the previous {state.lower()} container.")
            reset_shell(session)
            if state.lower() != "deleting":
                http_json("DELETE", url, token)
            wait_until_gone(session, url)
        else:
            set_status(session, "starting", detail or f"Container {name} is {state}.")
            wait_until_running(session, url)
            if same_terminal(http_json("GET", url, access_token(session))[1]):
                return
            reset_shell(session)
            http_json("DELETE", url, access_token(session))
            wait_until_gone(session, url)
    account = ensure_home(session, location)
    set_status(session, "starting", f"Starting container {name} in {location}. Reconnects attach to the same session.")
    body = {
        "location": location,
        "properties": {
            "osType": "Linux",
            "restartPolicy": "Always",
            "containers": [{
                "name": "az",
                "properties": {
                    "image": IMAGE,
                    "command": ["/usr/local/bin/workshop-init"],
                    "environmentVariables": [
                        {"name": "HOME", "value": HOME_MOUNT},
                        {"name": "AZURE_EXTENSION_DIR", "value": "/opt/az/extensions"},
                        {"name": "PS1", "value": "az# "},
                        {"name": "TERM", "value": "xterm-256color"},
                    ],
                    "volumeMounts": [{"name": "home", "mountPath": HOME_MOUNT}],
                    "resources": {"requests": {"cpu": 1, "memoryInGB": 2}},
                },
            }],
            "volumes": [{
                "name": "home",
                "azureFile": {
                    "shareName": "home",
                    "storageAccountName": account,
                    "storageAccountKey": session.get("storageKey", ""),
                },
            }],
        },
    }
    status, created = http_json("PUT", url, access_token(session), body)
    if status not in (200, 201):
        raise RuntimeError(azure_error(created))
    wait_until_running(session, url)


def container_instance(payload):
    containers = (payload.get("properties") or {}).get("containers") or []
    if not containers:
        return {}
    return (containers[0].get("properties") or {}).get("instanceView") or {}


def started_at(value):
    if not value:
        return None
    text = str(value).replace("Z", "+00:00")
    if "." in text:
        head, tail = text.split(".", 1)
        split = len(tail)
        for index, char in enumerate(tail):
            if char in "+-":
                split = index
                break
        fraction = tail[:split]
        zone = tail[split:]
        text = head + "." + (fraction + "000000")[:6] + zone
    try:
        return datetime.datetime.fromisoformat(text).timestamp()
    except ValueError:
        return None


def wait_until_running(session, url):
    # Azure reports Running a little before exec can see the container.
    # Attaching in that gap closes the shell with "code package" not found.
    deadline = time.time() + 480
    last = ""
    running_since = None
    while time.time() < deadline:
        status, payload = http_json("GET", url, access_token(session))
        if status == 200:
            state, detail = container_state(payload)
            message = detail or f"Container is {state}."
            if message != last:
                set_status(session, "starting", message)
                last = message
            if state == "Running":
                view = container_instance(payload)
                current = view.get("currentState") or {}
                previous = view.get("previousState") or {}
                restarts = view.get("restartCount") or 0
                if restarts >= 3:
                    reason = previous.get("detailStatus") or current.get("detailStatus") or detail
                    raise RuntimeError(reason or "The shell container keeps restarting.")
                if current.get("state") not in (None, "Running"):
                    running_since = None
                else:
                    if running_since is None:
                        running_since = time.time()
                    started = started_at(current.get("startTime"))
                    age = time.time() - started if started else time.time() - running_since
                    if age >= 12:
                        set_status(session, "ready", "Container is running. Attaching the terminal.")
                        return
                    set_status(session, "starting", "Container is running. Waiting until Azure can attach a shell.")
            elif state in ("Failed", "Stopped", "Terminated"):
                raise RuntimeError(detail or f"Container entered {state}.")
            else:
                running_since = None
        elif status not in (404, 409):
            raise RuntimeError(azure_error(payload))
        time.sleep(3)
    raise RuntimeError("The container did not start within 8 minutes.")


def wait_until_gone(session, url):
    deadline = time.time() + 180
    while time.time() < deadline:
        status, payload = http_json("GET", url, access_token(session))
        if status == 404:
            return
        if status not in (200, 202, 204, 409):
            raise RuntimeError(azure_error(payload))
        time.sleep(2)
    raise RuntimeError("The container is still being removed. Wait a moment and choose Remove again.")


def delete_container(session):
    for name in container_names(session):
        url = group_url(session, name)
        status, payload = http_json("DELETE", url, access_token(session))
        if status == 404:
            continue
        if status not in (200, 202, 204):
            raise RuntimeError(azure_error(payload))
        wait_until_gone(session, url)


def provision(session):
    try:
        with provision_lock(session):
            ensure_container(session)
    except Exception as error:
        set_status(session, "error", redact(error, [session.get("password", ""), session.get("storageKey", "")]))
        session["password"] = ""
    finally:
        session.pop("storageKey", None)


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


def shell_prompt(data):
    visible = data.replace(b"\r", b"")
    visible = re.sub(rb"\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)", b"", visible)
    visible = re.sub(rb"\x1b\[[0-9;?]*[A-Za-z]", b"", visible)
    return bool(re.search(rb"(?:\]#|az#)\s*$", visible))


def login_script(session):
    # The secret arrives on stdin of `read`, after history is disabled.
    # It is not part of this command text. Leave the screen in place so a
    # dropped connection does not come back blank.
    return (
        "echo 'Signing in to Azure...'\n"
        "az login --service-principal --username "
        f"{quote(session['clientId'])} --password \"$AZURE_CLIENT_SECRET\" "
        f"--tenant {quote(session['tenant'])} </dev/null >/tmp/az-login.txt 2>&1\n"
        "unset AZURE_CLIENT_SECRET\n"
        f"az account set --subscription {quote(session['subscription'])} </dev/null\n"
        "if az account show </dev/null >/dev/null 2>&1; then rm -f /tmp/az-login.txt; echo 'Azure CLI is ready.'; "
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
        self.fin = bool(header[0] & 0x80)
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
        {"command": EXEC_COMMAND, "terminalSize": {"rows": rows, "cols": columns}},
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
BRIDGES = {}


def claim_bridge(session):
    """One exec at a time. A reattach closes the previous sockets first."""
    claim = {"stop": threading.Event(), "done": threading.Event(), "sockets": []}
    with STATE_LOCK:
        previous = BRIDGES.get(shell_key(session))
        BRIDGES[shell_key(session)] = claim
    if previous is not None:
        previous["stop"].set()
        for sock in list(previous.get("sockets") or []):
            try:
                sock.shutdown(socket.SHUT_RDWR)
            except OSError:
                pass
            try:
                sock.close()
            except OSError:
                pass
        previous["done"].wait(timeout=3)
    return claim


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
    claim = claim_bridge(session)
    stop = claim["stop"]
    azure_sock = None
    browser_send_lock = threading.Lock()
    last_ping = 0
    last_azure = 0

    def send_socket(sock, payload, opcode, mask, lock):
        frame = encode_frame(payload, opcode, mask)
        with lock:
            sock.sendall(frame)

    def send_browser(payload, opcode=0x2):
        send_socket(browser, payload, opcode, False, browser_send_lock)

    def maybe_ping():
        nonlocal last_ping
        # Protocol pings get the App Service proxy to drop the browser socket.
        # A NUL data frame keeps that proxy awake and does not change the screen.
        if time.time() - last_ping < 20 or stop.is_set():
            return
        last_ping = time.time()
        try:
            send_browser(b"\x00", 0x2)
        except OSError:
            stop.set()

    def maybe_azure():
        nonlocal last_azure
        # The container exec socket closes when it receives no frames. A ping
        # is not shell input, so a quiet prompt stays put.
        if azure_sock is None or time.time() - last_azure < 4 or stop.is_set():
            return
        last_azure = time.time()
        try:
            send_azure(b"", 0x9)
        except OSError:
            stop.set()

    try:
        try:
            browser.setsockopt(socket.IPPROTO_TCP, socket.TCP_NODELAY, 1)
            browser.setsockopt(socket.SOL_SOCKET, socket.SO_KEEPALIVE, 1)
        except OSError:
            pass
        claim["sockets"].append(browser)

        def attach_shell():
            # Exec can succeed in ARM and still close at once while the node
            # is registering the container. Retry that case in this socket.
            announced = False
            last_reason = ""
            deadline = time.time() + 90
            while time.time() < deadline and not stop.is_set():
                sock = None
                try:
                    uri, shell_password = exec_terminal(session, columns, rows)
                    sock, reader = open_azure_socket(uri)
                    sock.settimeout(5)
                    sock.sendall(encode_frame(shell_password.encode(), 0x1, True))
                    early = []
                    probe_until = time.time() + 8
                    while time.time() < probe_until and not stop.is_set():
                        try:
                            opcode, payload = reader.read_frame()
                        except TimeoutError:
                            sock.settimeout(20)
                            return sock, reader, early
                        except ssl.SSLWantReadError:
                            continue
                        if opcode == 0x8:
                            reason = payload[2:].decode("utf-8", "replace") if len(payload) > 2 else ""
                            last_reason = reason
                            break
                        early.append((opcode, payload))
                        if opcode in (0x1, 0x2) and payload:
                            sock.settimeout(20)
                            return sock, reader, early
                except RuntimeError as error:
                    last_reason = str(error)
                    text = last_reason.lower()
                    if "code package" not in text and "transition" not in text and "not running" not in text:
                        raise
                except OSError as error:
                    last_reason = str(error)
                if sock is not None:
                    try:
                        sock.close()
                    except OSError:
                        pass
                if not announced:
                    announced = True
                    send_browser(b"\r\nWaiting for Azure to accept the shell...\r\n", 0x1)
                time.sleep(5)
            if stop.is_set():
                return None, None, []
            if "code package" in last_reason.lower():
                raise RuntimeError("Azure could not attach a shell to the container yet. Press Start again.")
            raise RuntimeError(last_reason or "Azure did not accept the shell.")

        azure_sock, reader, early = attach_shell()
        if azure_sock is None or stop.is_set():
            return
        try:
            azure_sock.settimeout(None)
            browser.settimeout(None)
        except OSError:
            pass
        claim["sockets"].append(azure_sock)
        azure_send_lock = threading.Lock()

        def send_azure(payload, opcode=0x2):
            send_socket(azure_sock, payload, opcode, True, azure_send_lock)

        try:
            browser.settimeout(20)
        except OSError:
            pass

        recent = [b""]
        generation = [0]

        def handle_azure(opcode, payload):
            if opcode == 0x8:
                reason = payload[2:122] if len(payload) > 2 else b""
                note = b"\r\n[shell closed"
                if reason:
                    note += b": " + reason
                note += b"]\r\n"
                try:
                    send_browser(note, 0x1)
                except OSError:
                    pass
                stop.set()
                return
            if opcode == 0x9:
                send_azure(payload, 0xA)
                return
            if opcode in (0x1, 0x2) and payload:
                recent[0] = (recent[0] + payload)[-800:]
                generation[0] += 1
                if READY_MARK in recent[0]:
                    state = shell_state(session)
                    with state["lock"]:
                        state["logged_in"] = True
                elif FAILED_MARK in recent[0]:
                    state = shell_state(session)
                    with state["lock"]:
                        state["login_sent"] = False
                send_browser(payload, 0x2)

        for opcode, payload in early:
            handle_azure(opcode, payload)
            if stop.is_set():
                return

        def readable(sock, timeout):
            try:
                ready, _, _ = select.select([sock], [], [], timeout)
            except (OSError, ValueError):
                stop.set()
                return False
            return bool(ready)

        def pump_azure():
            if stop.is_set() or not readable(azure_sock, 0.05):
                return False
            try:
                frame = reader.read_frame()
            except (TimeoutError, ssl.SSLWantReadError):
                return False
            except OSError:
                stop.set()
                return False
            try:
                handle_azure(*frame)
            except OSError:
                stop.set()
                return False
            return True

        side = {"pump": lambda: None}

        def wait_for(predicate, previous, timeout):
            deadline = time.time() + timeout
            stable_since = None
            while time.time() < deadline and not stop.is_set():
                pump_azure()
                side["pump"]()
                maybe_ping()
                maybe_azure()
                if generation[0] != previous and predicate(recent[0]):
                    if stable_since is None:
                        stable_since = time.time()
                    elif time.time() - stable_since >= 0.2:
                        return True
                else:
                    stable_since = None
            return False

        def sign_in():
            # The home directory keeps Azure CLI tokens. Reconnects reuse them
            # instead of sitting in a silent login that the browser treats as dead.
            if not wait_for(shell_prompt, -1, 20) or stop.is_set():
                return
            state = shell_state(session)
            with state["lock"]:
                announce = bool(state["logged_in"])
                busy = bool(state["login_sent"])
            try:
                # Already inside the persistent session. Typing here would land
                # in whatever command the user is running.
                if announce or busy or stop.is_set():
                    return
                stamp = generation[0]
                send_azure(
                    b"stty -echo\n"
                    b"if timeout 20 az account show </dev/null >/dev/null 2>&1; then echo 'Azure CLI is ready.'; "
                    b"else echo AZ_NEEDS_LOGIN; fi\n"
                    b"stty echo\n"
                )
                seen = lambda data: READY_MARK in data or b"AZ_NEEDS_LOGIN" in data
                if not wait_for(seen, stamp, 25) or stop.is_set() or READY_MARK in recent[0]:
                    return
                password = ""
                script = ""
                with state["lock"]:
                    if state["logged_in"] or state["login_sent"] or stop.is_set():
                        return
                    password = session.get("password") or ""
                    if password and "\n" not in password and "\r" not in password:
                        state["login_sent"] = True
                        script = login_script(session)
                    else:
                        password = ""
                if not password:
                    return
                stamp = generation[0]
                send_azure(
                    b"set +o history\nHISTFILE=/dev/null\nunset HISTFILE\nhistory -c\nrm -f ~/.bash_history\n"
                    b"stty -echo\nbind 'set enable-bracketed-paste off'\n"
                )
                if not wait_for(shell_prompt, stamp, 8) or stop.is_set():
                    send_azure(b"stty echo\n")
                    with state["lock"]:
                        if not state["logged_in"]:
                            state["login_sent"] = False
                    return
                send_azure(b"read -r AZURE_CLIENT_SECRET\n")
                quiet = time.time() + 0.6
                while time.time() < quiet and not stop.is_set():
                    pump_azure()
                    side["pump"]()
                    maybe_azure()
                if stop.is_set():
                    with state["lock"]:
                        if not state["logged_in"]:
                            state["login_sent"] = False
                    return
                secret = password
                password = ""
                send_azure((secret + "\n").encode())
                secret = ""
                send_azure(script.encode())
                script = ""
            except OSError:
                with state["lock"]:
                    if not state["logged_in"]:
                        state["login_sent"] = False

        with BROWSER_LOCK:
            browser_reader = BROWSER_READERS[id(browser)]

        def read_browser():
            if stop.is_set() or not readable(browser, 0.05):
                return
            try:
                opcode, payload = browser_reader.read_frame()
            except TimeoutError:
                return
            except OSError:
                stop.set()
                return
            try:
                if opcode == 0x8:
                    stop.set()
                elif opcode == 0x9:
                    send_browser(payload, 0xA)
                elif opcode in (0x1, 0x2) and payload:
                    send_azure(payload, 0x2)
            except OSError:
                stop.set()

        side["pump"] = read_browser
        sign_in()
        while not stop.is_set():
            pump_azure()
            read_browser()
            maybe_ping()
            maybe_azure()
    except Exception as error:
        message = redact(error, [session.get("password", "")]).encode()
        try:
            send_browser(b"\r\n" + message + b"\r\n", 0x1)
        except OSError:
            pass
    finally:
        stop.set()
        state = shell_state(session)
        with state["lock"]:
            if not state["logged_in"]:
                state["login_sent"] = False
        if azure_sock is not None:
            try:
                azure_sock.close()
            except OSError:
                pass
        try:
            send_browser(struct.pack("!H", 1000), 0x8)
        except OSError:
            pass
        claim["done"].set()


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
    # A buffered read would swallow bytes that arrive with the upgrade request
    # and the websocket parser would never see them.
    rbufsize = 0

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
            "connect-src 'self'; img-src 'self'; frame-src https://fjcloud.github.io; "
            "frame-ancestors 'none'; base-uri 'none'; form-action 'self'",
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
        if path == "/api/terminal":
            self.resume_session()
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
            self._json(401, {"message": "That terminal session is gone. Press Start again."})
            return
        try:
            delete_container(session)
            reset_shell(session)
        except Exception as error:
            self._json(502, {"message": redact(error, [session.get("password", "")])})
            return
        set_status(session, "deleted", f"Removed container {group_name(session)}. The home directory was kept.")
        session["password"] = ""
        session["token"] = ""
        self._json(200, {"status": "deleted"})

    def session_view(self, session):
        return {
            "id": session["id"],
            "status": session["status"],
            "message": session["message"],
            "group": group_name(session) if session.get("resourceGroup") else "",
            "location": session.get("location", ""),
        }

    def session_from_cookie(self):
        supplied = self.cookie_value()
        if not supplied:
            return None
        with SESSIONS_LOCK:
            for session in SESSIONS.values():
                if hmac.compare_digest(supplied, session["secret"]):
                    return session
        return None

    def resume_session(self):
        if not self.origin_allowed():
            self._json(403, {"message": "Open the terminal from this site."})
            return
        session = self.session_from_cookie()
        if not session or session.get("status") == "deleted":
            self._json(401, {"message": "There is no terminal to reattach."})
            return
        self._json(200, self.session_view(session))

    def create_session(self, payload):
        problem = validate_payload(payload)
        if problem:
            self._json(400, {"message": problem})
            return
        incoming = {
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
            "message": "Looking for an existing terminal.",
            "token": "",
            "token_expires": 0,
        }
        try:
            name = group_name(incoming)
        except ValueError as error:
            self._json(400, {"message": str(error)})
            return
        key = client_key(incoming)
        with SESSIONS_LOCK:
            session = None
            for item in SESSIONS.values():
                if item.get("status") != "deleted" and client_key(item) == key:
                    session = item
                    break
            if session is None:
                session = incoming
                SESSIONS[session["id"]] = session
            else:
                session["password"] = incoming["password"]
                session["tenant"] = incoming["tenant"]
                session["subscription"] = incoming["subscription"]
                session["columns"] = incoming["columns"]
                session["rows"] = incoming["rows"]
                if session["status"] == "error":
                    session["status"] = "starting"
                    session["message"] = "Looking for an existing terminal."
        start_background(provision, (session,))
        self._json(
            200,
            {"id": session["id"], "group": name, "status": session["status"]},
            session_cookie(session["secret"], self.https_request()),
        )

    def get_session(self, session_id):
        session = self.authorized(session_id)
        if not session:
            self._json(401, {"message": "That terminal session is gone. Press Start again."})
            return
        self._json(200, self.session_view(session))

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
