"""Verify live-query failures and quiet-model behavior without an actual cluster."""
import contextlib
import importlib.util
import io
import json
from pathlib import Path
import unittest
from unittest.mock import patch

spec=importlib.util.spec_from_file_location('observability',Path(__file__).with_name('check-observability.py'))
module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)

class ObservabilityCheck(unittest.TestCase):
    def run_check(self, result):
        response=io.StringIO(json.dumps(result))
        with patch.object(module,'oc',return_value='private-test-token'),patch.object(module.urllib.request,'urlopen',return_value=response),contextlib.redirect_stdout(io.StringIO()):
            return module.main()

    def test_success_response_without_series_is_not_healthy(self):
        with self.assertRaises(RuntimeError):
            self.run_check({'status':'success','data':{'result':[]}})

    def test_prometheus_error_is_not_healthy(self):
        with self.assertRaises(RuntimeError):
            self.run_check({'status':'error','error':'bad expression'})

    def test_quiet_latency_is_allowed_and_token_is_not_printed(self):
        def response(request, **kwargs):
            query=module.urllib.parse.parse_qs(module.urllib.parse.urlparse(request.full_url).query)['query'][0]
            value='NaN' if query.startswith('histogram_quantile') else '1'
            results = [{'metric':{'namespace':namespace},'value':[1,value]} for namespace in ('speech-serving','kuadrant-system')] if query.startswith('min by (namespace)') else [{'metric':{},'value':[1,value]}]
            return io.StringIO(json.dumps({'status':'success','data':{'result':results}}))
        output=io.StringIO()
        with patch.object(module,'oc',return_value='private-test-token'),patch.object(module.urllib.request,'urlopen',side_effect=response),contextlib.redirect_stdout(output):
            module.main()
        data=json.loads(output.getvalue())
        self.assertEqual(len(data['checks']),9)
        self.assertNotIn('private-test-token',output.getvalue())

    def test_no_token_stops_before_any_http_request(self):
        with patch.object(module,'oc',return_value=''),patch.object(module.urllib.request,'urlopen') as request,self.assertRaises(RuntimeError):
            module.main()
        request.assert_not_called()
