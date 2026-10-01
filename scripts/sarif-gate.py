"""Prints every finding in the given SARIF files and exits 1 if any has a security severity of
7.0 (high) or more, unless it is listed, with its reason, in .github/codeql-accepted.json.
Used by the CodeQL workflow (.github/workflows/codeql.yml)."""

import json
import os
import sys

THRESHOLD = 7.0
ACCEPTED = os.path.join(os.path.dirname(__file__), "..", ".github", "codeql-accepted.json")


def load_accepted() -> list[dict]:
    try:
        with open(ACCEPTED) as f:
            return json.load(f)
    except FileNotFoundError:
        return []


def main(paths: list[str]) -> int:
    accepted = load_accepted()
    blocking = 0
    total = 0
    for path in paths:
        with open(path) as f:
            sarif = json.load(f)
        for run in sarif.get("runs", []):
            rules = {}
            tool = run.get("tool", {})
            for component in [tool.get("driver", {}), *tool.get("extensions", [])]:
                for rule in component.get("rules", []):
                    rules[rule["id"]] = rule
            for result in run.get("results", []):
                total += 1
                rule_id = result.get("ruleId", "")
                rule = rules.get(rule_id, {})
                severity = float(rule.get("properties", {}).get("security-severity", 0) or 0)
                loc = result.get("locations", [{}])[0].get("physicalLocation", {})
                uri = loc.get("artifactLocation", {}).get("uri", "?")
                where = f'{uri}:{loc.get("region", {}).get("startLine", "?")}'
                is_accepted = any(a["rule"] == rule_id and a["path"] == uri for a in accepted)
                if severity < THRESHOLD:
                    flag = "note"
                elif is_accepted:
                    flag = "accepted"
                else:
                    flag = "BLOCKING"
                    blocking += 1
                print(f'{flag:8} {severity:4.1f} {rule_id} {where}: {result.get("message", {}).get("text", "")}')
    print(f"{total} findings, {blocking} blocking (security severity {THRESHOLD} or more, not accepted)")
    return 1 if blocking else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
