"""Check failures cannot become invented classifications or unsupported prices."""
import copy
import json
import os
import pathlib
import tempfile
import unittest
from unittest.mock import patch

TEMP = tempfile.TemporaryDirectory()
os.environ.setdefault("DATA_DIR", TEMP.name)
import platform_service as service
from probe_agent import market_evidence


def response_for(item_id="item-039"):
    evidence = {"status": "ok", "item_id": item_id, "card_id": "sv01-201", "source_url": "https://api.tcgdex.net/v2/en/cards/sv01-201", "pricing": {"cardmarket": {"unit": "EUR", "trend": 4.09}}}
    return {"status": "completed", "output": [{"type": "mcp_call", "server_label": "card_scout_market", "name": "get_card_market_data", "error": None, "arguments": json.dumps({"item_id": item_id}), "output": json.dumps(evidence)}, {"type": "message", "content": [{"type": "output_text", "text": "The price is €9999"}]}]}


class EvidenceTests(unittest.TestCase):
    def test_price_is_from_tool_not_model_claim(self):
        self.assertEqual(market_evidence(response_for(), "item-039")["pricing"]["cardmarket"]["trend"], 4.09)

    def test_answer_without_tool_is_rejected(self):
        reply = response_for()
        reply["output"] = reply["output"][1:]
        with self.assertRaises(ValueError):
            market_evidence(reply, "item-039")

    def test_wrong_item_incomplete_and_wrong_source_are_rejected(self):
        for mode in ("item", "status", "source"):
            reply = response_for()
            if mode == "item":
                reply["output"][0]["arguments"] = '{"item_id":"item-001"}'
            elif mode == "status":
                reply["status"] = "incomplete"
            else:
                evidence = json.loads(reply["output"][0]["output"])
                evidence["source_url"] = "https://example.com/another-card"
                reply["output"][0]["output"] = json.dumps(evidence)
            with self.subTest(mode=mode), self.assertRaises(ValueError):
                market_evidence(reply, "item-039")


class BatchTests(unittest.TestCase):
    def test_frozen_dataset_is_balanced_and_held_out(self):
        self.assertEqual(len(service.ITEMS), 51)
        counts = {kind: sum(r["rarity_bucket"] == kind for r in service.REFERENCES.values()) for kind in ("common", "uncommon", "rare_or_higher", "not_applicable")}
        self.assertEqual(counts, {"common": 16, "uncommon": 16, "rare_or_higher": 16, "not_applicable": 3})
        self.assertTrue(all(set(r) == {"id", "image_url", "image_sha256"} for r in service.ITEMS.values()))

    def test_missing_probabilities_are_not_accepted(self):
        with self.assertRaises(ValueError):
            service.validate_answer({"answers": {"rarity": {"choice": "rare_or_higher", "probabilities": {"rare_or_higher": 1}}}})

    def test_inference_failure_stays_an_error(self):
        item_id, item = next(iter(service.ITEMS.items()))
        initial = copy.deepcopy(service.STATE)
        self.addCleanup(lambda: service.STATE.update(initial))
        with patch.object(service, "ITEMS", {item_id: item}), patch.object(service, "post_json", side_effect=RuntimeError("provider offline")), patch.dict(os.environ, {"DECISION_URL": "http://model.invalid"}):
            service.STATE.update({"results": {}, "prices": {}, "running": True})
            service.analyze()
        result = service.STATE["results"][item_id]
        self.assertEqual(result["status"], "error")
        self.assertNotIn("answers", result)
        self.assertFalse(service.STATE["running"])


if __name__ == "__main__":
    unittest.main()
