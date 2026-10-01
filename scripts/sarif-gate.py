"""Prints every finding in the given SARIF files and exits 1 if any has a security severity of
7.0 (high) or more. Used by the CodeQL workflow (.github/workflows/codeql.yml)."""

import json
import sys

THRESHOLD = 7.0


def main(paths: list[str]) -> int:
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
                rule = rules.get(result.get("ruleId", ""), {})
                severity = float(rule.get("properties", {}).get("security-severity", 0) or 0)
                loc = result.get("locations", [{}])[0].get("physicalLocation", {})
                where = f'{loc.get("artifactLocation", {}).get("uri", "?")}:{loc.get("region", {}).get("startLine", "?")}'
                flag = "BLOCKING" if severity >= THRESHOLD else "note"
                print(f'{flag:8} {severity:4.1f} {result.get("ruleId")} {where}: {result.get("message", {}).get("text", "")}')
                if severity >= THRESHOLD:
                    blocking += 1
    print(f"{total} findings, {blocking} at security severity {THRESHOLD} or more")
    return 1 if blocking else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
