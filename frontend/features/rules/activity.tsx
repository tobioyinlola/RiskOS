"use client";

import { useState } from "react";

import { Select } from "@/components/ui/field";
import { Metric, Panel, PanelBody, PanelHeader } from "@/components/ui/panel";
import { ErrorState, Loading } from "@/components/ui/states";
import type { Schemas } from "@/lib/api/client";
import { formatAge, formatCount, formatTimestamp } from "@/lib/format";
import { useRuleStats } from "@/lib/queries";

type RuleStat = Schemas["RuleStats"]["rules"][number];
type Day = RuleStat["daily"][number];

const dayLabel = (d: string) =>
  new Date(`${d}T00:00:00Z`).toLocaleDateString("en-GB", {
    day: "numeric",
    month: "short",
    timeZone: "UTC",
  });

/** Matches per day as small bars, with the counts spoken for screen readers. */
export function Sparkline({ daily, label }: { daily: Day[]; label: string }) {
  const max = Math.max(1, ...daily.map((d) => d.matches));
  const w = 5;
  const gap = 2;
  const h = 22;
  return (
    <svg
      width={daily.length * (w + gap)}
      height={h}
      role="img"
      aria-label={`${label}: ${daily.map((d) => `${formatCount(d.matches)} on ${dayLabel(d.day)}`).join(", ")}`}
      className="shrink-0"
    >
      {daily.map((d, i) => {
        const bar = d.matches ? Math.max(2, (d.matches / max) * h) : 1;
        return (
          <rect
            key={d.day}
            x={i * (w + gap)}
            y={h - bar}
            width={w}
            height={bar}
            rx={1}
            className={
              d.matches
                ? "fill-current text-text-primary"
                : "fill-current text-border-strong"
            }
          />
        );
      })}
    </svg>
  );
}

/** One rule's activity: matches per day over 7 or 30 days, its share of events and last match. */
export function RuleActivity({ ruleId }: { ruleId: string }) {
  const [days, setDays] = useState(7);
  const q = useRuleStats(days);
  if (q.isLoading) return <Loading label="Loading the rule's activity" />;
  if (q.isError) return <ErrorState error={q.error} />;
  const st = q.data!;
  const r = st.rules.find((x) => x.rule_id === ruleId);
  const matches = r?.matches ?? 0;
  const share = st.evaluations ? (matches / st.evaluations) * 100 : 0;
  const max = Math.max(1, ...(r?.daily ?? []).map((d) => d.matches));
  return (
    <div className="flex flex-col gap-5">
      <div className="flex items-center gap-2">
        <Select
          aria-label="Period"
          className="h-control-sm w-auto text-body-sm"
          value={days}
          onChange={(e) => setDays(Number(e.target.value))}
        >
          <option value={7}>Last 7 days</option>
          <option value={30}>Last 30 days</option>
        </Select>
        <span className="text-label text-text-muted">Updates every minute</span>
      </div>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <Metric label="Events matched" value={formatCount(matches)} />
        <Metric
          label="Share of all events"
          value={`${share > 0 && share < 0.1 ? "<0.1" : share.toFixed(1)}%`}
        />
        <Metric
          label="Last match"
          value={r?.last_matched_at ? formatAge(r.last_matched_at) : "Never"}
        />
      </div>
      <Panel>
        <PanelHeader
          title="Matches per day"
          legend="UTC days, active and monitoring alike"
        />
        <PanelBody>
          {r ? (
            <figure>
              <div
                className="flex h-40 items-end gap-1"
                role="img"
                aria-label={`Matches per day: ${r.daily.map((d) => `${formatCount(d.matches)} on ${dayLabel(d.day)}`).join(", ")}`}
              >
                {r.daily.map((d) => (
                  <div
                    key={d.day}
                    title={`${dayLabel(d.day)}: ${formatCount(d.matches)}`}
                    className={`flex-1 rounded-t-sm ${d.matches ? "bg-ink" : "bg-border-subtle"}`}
                    style={{
                      height: `${d.matches ? Math.max(4, (d.matches / max) * 100) : 2}%`,
                    }}
                  />
                ))}
              </div>
              <figcaption className="mt-2 flex justify-between text-label text-text-muted">
                <span>{dayLabel(r.daily[0]!.day)}</span>
                <span>{dayLabel(r.daily.at(-1)!.day)}</span>
              </figcaption>
            </figure>
          ) : (
            <p className="text-body-sm text-text-secondary">No activity yet.</p>
          )}
          {r?.last_matched_at ? (
            <p className="mt-3 text-label text-text-muted">
              Last matched {formatTimestamp(r.last_matched_at)}, out of{" "}
              {formatCount(st.evaluations)} events in the period.
            </p>
          ) : null}
        </PanelBody>
      </Panel>
    </div>
  );
}
