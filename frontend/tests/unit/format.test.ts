import { describe, expect, it } from "vitest";

import { bufferToB64url, b64urlToBuffer } from "@/lib/webauthn";
import {
  formatAge,
  formatAmount,
  formatTimestamp,
  humanize,
} from "@/lib/format";

describe("format", () => {
  it("formats minor units with the currency's decimals and code", () => {
    expect(formatAmount(450000, "NGN")).toBe("4,500.00 NGN");
    expect(formatAmount(1500, "JPY")).toBe("1,500 JPY");
  });

  it("gives compact ages", () => {
    const now = new Date("2026-09-30T12:00:00Z");
    expect(formatAge("2026-09-30T11:59:48Z", now)).toBe("12s");
    expect(formatAge("2026-09-30T11:57:00Z", now)).toBe("3m");
    expect(formatAge("2026-09-30T08:00:00Z", now)).toBe("4h");
    expect(formatAge("2026-09-27T12:00:00Z", now)).toBe("3d");
  });

  it("prints UTC timestamps and sentence-case labels", () => {
    expect(formatTimestamp("2026-09-30T14:02:11.123Z")).toBe(
      "2026-09-30 14:02:11 UTC",
    );
    expect(humanize("false_positive")).toBe("False positive");
    expect(humanize("alert.created")).toBe("Alert created");
  });
});

describe("passkey encoding", () => {
  it("round-trips base64url without padding", () => {
    const bytes = new Uint8Array([0, 1, 250, 251, 252, 253, 254, 255]);
    const s = bufferToB64url(bytes.buffer);
    expect(s).not.toMatch(/[+/=]/);
    expect(new Uint8Array(b64urlToBuffer(s))).toEqual(bytes);
  });
});
