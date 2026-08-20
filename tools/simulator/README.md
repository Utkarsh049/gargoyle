# Gargoyle Traffic Simulator

A standalone Python CLI tool for generating realistic normal and attack HTTP traffic profiles against the Gargoyle API Gateway and recording ground-truth dataset logs (`.jsonl` / `.csv`).

## Features

- **Normal Traffic**: Simulates legitimate user browsing across realistic endpoints with authentic browser User-Agents and natural timing jitter.
- **Endpoint Sweep**: Simulates rapid path enumeration and directory busting with tight, low-variance intervals.
- **Credential Stuffing**: High-frequency authentication requests with rotating username/password payloads.
- **Rate-Limit Probing**: Robotic pacing sitting right below configured rate limits to test timing heuristic detection.
- **Mixed Traffic**: Blended workload containing normal background traffic punctuated with attack bursts.
- **Ground Truth Logging**: Independent recording of raw request facts (`timestamp`, `client_id`, `endpoint`, `status_code`, `latency_ms`, `sequence_num`, `true_label`).

## Usage

### Prerequisites
Python 3.8+ (uses standard library modules only, no external pip dependencies required).

### Basic Commands

```bash
# 1. Normal user browsing (100 requests)
python3 tools/simulator/simulate.py --target-url http://localhost:8080 --api-key <YOUR_API_KEY> --mode normal -n 100 -o normal_traffic.jsonl

# 2. Endpoint sweep / path scraping attack
python3 tools/simulator/simulate.py --target-url http://localhost:8080 --api-key <YOUR_API_KEY> --mode sweep -n 50 -o sweep_attack.jsonl

# 3. Credential stuffing attack against auth endpoints
python3 tools/simulator/simulate.py --target-url http://localhost:8080 --api-key <YOUR_API_KEY> --mode stuffing -n 50 -o stuffing_attack.jsonl

# 4. Rate-limit probing with robotic timing
python3 tools/simulator/simulate.py --target-url http://localhost:8080 --api-key <YOUR_API_KEY> --mode rate_probe -n 50 -o rate_probe.jsonl

# 5. Mixed realistic profile (Normal + Intermittent Attacks)
python3 tools/simulator/simulate.py --target-url http://localhost:8080 --api-key <YOUR_API_KEY> --mode mixed -n 200 -o mixed_dataset.csv
```

### CLI Options

| Flag | Default | Description |
| :--- | :--- | :--- |
| `--target-url` | `http://localhost:8080` | Gateway base URL |
| `--api-key` | Env `GARGOYLE_API_KEY` | API key sent in `X-Gargoyle-Key` header |
| `--client-id` | `sim-client-1` | Client label in ground-truth records |
| `--mode` | `normal` | Traffic mode: `normal`, `sweep`, `stuffing`, `rate_probe`, `mixed` |
| `-n`, `--requests` | `100` | Number of requests to generate |
| `-o`, `--output` | `ground_truth.jsonl` | Output file path (`.jsonl` or `.csv`) |
| `--no-delay` | `False` | Send requests at maximum rate without delays |

## Ground-Truth Schema

Each record captures raw request-level facts:

```json
{
  "timestamp": "2026-08-19T12:00:00.123456Z",
  "batch_id": "a1b2c3d4",
  "sequence_num": 1,
  "client_id": "sim-client-1",
  "source_identifier": "127.0.0.1",
  "method": "GET",
  "endpoint": "/api/v1/products",
  "headers": {
    "X-Gargoyle-Key": "gk_live_...",
    "User-Agent": "Mozilla/5.0 ...",
    "Accept": "text/html,..."
  },
  "status_code": 200,
  "latency_ms": 14.2,
  "true_label": "normal"
}
```
