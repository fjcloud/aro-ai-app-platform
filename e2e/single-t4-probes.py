"""Functional model probes, not an equivalence or throughput benchmark."""

import json
import os
from pathlib import Path
import ssl
import time
import urllib.request

base = os.environ.get("BENCH_BASE", "http://127.0.0.1:8000/v1")
model = os.environ["BENCH_MODEL"]
out = Path(os.environ.get("BENCH_OUTPUT", "/tmp/" + model + "-probes.jsonl"))
context = (
    ssl._create_unverified_context()
    if base.startswith("https://127.0.0.1")
    else ssl.create_default_context()
)


def emit(d):
    d = {"model": model, **d}
    line = json.dumps(d, ensure_ascii=False)
    print(line, flush=True)
    with out.open("a") as f:
        f.write(line + "\n")


def post(messages, case, max_tokens=512, **extra):
    payload = {
        "model": model,
        "messages": messages,
        "temperature": 0,
        "max_tokens": max_tokens,
        "chat_template_kwargs": {"enable_thinking": False},
        "cache_salt": "single-t4-20261022-" + case,
        **extra,
    }
    req = urllib.request.Request(
        base + "/chat/completions",
        data=json.dumps(payload).encode(),
        headers={"Content-Type": "application/json"},
    )
    start = time.monotonic()
    with urllib.request.urlopen(req, context=context, timeout=360) as r:
        d = json.load(r)
    return d, {
        "case": case,
        "seconds": round(time.monotonic() - start, 3),
        "usage": d.get("usage"),
        "response": d["choices"][0],
    }


def simple(case, prompt, check, max_tokens=512):
    try:
        d, r = post([{"role": "user", "content": prompt}], case, max_tokens)
        r["source_returned" if case.startswith("go-") else "passed"] = (
            bool(check(d["choices"][0]["message"].get("content") or ""))
            and d["choices"][0]["finish_reason"] == "stop"
        )
        emit(r)
    except Exception as e:
        emit(
            {
                "case": case,
                "source_returned" if case.startswith("go-") else "passed": False,
                "error": str(e),
            }
        )


for i in range(3):
    simple(
        "five-go-tips-" + str(i + 1),
        "Write exactly five short numbered tips for building a reliable Go HTTP service.",
        lambda s: bool(s),
        256,
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
                "additionalProperties": False,
            },
        },
    }
]
messages = [
    {"role": "user", "content": "Use the multiply tool to calculate 7 times 8."}
]
try:
    d, r = post(messages, "tool-call", tools=tools, tool_choice="auto")
    calls = d["choices"][0]["message"].get("tool_calls", [])
    args = json.loads(calls[0]["function"]["arguments"]) if calls else {}
    r["passed"] = bool(
        calls
        and calls[0]["function"]["name"] == "multiply"
        and (args.get("a"), args.get("b")) in ((7, 8), (8, 7))
    )
    emit(r)
    if r["passed"]:
        d, r = post(
            messages
            + [
                d["choices"][0]["message"],
                {"role": "tool", "tool_call_id": calls[0]["id"], "content": "56"},
            ],
            "tool-result",
            tools=tools,
            tool_choice="none",
        )
        r["passed"] = (
            "56" in (d["choices"][0]["message"].get("content") or "")
            and d["choices"][0]["finish_reason"] == "stop"
        )
        emit(r)
except Exception as e:
    emit({"case": "tool-call-or-result", "passed": False, "error": str(e)})
for verb, case in [("Read", "long-prompt"), ("Scan", "long-prompt-new-prefix")]:
    simple(
        case,
        verb
        + " this filler:\n"
        + ("red hat cloud workshop. " * 2500)
        + "\nNow answer exactly GPU-CONTEXT-OK.",
        lambda s: "GPU-CONTEXT-OK" in s,
        64,
    )
simple(
    "json-instructions",
    'Return only a JSON object with these exact keys and values: "cloud" is "azure", "gpu_count" is the integer 1, "runtime" is "vllm". No explanation or markdown.',
    lambda s: json.loads(s) == {"cloud": "azure", "gpu_count": 1, "runtime": "vllm"},
    256,
)
for case, prompt in [
    (
        "go-first-unique",
        "Write Go source for package candidate defining FirstUnique(values []string) (string, bool). Return the first string occurring exactly once in the entire input, preserving input order. Return empty string and false if none exists. Treat empty strings as valid values. Handle nil input. No imports, I/O, or other functions. Output only Go source, without markdown.",
    ),
    (
        "go-retry-after",
        "Write Go source for package candidate defining ParseRetryAfter(value string, now time.Time) (time.Duration, error). Parse an HTTP Retry-After header: trim surrounding whitespace; accept only nonnegative decimal integer seconds or an HTTP date in a format accepted by http.ParseTime. Reject signed integers, negative values, invalid text and duration overflow. For a valid past date return zero and nil. Do not round durations for future dates. Include needed imports and the function. No I/O, init functions, or global variables. Output only Go source, without markdown.",
    ),
]:
    # This checks source delivery only. Compile and run the generated Go against
    # the fixtures in e2e/model-code-tests before judging code correctness.
    simple(case, prompt, lambda s: "package candidate" in s, 1536)
emit({"case": "complete"})
