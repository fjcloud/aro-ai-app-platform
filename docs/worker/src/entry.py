"""Cloudflare entry for the existing terminal server.

terminal_server.py still owns Azure, sign-in, and the terminal bridge.
A Worker has no listening socket and cannot start threads, so this file
turns each fetch into the HTTP connection that server already expects.
The session cookie carries the sealed session so a later request can
run on another isolate.
"""

import asyncio
import base64
import hashlib
import hmac
import io
import json
import os
import struct
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

from pyodide.ffi import create_proxy, run_sync
from workers import Request, Response, WorkerEntrypoint, wait_until

sys.path.insert(0, str(Path(__file__).resolve().parents[2]))
import terminal_server

SESSION_FIELDS = (
    "id",
    "secret",
    "guid",
    "clientId",
    "password",
    "tenant",
    "subscription",
    "resourceGroup",
    "columns",
    "rows",
    "status",
    "message",
    "location",
)
KEY = b""
QUEUED_BRIDGE = []
REAL_BRIDGE = terminal_server.bridge


def set_key(value):
    global KEY
    if not value:
        return
    KEY = value.encode() if isinstance(value, str) else bytes(value)


def _stream(nonce, length):
    output = bytearray()
    counter = 0
    while len(output) < length:
        output += hmac.new(KEY, nonce + counter.to_bytes(4, "big"), hashlib.sha256).digest()
        counter += 1
    return output


def seal(session):
    payload = {}
    for field in SESSION_FIELDS:
        value = session.get(field, "")
        if field in ("columns", "rows"):
            value = int(value or 0)
        payload[field] = value
    raw = json.dumps(payload, separators=(",", ":")).encode()
    nonce = os.urandom(16)
    cipher = bytes(left ^ right for left, right in zip(raw, _stream(nonce, len(raw))))
    mac = hmac.new(KEY, b"mac1" + nonce + cipher, hashlib.sha256).digest()
    return base64.urlsafe_b64encode(nonce + mac + cipher).decode().rstrip("=")


def unseal(token):
    if not KEY or not token:
        return None
    try:
        padded = token + "=" * (-len(token) % 4)
        blob = base64.urlsafe_b64decode(padded.encode())
        nonce, mac, cipher = blob[:16], blob[16:48], blob[48:]
        expected = hmac.new(KEY, b"mac1" + nonce + cipher, hashlib.sha256).digest()
        if not hmac.compare_digest(mac, expected):
            return None
        raw = bytes(left ^ right for left, right in zip(cipher, _stream(nonce, len(cipher))))
        payload = json.loads(raw.decode())
    except Exception:
        return None
    if not isinstance(payload, dict) or not payload.get("id") or not payload.get("secret"):
        return None
    return payload


def session_cookie(secret, secure):
    session = None
    with terminal_server.SESSIONS_LOCK:
        for item in terminal_server.SESSIONS.values():
            candidate = item.get("secret") or ""
            if len(candidate) == len(secret) and hmac.compare_digest(candidate, secret):
                session = item
                break
    if session is None or not KEY:
        return terminal_server._original_session_cookie(secret, secure)
    parts = [
        f"{terminal_server.COOKIE}={seal(session)}",
        "HttpOnly",
        "SameSite=Strict",
        "Path=/",
        "Max-Age=28800",
    ]
    if secure:
        parts.append("Secure")
    return "; ".join(parts)


def authorized(self, session_id):
    supplied = self.cookie_value()
    if not supplied:
        return None
    with terminal_server.SESSIONS_LOCK:
        current = terminal_server.SESSIONS.get(session_id)
    restored = unseal(supplied)
    if current is not None:
        secret = current.get("secret") or ""
        if len(supplied) == len(secret):
            try:
                if hmac.compare_digest(supplied, secret):
                    return current
            except Exception:
                return None
        if restored and restored.get("id") == session_id and len(restored.get("secret", "")) == len(secret):
            if hmac.compare_digest(restored["secret"], secret):
                return current
        return None
    if not restored or restored.get("id") != session_id:
        return None
    with terminal_server.SESSIONS_LOCK:
        terminal_server.SESSIONS[session_id] = restored
    return restored


def queue_bridge(browser, session, columns, rows):
    with terminal_server.BROWSER_LOCK:
        reader = terminal_server.BROWSER_READERS.get(id(browser))
    QUEUED_BRIDGE.append((browser, session, columns, rows, reader))


