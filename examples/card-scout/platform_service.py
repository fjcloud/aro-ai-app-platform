#!/usr/bin/env python3
"""Prepared batch analysis, native OGX research and read-only MCP for Card Scout."""
import asyncio
import base64
import copy
import datetime
import hashlib
import json
import os
import pathlib
import threading
import time
import urllib.request

import uvicorn
from opentelemetry.propagate import inject
from opentelemetry.trace import Status, StatusCode
from starlette.concurrency import run_in_threadpool
from starlette.responses import FileResponse, JSONResponse
from starlette.routing import Route

from analyze_batch import QUESTIONS
from market_tools import mcp
from observability import PROVIDER, TRACER, record, trace_link
from prepare_dataset import CACHE, ROOT
from probe_agent import market_evidence

INPUTS = json.loads((ROOT / "data" / "inputs.json").read_text())
REFERENCES = {r["id"]: r for r in json.loads((ROOT / "data" / "references.json").read_text())["items"]}
ITEMS = {r["id"]: r for r in INPUTS["items"]}
DATA = pathlib.Path(os.getenv("DATA_DIR", "/data"))
DATA.mkdir(parents=True, exist_ok=True)
STATE_FILE = DATA / "card-scout-v1.json"
SNAPSHOT = hashlib.sha256((ROOT / "data" / "inputs.json").read_bytes()).hexdigest()
LOCK = threading.RLock()
RESEARCH_LOCK = threading.Lock()
STATE = {"snapshot": SNAPSHOT, "results": {}, "prices": {}, "running": False, "analysis_trace_id": "", "analysis_trace_url": ""}
if STATE_FILE.exists():
    saved = json.loads(STATE_FILE.read_text())
    if saved.get("snapshot") == SNAPSHOT:
        STATE.update(saved)
        STATE["running"] = False


def persist():
    temporary = STATE_FILE.with_suffix(".tmp")
    temporary.write_text(json.dumps(STATE))
    temporary.replace(STATE_FILE)


def post_json(url, payload, headers=None):
    request = urllib.request.Request(url, data=json.dumps(payload).encode(), headers={"Content-Type": "application/json", **(headers or {})})
    with urllib.request.urlopen(request, timeout=120) as response:
        raw = response.read(2 * 1024 * 1024 + 1)
    if len(raw) > 2 * 1024 * 1024:
        raise ValueError("Upstream response exceeds limit")
    return json.loads(raw)


def now():
    return datetime.datetime.now(datetime.timezone.utc).isoformat()


def validate_answer(reply):
    answers = reply.get("answers", {})
    expected = {"card_kind": {"pokemon_tcg", "other_card", "not_a_card", "uncertain"},
                "rarity": {"common", "uncommon", "rare_or_higher", "not_applicable", "uncertain"}}
    for name, options in expected.items():
        answer = answers.get(name, {})
        probabilities = answer.get("probabilities", {})
        if answer.get("choice") not in options or set(probabilities) != options:
            raise ValueError("Decision response has an invalid choice or missing probabilities")
        if any(not isinstance(p, (int, float)) or not 0 <= p <= 1 for p in probabilities.values()) or abs(sum(probabilities.values()) - 1) > 0.02:
            raise ValueError("Invalid decision probabilities")
    return answers


