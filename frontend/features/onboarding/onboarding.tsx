"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { useMemo, useState, type ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { Field, Input, Select } from "@/components/ui/field";
import {
  Check,
  CheckCircle,
  Close,
  Globe,
  History,
  Lock,
  Organization,
  Report,
  ShieldCheck,
  Sliders,
  type Icon,
} from "@/components/ui/icons";
import { StatusPill } from "@/components/ui/pill";
import { ErrorState, Loading, Notice } from "@/components/ui/states";
import { Wordmark } from "@/components/ui/wordmark";
import { api, ApiError, unwrap, type Schemas } from "@/lib/api/client";
import { cn } from "@/lib/cn";
import { useSession } from "@/lib/session";

type Answers = Schemas["OnboardingAnswers"];
type OnboardingT = Schemas["Onboarding"];

export const ONBOARDING_KEY = ["onboarding"] as const;

/** The workspace's onboarding; null for workspaces created by RiskOS (they have none). */
export function useOnboarding(enabled = true) {
  return useQuery({
    queryKey: ONBOARDING_KEY,
    enabled,
    queryFn: async () => {
      try {
        return unwrap(await api().GET("/api/v1/onboarding"));
      } catch (e) {
        if (e instanceof ApiError && e.status === 404) return null;
        throw e;
      }
    },
  });
}

/* ---------------------------------------------------------------- vocabularies */

const INDUSTRIES: { value: NonNullable<Answers["industry"]>; label: string }[] =
  [
    { value: "payments", label: "Payments" },
    { value: "lending", label: "Lending" },
    { value: "banking", label: "Banking" },
    { value: "crypto", label: "Crypto / VASP" },
    { value: "marketplace", label: "Marketplace" },
    { value: "insurance", label: "Insurance" },
    { value: "gaming", label: "Gaming" },
    { value: "other", label: "Other" },
  ];
const EVENTS: {
  value: NonNullable<Answers["monitored_events"]>[number];
  label: string;
}[] = [
  { value: "payments", label: "Payments" },
  { value: "payouts", label: "Payouts" },
  { value: "transfers", label: "Transfers" },
  { value: "card_transactions", label: "Card transactions" },
  { value: "logins", label: "Logins" },
  { value: "signups", label: "Sign-ups" },
];
const VOLUMES: {
  value: NonNullable<Answers["monthly_volume"]>;
  label: string;
}[] = [
  { value: "under_10k", label: "Under 10k" },
  { value: "10k_100k", label: "10k – 100k" },
  { value: "100k_1m", label: "100k – 1M" },
  { value: "1m_10m", label: "1M – 10M" },
  { value: "over_10m", label: "Over 10M" },
];
const GO_LIVE: { value: NonNullable<Answers["go_live"]>; label: string }[] = [
  { value: "now", label: "Now" },
  { value: "within_3_months", label: "Within 3 months" },
  { value: "within_6_months", label: "Within 6 months" },
  { value: "later", label: "Later" },
];
const PRIVACY: {
  value: NonNullable<Answers["data_protection"]>[number];
  label: string;
}[] = [
  { value: "gdpr", label: "GDPR (EU)" },
  { value: "uk_gdpr", label: "UK GDPR" },
  { value: "ndpr", label: "NDPR (Nigeria)" },
  { value: "popia", label: "POPIA (South Africa)" },
  { value: "ccpa", label: "CCPA (California)" },
  { value: "lgpd", label: "LGPD (Brazil)" },
  { value: "other", label: "Other" },
];
const RESIDENCY: {
  value: NonNullable<Answers["data_residency"]>;
  label: string;
}[] = [
  { value: "eu", label: "European Union" },
  { value: "uk", label: "United Kingdom" },
  { value: "us", label: "United States" },
  { value: "africa", label: "Africa" },
  { value: "no_preference", label: "No preference" },
];

// ISO 3166-1 alpha-2 codes; names come from the browser (Intl.DisplayNames).
const COUNTRY_CODES =
  "AD AE AF AG AI AL AM AO AR AT AU AW AZ BA BB BD BE BF BG BH BI BJ BM BN BO BR BS BT BW BY BZ CA CD CF CG CH CI CL CM CN CO CR CU CV CY CZ DE DJ DK DM DO DZ EC EE EG ER ES ET FI FJ FM FR GA GB GD GE GH GI GM GN GQ GR GT GW GY HK HN HR HT HU ID IE IL IN IQ IR IS IT JM JO JP KE KG KH KI KM KN KR KW KY KZ LA LB LC LI LK LR LS LT LU LV LY MA MC MD ME MG MH MK ML MM MN MO MR MT MU MV MW MX MY MZ NA NE NG NI NL NO NP NR NZ OM PA PE PG PH PK PL PR PS PT PW PY QA RO RS RU RW SA SB SC SD SE SG SI SK SL SM SN SO SR SS ST SV SY SZ TD TG TH TJ TL TM TN TO TR TT TV TW TZ UA UG US UY UZ VC VE VN VU WS YE ZA ZM ZW".split(
    " ",
  );

