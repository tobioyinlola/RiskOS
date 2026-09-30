import { render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { SectionPlaceholder } from "@/components/section-placeholder";
import { AppShell } from "@/components/shell/app-shell";
import { navSections } from "@/lib/navigation";

vi.mock("next/navigation", () => ({ usePathname: () => "/alerts" }));

describe("AppShell", () => {
  it("renders every section in the primary navigation and marks the current one", () => {
    render(
      <AppShell>
        <p>content</p>
      </AppShell>,
    );

    const nav = screen.getByRole("navigation", { name: "Primary" });
    const links = within(nav).getAllByRole("link");
    expect(links.map((l) => l.textContent)).toEqual(
      navSections.map((s) => s.label),
    );
    expect(within(nav).getByRole("link", { name: "Alerts" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(
      within(nav).getByRole("link", { name: "Cases" }),
    ).not.toHaveAttribute("aria-current");
    expect(screen.getByText("content")).toBeInTheDocument();
  });
});

describe("SectionPlaceholder", () => {
  it("states the section is not built and shows no data", () => {
    render(<SectionPlaceholder href="/alerts" />);

    expect(
      screen.getByRole("heading", { level: 1, name: "Alerts" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Not built yet")).toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });
});
