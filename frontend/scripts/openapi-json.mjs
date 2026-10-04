// Writes the public API contract as JSON for the developer documentation's API reference
// (app/(marketing)/docs/reference), so the reference is built from the same file the server and
// the client are generated from. Run by `pnpm generate:api` (make generate); CI checks drift.
import { readFileSync, writeFileSync } from "node:fs";

import { parse } from "yaml";

const spec = parse(readFileSync("../backend/api/openapi.yaml", "utf8"));
writeFileSync("lib/api/openapi.gen.json", `${JSON.stringify(spec, null, 1)}\n`);
