import type { Route } from "next";

/** The developer documentation's pages, in reading order (app/(marketing)/docs). */
export interface DocLink {
  href: Route;
  label: string;
}

export const DOCS_NAV: { title: string; links: DocLink[] }[] = [
  {
    title: "Getting started",
    links: [
      { href: "/docs", label: "Introduction" },
      { href: "/docs/quickstart" as Route, label: "Quickstart" },
      {
        href: "/docs/authentication" as Route,
        label: "Authentication and keys",
      },
    ],
  },
  {
    title: "Integrating",
    links: [
      { href: "/docs/events" as Route, label: "Sending events" },
      { href: "/docs/decisions" as Route, label: "Acting on decisions" },
      { href: "/docs/errors" as Route, label: "Errors, limits and retries" },
      { href: "/docs/webhooks" as Route, label: "Webhooks" },
    ],
  },
  {
    title: "Configuring",
    links: [
      { href: "/docs/rules" as Route, label: "Rules" },
      { href: "/docs/compliance" as Route, label: "Compliance and rule packs" },
      { href: "/docs/alerts-and-cases" as Route, label: "Alerts and cases" },
    ],
  },
  {
    title: "Launch",
    links: [{ href: "/docs/going-live" as Route, label: "Going live" }],
  },
  {
    title: "Reference",
    links: [{ href: "/docs/reference" as Route, label: "API reference" }],
  },
];

/** The base URL examples use: the API of your RiskOS deployment. */
export const API_URL_EXAMPLE = "https://api.your-riskos-host.example";