def worker_urlopen(request, data=None, timeout=None, **kwargs):
    # The Worker socket layer rejects urllib's HTTPS handshake. Use fetch,
    # which has no browser Origin, and keep the Azure calls in terminal_server.py.
    import js
    from js import Object
    from pyodide.ffi import to_js

    headers = {}
    for key, value in request.header_items():
        headers[key] = value
    options = {"method": request.get_method(), "headers": headers}
    if request.data is not None:
        options["body"] = request.data.decode()
    response = run_sync(js.fetch(request.full_url, to_js(options, dict_converter=Object.fromEntries)))
    raw = run_sync(response.arrayBuffer()).to_bytes()
    status = int(response.status)
    if status >= 400:
        raise urllib.error.HTTPError(
            request.full_url, status, str(response.statusText or ""), None, io.BytesIO(raw)
        )
    return _Fetched(status, raw)


class _Fetched:
    def __init__(self, status, data):
        self.status = status
        self._data = data

    def read(self):
        return self._data

    def __enter__(self):
        return self

    def __exit__(self, *_args):
        return False


def install_patches():
    if getattr(terminal_server, "_worker_patched", False):
        return
    terminal_server._original_session_cookie = terminal_server.session_cookie
    terminal_server.session_cookie = session_cookie
    terminal_server.Handler.authorized = authorized
    terminal_server.bridge = queue_bridge
    original_sleep = time.sleep

    def yielding_sleep(seconds):
        if seconds <= 0:
            return
        def executor(resolve, _reject):
            js_set_timeout(resolve, int(seconds * 1000))
        proxy = create_proxy(executor)
        try:
            run_sync(js_promise(proxy))
        finally:
            proxy.destroy()

    time.sleep = yielding_sleep
    urllib.request.urlopen = worker_urlopen
    terminal_server.open_azure_socket = worker_open_azure_socket
    terminal_server._worker_sleep = original_sleep
    terminal_server._worker_patched = True


def js_promise(executor):
    import js
    return js.Promise.new(executor)


def js_set_timeout(callback, millis):
    import js
    js.setTimeout(callback, millis)


class Conn:
    def __init__(self, request):
        self._read = bytearray(request)
        self._read_at = 0
        self._out = bytearray()
        self.closed = False
        self.timeout = None
        self._resolve = None
        self.head = request.startswith(b"HEAD ")

    def settimeout(self, timeout):
        self.timeout = timeout

    def setsockopt(self, *args):
        return None

    def makefile(self, mode="rb", buffering=-1):
        return SocketFile(self)

    def _take(self, size):
        if self._read_at >= len(self._read):
            return None
        end = min(len(self._read), self._read_at + size)
        data = bytes(self._read[self._read_at:end])
        self._read_at = end
        return data

    def recv(self, size):
        data = self._take(size)
        if data is not None:
            return data
        if self.closed:
            return b""
        self._wait()
        data = self._take(size)
        if data is not None:
            return data
        if self.closed:
            return b""
        raise TimeoutError("socket timed out")

    def _wait(self):
        settled = {"done": False}

        def executor(resolve, _reject):
            if self._read_at < len(self._read) or self.closed:
                resolve(None)
                return
            self._resolve = resolve
            if self.timeout is not None:
                js_set_timeout(resolve, max(1, int(self.timeout * 1000)))

        proxy = create_proxy(executor)
        try:
            run_sync(js_promise(proxy))
        finally:
            self._resolve = None
            proxy.destroy()
            settled["done"] = True

    def sendall(self, data):
        self._out += data

    def push(self, data):
        self._read += data
        resolve = self._resolve
        self._resolve = None
        if resolve is not None:
            resolve(None)

    def pull(self):
        data = bytes(self._out)
        self._out.clear()
        return data

    def take_response(self):
        raw = bytes(self._out)
        marker = raw.find(b"\r\n\r\n")
        if marker < 0:
            return None
        head = raw[:marker].decode("iso-8859-1", "replace")
        rest = raw[marker + 4:]
        lines = head.split("\r\n")
        status = int(lines[0].split(" ", 2)[1])
        headers = []
        length = 0
        for line in lines[1:]:
            name, _, value = line.partition(":")
            headers.append((name.strip(), value.strip()))
            if name.lower() == "content-length":
                length = int(value.strip() or "0")
        if status != 101 and len(rest) < length:
            if self.head:
                body, leftover = b"", rest
                self._out = bytearray(leftover)
                return status, headers, body
            return None
        if status == 101:
            body, leftover = b"", rest
        else:
            body, leftover = rest[:length], rest[length:]
        self._out = bytearray(leftover)
        return status, headers, body

    def shutdown(self, how):
        self.close()

    def close(self):
        self.closed = True
        resolve = self._resolve
        self._resolve = None
        if resolve is not None:
            resolve(None)


