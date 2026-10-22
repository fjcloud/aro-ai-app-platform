import json, time, urllib.request

out = "/tmp/qwen27b-offload-probes.jsonl"


def call(case, messages, check, **kwargs):
    p = {
        "model": "qwen27b-offload",
        "temperature": 0,
        "max_tokens": 128,
        "chat_template_kwargs": {"enable_thinking": False},
        "messages": messages,
        **kwargs,
    }
    t = time.monotonic()
    r = urllib.request.Request(
        "http://127.0.0.1:8000/v1/chat/completions",
        data=json.dumps(p).encode(),
        headers={"Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(r, timeout=600) as f:
            d = json.load(f)
        result = {
            "model": "qwen27b-offload",
            "case": case,
            "seconds": round(time.monotonic() - t, 3),
            "usage": d.get("usage"),
            "response": d["choices"][0],
            "passed": check(d["choices"][0]),
        }
    except Exception as e:
        d = {}
        result = {
            "model": "qwen27b-offload",
            "case": case,
            "passed": False,
            "error": str(e),
        }
    line = json.dumps(result)
    print(line, flush=True)
    with open(out, "a") as f:
        f.write(line + "\n")
    return d


call(
    "short-generation",
    [{"role": "user", "content": "Reply exactly GPU-OFFLOAD-OK."}],
    lambda c: "GPU-OFFLOAD-OK" in (c["message"].get("content") or ""),
)
tools = [
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
]
m = [{"role": "user", "content": "Use the multiply tool to calculate 7 times 8."}]


def check(c):
    calls = c["message"].get("tool_calls") or []
    return bool(
        calls
        and calls[0]["function"]["name"] == "multiply"
        and sorted(json.loads(calls[0]["function"]["arguments"]).values()) == [7, 8]
    )


d = call("tool-call", m, check, tools=tools, tool_choice="auto")
if d and check(d["choices"][0]):
    msg = d["choices"][0]["message"]
    call(
        "tool-result",
        m
        + [
            msg,
            {
                "role": "tool",
                "tool_call_id": msg["tool_calls"][0]["id"],
                "content": "56",
            },
        ],
        lambda c: "56" in (c["message"].get("content") or ""),
        tools=tools,
        tool_choice="none",
    )
call(
    "long-prompt",
    [
        {
            "role": "user",
            "content": "Read this filler:\n"
            + ("red hat cloud workshop. " * 2500)
            + "\nNow answer exactly GPU-CONTEXT-OK.",
        }
    ],
    lambda c: "GPU-CONTEXT-OK" in (c["message"].get("content") or ""),
    max_tokens=64,
)
print(json.dumps({"case": "complete"}), flush=True)
