import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { TickScoreBar } from "@/components/ui/bars";
import { Button } from "@/components/ui/button";
import { Figure } from "@/components/ui/figure";
import { DecisionPill } from "@/components/ui/pill";
import { cn } from "@/lib/cn";

describe("cn", () => {
  it("keeps a text color beside a type-scale size", () => {
    expect(cn("text-white", "text-body")).toBe("text-white text-body");
    expect(cn("text-label text-risk-high")).toBe("text-label text-risk-high");
  });

  it("still lets a later size or color win over an earlier one", () => {
    expect(cn("text-body", "text-label")).toBe("text-label");
    expect(cn("text-text-primary", "text-accent")).toBe("text-accent");
    expect(cn("shadow-card", "shadow-overlay")).toBe("shadow-overlay");
  });
});

describe("Figure", () => {
  it("renders decimals in the faint color", () => {
    const { container } = render(<Figure value="32,940.093" />);
    expect(container).toHaveTextContent("32,940.093");
    expect(screen.getByText("093")).toHaveClass("text-text-faint");
  });

  it("leaves whole numbers and placeholders alone", () => {
    const { container } = render(<Figure value="1,204" />);
    expect(container.querySelector(".text-text-faint")).toBeNull();
    render(<Figure value="—" />);
    expect(screen.getByText("—")).toBeInTheDocument();
  });
});

describe("Button", () => {
  it("draws the primary pill in ink with white text", () => {
    render(
      <Button variant="primary" arrow>
        Review alerts
      </Button>,
    );
    const b = screen.getByRole("button", { name: "Review alerts" });
    expect(b).toHaveClass("bg-ink", "text-white", "rounded-full");
    expect(b.querySelector("svg")).not.toBeNull();
  });
});

describe("DecisionPill", () => {
  it("names the decision on its tone", () => {
    render(<DecisionPill decision="block" />);
    expect(screen.getByText("Block")).toHaveClass("text-status-danger");
  });
});

describe("TickScoreBar", () => {
  it("is a meter filled to the value", () => {
    const { container } = render(
      <TickScoreBar value={50} ticks={10} label="Mean risk score 50 of 100" />,
    );
    const meter = screen.getByRole("meter", {
      name: "Mean risk score 50 of 100",
    });
    expect(meter).toHaveAttribute("aria-valuenow", "50");
    expect(container.querySelectorAll(".bg-brand-blue")).toHaveLength(5);
  });
});
