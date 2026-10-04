import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { PenaltyRegister } from "@/features/compliance/penalties";
import { countryStatus } from "@/features/rules/countries";
import type { Schemas } from "@/lib/api/client";
import { flag, lineageText, PROVENANCE } from "@/lib/compliance";

type Rule = Schemas["ComplianceRule"];
type Activation = Schemas["JurisdictionActivation"];

const rule = (r: Partial<Rule>): Rule =>
  ({
    id: "GH-TM-01",
    name: "New device, new payee, then large transfer",
    ...r,
  }) as Rule;

describe("lineageText", () => {
  it("says a parameterised rule is a country version, and what it changes", () => {
    expect(
      lineageText(
        rule({
          lineage: { kind: "param", of: ["ATO-01"] },
          changes: ["floor"],
        }),
      ),
    ).toBe("A country version of Nigeria's ATO-01, with its own floor.");
  });
  it("says an inherited rule comes from Nigeria's library under its own ID", () => {
    expect(
      lineageText(
        rule({ id: "SEC-05", inherited_from: "ng", changes: ["high_value"] }),
      ),
    ).toBe(
      "Inherited from the Nigeria library's SEC-05, with its own high_value.",
    );
  });
  it("names a new local rule's related rules when the library does", () => {
    expect(
      lineageText(
        rule({ lineage: { kind: "new_local", of: ["CRY-01", "CRY-08"] } }),
      ),
    ).toBe("New for this country (related: CRY-01 and CRY-08).");
    expect(lineageText(rule({ lineage: { kind: "new_local", of: [] } }))).toBe(
      "New for this country.",
    );
  });
  it("does not invent a Nigerian rule the library does not name", () => {
    expect(lineageText(rule({ lineage: { kind: "param", of: [] } }))).toBe(
      "A country version of a Nigerian practice.",
    );
  });
});

describe("country helpers", () => {
  it("draws flags from country codes", () => {
    expect(flag("GH")).toBe("🇬🇭");
    expect(flag("rw")).toBe("🇷🇼");
  });
  it("labels the library's design as starting values, not regulation", () => {
    expect(PROVENANCE.my_design.meaning).toMatch(/not regulation/);
  });
  it("says where the workspace stands in each country", () => {
    const act = (a: Partial<Activation>): Activation =>
      ({
        jurisdiction: "NG",
        status: "live",
        fallback: false,
        booking_entity: "x",
        ...a,
      }) as Activation;
    const acts = [
      act({ jurisdiction: "NG", fallback: true }),
      act({ jurisdiction: "GH", status: "shadow" }),
    ];
    expect(countryStatus("NG", acts).label).toBe("Default country");
    expect(countryStatus("GH", acts).label).toBe("Monitoring");
    expect(countryStatus("RW", acts).label).toBe("Not switched on");
  });
});

describe("PenaltyRegister", () => {
  it("marks estimates that are the library's inference", () => {
    render(
      <PenaltyRegister
        pack={
          {
            name: "Ghana",
            currency: "GHS",
            penalties: [
              {
                provision: "Act 987 s.35: CDD non-compliance",
                penalty: "2,000 units",
                estimate: "GHS 24,000",
                inference: true,
                sources: ["GS24"],
              },
            ],
          } as Schemas["CompliancePack"]
        }
      />,
    );
    const row = screen.getByRole("row", { name: /CDD non-compliance/ });
    expect(within(row).getByText("GHS 24,000")).toBeInTheDocument();
    expect(within(row).getByText("Estimate")).toBeInTheDocument();
    expect(
      screen.getByText(/one penalty unit is taken as GHS 12/),
    ).toBeInTheDocument();
  });
  it("shows nothing for a country without a register", () => {
    const { container } = render(
      <PenaltyRegister
        pack={
          {
            name: "Nigeria",
            penalties: [],
          } as unknown as Schemas["CompliancePack"]
        }
      />,
    );
    expect(container).toBeEmptyDOMElement();
  });
});
