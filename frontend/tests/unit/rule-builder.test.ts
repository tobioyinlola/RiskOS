import { describe as group, expect, it } from "vitest";

import {
  codeFromName,
  describe,
  fromCondition,
  newClause,
  toCondition,
  treeText,
  type CatalogField,
  type Draft,
} from "@/lib/rule-builder";

const field = (
  f: Partial<CatalogField> & Pick<CatalogField, "name" | "type">,
): CatalogField => ({
  operators: [],
  label: f.name,
  group: "Event",
  events: [],
  ...f,
});

const CATALOG: CatalogField[] = [
  field({
    name: "event.type",
    type: "string",
    label: "Event type",
    values: [
      { value: "transaction", label: "Transaction" },
      { value: "login", label: "Login" },
    ],
  }),
  field({
    name: "transaction.amount",
    type: "number",
    label: "Amount",
    unit: "money",
    events: ["transaction"],
  }),
  field({
    name: "feature.new_beneficiary",
    type: "boolean",
    label: "New beneficiary for this customer",
  }),
  field({
    name: "beneficiary.country",
    type: "string",
    label: "Beneficiary's country",
    kind: "country",
  }),
  field({
    name: "transaction.channel",
    type: "string",
    label: "Channel",
    values: [
      { value: "card", label: "Card" },
      { value: "wallet", label: "Wallet" },
    ],
  }),
];

const clause = (
  field: string,
  op: Draft["items"][number] extends never
    ? never
    : "eq" | "gt" | "in" | "not_in",
  value: unknown,
) => ({
  ...newClause(field, op),
  value: value as never,
});

group("the rule builder", () => {
  it("turns When / If / Then into the engine's condition, money in minor units", () => {
    const d: Draft = {
      when: ["transaction"],
      match: "all",
      items: [
        clause("transaction.amount", "gt", 1_000_000),
        clause("feature.new_beneficiary", "eq", true),
      ],
    };
    expect(toCondition(d, CATALOG)).toEqual({
      tree: {
        all: [
          { field: "event.type", op: "eq", value: "transaction" },
          { field: "transaction.amount", op: "gt", value: 100_000_000 },
          { field: "feature.new_beneficiary", op: "eq", value: true },
        ],
      },
    });
  });

  it("keeps an or-list and groups, and reads them back", () => {
    const d: Draft = {
      when: ["transaction", "login"],
      match: "any",
      items: [
        clause("beneficiary.country", "in", ["RU", "KP"]),
        {
          kind: "group",
          id: "g",
          match: "all",
          clauses: [
            clause("transaction.channel", "not_in", ["card"]),
            clause("feature.new_beneficiary", "eq", false),
          ],
        },
      ],
    };
    const r = toCondition(d, CATALOG);
    expect(r).toEqual({
      tree: {
        all: [
          { field: "event.type", op: "in", value: ["transaction", "login"] },
          {
            any: [
              { field: "beneficiary.country", op: "in", value: ["RU", "KP"] },
              {
                all: [
                  { field: "transaction.channel", op: "ne", value: "card" },
                  { field: "feature.new_beneficiary", op: "eq", value: false },
                ],
              },
            ],
          },
        ],
      },
    });
    const back = fromCondition("tree" in r ? r.tree : null, CATALOG);
    expect(back).not.toBeNull();
    expect(back!.when).toEqual(["transaction", "login"]);
    expect(back!.match).toBe("any");
    expect(back!.items).toHaveLength(2);
    expect(back!.items[1]).toMatchObject({ kind: "group", match: "all" });
  });

  it("reads stored kobo back as naira", () => {
    const d = fromCondition(
      { field: "transaction.amount", op: "gte", value: 5_000_050 },
      CATALOG,
    );
    expect(d!.items[0]).toMatchObject({ op: "gte", value: 50_000.5 });
  });

  it("says the rule in plain words", () => {
    const d: Draft = {
      when: ["transaction"],
      match: "all",
      items: [
        clause("transaction.amount", "gt", 1_000_000),
        clause("beneficiary.country", "in", ["GB", "US"]),
      ],
    };
    expect(describe(d, CATALOG, { weight: 40, severity: "high" })).toBe(
      "When a transaction happens, if amount is more than 1,000,000.00 and beneficiary's country is United Kingdom or United States, add 40 risk points (high severity).",
    );
  });

  it("names what is missing instead of saving half a rule", () => {
    const d: Draft = {
      when: [],
      match: "all",
      items: [newClause("transaction.amount", "gt")],
    };
    expect(toCondition(d, CATALOG)).toEqual({
      problem: "Fill in a value for “Amount”.",
    });
    expect(toCondition({ when: [], match: "all", items: [] }, CATALOG)).toEqual(
      {
        problem: "Add at least one condition.",
      },
    );
  });

  it("leaves logic it cannot show to words", () => {
    const tree = {
      not: { field: "feature.new_beneficiary", op: "eq", value: true },
    };
    expect(fromCondition(tree, CATALOG)).toBeNull();
    expect(treeText(tree, CATALOG)).toBe(
      "not new beneficiary for this customer is yes",
    );
  });

  it("makes a unique code from the name", () => {
    expect(codeFromName("Big transfer to a new beneficiary!", new Set())).toBe(
      "big_transfer_to_a_new_beneficiary",
    );
    expect(codeFromName("Big transfer", new Set(["big_transfer"]))).toBe(
      "big_transfer_2",
    );
    expect(codeFromName("3D Secure off", new Set())).toBe("rule_3d_secure_off");
  });
});
