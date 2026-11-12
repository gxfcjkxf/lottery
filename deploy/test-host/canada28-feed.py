#!/usr/bin/env python3
"""Normalize the approved JND28 latest/countdown APIs for the mapped reader."""
import json
import logging
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.request import Request, urlopen

ORIGIN = "https://jnd-28.net"


def fetch(path):
    request = Request(ORIGIN + path, headers={
        "Accept": "application/json", "User-Agent": "LotteryCanada28/1.0",
    })
    with urlopen(request, timeout=3) as response:
        return json.load(response)


def normalize(latest, countdown):
    period = latest["draw_number"]
    digits = [latest[f"canada28_num{i}"] for i in (1, 2, 3)]
    if type(period) is not int or period <= 0:
        raise ValueError("Invalid latest period")
    if any(type(n) is not int or not 0 <= n <= 9 for n in digits):
        raise ValueError("Invalid digits")
    if sum(digits) != latest["canada28_result"]:
        raise ValueError("Result sum mismatch")
    drawn = datetime.fromisoformat(latest["draw_date"])
    if drawn.utcoffset() is None or drawn > datetime.now(timezone.utc):
        raise ValueError("Invalid draw time")
    next_period = countdown["next_draw_number"]
    timestamp = countdown["estimated_timestamp"]
    if type(next_period) is not int or next_period != period + 1:
        raise ValueError("Latest/countdown period mismatch")
    if countdown.get("rest_status") == "resting" and timestamp is None:
        return {
            "period_no": str(period), "drawn_at": drawn.isoformat(),
            "result": {"digits": digits},
            "next_period_no": None, "next_draw_at": None,
        }
    if type(timestamp) is not int:
        raise ValueError("Invalid next draw timestamp")
    next_draw = datetime.fromtimestamp(timestamp / 1000, timezone.utc)
    if next_draw <= drawn:
        raise ValueError("Invalid next draw time")
    return {
        "period_no": str(period), "drawn_at": drawn.isoformat(),
        "result": {"digits": digits},
        "next_period_no": str(next_period), "next_draw_at": next_draw.isoformat(),
    }


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path != "/draw":
            self.send_error(404)
            return
        try:
            result = normalize(fetch("/api/latest"), fetch("/api/countdown"))
        except Exception as error:
            logging.error("JND28 source read failed: %s: %s", type(error).__name__, error)
            self.send_error(502)
            return
        body = json.dumps(result, separators=(",", ":")).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Cache-Control", "no-store")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *_):
        pass


if __name__ == "__main__":
    HTTPServer(("127.0.0.1", 8097), Handler).serve_forever()
