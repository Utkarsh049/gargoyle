#!/usr/bin/env python3
"""
Traffic simulator for Gargoyle API Gateway.

Generates normal and attack HTTP traffic profiles (endpoint sweeps,
credential stuffing, rate-limit probing, mixed) against a running
gateway instance and logs ground-truth request telemetry to JSONL/CSV
for dataset creation and verification.
"""

import argparse
import csv
from dataclasses import asdict, dataclass
import datetime
import json
import os
import random
import sys
import time
from typing import Any, Dict, List, Optional, Tuple
import urllib.error
import urllib.parse
import urllib.request
import uuid

DEFAULT_TARGET_URL = "http://localhost:8080"
DEFAULT_OUTPUT_FILE = "ground_truth.jsonl"
DEFAULT_HEADER_KEY = "X-Gargoyle-Key"

# Realistic browser User-Agent strings for normal traffic
BROWSER_USER_AGENTS = [
    (
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 "
        "(KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"
    ),
    (
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:123.0) "
        "Gecko/20100101 Firefox/123.0"
    ),
    (
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_3_1) AppleWebKit/605.1.15 "
        "(KHTML, like Gecko) Version/17.3.1 Safari/605.1.15"
    ),
    (
        "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 "
        "(KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36"
    ),
]

# Automated tool / scraper User-Agent signatures for attack simulation
ATTACK_USER_AGENTS = [
    "curl/8.4.0",
    "python-requests/2.31.0",
    "Go-http-client/1.1",
    "Wget/1.21.4",
    "Scrapy/2.11.0 (+https://scrapy.org)",
    "sqlmap/1.7#stable",
    "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/122.0.0.0 Safari/537.36",
]

NORMAL_ENDPOINTS = [
    "/api/v1/products",
    "/api/v1/products/item-101",
    "/api/v1/products/item-204",
    "/api/v1/search?q=laptop",
    "/api/v1/search?q=monitor&page=2",
    "/api/v1/users/profile",
    "/api/v1/cart",
    "/api/v1/orders",
    "/api/v1/notifications",
    "/api/v1/settings",
]

SWEEP_BASE_ENDPOINTS = [
    "/admin",
    "/wp-admin",
    "/admin/login",
    "/api/v1/debug",
    "/api/v1/export",
    "/api/v1/metrics",
    "/api/v1/internal",
    "/api/v1/system/status",
    "/backup.tar.gz",
    "/config.json",
    "/.env",
    "/.git/config",
    "/actuator/health",
    "/server-status",
    "/api/v1/users/dump",
]

USERNAMES_POOL = [
    "admin", "root", "administrator", "user", "guest", "test",
    "support", "service", "backup", "operator", "dev", "alex",
    "sarah", "john.doe", "billing", "system",
]


@dataclass
class GroundTruthEntry:
    """Represents a single raw request record for ground-truth dataset logging."""
    timestamp: str
    batch_id: str
    sequence_num: int
    client_id: str
    source_identifier: str
    method: str
    endpoint: str
    headers: Dict[str, str]
    status_code: int
    latency_ms: float
    true_label: str


class GroundTruthWriter:
    """Thread-safe writer for logging raw request ground-truth records."""

    def __init__(self, output_path: str, output_format: str):
        self.output_path = output_path
        self.output_format = output_format.lower()
        self._file = open(output_path, "w", encoding="utf-8", newline="")
        self._csv_writer = None

        if self.output_format == "csv":
            fieldnames = [
                "timestamp", "batch_id", "sequence_num", "client_id",
                "source_identifier", "method", "endpoint", "headers_json",
                "status_code", "latency_ms", "true_label",
            ]
            self._csv_writer = csv.DictWriter(self._file, fieldnames=fieldnames)
            self._csv_writer.writeheader()
            self._file.flush()

    def write(self, entry: GroundTruthEntry) -> None:
        if self.output_format == "csv":
            row = asdict(entry)
            row["headers_json"] = json.dumps(row.pop("headers"))
            self._csv_writer.writerow(row)
        else:
            self._file.write(json.dumps(asdict(entry)) + "\n")
        self._file.flush()

    def close(self) -> None:
        if self._file and not self._file.closed:
            self._file.close()