function useCountries() {
  return useMemo(() => {
    let names: Intl.DisplayNames | undefined;
    try {
      names = new Intl.DisplayNames(["en"], { type: "region" });
    } catch {
      names = undefined;
    }
    return COUNTRY_CODES.map((code) => ({
      code,
      name: names?.of(code) ?? code,
    })).sort((a, b) => a.name.localeCompare(b.name));
  }, []);
}

/* ---------------------------------------------------------------- small controls */

/** Single choice as pills (a radio group). */
function ChoicePills<T extends string>({
  label,
  options,
  value,
  onChange,
  disabled,
}: {
  label: string;
  options: { value: T; label: string }[];
  value: T | undefined;
  onChange: (v: T) => void;
  disabled?: boolean;
}) {
  return (
    <fieldset className="flex flex-col gap-2">
      <legend className="mb-2 text-label text-text-primary">{label}</legend>
      <div className="flex flex-wrap gap-2">
        {options.map((o) => (
          <label
            key={o.value}
            className={cn(
              "inline-flex h-control-md cursor-pointer items-center rounded-full border px-4 text-body-sm font-medium transition-colors duration-150 has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 has-[:focus-visible]:outline-focus-ring",
              value === o.value
                ? "border-ink bg-ink text-white"
                : "border-border-strong bg-bg-surface text-text-secondary hover:border-border-control hover:text-text-primary",
              disabled && "pointer-events-none opacity-60",
            )}
          >
            <input
              type="radio"
              className="sr-only"
              name={label}
              checked={value === o.value}
              onChange={() => onChange(o.value)}
              disabled={disabled}
            />
            {o.label}
          </label>
        ))}
      </div>
    </fieldset>
  );
}

/** Several choices as toggling pills (checkboxes). */
function MultiPills<T extends string>({
  label,
  hint,
  options,
  value,
  onChange,
  disabled,
}: {
  label: string;
  hint?: string;
  options: { value: T; label: string }[];
  value: T[] | undefined;
  onChange: (v: T[]) => void;
  disabled?: boolean;
}) {
  const set = new Set(value ?? []);
  return (
    <fieldset className="flex flex-col gap-2">
      <legend className="mb-1 text-label text-text-primary">{label}</legend>
      {hint ? (
        <p className="mb-1 text-body-sm text-text-muted">{hint}</p>
      ) : null}
      <div className="flex flex-wrap gap-2">
        {options.map((o) => {
          const on = set.has(o.value);
          return (
            <label
              key={o.value}
              className={cn(
                "inline-flex h-control-md cursor-pointer items-center gap-1.5 rounded-full border px-4 text-body-sm font-medium transition-colors duration-150 has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 has-[:focus-visible]:outline-focus-ring",
                on
                  ? "border-brand-blue bg-blue-50 text-blue-600"
                  : "border-border-strong bg-bg-surface text-text-secondary hover:border-border-control hover:text-text-primary",
                disabled && "pointer-events-none opacity-60",
              )}
            >
              <input
                type="checkbox"
                className="sr-only"
                checked={on}
                disabled={disabled}
                onChange={() => {
                  const next = new Set(set);
                  if (on) next.delete(o.value);
                  else next.add(o.value);
                  onChange(
                    options.map((x) => x.value).filter((v) => next.has(v)),
                  );
                }}
              />
              {on ? <Check className="size-3.5" aria-hidden="true" /> : null}
              {o.label}
            </label>
          );
        })}
      </div>
    </fieldset>
  );
}

function YesNo({
  label,
  value,
  onChange,
  disabled,
}: {
  label: string;
  value: boolean | undefined;
  onChange: (v: boolean) => void;
  disabled?: boolean;
}) {
  return (
    <ChoicePills
      label={label}
      options={[
        { value: "yes", label: "Yes" },
        { value: "no", label: "No" },
      ]}
      value={value === undefined ? undefined : value ? "yes" : "no"}
      onChange={(v) => onChange(v === "yes")}
      disabled={disabled}
    />
  );
}

