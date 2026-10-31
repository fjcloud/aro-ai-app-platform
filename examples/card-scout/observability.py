"""MLflow OTLP tracing; credentials and image bytes never enter span attributes."""
import json
import os
import pathlib
import tempfile
from urllib.parse import quote, urlencode

import certifi

from opentelemetry import trace
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor, SpanExporter, SpanExportResult


class RotatingExporter(SpanExporter):
    def export(self, spans):
        try:
            token = pathlib.Path(os.getenv("SERVICE_ACCOUNT_TOKEN_FILE", "/var/run/secrets/kubernetes.io/serviceaccount/token")).read_text().strip()
            trust = pathlib.Path(certifi.where()).read_bytes()
            if os.getenv("SERVICE_CA_FILE"):
                trust += b"\n" + pathlib.Path(os.environ["SERVICE_CA_FILE"]).read_bytes()
            ca = tempfile.NamedTemporaryFile(prefix="card-scout-ca-", suffix=".pem")
            ca.write(trust)
            ca.flush()
            exporter = OTLPSpanExporter(
                endpoint=os.environ["OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"],
                certificate_file=ca.name,
                headers={"Authorization": "Bearer " + token,
                         "x-mlflow-experiment-id": os.environ["MLFLOW_EXPERIMENT_ID"],
                         "X-MLFLOW-WORKSPACE": os.environ["MLFLOW_WORKSPACE"]}, timeout=8)
            try:
                return exporter.export(spans)
            finally:
                exporter.shutdown()
                ca.close()
        except Exception:
            # Exporter bodies/headers may contain credentials: do not log them.
            return SpanExportResult.FAILURE


PROVIDER = TracerProvider(resource=Resource.create({"service.name": "card-scout-platform"}))
if os.getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"):
    PROVIDER.add_span_processor(BatchSpanProcessor(RotatingExporter(), schedule_delay_millis=500))
trace.set_tracer_provider(PROVIDER)
TRACER = trace.get_tracer("card-scout")


def record(span, key, value):
    text = json.dumps(value, ensure_ascii=False, default=str)
    if len(text) > 60000:
        text = json.dumps({"truncated": True, "excerpt": text[:58000]})
    span.set_attribute("mlflow." + key, text)


def trace_link(span):
    trace_id = f"tr-{span.get_span_context().trace_id:032x}"
    endpoint = os.getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
    experiment, workspace = os.getenv("MLFLOW_EXPERIMENT_ID"), os.getenv("MLFLOW_WORKSPACE")
    if not endpoint.endswith("/v1/traces") or not experiment or not workspace:
        return trace_id, ""
    base = endpoint[:-len("/v1/traces")]
    return trace_id, base + "/#/experiments/" + quote(experiment, safe="") + "/traces?" + urlencode({"selectedEvaluationId": trace_id, "workspace": workspace})
