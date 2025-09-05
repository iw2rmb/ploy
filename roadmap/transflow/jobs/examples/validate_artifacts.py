#!/usr/bin/env python3
"""
Validate Transflow job artifacts against JSON Schemas.

Usage:
  validate_artifacts.py --schema plan --file /path/to/plan.json
  validate_artifacts.py --schema next --file /path/to/next.json
  validate_artifacts.py --schema inputs --file /path/to/inputs.json
  validate_artifacts.py --schema history --file /path/to/history.json
  validate_artifacts.py --schema branch_record --file /path/to/branch.json
  validate_artifacts.py --schema run_manifest --file /path/to/run_manifest.json
  validate_artifacts.py --schema kb_summary --file /path/to/summary.json
  validate_artifacts.py --schema kb_snapshot --file /path/to/snapshot.json
"""

import argparse
import json
import sys
from pathlib import Path

try:
    import jsonschema
except ImportError:
    print("ERROR: jsonschema module not installed. pip install jsonschema", file=sys.stderr)
    sys.exit(2)

SCHEMA_MAP = {
    'plan': 'plan.schema.json',
    'next': 'next.schema.json',
    'inputs': 'inputs.schema.json',
    'history': 'history.schema.json',
    'branch_record': 'branch_record.schema.json',
    'run_manifest': 'run_manifest.schema.json',
    'kb_summary': 'kb_summary.schema.json',
    'kb_snapshot': 'kb_snapshot.schema.json',
}

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('--schema', required=True, choices=SCHEMA_MAP.keys())
    ap.add_argument('--file', required=True)
    args = ap.parse_args()

    base = Path(__file__).resolve().parents[1] / 'schemas'
    schema_path = base / SCHEMA_MAP[args.schema]
    data_path = Path(args.file)

    if not schema_path.exists():
        print(f"ERROR: schema not found: {schema_path}", file=sys.stderr)
        return 2
    if not data_path.exists():
        print(f"ERROR: file not found: {data_path}", file=sys.stderr)
        return 2

    with open(schema_path, 'r') as f:
        schema = json.load(f)
    with open(data_path, 'r') as f:
        data = json.load(f)

    try:
        jsonschema.validate(instance=data, schema=schema)
        print("OK")
        return 0
    except jsonschema.ValidationError as e:
        print(f"INVALID: {e.message}", file=sys.stderr)
        return 1

if __name__ == '__main__':
    sys.exit(main())

