import json, time, urllib.request, os

base = "http://127.0.0.1:8000/v1"
model = os.environ.get("BENCH_MODEL", "bonsai2")
payload = {
    "model": model,
    "stream": True,
    "temperature": 0,
    "max_tokens": 256,
    "chat_template_kwargs": {"enable_thinking": False},
    "messages": [
        {"role": "user", "content": "Use the multiply tool to calculate 7 times 8."}
    ],
    "tools": [
        {
            "type": "function",
            "function": {
                "name": "multiply",
                "description": "Multiply two integers",
                "parameters": {
                    "type": "object",
                    "properties": {"a": {"type": "integer"}, "b": {"type": "integer"}},
                    "required": ["a", "b"],
                },
            },
        }
    ],
    "tool_choice": "auto",
}
r = urllib.request.Request(
    base + "/chat/completions",
    data=json.dumps(payload).encode(),
    headers={"Content-Type": "application/json"},
)
start = time.monotonic()
calls = {}
finish = None
with urllib.request.urlopen(r, timeout=180) as f:
    for raw in f:
        line = raw.decode().strip()
        if not line.startswith("data: ") or line == "data: [DONE]":
            continue
        d = json.loads(line[6:])
        for choice in d.get("choices", []):
            finish = choice.get("finish_reason") or finish
            for call in choice.get("delta", {}).get("tool_calls", []):
                c = calls.setdefault(call["index"], {"name": "", "arguments": ""})
                fn = call.get("function", {})
                c["name"] += fn.get("name") or ""
                c["arguments"] += fn.get("arguments") or ""
passed = finish == "tool_calls" and any(
    c["name"] == "multiply" and sorted(json.loads(c["arguments"]).values()) == [7, 8]
    for c in calls.values()
)
print(
    json.dumps(
        {
            "model": model,
            "case": "streaming-tool-call",
            "passed": passed,
            "seconds": round(time.monotonic() - start, 3),
            "finish_reason": finish,
            "tool_calls": calls,
        }
    ),
    flush=True,
)
assert passed, calls