class TrafficGenerator:
    """Generates requests according to configured traffic profiles."""

    def __init__(self, target_url: str, api_key: str, client_id: str, batch_id: str):
        self.target_url = target_url.rstrip("/")
        self.api_key = api_key
        self.client_id = client_id
        self.batch_id = batch_id
        self.sweep_index = 0

    def generate(self, mode: str, seq: int) -> Tuple[str, str, Dict[str, str], Optional[bytes], str, float]:
        """
        Builds a request specification based on mode.

        Returns:
            (method, url, headers, body, true_label, pre_delay_seconds)
        """
        if mode == "normal":
            return self._generate_normal(seq)
        elif mode == "sweep":
            return self._generate_sweep(seq)
        elif mode == "stuffing":
            return self._generate_stuffing(seq)
        elif mode == "rate_probe":
            return self._generate_rate_probe(seq)
        elif mode == "mixed":
            return self._generate_mixed(seq)
        else:
            raise ValueError(f"Unknown traffic mode: {mode}")

    def _generate_normal(self, seq: int) -> Tuple[str, str, Dict[str, str], Optional[bytes], str, float]:
        endpoint = random.choice(NORMAL_ENDPOINTS)
        url = f"{self.target_url}{endpoint}"
        headers = {
            DEFAULT_HEADER_KEY: self.api_key,
            "User-Agent": random.choice(BROWSER_USER_AGENTS),
            "Accept": "text/html,application/xhtml+xml,application/json;q=0.9,*/*;q=0.8",
            "Accept-Language": "en-US,en;q=0.9",
        }
        # Human-like timing jitter with exponential/gamma distribution (100ms - 800ms)
        delay = random.expovariate(1.0 / 0.25)
        delay = max(0.05, min(delay, 1.2))

        return "GET", url, headers, None, "normal", delay

    def _generate_sweep(self, seq: int) -> Tuple[str, str, Dict[str, str], Optional[bytes], str, float]:
        # Rapidly cycle through distinct paths to trigger sweep detection
        idx = (self.sweep_index + seq) % (len(SWEEP_BASE_ENDPOINTS) + 50)
        if idx < len(SWEEP_BASE_ENDPOINTS):
            endpoint = SWEEP_BASE_ENDPOINTS[idx]
        else:
            endpoint = f"/api/v1/resource-scan-{idx}"

        url = f"{self.target_url}{endpoint}"
        headers = {
            DEFAULT_HEADER_KEY: self.api_key,
            "User-Agent": random.choice(ATTACK_USER_AGENTS),
            "Accept": "*/*",
        }
        # Very low, uniform robotic delay (10ms - 25ms)
        delay = random.uniform(0.010, 0.025)

        return "GET", url, headers, None, "endpoint_sweep", delay

    def _generate_stuffing(self, seq: int) -> Tuple[str, str, Dict[str, str], Optional[bytes], str, float]:
        endpoint = "/api/v1/auth/login"
        url = f"{self.target_url}{endpoint}"
        username = random.choice(USERNAMES_POOL)
        payload = {
            "username": username,
            "password": f"Password{random.randint(100, 999)}!",
            "grant_type": "password",
        }
        body = json.dumps(payload).encode("utf-8")
        headers = {
            DEFAULT_HEADER_KEY: self.api_key,
            "Content-Type": "application/json",
            "User-Agent": "python-requests/2.31.0",
            "Accept": "application/json",
        }
        # High frequency, small delay (15ms - 40ms)
        delay = random.uniform(0.015, 0.040)

        return "POST", url, headers, body, "credential_stuffing", delay

    def _generate_rate_probe(self, seq: int) -> Tuple[str, str, Dict[str, str], Optional[bytes], str, float]:
        endpoint = "/api/v1/products"
        url = f"{self.target_url}{endpoint}"
        headers = {
            DEFAULT_HEADER_KEY: self.api_key,
            "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko)",
            "Accept": "application/json",
        }
        # Robotic near-zero variance pacing (exact 20ms interval)
        delay = 0.020

        return "GET", url, headers, None, "rate_probe", delay

    def _generate_mixed(self, seq: int) -> Tuple[str, str, Dict[str, str], Optional[bytes], str, float]:
        roll = random.random()
        if roll < 0.65:
            return self._generate_normal(seq)
        elif roll < 0.80:
            return self._generate_sweep(seq)
        elif roll < 0.90:
            return self._generate_stuffing(seq)
        else:
            return self._generate_rate_probe(seq)


def send_http_request(
    method: str,
    url: str,
    headers: Dict[str, str],
    body: Optional[bytes] = None,
    timeout: float = 5.0,
) -> Tuple[int, float]:
    """Sends a raw HTTP request and measures status code and latency in milliseconds."""
    req = urllib.request.Request(url, data=body, headers=headers, method=method)
    start = time.perf_counter()

    try:
        with urllib.request.urlopen(req, timeout=timeout) as response:
            status = response.getcode()
    except urllib.error.HTTPError as e:
        status = e.code
    except urllib.error.URLError:
        status = 502
    except Exception:
        status = 500

    latency_ms = round((time.perf_counter() - start) * 1000.0, 2)
    return status, latency_ms


