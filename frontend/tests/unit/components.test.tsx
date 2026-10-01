import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Button } from "@/components/ui/button";
import { ReasonList } from "@/components/ui/reason-list";
import { RiskBadge } from "@/components/ui/risk-badge";
import { RiskScore } from "@/components/ui/risk-score";
import { TrendLine } from "@/components/ui/chart";
import { sortQueue } from "@/features/alerts/alerts";
import type { Schemas } from "@/lib/api/client";

describe("RiskBadge", () => {
  it("shows the band word, its glyph and the score on the band's ground", () => {
    render(<RiskBadge score={81} />);
    const badge = screen.getByLabelText("High, score 81");
    expect(badge).toHaveTextContent("High · 81");
    expect(badge).toHaveClass("text-risk-high", "bg-risk-high-bg");
    expect(badge.querySelector("svg path")).not.toBeNull();
  });

  it("uses an octagon for Critical and a circle for Minimal", () => {
    render(<RiskBadge score={96} />);
    render(<RiskBadge score={8} showScore={false} />);
    expect(
      screen
        .getByLabelText("Critical, score 96")
        .querySelector("path")
        ?.getAttribute("d"),
    ).toContain("l3 3v4");
    expect(
      screen.getByLabelText("Minimal").querySelector("circle"),
    ).not.toBeNull();
  });
});

describe("RiskScore", () => {
  it("is a meter with a spoken label, the score and its band", () => {
    render(
      <RiskScore
        score={87}
        band="high"
        decision="step_up"
        engineVersion="risk-engine-2.0.0"
      />,
    );
    const meter = screen.getByRole("meter", { name: "Risk score 87, High" });
    expect(meter).toHaveTextContent("87/ 100High");
    expect(meter).toHaveAttribute("aria-valuenow", "87");
    expect(screen.getByText("Step-up")).toBeInTheDocument();
  });

  it("marks a custom calibration", () => {
    render(<RiskScore score={20} decision="allow" customCalibration />);
    expect(screen.getByText("Custom thresholds")).toBeInTheDocument();
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

describe("TrendLine and Button", () => {
  it("draws the primary series in accent", () => {
    render(<TrendLine points={[1, 1, 5, 1]} label="Scores" />);
    const line = screen
      .getByRole("img", { name: "Scores" })
      .querySelector('polyline[data-series="primary"]');
    expect(line).toHaveClass("stroke-accent");
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