class SocketFile:
    def __init__(self, conn):
        self.conn = conn
        self.closed = False

    def read(self, size=-1):
        if size is None or size < 0:
            chunks = []
            while True:
                chunk = self.conn.recv(65536)
                if not chunk:
                    break
                chunks.append(chunk)
            return b"".join(chunks)
        return self.conn.recv(size)

    def readline(self, size=-1):
        data = bytearray()
        while True:
            if size >= 0 and len(data) >= size:
                break
            char = self.conn.recv(1)
            if not char:
                break
            data += char
            if char == b"\n":
                break
        return bytes(data)

    def write(self, data):
        if isinstance(data, str):
            data = data.encode()
        self.conn.sendall(data)
        return len(data)

    def flush(self):
        return None

    def close(self):
        self.closed = True


class OutFrames:
    def __init__(self):
        self.buffer = b""

    def feed(self, data):
        self.buffer += data
        frames = []
        while True:
            if len(self.buffer) < 2:
                break
            length = self.buffer[1] & 0x7F
            start = 2
            if length == 126:
                if len(self.buffer) < 4:
                    break
                length = struct.unpack("!H", self.buffer[2:4])[0]
                start = 4
            elif length == 127:
                if len(self.buffer) < 10:
                    break
                length = struct.unpack("!Q", self.buffer[2:10])[0]
                start = 10
            if len(self.buffer) < start + length:
                break
            opcode = self.buffer[0] & 0x0F
            payload = self.buffer[start:start + length]
            self.buffer = self.buffer[start + length:]
            frames.append((opcode, payload))
        return frames


def client_frame(payload, opcode):
    mask = os.urandom(4)
    length = len(payload)
    if length < 126:
        header = bytes([0x80 | opcode, 0x80 | length])
    elif length < 65536:
        header = bytes([0x80 | opcode, 0x80 | 126]) + struct.pack("!H", length)
    else:
        header = bytes([0x80 | opcode, 0x80 | 127]) + struct.pack("!Q", length)
    masked = bytes(byte ^ mask[index % 4] for index, byte in enumerate(payload))
    return header + mask + masked


def request_bytes(request, body):
    url = urllib.parse.urlparse(request.url)
    path = url.path or "/"
    if url.query:
        path += "?" + url.query
    lines = [f"{request.js_object.method} {path} HTTP/1.1", "Connection: close"]
    saw_proto = False
    saw_forwarded = False
    for name, value in request.js_object.headers:
        lines.append(f"{name}: {value}")
        lowered = name.lower()
        if lowered == "x-forwarded-proto":
            saw_proto = True
        elif lowered == "x-forwarded-for":
            saw_forwarded = True
    if url.scheme == "https" and not saw_proto:
        lines.append("X-Forwarded-Proto: https")
    if not saw_forwarded:
        client = getattr(request.js_object.headers, "get", None)
        connecting = ""
        if client:
            connecting = request.js_object.headers.get("cf-connecting-ip") or ""
        if connecting:
            lines.append(f"X-Forwarded-For: {connecting}")
    if body and not any(line.lower().startswith("content-length:") for line in lines):
        lines.append(f"Content-Length: {len(body)}")
    return ("\r\n".join(lines) + "\r\n\r\n").encode() + body


def forward_frames(server, frames):
    for opcode, payload in frames:
        if opcode == 1:
            server.send(payload.decode("utf-8", "replace"))
        elif opcode == 2:
            server.send(payload)


def pop_client_frame(buffer):
    if len(buffer) < 2:
        return None, buffer
    opcode = buffer[0] & 0x0F
    masked = bool(buffer[1] & 0x80)
    length = buffer[1] & 0x7F
    start = 2
    if length == 126:
        if len(buffer) < 4:
            return None, buffer
        length = struct.unpack("!H", buffer[2:4])[0]
        start = 4
    elif length == 127:
        if len(buffer) < 10:
            return None, buffer
        length = struct.unpack("!Q", buffer[2:10])[0]
        start = 10
    extra = 4 if masked else 0
    if len(buffer) < start + extra + length:
        return None, buffer
    if masked:
        key = buffer[start:start + 4]
        start += 4
        payload = bytes(byte ^ key[index % 4] for index, byte in enumerate(buffer[start:start + length]))
    else:
        payload = bytes(buffer[start:start + length])
    return (opcode, payload), buffer[start + length:]


