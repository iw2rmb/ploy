# Pseudocode: KB compactor that rebuilds summaries and snapshot manifest

import os, json, time
from pathlib import Path

def normalize_patch(diff_text: str) -> str:
    # Strip timestamps/metadata lines; keep hunks only
    return diff_text

def compute_score(wins, fails, size_bonus):
    total = wins + fails
    wr = wins / total if total else 0.0
    return wr + size_bonus

def compact(kb_dir: str, out_dir: str):
    kb = Path(kb_dir)
    out = Path(out_dir)
    out.mkdir(parents=True, exist_ok=True)
    counts = {}

    for lang_dir in (kb / 'healing' / 'errors').glob('*'):
        lang = lang_dir.name
        counts.setdefault(lang, {"errors": 0, "cases": 0, "patches": 0})
        for sig_dir in lang_dir.glob('*'):
            counts[lang]["errors"] += 1
            cases_dir = sig_dir / 'cases'
            wins = 0; fails = 0
            promoted = []
            if cases_dir.exists():
                for case in cases_dir.glob('*.json'):
                    counts[lang]["cases"] += 1
                    data = json.load(open(case))
                    outcome = data.get('outcome')
                    if outcome == 'success': wins += 1
                    elif outcome == 'failed': fails += 1
            # example promotion logic stub
            if wins >= 2:
                promoted.append({"kind": "patch_fingerprint", "ref": "fp-abc", "score": compute_score(wins, fails, 0.1), "wins": wins, "failures": fails})
            summary = {"language": lang, "signature": sig_dir.name, "promoted": promoted, "stats": {"cases": wins+fails, "recent_window": 0}}
            with open(sig_dir / 'summary.json', 'w') as f:
                json.dump(summary, f, indent=2)
    snapshot = {
        "timestamp": time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()),
        "languages": list(counts.keys()),
        "counts": counts,
        "versions": {"embedding_model": "", "index_format": ""}
    }
    with open(out / 'snapshot.json', 'w') as f:
        json.dump(snapshot, f, indent=2)

if __name__ == '__main__':
    compact(os.environ.get('KB_DIR', '/workspace/kb'), os.environ.get('OUT_DIR', '/workspace/out'))