def analyze():
    with TRACER.start_as_current_span("cards.analyze_batch", attributes={"mlflow.spanType": "CHAIN"}) as root:
        tid, link = trace_link(root)
        record(root, "spanInputs", {"dataset_snapshot": SNAPSHOT, "images": len(ITEMS), "model": os.getenv("DECISION_MODEL", "vllm-sr/d3-mini")})
        with LOCK:
            STATE["analysis_trace_id"], STATE["analysis_trace_url"] = tid, link
        try:
            for item_id, item in ITEMS.items():
                with LOCK:
                    if STATE["results"].get(item_id, {}).get("status") == "done":
                        continue
                    STATE["results"][item_id] = {"status": "analyzing"}
                with TRACER.start_as_current_span("d3.classify_card", attributes={"mlflow.spanType": "LLM", "gen_ai.request.model": os.getenv("DECISION_MODEL", "vllm-sr/d3-mini")}) as span:
                    record(span, "spanInputs", {"item_id": item_id, "image_sha256": item["image_sha256"], "questions": QUESTIONS})
                    started = time.monotonic()
                    try:
                        image = (CACHE / item["image_sha256"]).read_bytes()
                        if hashlib.sha256(image).hexdigest() != item["image_sha256"]:
                            raise ValueError("Dataset image checksum mismatch")
                        mime = "image/jpeg" if image[:2] == b"\xff\xd8" else "image/png"
                        payload = {"model": os.getenv("DECISION_MODEL", "vllm-sr/d3-mini"),
                                   "state": "Inspect the attached image and classify the card and its official printed rarity. Judge only from visible evidence.",
                                   "images": [f"data:{mime};base64," + base64.b64encode(image).decode()], "questions": QUESTIONS}
                        reply = post_json(os.environ["DECISION_URL"], payload)
                        answers = validate_answer(reply)
                        result = {"status": "done", "answers": answers, "elapsed_seconds": round(time.monotonic() - started, 3)}
                        record(span, "spanOutputs", {"answers": answers, "usage": reply.get("usage")})
                    except Exception:
                        result = {"status": "error", "error": "Model analysis failed. Retry the batch; no result was substituted."}
                        span.set_status(Status(StatusCode.ERROR, result["error"]))
                        record(span, "spanOutputs", result)
                    with LOCK:
                        STATE["results"][item_id] = result
                        persist()
            record(root, "spanOutputs", {"completed": sum(r["status"] == "done" for r in STATE["results"].values()), "errors": sum(r["status"] == "error" for r in STATE["results"].values())})
        finally:
            with LOCK:
                STATE["running"] = False
                persist()
    PROVIDER.force_flush(timeout_millis=10000)


def public_batch():
    with LOCK:
        state = copy.deepcopy(STATE)
    items = []
    for item_id, reference in REFERENCES.items():
        decision = state["results"].get(item_id, {"status": "pending"})
        answers = decision.get("answers", {})
        kind = answers.get("card_kind", {}).get("choice")
        rarity = answers.get("rarity", {}).get("choice")
        items.append({"id": item_id, "image_url": "/assets/" + item_id,
                      "name": reference["name"], "set_name": reference.get("set_name"),
                      "year": reference.get("release_date", "")[:4] or None,
                      "status": decision["status"], "error": decision.get("error"),
                      "kind": kind, "rarity": rarity, "probabilities": {k: v.get("probabilities") for k, v in answers.items()},
                      "needs_review": kind == "uncertain" or rarity == "uncertain",
                      "research": state["prices"].get(item_id)})
    tiers = {"rare_or_higher": 4, "uncertain": 3, "uncommon": 2, "common": 1}
    items.sort(key=lambda i: (tiers.get(i["rarity"], 0) if i["kind"] == "pokemon_tcg" else 0,
                             (i["probabilities"].get("rarity") or {}).get("rare_or_higher", 0)), reverse=True)
    return {"dataset": {"name": "Mixed-era collection", "snapshot_at": INPUTS["snapshot_at"], "items": len(ITEMS), "pokemon_cards": 48, "controls": 3, "years": "1999–2023", "identity_method": "Known catalogue references in the prepared dataset"},
            "running": state["running"], "completed": sum(i["status"] == "done" for i in items),
            "failed": sum(i["status"] == "error" for i in items), "items": items,
            "trace_id": state["analysis_trace_id"], "trace_url": state["analysis_trace_url"]}


def visible_response(reply):
    output = []
    for item in reply.get("output", []):
        if item.get("type") == "mcp_call":
            output.append({k: item.get(k) for k in ("type", "name", "server_label", "arguments", "output", "error")})
        elif item.get("type") == "message":
            output.append({"type": "message", "text": "\n".join(c.get("text", "") for c in item.get("content", []) if c.get("type") == "output_text")})
    return {"status": reply.get("status"), "output": output, "usage": reply.get("usage")}