/** Countries as removable chips with a select to add one. */
function CountryList({
  label,
  value,
  onChange,
  disabled,
}: {
  label: string;
  value: string[] | undefined;
  onChange: (v: string[]) => void;
  disabled?: boolean;
}) {
  const countries = useCountries();
  const names = new Map(countries.map((c) => [c.code, c.name]));
  const list = value ?? [];
  return (
    <div className="flex flex-col gap-2">
      <Field
        label={label}
        hint="Where your customers are. Add each country you serve."
      >
        {(p) => (
          <Select
            {...p}
            value=""
            disabled={disabled || list.length >= 60}
            onChange={(e) => {
              if (e.target.value && !list.includes(e.target.value))
                onChange([...list, e.target.value]);
            }}
          >
            <option value="">Add a country…</option>
            {countries
              .filter((c) => !list.includes(c.code))
              .map((c) => (
                <option key={c.code} value={c.code}>
                  {c.name}
                </option>
              ))}
          </Select>
        )}
      </Field>
      {list.length ? (
        <ul className="flex flex-wrap gap-2" aria-label="Countries served">
          {list.map((code) => (
            <li
              key={code}
              className="inline-flex items-center gap-1 rounded-full border border-border-strong bg-bg-surface py-1 pr-1 pl-3 text-body-sm"
            >
              {names.get(code) ?? code}
              <button
                type="button"
                disabled={disabled}
                onClick={() => onChange(list.filter((c) => c !== code))}
                className="inline-flex size-6 items-center justify-center rounded-full text-text-muted hover:bg-bg-sunken hover:text-text-primary"
                aria-label={`Remove ${names.get(code) ?? code}`}
              >
                <Close className="size-3.5" />
              </button>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}

function Check2({
  checked,
  onChange,
  disabled,
  children,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  disabled?: boolean;
  children: ReactNode;
}) {
  return (
    <label className="flex cursor-pointer items-start gap-3 rounded-md border border-border-subtle bg-bg-surface p-4 text-body-sm has-[:checked]:border-brand-blue has-[:checked]:bg-blue-50">
      <input
        type="checkbox"
        className="mt-0.5 size-4 accent-ink"
        checked={checked}
        disabled={disabled}
        onChange={(e) => onChange(e.target.checked)}
      />
      <span>{children}</span>
    </label>
  );
}

/* ---------------------------------------------------------------- steps */

interface StepProps {
  a: Answers;
  set: (patch: Partial<Answers>) => void;
  readOnly: boolean;
}

const STEPS: {
  key: string;
  title: string;
  accent: string;
  label: string;
  intro: string;
  icon: Icon;
  fields: (keyof Answers)[];
  render: (p: StepProps) => ReactNode;
}[] = [
  {
    key: "business",
    label: "Business",
    title: "Tell us about your",
    accent: "business.",
    intro: "The legal entity that will use RiskOS, as registered.",
    icon: Organization,
    fields: [
      "legal_name",
      "country",
      "registration_number",
      "website",
      "industry",
    ],
    render: ({ a, set, readOnly }) => (
      <>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Legal name">
            {(p) => (
              <Input
                {...p}
                value={a.legal_name ?? ""}
                onChange={(e) => set({ legal_name: e.target.value })}
                disabled={readOnly}
                maxLength={200}
              />
            )}
          </Field>
          <Field label="Trading name" hint="If different">
            {(p) => (
              <Input
                {...p}
                value={a.trading_name ?? ""}
                onChange={(e) => set({ trading_name: e.target.value })}
                disabled={readOnly}
                maxLength={200}
              />
            )}
          </Field>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <CountrySelect
            value={a.country}
            onChange={(country) => set({ country })}
            disabled={readOnly}
          />
          <Field label="Registration number" hint="Company or business number">
            {(p) => (
              <Input
                {...p}
                value={a.registration_number ?? ""}
                onChange={(e) => set({ registration_number: e.target.value })}
                disabled={readOnly}
                maxLength={100}
              />
            )}
          </Field>
        </div>
        <Field label="Website">
          {(p) => (
            <Input
              {...p}
              type="url"
              placeholder="https://"
              value={a.website ?? ""}
              onChange={(e) => set({ website: e.target.value })}
              disabled={readOnly}
              maxLength={300}
            />
          )}
        </Field>
        <ChoicePills
          label="Industry"
          options={INDUSTRIES}
          value={a.industry}
          onChange={(industry) => set({ industry })}
          disabled={readOnly}
        />
      </>
    ),
  },
  {
    key: "regulation",
    label: "Regulation",
    title: "How you are",
    accent: "regulated.",
    intro: "Your licences and anti-money-laundering programme.",
    icon: ShieldCheck,
    fields: [
      "regulated",
      "regulator",
      "licence_number",
      "aml_program",
      "compliance_officer_name",
      "compliance_officer_email",
    ],
    render: ({ a, set, readOnly }) => (
      <>
        <YesNo
          label="Are you licensed or registered with a financial regulator?"
          value={a.regulated}
          onChange={(regulated) => set({ regulated })}
          disabled={readOnly}
        />
        {a.regulated ? (
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Regulator">
              {(p) => (
                <Input
                  {...p}
                  value={a.regulator ?? ""}
                  onChange={(e) => set({ regulator: e.target.value })}
                  disabled={readOnly}
                  maxLength={200}
                />
              )}
            </Field>
            <Field label="Licence number">
              {(p) => (
                <Input
                  {...p}
                  value={a.licence_number ?? ""}
                  onChange={(e) => set({ licence_number: e.target.value })}
                  disabled={readOnly}
                  maxLength={100}
                />
              )}
            </Field>
          </div>
        ) : null}
        <YesNo
          label="Do you have a documented AML/CFT programme?"
          value={a.aml_program}
          onChange={(aml_program) => set({ aml_program })}
          disabled={readOnly}
        />
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="Compliance officer">
            {(p) => (
              <Input
                {...p}
                autoComplete="off"
                value={a.compliance_officer_name ?? ""}
                onChange={(e) =>
                  set({ compliance_officer_name: e.target.value })
                }
                disabled={readOnly}
                maxLength={100}
              />
            )}
          </Field>
          <Field label="Compliance officer's email">
            {(p) => (
              <Input
                {...p}
                type="email"
                value={a.compliance_officer_email ?? ""}
                onChange={(e) =>
                  set({ compliance_officer_email: e.target.value })
                }
                disabled={readOnly}
                maxLength={254}
              />
            )}
          </Field>
        </div>
        <Field label="Sanctions and PEP screening provider" hint="Optional">
          {(p) => (
            <Input
              {...p}
              value={a.screening_provider ?? ""}
              onChange={(e) => set({ screening_provider: e.target.value })}
              disabled={readOnly}
              maxLength={200}
            />
          )}
        </Field>
      </>
    ),
  },
  {
    key: "use",
    label: "Use case",
    title: "What you will",
    accent: "monitor.",
    intro: "So we can size your workspace and tune the defaults.",
    icon: Sliders,
    fields: ["monitored_events", "monthly_volume", "regions_served", "go_live"],
    render: ({ a, set, readOnly }) => (
      <>
        <MultiPills
          label="Events you will send"
          hint="Choose all that apply."
          options={EVENTS}
          value={a.monitored_events}
          onChange={(monitored_events) => set({ monitored_events })}
          disabled={readOnly}
        />
        <ChoicePills
          label="Expected events per month"
          options={VOLUMES}
          value={a.monthly_volume}
          onChange={(monthly_volume) => set({ monthly_volume })}
          disabled={readOnly}
        />
        <CountryList
          label="Countries you serve"
          value={a.regions_served}
          onChange={(regions_served) => set({ regions_served })}
          disabled={readOnly}
        />
        <ChoicePills
          label="When do you plan to go live?"
          options={GO_LIVE}
          value={a.go_live}
          onChange={(go_live) => set({ go_live })}
          disabled={readOnly}
        />
      </>
    ),
  },
  {
    key: "data",
    label: "Data & privacy",
    title: "How you handle",
    accent: "data.",
    intro: "The privacy rules you follow and where your data should live.",
    icon: Lock,
    fields: ["data_protection", "data_residency", "accept_terms", "accept_dpa"],
    render: ({ a, set, readOnly }) => (
      <>
        <MultiPills
          label="Data protection laws you follow"
          options={PRIVACY}
          value={a.data_protection}
          onChange={(data_protection) => set({ data_protection })}
          disabled={readOnly}
        />
        <ChoicePills
          label="Preferred data residency"
          options={RESIDENCY}
          value={a.data_residency}
          onChange={(data_residency) => set({ data_residency })}
          disabled={readOnly}
        />
        <Field label="Data protection officer's email" hint="Optional">
          {(p) => (
            <Input
              {...p}
              type="email"
              value={a.dpo_email ?? ""}
              onChange={(e) => set({ dpo_email: e.target.value })}
              disabled={readOnly}
              maxLength={254}
            />
          )}
        </Field>
        <div className="flex flex-col gap-3">
          <Check2
            checked={!!a.accept_terms}
            onChange={(accept_terms) => set({ accept_terms })}
            disabled={readOnly}
          >
            I accept the RiskOS terms of service on behalf of{" "}
            {a.legal_name || "my business"}.
          </Check2>
          <Check2
            checked={!!a.accept_dpa}
            onChange={(accept_dpa) => set({ accept_dpa })}
            disabled={readOnly}
          >
            I accept the data processing agreement, under which RiskOS processes
            event data on our behalf.
          </Check2>
        </div>
      </>
    ),
  },
];

function CountrySelect({
  value,
  onChange,
  disabled,
}: {
  value: string | undefined;
  onChange: (v: string) => void;
  disabled?: boolean;
}) {
  const countries = useCountries();
  return (
    <Field label="Country of incorporation">
      {(p) => (
        <Select
          {...p}
          value={value ?? ""}
          onChange={(e) => onChange(e.target.value)}
          disabled={disabled}
        >
          <option value="" disabled>
            Choose a country…
          </option>
          {countries.map((c) => (
            <option key={c.code} value={c.code}>
              {c.name}
            </option>
          ))}
        </Select>
      )}
    </Field>
  );
}

const FIELD_LABEL: Partial<Record<keyof Answers, string>> = {
  legal_name: "Legal name",
  country: "Country of incorporation",
  registration_number: "Registration number",
  website: "Website",
  industry: "Industry",
  regulated: "Regulated or not",
  regulator: "Regulator",
  licence_number: "Licence number",
  aml_program: "AML/CFT programme",
  compliance_officer_name: "Compliance officer",
  compliance_officer_email: "Compliance officer's email",
  monitored_events: "Events you will send",
  monthly_volume: "Expected volume",
  regions_served: "Countries you serve",
  go_live: "Go-live plan",
  data_protection: "Data protection laws",
  data_residency: "Data residency",
  accept_terms: "Terms of service",
  accept_dpa: "Data processing agreement",
};

/* ---------------------------------------------------------------- page */

/** /onboarding: the compliance onboarding of a self-serve workspace. */
export function Onboarding() {
  const { session, can, refresh } = useSession();
  const q = useOnboarding();
  const qc = useQueryClient();
  const [step, setStep] = useState(0);
  const [answers, setAnswers] = useState<Answers>({});
  const [loaded, setLoaded] = useState(false);
  const [editing, setEditing] = useState(false);

  // Take the saved answers once they arrive (adjusting state while rendering, once).
  if (q.data && !loaded) {
    const missing = q.data.missing;
    setAnswers(q.data.answers);
    setLoaded(true);
    // Resume at the first step with something missing.
    const first = STEPS.findIndex((s) =>
      s.fields.some((f) => missing.includes(f)),
    );
    setStep(first === -1 ? STEPS.length : first);
  }

  const save = useMutation({
    mutationFn: async (a: Answers) =>
      unwrap(await api().PUT("/api/v1/onboarding", { body: a })),
    onSuccess: (o) => qc.setQueryData(ONBOARDING_KEY, o),
  });
  const submit = useMutation({
    mutationFn: async () =>
      unwrap(await api().POST("/api/v1/onboarding/submit")),
    onSuccess: async (o) => {
      qc.setQueryData(ONBOARDING_KEY, o);
      await refresh();
    },
  });

  const o = q.data;
  const canEdit =
    can("team:write") && (o?.status === "draft" || o?.status === "rejected");

  const goTo = async (next: number) => {
    if (canEdit) {
      try {
        await save.mutateAsync(answers);
      } catch {
        return; // the error shows below the form
      }
    }
    setStep(next);
    window.scrollTo({ top: 0 });
  };

  let body: ReactNode;
  if (q.isLoading || (o && !loaded))
    body = <Loading label="Loading your onboarding" />;
  else if (q.isError) body = <ErrorState error={q.error} />;
  else if (!o) {
    body = (
      <Status
        icon={CheckCircle}
        title="Nothing to"
        accent="complete."
        tone="success"
      >
        This workspace was set up by RiskOS, so it has no compliance onboarding.
      </Status>
    );
  } else if (
    (o.status === "submitted" || o.status === "approved") &&
    !editing
  ) {
    body = <Submitted o={o} onReview={() => setEditing(true)} />;
  } else if (step >= STEPS.length) {
    body = (
      <Review
        o={o}
        answers={answers}
        canEdit={canEdit}
        onEdit={(i) => setStep(i)}
        onSubmit={() => submit.mutate()}
        submitting={submit.isPending}
        error={submit.error ?? save.error}
        onBack={() => (editing ? setEditing(false) : setStep(STEPS.length - 1))}
      />
    );
  } else {
    const s = STEPS[step]!;
    body = (
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void goTo(step + 1);
        }}
        className="flex flex-col gap-6"
      >
        <header>
          <p className="mb-2 text-body-sm text-text-muted">
            Step {step + 1} of {STEPS.length + 1}
          </p>
          <h1 className="text-display">
            {s.title} <span className="accent-italic">{s.accent}</span>
          </h1>
          <p className="mt-3 text-body-lg text-text-secondary">{s.intro}</p>
        </header>
        {o.status === "rejected" && o.review_note ? (
          <Notice tone="warn">
            <b className="font-semibold">RiskOS asked for changes:</b>{" "}
            {o.review_note}
          </Notice>
        ) : null}
        {!can("team:write") ? (
          <Notice tone="info">
            Only workspace administrators can change the onboarding.
          </Notice>
        ) : null}
        {s.render({
          a: answers,
          set: (patch) => setAnswers((a) => ({ ...a, ...patch })),
          readOnly: !canEdit,
        })}
        {save.error ? (
          <Notice tone="error">
            {save.error instanceof ApiError
              ? save.error.message
              : "Saving failed. Try again."}
          </Notice>
        ) : null}
        <div className="flex flex-wrap items-center gap-3 border-t border-border-subtle pt-6">
          <Button
            type="submit"
            variant="primary"
            arrow
            disabled={save.isPending}
          >
            {canEdit ? "Save and continue" : "Continue"}
          </Button>
          {step > 0 ? (
            <Button variant="ghost" onClick={() => setStep(step - 1)}>
              Back
            </Button>
          ) : null}
          <span className="ml-auto text-label text-text-muted">
            {canEdit ? "Saved as you go. You can leave and come back." : null}
          </span>
        </div>
      </form>
    );
  }

  const done = (i: number) =>
    o && !STEPS[i]!.fields.some((f) => o.missing.includes(f));
  return (
    <div className="min-h-dvh bg-bg-surface">
      <header className="sticky top-0 z-10 border-b border-border-subtle bg-bg-surface/90 backdrop-blur-md">
        <div className="mx-auto flex h-16 max-w-6xl items-center gap-4 px-6">
          <Link href="/" aria-label="RiskOS console">
            <Wordmark />
          </Link>
          <span className="hidden truncate text-body-sm text-text-muted sm:inline">
            {session.tenant.name}
          </span>
          <Button size="sm" className="ml-auto" asChild>
            <Link href="/">Save and go to console</Link>
          </Button>
        </div>
      </header>
      <div className="mx-auto grid max-w-6xl gap-10 px-6 py-10 lg:grid-cols-[260px_minmax(0,1fr)]">
        <aside className="lg:sticky lg:top-26 lg:self-start">
          <p className="mb-1 text-heading-3">Compliance onboarding</p>
          <p className="mb-5 text-body-sm text-text-secondary">
            Production opens once RiskOS reviews these answers, usually within
            one business day. Test works now.
          </p>
          <ol className="flex flex-col gap-1" aria-label="Onboarding steps">
            {[
              ...STEPS.map((s) => ({ label: s.label, icon: s.icon })),
              { label: "Review and submit", icon: Report },
            ].map((s, i) => {
              const current =
                o && !(o.status === "submitted" || o.status === "approved")
                  ? i === step
                  : false;
              return (
                <li key={s.label}>
                  <button
                    type="button"
                    disabled={
                      !o || o.status === "submitted" || o.status === "approved"
                    }
                    onClick={() => void goTo(i)}
                    aria-current={current ? "step" : undefined}
                    className={cn(
                      "flex w-full items-center gap-3 rounded-full px-3 py-2 text-left text-body transition-colors duration-150 disabled:cursor-default",
                      current
                        ? "bg-blue-50 font-medium text-accent"
                        : "text-text-secondary enabled:hover:bg-bg-raised",
                    )}
                  >
                    <span
                      className={cn(
                        "flex size-7 shrink-0 items-center justify-center rounded-full border [&_svg]:size-icon-sm",
                        i < STEPS.length && done(i)
                          ? "border-ink bg-ink text-white"
                          : "border-border-strong bg-bg-surface",
                      )}
                    >
                      {i < STEPS.length && done(i) ? (
                        <Check aria-hidden="true" />
                      ) : (
                        <s.icon aria-hidden="true" />
                      )}
                    </span>
                    {s.label}
                  </button>
                </li>
              );
            })}
          </ol>
        </aside>
        <main className="max-w-2xl min-w-0">{body}</main>
      </div>
    </div>
  );
}

function Review({
  o,
  answers,
  canEdit,
  onEdit,
  onSubmit,
  submitting,
  error,
  onBack,
}: {
  o: OnboardingT;
  answers: Answers;
  canEdit: boolean;
  onEdit: (step: number) => void;
  onSubmit: () => void;
  submitting: boolean;
  error: Error | null;
  onBack: () => void;
}) {
  const countries = useCountries();
  const countryName = (c: string) =>
    countries.find((x) => x.code === c)?.name ?? c;
  const show = (k: keyof Answers): string => {
    const v = answers[k];
    if (v === undefined || v === "" || (Array.isArray(v) && v.length === 0))
      return "—";
    if (typeof v === "boolean") return v ? "Yes" : "No";
    if (k === "country") return countryName(v as string);
    if (k === "regions_served")
      return (v as string[]).map(countryName).join(", ");
    const all = [
      ...INDUSTRIES,
      ...EVENTS,
      ...VOLUMES,
      ...GO_LIVE,
      ...PRIVACY,
      ...RESIDENCY,
    ] as { value: string; label: string }[];
    const label = (x: string) => all.find((o) => o.value === x)?.label ?? x;
    return Array.isArray(v) ? v.map(label).join(", ") : label(String(v));
  };
  return (
    <div className="flex flex-col gap-6">
      <header>
        <p className="mb-2 text-body-sm text-text-muted">
          Step {STEPS.length + 1} of {STEPS.length + 1}
        </p>
        <h1 className="text-display">
          Review and <span className="accent-italic">submit.</span>
        </h1>
        <p className="mt-3 text-body-lg text-text-secondary">
          Check your answers. RiskOS reviews them and opens production, or asks
          for changes.
        </p>
      </header>
      {o.missing.length ? (
        <Notice tone="warn">
          Still needed:{" "}
          {o.missing
            .map((m) => FIELD_LABEL[m as keyof Answers] ?? m)
            .join(", ")}
          .
        </Notice>
      ) : null}
      {STEPS.map((s, i) => (
        <section
          key={s.key}
          className="rounded-lg border border-border-subtle bg-bg-surface shadow-card"
        >
          <header className="flex items-center justify-between gap-3 px-5 pt-5 pb-2">
            <h2 className="flex items-center gap-2 text-heading-3">
              <s.icon
                className="size-icon-md text-text-secondary"
                aria-hidden="true"
              />
              {s.label}
            </h2>
            {canEdit ? (
              <Button size="sm" variant="ghost" onClick={() => onEdit(i)}>
                Edit
              </Button>
            ) : null}
          </header>
          <dl className="grid grid-cols-1 gap-x-6 gap-y-3 px-5 pb-5 sm:grid-cols-2">
            {s.fields
              .concat(
                s.key === "business"
                  ? ["trading_name"]
                  : s.key === "regulation"
                    ? ["screening_provider"]
                    : s.key === "data"
                      ? ["dpo_email"]
                      : [],
              )
              .map((f) =>
                f === "regulator" || f === "licence_number" ? (
                  answers.regulated ? (
                    <Row key={f} k={f} v={show(f)} />
                  ) : null
                ) : (
                  <Row key={f} k={f} v={show(f)} />
                ),
              )}
          </dl>
        </section>
      ))}
      {error ? (
        <Notice tone="error">
          {error instanceof ApiError
            ? error.message
            : "Submitting failed. Try again."}
        </Notice>
      ) : null}
      <div className="flex flex-wrap items-center gap-3 border-t border-border-subtle pt-6">
        {canEdit ? (
          <Button
            variant="primary"
            arrow
            disabled={submitting || o.missing.length > 0}
            onClick={onSubmit}
          >
            Submit for review
          </Button>
        ) : null}
        <Button variant="ghost" onClick={onBack}>
          Back
        </Button>
      </div>
    </div>
  );
}

function Row({ k, v }: { k: keyof Answers; v: string }) {
  const labels: Partial<Record<keyof Answers, string>> = {
    ...FIELD_LABEL,
    trading_name: "Trading name",
    screening_provider: "Screening provider",
    dpo_email: "DPO email",
  };
  return (
    <div className="min-w-0">
      <dt className="text-label text-text-muted">{labels[k] ?? k}</dt>
      <dd className="mt-0.5 text-body break-words">{v}</dd>
    </div>
  );
}

function Status({
  icon: StatusIcon,
  title,
  accent,
  tone,
  children,
}: {
  icon: Icon;
  title: string;
  accent: string;
  tone: "success" | "info";
  children: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-4">
      <span
        className={cn(
          "inline-flex size-14 items-center justify-center rounded-lg [&_svg]:size-7",
          tone === "success"
            ? "bg-status-success-bg text-status-success"
            : "bg-blue-50 text-accent",
        )}
      >
        <StatusIcon aria-hidden="true" />
      </span>
      <h1 className="text-display">
        {title} <span className="accent-italic">{accent}</span>
      </h1>
      <div className="text-body-lg text-text-secondary">{children}</div>
    </div>
  );
}

function Submitted({ o, onReview }: { o: OnboardingT; onReview: () => void }) {
  const fmt = (d: string | null | undefined) =>
    d
      ? new Intl.DateTimeFormat("en-GB", {
          day: "numeric",
          month: "short",
          hour: "2-digit",
          minute: "2-digit",
        }).format(new Date(d))
      : "";
  const approved = o.status === "approved";
  return (
    <div className="flex flex-col gap-8">
      <Status
        icon={approved ? CheckCircle : History}
        title={approved ? "Production is" : "Thanks, we are"}
        accent={approved ? "open." : "reviewing."}
        tone={approved ? "success" : "info"}
      >
        {approved
          ? "RiskOS approved your onboarding. Switch to Production in the console to use live keys."
          : "RiskOS is reviewing your answers. Keep building in the test environment; we will email you when production opens."}
      </Status>
      <ol className="flex flex-col gap-0 border-l border-border-strong pl-6">
        {[
          {
            done: true,
            label: "Workspace created",
            detail: "Test environment ready",
          },
          {
            done: true,
            label: "Onboarding submitted",
            detail: fmt(o.submitted_at),
          },
          {
            done: approved,
            label: approved ? "Approved" : "Under review",
            detail: approved
              ? fmt(o.reviewed_at)
              : "Usually within one business day",
          },
          {
            done: approved,
            label: "Production open",
            detail: approved ? "Live keys available" : "",
          },
        ].map((s) => (
          <li key={s.label} className="relative pb-6 last:pb-0">
            <span
              aria-hidden="true"
              className={cn(
                "absolute top-0.5 -left-[33px] flex size-4 items-center justify-center rounded-full ring-4 ring-bg-surface",
                s.done
                  ? "bg-ink"
                  : "border border-border-control bg-bg-surface",
              )}
            />
            <p className="text-body font-medium">{s.label}</p>
            {s.detail ? (
              <p className="text-body-sm text-text-muted">{s.detail}</p>
            ) : null}
          </li>
        ))}
      </ol>
      <div className="flex flex-wrap gap-3">
        <Button variant="primary" arrow asChild>
          <Link href="/">Go to the console</Link>
        </Button>
        <Button icon={<Globe />} onClick={onReview}>
          View my answers
        </Button>
      </div>
      {approved ? null : (
        <p className="flex items-center gap-2 text-body-sm text-text-muted">
          <StatusPill tone="info">Submitted</StatusPill> Answers are locked
          while RiskOS reviews them.
        </p>
      )}
    </div>
  );
}

/** A banner for the console while production is locked: where the onboarding stands. */
export function OnboardingBanner() {
  const { session } = useSession();
  const q = useOnboarding(!session.tenant.production_enabled);
  if (session.tenant.production_enabled || !q.data) return null;
  const o = q.data;
  const text =
    o.status === "submitted"
      ? "RiskOS is reviewing your onboarding. Production opens once it is approved."
      : o.status === "rejected"
        ? "RiskOS asked for changes to your onboarding before opening production."
        : "Complete your compliance onboarding to open production. Test works now.";
  return (
    <div className="bg-feature mb-6 flex flex-wrap items-center gap-4 rounded-lg border border-border-subtle p-5 shadow-card">
      <span className="inline-flex size-11 shrink-0 items-center justify-center rounded-md bg-bg-surface text-accent shadow-card [&_svg]:size-icon-md">
        <Lock aria-hidden="true" />
      </span>
      <div className="min-w-0 flex-1">
        <p className="text-heading-3">
          Production is <span className="accent-italic">locked</span>
          <StatusPill
            className="ml-2 align-middle"
            tone={
              o.status === "rejected"
                ? "danger"
                : o.status === "submitted"
                  ? "info"
                  : "warning"
            }
          >
            {o.status === "draft"
              ? `${o.missing.length} answers to go`
              : o.status === "submitted"
                ? "In review"
                : "Changes requested"}
          </StatusPill>
        </p>
        <p className="mt-1 text-body-sm text-text-secondary">{text}</p>
      </div>
      <Button variant="primary" size="sm" arrow asChild>
        <Link href="/onboarding">
          {o.status === "submitted" ? "View status" : "Continue onboarding"}
        </Link>
      </Button>
    </div>
  );
}
