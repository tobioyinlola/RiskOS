"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";

import { Button } from "@/components/ui/button";
import { KeyValues, pillTabClass } from "@/components/ui/misc";
import { Panel, PanelHeader } from "@/components/ui/panel";
import { StatusPill, type Tone } from "@/components/ui/pill";
import { ErrorState, SkeletonRows } from "@/components/ui/states";
import type { Schemas } from "@/lib/api/client";
import { flag } from "@/lib/compliance";
import { formatCount } from "@/lib/format";
import {
  useActivations,
  useCompliancePacks,
  useJurisdictions,
} from "@/lib/queries";

type Activation = Schemas["JurisdictionActivation"];

/** The "all countries" choice of the switcher. */
export const ALL = "ALL";

/**
 * The country the Rules pages show, from the address (?country=GH), else the workspace's
 * fallback country. `all` is the All countries overview.
 */
export function useCountry(): {
  country: string;
  all: boolean;
  fallback: string | undefined;
  setCountry: (code: string) => void;
} {
  const params = useSearchParams();
  const router = useRouter();
  const path = usePathname();
  const acts = useActivations();
  const fallback = acts.data?.data.find((a) => a.fallback)?.jurisdiction;
  const asked = params.get("country") ?? "";
  const setCountry = (code: string) => {
    const next = new URLSearchParams(params.toString());
    next.set("country", code);
    router.replace(`${path}?${next.toString()}` as "/rules", { scroll: false });
  };
  return {
    country: asked === ALL ? "" : asked || fallback || "",
    all: asked === ALL,
    fallback,
    setCountry,
  };
}

/** How the workspace stands in a country, in words. */
export function countryStatus(
  code: string,
  acts: Activation[],
): { label: string; tone: Tone } {
  const mine = acts.filter((a) => a.jurisdiction === code);
  if (mine.some((a) => a.fallback))
    return { label: "Default country", tone: "success" };
  if (mine.some((a) => a.status === "live"))
    return { label: "Switched on", tone: "success" };
  if (mine.some((a) => a.status === "shadow"))
    return { label: "Monitoring", tone: "warning" };
  if (mine.length) return { label: "Paused", tone: "neutral" };
  return { label: "Not switched on", tone: "neutral" };
}

/** Pick a country to read its rules, or every country side by side. */
export function CountrySwitcher() {
  const jurisdictions = useJurisdictions();
  const acts = useActivations();
  const { country, all, setCountry } = useCountry();
  if (jurisdictions.isError)
    return <ErrorState error={jurisdictions.error} className="mb-4" />;
  const list = jurisdictions.data?.data ?? [];
  const chip = (selected: boolean) => pillTabClass(selected, "gap-2");
  return (
    <nav aria-label="Country" className="mb-5">
      <ul className="flex flex-wrap gap-2">
        <li>
          <button
            type="button"
            className={chip(all)}
            aria-pressed={all}
            onClick={() => setCountry(ALL)}
          >
            All countries
          </button>
        </li>
        {list.map((j) => {
          const selected = !all && country === j.code;
          const status = countryStatus(j.code, acts.data?.data ?? []);
          return (
            <li key={j.code}>
              <button
                type="button"
                className={chip(selected)}
                aria-pressed={selected}
                onClick={() => setCountry(j.code)}
              >
                <span aria-hidden="true">{flag(j.code)}</span>
                {j.name}
                <span className="text-label font-normal">· {status.label}</span>
              </button>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}

/** Every country with a pack: where the workspace stands and what each pack holds. */
export function AllCountries() {
  const jurisdictions = useJurisdictions();
  const packs = useCompliancePacks();
  const acts = useActivations();
  const { setCountry } = useCountry();
  if (jurisdictions.isLoading || packs.isLoading)
    return (
      <Panel>
        <table className="w-full">
          <tbody>
            <SkeletonRows columns={3} rows={3} />
          </tbody>
        </table>
      </Panel>
    );
  if (jurisdictions.isError) return <ErrorState error={jurisdictions.error} />;
  if (packs.isError) return <ErrorState error={packs.error} />;
  return (
    <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
      {(jurisdictions.data?.data ?? []).map((j) => {
        const p = packs.data?.data.find((x) => x.jurisdiction === j.code);
        const status = countryStatus(j.code, acts.data?.data ?? []);
        const entities = (acts.data?.data ?? [])
          .filter((a) => a.jurisdiction === j.code)
          .map((a) => a.booking_entity);
        return (
          <Panel key={j.code}>
            <PanelHeader
              title={
                <span className="flex items-center gap-2">
                  <span aria-hidden="true">{flag(j.code)}</span>
                  {j.name}
                </span>
              }
              actions={
                <StatusPill tone={status.tone}>{status.label}</StatusPill>
              }
            />
            <div className="flex flex-col gap-4 px-5 pb-5">
              <KeyValues
                items={[
                  [
                    "Rules",
                    p
                      ? `${formatCount(p.controls)} required controls, ${formatCount(p.signals)} risk signals, ${formatCount(p.governance)} governance presets`
                      : "–",
                  ],
                  ["Regulators", j.regulators.join(", ")],
                  ["Currency", j.currency],
                  ["Time zone", j.timezone],
                  ...(entities.length
                    ? ([["Booking entities", entities.join(", ")]] as [
                        string,
                        string,
                      ][])
                    : []),
                ]}
              />
              <div>
                <Button size="sm" onClick={() => setCountry(j.code)}>
                  See {j.name}&apos;s rules
                </Button>
              </div>
            </div>
          </Panel>
        );
      })}
    </div>
  );
}
