import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('feed', Path(__file__).resolve().parents[2] / 'deploy/test-host/canada28-feed.py')
feed = importlib.util.module_from_spec(spec)
spec.loader.exec_module(feed)

class Canada28RestTests(unittest.TestCase):
    def setUp(self):
        self.latest = dict(draw_number=3505236, draw_date='2026-11-10T18:56:30+08:00', canada28_num1=7, canada28_num2=6, canada28_num3=8, canada28_result=21)
        self.countdown = dict(next_draw_number=3505237, estimated_timestamp=None, rest_status='resting')

    def test_rest_keeps_result_without_fabricating_schedule(self):
        result = feed.normalize(self.latest, self.countdown)
        self.assertEqual(result['result']['digits'], [7, 6, 8])
        self.assertEqual(result['period_no'], '3505236')
        self.assertIsNone(result['next_period_no'])
        self.assertIsNone(result['next_draw_at'])

    def test_reopening_uses_upstream_timestamp(self):
        self.countdown.update(rest_status='active', estimated_timestamp=1794310200000)
        result = feed.normalize(self.latest, self.countdown)
        self.assertEqual(result['next_period_no'], '3505237')
        self.assertEqual(result['next_draw_at'], '2026-11-10T11:30:00+00:00')

    def test_null_without_rest_is_rejected(self):
        self.countdown['rest_status'] = 'active'
        with self.assertRaisesRegex(ValueError, 'Invalid next draw timestamp'):
            feed.normalize(self.latest, self.countdown)

    def test_rest_still_validates_results_and_periods(self):
        self.latest['canada28_result'] = 20
        with self.assertRaisesRegex(ValueError, 'Result sum mismatch'):
            feed.normalize(self.latest, self.countdown)
        self.latest['canada28_result'] = 21
        self.countdown['next_draw_number'] = 3505238
        with self.assertRaisesRegex(ValueError, 'period mismatch'):
            feed.normalize(self.latest, self.countdown)

if __name__ == '__main__':
    unittest.main()
