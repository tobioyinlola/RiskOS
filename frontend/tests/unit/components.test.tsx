import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Button } from "@/components/instrument/button";
import { ReasonList } from "@/components/instrument/reason-list";
import { RiskBadge } from "@/components/instrument/risk-badge";
import { RiskScore } from "@/components/instrument/risk-score";
import { TraceLine } from "@/components/instrument/trace-line";
import { sortQueue } from "@/features/alerts/alerts";
import type { Schemas } from "@/lib/api/client";

describe("RiskBadge", () => {
  it("shows the band three ways: bars, word and score", () => {
    render(<RiskBadge score={81} />);
    const badge = screen.getByLabelText("High, score 81");
    expect(badge).toHaveTextContent("High81");
    expect(badge.querySelectorAll("i.opacity-100")).toHaveLength(4);
  });

  it("pads the score to two digits", () => {
    render(<RiskBadge score={8} />);
    expect(screen.getByLabelText("Minimal, score 8")).toHaveTextContent(
      "Minimal08",
    );
  });
});

describe("RiskScore", () => {
  it("is a meter with a spoken label and a three-digit readout", () => {
    render(
      <RiskScore
        score={87}
        band="high"
        decision="step_up"
        engineVersion="risk-engine-2.0.0"
      />,
    );
    const meter = screen.getByRole("meter", {
      name: "Risk score 87 of 100, High",
    });
    expect(meter).toHaveTextContent("087");
    expect(meter).toHaveAttribute("aria-valuenow", "87");
    expect(screen.getByText("Step-up")).toBeInTheDocument();
  });

  it("marks a custom calibration", () => {
    render(<RiskScore score={20} decision="allow" customCalibration />);
    expect(screen.getByText("Custom calibration")).toBeInTheDocument();
  });
});

describe("ReasonList", () => {
  it("ranks reasons by contribution with direction arrows", () => {
    render(
      <ReasonList
        reasons={[
          {
            label: "New country for this user",
            category: "history",
            contribution: 10,
          },
          { label: "Unusual amount", category: "history", contribution: 24 },
          {
            label: "Long-standing account",
            category: "rule",
            contribution: -8,
          },
        ]}
      />,
    );
    const items = screen.getAllByRole("listitem");
    expect(items[0]).toHaveTextContent("Unusual amount");
    expect(items[2]).toHaveTextContent("−8");
    expect(within(items[2]!).getByLabelText("lowers risk")).toHaveTextContent(
      "▼",
    );
  });
});

describe("TraceLine and Button", () => {
  it("draws a labelled trace", () => {
    render(<TraceLine points={[1, 1, 5, 1]} label="Scores" />);
    expect(
      screen
        .getByRole("img", { name: "Scores" })
        .querySelector("polyline.trace"),
    ).not.toBeNull();
  });

  it("shows a shortcut with the label", () => {
    render(<Button shortcut="r">Review</Button>);
    const b = screen.getByRole("button", { name: /Review/ });
    expect(b).toHaveAttribute("aria-keyshortcuts", "r");
    expect(within(b).getByText("R")).toBeInTheDocument();
  });
});

describe("alert queue order", () => {
  it("sorts by band, then score, then oldest first", () => {
    const base: Omit<
      Schemas["Alert"],
      "id" | "severity" | "max_score" | "last_seen_at"
    > = {
      kind: "risk",
      subject: { type: "user", ref: "u" },
      title: "t",
      status: "open",
      assignee: null,
      case_id: null,
      evaluation_count: 1,
      first_seen_at: "",
      created_at: "",
      updated_at: "",
    };
    const sorted = sortQueue([
      {
        ...base,
        id: "a",
        severity: "high",
        max_score: 72,
        last_seen_at: "2026-09-30T10:00:00Z",
      },
      {
        ...base,
        id: "b",
        severity: "critical",
        max_score: 95,
        last_seen_at: "2026-09-30T11:00:00Z",
      },
      {
        ...base,
        id: "c",
        severity: "high",
        max_score: 72,
        last_seen_at: "2026-09-30T09:00:00Z",
      },
      {
        ...base,
        id: "d",
        severity: "high",
        max_score: 80,
        last_seen_at: "2026-09-30T12:00:00Z",
      },
    ]);
    expect(sorted.map((a) => a.id)).toEqual(["b", "d", "c", "a"]);
  });
});