def research(item_id):
    with RESEARCH_LOCK:
        with LOCK:
            decision = STATE["results"].get(item_id, {})
            if decision.get("answers", {}).get("card_kind", {}).get("choice") != "pokemon_tcg":
                raise ValueError("Analyze this item as a Pokemon card before checking its price")
            predicted = decision["answers"]["rarity"]["choice"]
        with TRACER.start_as_current_span("card.research", attributes={"mlflow.spanType": "CHAIN"}) as root:
            record(root, "spanInputs", {"item_id": item_id, "predicted_rarity": predicted})
            with TRACER.start_as_current_span("agent.responses", attributes={"mlflow.spanType": "LLM", "gen_ai.request.model": os.environ["AGENT_MODEL"]}) as span:
                carrier = {}
                inject(carrier)
                payload = {"model": os.environ["AGENT_MODEL"],
                           "input": f"Check {item_id} from our batch. D3 predicted {predicted}. Verify its rarity and report its Cardmarket EUR trend and 7-day average.",
                           "instructions": (ROOT / "agent-instructions.txt").read_text(),
                           "tools": [{"type": "mcp", "server_label": "card_scout_market", "server_url": os.environ["CARD_MCP_URL"], "headers": carrier, "require_approval": "never", "allowed_tools": ["get_card_market_data"]}],
                           "tool_choice": {"type": "mcp", "server_label": "card_scout_market", "name": "get_card_market_data"},
                           "reasoning": {"effort": "none"}, "max_output_tokens": 1000, "max_infer_iters": 4,
                           "parallel_tool_calls": False, "store": False, "stream": False}
                record(span, "spanInputs", payload)
                key = pathlib.Path(os.getenv("AGENT_KEY_FILE", "/etc/model/api-key")).read_text().strip()
                try:
                    reply = post_json(os.environ["AGENT_URL"].rstrip("/") + "/responses", payload, {"X-OGX-Provider-Data": json.dumps({"vllm_api_token": key})})
                    if key and key in json.dumps(reply):
                        raise ValueError("Unexpected credential in model response")
                    evidence = market_evidence(reply, item_id)
                    if evidence["status"] != "ok" or evidence["card_id"] != REFERENCES[item_id]["card_id"]:
                        raise ValueError("Agent evidence does not match this catalogue card")
                    record(span, "spanOutputs", visible_response(reply))
                    usage = reply.get("usage") or {}
                    for name in ("input_tokens", "output_tokens"):
                        if isinstance(usage.get(name), int):
                            span.set_attribute("gen_ai.usage." + name, usage[name])
                except Exception:
                    span.set_status(Status(StatusCode.ERROR, "Agent lookup failed or lacked matching tool evidence"))
                    raise RuntimeError("The agent could not verify this card's market data. Please retry.") from None
            provider = (evidence.get("pricing") or {}).get("cardmarket") or {}
            euro = provider.get("unit") == "EUR"
            result = {"status": "ok", "card": evidence["card"], "currency": "EUR" if euro else None,
                      "trend": provider.get("trend") if euro else None, "average_7d": provider.get("avg7") if euro else None,
                      "provider": "Cardmarket", "provider_updated_at": provider.get("updated"),
                      "source_url": evidence["source_url"], "retrieved_at": evidence["retrieved_at"],
                      "summary": "\n".join(i["text"] for i in visible_response(reply)["output"] if i["type"] == "message"),
                      "note": "Market reference; language, finish and condition can change the value."}
            result["trace_id"], result["trace_url"] = trace_link(root)
            record(root, "spanOutputs", result)
            with LOCK:
                STATE["prices"][item_id] = result
                persist()
        PROVIDER.force_flush(timeout_millis=10000)
        return result


async def batch(request):
    return JSONResponse(public_batch())


async def start_analysis(request):
    with LOCK:
        pending = any(STATE["results"].get(i, {}).get("status") != "done" for i in ITEMS)
        if not STATE["running"] and pending:
            STATE["running"] = True
            threading.Thread(target=analyze, daemon=True).start()
    return JSONResponse(public_batch(), status_code=202)


async def card_research(request):
    item_id = request.path_params["item_id"]
    if item_id not in ITEMS:
        return JSONResponse({"error": "Unknown batch item"}, status_code=404)
    try:
        result = await run_in_threadpool(research, item_id)
        return JSONResponse(result)
    except ValueError as exc:
        return JSONResponse({"error": str(exc)}, status_code=409)
    except Exception:
        return JSONResponse({"error": "Price research failed. No price was substituted; please retry."}, status_code=502)


async def asset(request):
    item = ITEMS.get(request.path_params["item_id"])
    if item is None:
        return JSONResponse({"error": "Unknown batch item"}, status_code=404)
    path = CACHE / item["image_sha256"]
    if not path.is_file():
        return JSONResponse({"error": "Dataset image is unavailable"}, status_code=503)
    mime = "image/jpeg" if path.read_bytes()[:2] == b"\xff\xd8" else "image/png"
    return FileResponse(path, media_type=mime, headers={"Cache-Control": "public,max-age=86400"})


async def health(request):
    return JSONResponse({"status": "ok"})


app = mcp.streamable_http_app()
app.router.routes.extend([
    Route("/healthz", health), Route("/api/batch", batch),
    Route("/api/batch/analyze", start_analysis, methods=["POST"]),
    Route("/api/cards/{item_id}/research", card_research, methods=["POST"]),
    Route("/assets/{item_id}", asset),
])


if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=int(os.getenv("PORT", "8080")), access_log=False)