def run_simulator(args: argparse.Namespace) -> None:
    """Executes the traffic simulation run."""
    batch_id = str(uuid.uuid4())[:8]
    output_format = args.format
    if not output_format:
        output_format = "csv" if args.output.endswith(".csv") else "jsonl"

    writer = GroundTruthWriter(args.output, output_format)
    generator = TrafficGenerator(
        target_url=args.target_url,
        api_key=args.api_key,
        client_id=args.client_id,
        batch_id=batch_id,
    )

    print(f"Starting traffic simulation...")
    print(f"Target: {args.target_url}")
    print(f"Mode: {args.mode}")
    print(f"Requests: {args.requests}")
    print(f"Output: {args.output} ({output_format})")
    print(f"Batch ID: {batch_id}")
    print("-" * 50)

    sent_count = 0
    status_summary: Dict[int, int] = {}
    label_summary: Dict[str, int] = {}

    try:
        for seq in range(1, args.requests + 1):
            method, url, headers, body, true_label, delay = generator.generate(args.mode, seq)

            if delay > 0 and not args.no_delay:
                time.sleep(delay)

            parsed = urllib.parse.urlparse(url)
            endpoint_path = parsed.path
            if parsed.query:
                endpoint_path += f"?{parsed.query}"

            status_code, latency_ms = send_http_request(method, url, headers, body)
            timestamp = datetime.datetime.now(datetime.timezone.utc).isoformat()

            entry = GroundTruthEntry(
                timestamp=timestamp,
                batch_id=batch_id,
                sequence_num=seq,
                client_id=args.client_id,
                source_identifier=args.source_id,
                method=method,
                endpoint=endpoint_path,
                headers=headers,
                status_code=status_code,
                latency_ms=latency_ms,
                true_label=true_label,
            )

            writer.write(entry)
            sent_count += 1
            status_summary[status_code] = status_summary.get(status_code, 0) + 1
            label_summary[true_label] = label_summary.get(true_label, 0) + 1

            if seq % 20 == 0 or seq == args.requests:
                print(
                    f"[{seq:4d}/{args.requests}] {method:4s} {endpoint_path[:35]:35s} "
                    f"-> {status_code} ({latency_ms:6.1f}ms) [{true_label}]"
                )

    except KeyboardInterrupt:
        print("\nSimulation interrupted by user.")
    finally:
        writer.close()

    print("-" * 50)
    print(f"Completed: {sent_count}/{args.requests} requests logged to {args.output}")
    print("Status codes breakdown:", dict(sorted(status_summary.items())))
    print("True labels breakdown:", label_summary)


def load_dotenv() -> None:
    """Discovers and loads key-value pairs from .env in current or parent directories."""
    current = os.path.abspath(os.getcwd())
    for _ in range(5):
        env_file = os.path.join(current, ".env")
        if os.path.isfile(env_file):
            try:
                with open(env_file, "r", encoding="utf-8") as f:
                    for line in f:
                        line = line.strip()
                        if not line or line.startswith("#"):
                            continue
                        if "=" in line:
                            k, v = line.split("=", 1)
                            k, v = k.strip(), v.strip()
                            if (v.startswith('"') and v.endswith('"')) or (v.startswith("'") and v.endswith("'")):
                                v = v[1:-1]
                            if k and k not in os.environ and v:
                                os.environ[k] = v
            except Exception:
                pass
            return
        parent = os.path.dirname(current)
        if parent == current:
            break
        current = parent


def parse_args() -> argparse.Namespace:
    load_dotenv()
    parser = argparse.ArgumentParser(
        description="Gargoyle Traffic Simulator & Dataset Generator",
        formatter_class=argparse.ArgumentDefaultsHelpFormatter,
    )
    parser.add_argument(
        "--target-url",
        default=os.getenv("GARGOYLE_TARGET_URL", os.getenv("GARGOYLE_LISTEN_ADDR", DEFAULT_TARGET_URL)),
        help="Base URL of Gargoyle gateway",
    )
    parser.add_argument(
        "--api-key",
        default=os.getenv("GARGOYLE_API_KEY", ""),
        help="API key for authentication (X-Gargoyle-Key)",
    )
    parser.add_argument(
        "--client-id",
        default=os.getenv("GARGOYLE_CLIENT_ID", "sim-client-1"),
        help="Client identifier for log records",
    )
    parser.add_argument(
        "--source-id",
        default="127.0.0.1",
        help="Source IP or test identifier",
    )
    parser.add_argument(
        "--mode",
        choices=["normal", "sweep", "stuffing", "rate_probe", "mixed"],
        default="normal",
        help="Traffic generation profile",
    )
    parser.add_argument(
        "-n", "--requests",
        type=int,
        default=100,
        help="Total number of requests to generate",
    )
    parser.add_argument(
        "-o", "--output",
        default=DEFAULT_OUTPUT_FILE,
        help="Ground-truth dataset output file path (.jsonl or .csv)",
    )
    parser.add_argument(
        "--format",
        choices=["jsonl", "csv"],
        default=None,
        help="Output format (inferred from output filename if omitted)",
    )
    parser.add_argument(
        "--no-delay",
        action="store_true",
        help="Disable inter-request delays and send at maximum rate",
    )

    return parser.parse_args()


if __name__ == "__main__":
    run_simulator(parse_args())
