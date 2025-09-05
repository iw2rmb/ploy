# Pseudocode-only example for a LangGraph runner entrypoint.
# Reads env/args, validates inputs, runs planner or reducer, writes artifacts.

import os, sys, json
from pathlib import Path

def json_out(d):
    print(json.dumps(d, ensure_ascii=False))

def load_json(path):
    with open(path, 'r') as f:
        return json.load(f)

def write_json(path, data):
    Path(path).parent.mkdir(parents=True, exist_ok=True)
    with open(path, 'w') as f:
        json.dump(data, f, indent=2)

def normalize_error(err):
    # Toy normalization; real impl strips timestamps/paths, truncates noise
    return {
        "stdout": err.get("stdout", "")[:10000],
        "stderr": err.get("stderr", "")[:10000],
    }

def planner_main():
    out = os.environ.get('OUTPUT_DIR', '/workspace/out')
    ctx = os.environ.get('CONTEXT_DIR', '/workspace/context')
    inputs_path = os.path.join(ctx, 'inputs.json')
    inputs = load_json(inputs_path)

    # Normalize and (optionally) consult KB here
    last_error = normalize_error(inputs.get('last_error', {}))

    # Placeholder plan
    plan = {
        "plan_id": os.environ.get('RUN_ID', 'run-unknown'),
        "options": [
            {"id": "human-1", "type": "human", "success_criteria": "build passes after human push"},
            {"id": "llm-1", "type": "llm-exec", "inputs": {"prompt": "fix compile for java17"}},
            {"id": "orw-1", "type": "orw-gen", "inputs": {"target": "java17"}}
        ]
    }
    write_json(os.path.join(out, 'plan.json'), plan)
    write_json(os.path.join(out, 'manifest.json'), {
        "model": os.environ.get('MODEL', ''),
        "limits": os.environ.get('LIMITS', ''),
        "tools": os.environ.get('TOOLS', ''),
        "inputs_hash": hash(json.dumps(last_error)),
    })
    json_out({"ok": True, "plan": "out/plan.json"})
    return 0

def reducer_main(input_path):
    out = os.environ.get('OUTPUT_DIR', '/workspace/out')
    history = load_json(input_path)
    next_actions = {"action": "stop", "notes": f"winner={history.get('winner')}"}
    write_json(os.path.join(out, 'next.json'), next_actions)
    write_json(os.path.join(out, 'manifest.json'), {
        "model": os.environ.get('MODEL', ''),
        "limits": os.environ.get('LIMITS', ''),
        "tools": os.environ.get('TOOLS', ''),
    })
    json_out({"ok": True, "next": "out/next.json"})
    return 0

def main(argv):
    # args: --mode planner|reducer [--input path]
    mode = None
    input_path = None
    it = iter(argv)
    for a in it:
        if a == '--mode':
            mode = next(it, None)
        elif a == '--input':
            input_path = next(it, None)
    if mode == 'planner':
        sys.exit(planner_main())
    elif mode == 'reducer':
        if not input_path:
            json_out({"ok": False, "error": "missing --input"}); sys.exit(1)
        sys.exit(reducer_main(input_path))
    else:
        json_out({"ok": False, "error": "invalid --mode"}); sys.exit(1)

if __name__ == '__main__':
    main(sys.argv[1:])

