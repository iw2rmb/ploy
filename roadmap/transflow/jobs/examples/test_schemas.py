# Example unit tests (illustrative) to validate job artifacts against JSON Schemas

import json
from pathlib import Path
import jsonschema

SCHEMAS = Path(__file__).resolve().parents[1] / 'schemas'

def load_schema(name):
    with open(SCHEMAS / name, 'r') as f:
        return json.load(f)

def test_plan_schema_example():
    plan = {
        "plan_id": "run-123",
        "options": [
            {"id": "human-1", "type": "human"},
            {"id": "llm-1", "type": "llm-exec", "inputs": {"prompt": "fix"}},
            {"id": "orw-1", "type": "orw-gen", "inputs": {"target": "java17"}}
        ]
    }
    jsonschema.validate(plan, load_schema('plan.schema.json'))

def test_next_schema_example():
    nxt = {"action": "stop", "notes": "winner=llm-1"}
    jsonschema.validate(nxt, load_schema('next.schema.json'))

def test_inputs_schema_example():
    inputs = {
        "language": "java",
        "lane": "C",
        "last_error": {"stdout": "...", "stderr": "..."},
        "deps": {"pom.xml": "sha256:..."}
    }
    jsonschema.validate(inputs, load_schema('inputs.schema.json'))

def test_history_schema_example():
    history = {
        "plan_id": "run-123",
        "branches": [
            {"id": "llm-1", "status": "success", "artifact": "out/diff.patch"},
            {"id": "orw-1", "status": "failed"}
        ],
        "winner": "llm-1"
    }
    jsonschema.validate(history, load_schema('history.schema.json'))