class AzureWire:
    """Byte socket the existing bridge already speaks, backed by a Worker WebSocket."""

    def __init__(self, websocket):
        self.websocket = websocket
        self.inbound = Conn(b"")
        self.pending = b""
        self.timeout = None

        def on_message(event):
            data = event.data
            if isinstance(data, str):
                payload, opcode = data.encode(), 1
            else:
                payload = data.to_bytes() if hasattr(data, "to_bytes") else bytes(data)
                opcode = 2
            self.inbound.push(terminal_server.encode_frame(payload, opcode, False))

        def on_close(_event):
            self.inbound.close()

        self._message_proxy = create_proxy(on_message)
        self._close_proxy = create_proxy(on_close)
        websocket.binaryType = "arraybuffer"
        websocket.onmessage = self._message_proxy
        websocket.onclose = self._close_proxy

    def settimeout(self, timeout):
        self.timeout = timeout
        self.inbound.timeout = timeout

    def setsockopt(self, *args):
        return None

    def recv(self, size):
        self.inbound.timeout = self.timeout
        return self.inbound.recv(size)

    def sendall(self, data):
        self.pending += data
        while True:
            frame, self.pending = pop_client_frame(self.pending)
            if frame is None:
                return
            opcode, payload = frame
            if opcode == 1:
                self.websocket.send(payload.decode("utf-8", "replace"))
            elif opcode in (2, 9, 10):
                self.websocket.send(payload)
            elif opcode == 8:
                try:
                    self.websocket.close()
                except Exception:
                    pass

    def shutdown(self, how):
        self.close()

    def close(self):
        self.inbound.close()
        try:
            self.websocket.close()
        except Exception:
            pass


def worker_open_azure_socket(uri):
    import js
    from js import Object
    from pyodide.ffi import to_js

    parsed = urllib.parse.urlsplit(uri)
    if parsed.scheme == "wss":
        scheme = "https"
    elif parsed.scheme == "ws":
        scheme = "http"
    else:
        raise RuntimeError("Azure returned an unexpected terminal address")
    http_url = urllib.parse.urlunsplit(parsed._replace(scheme=scheme))
    response = run_sync(js.fetch(http_url, to_js({
        "headers": {"Upgrade": "websocket"},
    }, dict_converter=Object.fromEntries)))
    websocket = getattr(response, "webSocket", None)
    if websocket is None:
        raise RuntimeError("Azure did not open a terminal socket")
    wire = AzureWire(websocket)
    websocket.accept()
    return wire, terminal_server.FrameReader(wire)


class Server:
    timeout = None


class Default(WorkerEntrypoint):
    async def fetch(self, request):
        request = request if hasattr(request, "js_object") else Request(request)
        install_patches()
        key = ""
        try:
            key = self.env.SESSION_KEY
        except Exception:
            key = ""
        set_key(key)
        body = b""
        if request.js_object.method not in ("GET", "HEAD"):
            body = await request.bytes()
        conn = Conn(request_bytes(request, body))
        try:
            terminal_server.Handler(conn, ("0.0.0.0", 0), Server())
            ready = conn.take_response()
        except Exception as exc:
            return Response("terminal request failed", status=502)
        if ready is None:
            return Response("terminal request failed", status=502)
        status, headers, response_body = ready
        if status != 101:
            return Response(response_body, status=status, headers=headers)

        from js import WebSocketPair

        client, server = WebSocketPair.new().object_values()
        server.accept()
        server.binaryType = "arraybuffer"
        parser = OutFrames()
        job = QUEUED_BRIDGE.pop() if QUEUED_BRIDGE else None

        def on_message(event):
            data = event.data
            if isinstance(data, str):
                conn.push(client_frame(data.encode(), 1))
                return
            raw = data.to_bytes() if hasattr(data, "to_bytes") else bytes(data)
            conn.push(client_frame(raw, 2))

        def on_close(_event):
            conn.close()

        message_proxy = create_proxy(on_message)
        close_proxy = create_proxy(on_close)
        server.onmessage = message_proxy
        server.onclose = close_proxy

        async def drive():
            bridge_future = None
            try:
                if job is not None:
                    browser, session, columns, rows, reader = job

                    async def run_bridge():
                        await asyncio.sleep(0)
                        with terminal_server.BROWSER_LOCK:
                            terminal_server.BROWSER_READERS[id(browser)] = reader
                        try:
                            REAL_BRIDGE(browser, session, columns, rows)
                        finally:
                            with terminal_server.BROWSER_LOCK:
                                terminal_server.BROWSER_READERS.pop(id(browser), None)
                            conn.close()

                    bridge_future = asyncio.ensure_future(run_bridge())
                while True:
                    forward_frames(server, parser.feed(conn.pull()))
                    if bridge_future is not None and bridge_future.done():
                        forward_frames(server, parser.feed(conn.pull()))
                        break
                    if conn.closed and not conn.pull():
                        break
                    await asyncio.sleep(0.02)
            finally:
                try:
                    server.close()
                except Exception:
                    pass
                message_proxy.destroy()
                close_proxy.destroy()

        wait_until(create_proxy(asyncio.ensure_future(drive())))
        return Response(None, status=101, web_socket=client)
