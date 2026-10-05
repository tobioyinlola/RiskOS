"use client";

import { ActiveCountryRules, CountryActivations } from "./activations";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";

import { Button } from "@/components/ui/button";
import { Tab, TabList, TabPanel, Tabs } from "@/components/ui/misc";
import {
  Metric,
  PageHeader,
  Panel,
  PanelBody,
  PanelHeader,
} from "@/components/ui/panel";
import { StatusPill, type Tone } from "@/components/ui/pill";
import {
  Empty,
  ErrorState,
  Loading,
  Notice,
  SkeletonRows,
} from "@/components/ui/states";
import { DataCell, Table, Td, Th, Tr } from "@/components/ui/table";
import { useAction } from "@/lib/action";
import { api, unwrap, type Schemas } from "@/lib/api/client";
import { formatAge, formatCount, formatTimestamp } from "@/lib/format";
import {
  useCatalog,
  useCompliancePack,
  useRule,
  useRules,
  useRuleStats,
  useRuleVersions,
  useUserName,
} from "@/lib/queries";
import {
  emptyDraft,
  fromCondition,
  ruleSentence,
  type CatalogField,
} from "@/lib/rule-builder";
import { useSession } from "@/lib/session";

import { FXRatesTab } from "../compliance/fx";
import {
  ClocksTab,
  ExportLibrary,
  GapsTab,
  GlossaryTab,
  MatrixTab,
  NewerVersion,
  PendingApprovals,
  ProfileBar,
  SourcesTab,
} from "../compliance/library";
import { RuleActivity, Sparkline } from "./activity";
import { isChange, PendingChangeNotice, PendingChanges } from "./approvals";
import { RuleBuilder } from "./builder";
import { PenaltyRegister } from "../compliance/penalties";
import { AllCountries, CountrySwitcher, useCountry } from "./countries";
import { PresetRules } from "./presets";

type Rule = Schemas["Rule"];
type TabId = "presets" | "custom" | "references";

const STATE = (
  r: Pick<Rule, "enabled" | "mode">,
): { label: string; tone: Tone } =>
  !r.enabled
    ? { label: "Off", tone: "neutral" }
    : r.mode === "shadow"
      ? { label: "Monitoring", tone: "warning" }
      : { label: "Active", tone: "success" };

/**
 * Rules: the preset rules of the institution's jurisdiction pack, activated or turned off here;
 * the institution's own rules, built without code; and the pack's deadlines and references.
 */
export function RulesHome() {
  const { can } = useSession();
  const router = useRouter();
  const params = useSearchParams();
  // The default country's pack holds the institution profile; the switcher picks the country
  // the preset and reference tabs show.
  const pack = useCompliancePack();
  const { country, all, fallback } = useCountry();
  const shown = useCompliancePack(country || undefined);
  const compliance = can("compliance:read");
  const custom = can("rules:read");
  const tabs: TabId[] = [
    ...(compliance ? (["presets"] as const) : []),
    ...(custom ? (["custom"] as const) : []),
    ...(compliance ? (["references"] as const) : []),
  ];
  const asked = params.get("tab") as TabId | null;
  const tab = asked && tabs.includes(asked) ? asked : tabs[0];
  return (
    <>
      <PageHeader
        title="Rules"
        eyebrow={
          all
            ? "All countries"
            : shown.data
              ? `${shown.data.name} · version ${shown.data.version}`
              : undefined
        }
        description="Choose which of the preset rules apply to your institution, and add your own. Nothing here needs code."
        actions={
          custom && can("rules:write") ? (
            <Button variant="primary" asChild>
              <Link href="/rules/new">Create rule</Link>
            </Button>
          ) : null
        }
      />
      {compliance && pack.data ? (
        <>
          <ProfileBar pack={pack.data} />
          <NewerVersion pack={pack.data} />
          <PendingApprovals />
        </>
      ) : null}
      {custom ? <PendingChanges /> : null}
      {tab ? (
        <Tabs
          value={tab}
          onValueChange={(v) => {
            const next = new URLSearchParams(params.toString());
            next.set("tab", v);
            router.replace(`/rules?${next.toString()}` as "/rules", {
              scroll: false,
            });
          }}
        >
          <TabList aria-label="Rules" className="mb-4">
            {tabs.includes("presets") ? (
              <Tab value="presets">Preset rules</Tab>
            ) : null}
            {tabs.includes("custom") ? (
              <Tab value="custom">Your rules</Tab>
            ) : null}
            {tabs.includes("references") ? (
              <Tab value="references">Deadlines and references</Tab>
            ) : null}
          </TabList>
          {compliance && tab !== "custom" ? <CountrySwitcher /> : null}
          {tabs.includes("presets") ? (
            <TabPanel value="presets">
              {all ? (
                <AllCountries />
              ) : shown.isError ? (
                <ErrorState error={shown.error} />
              ) : (
                <>
                  {country ? <CountryActivations country={country} /> : null}
                  <PresetRules country={country} fallback={fallback} />
                </>
              )}
            </TabPanel>
          ) : null}
          {tabs.includes("custom") ? (
            <TabPanel value="custom">
              {compliance ? <ActiveCountryRules /> : null}
              <CustomRules />
            </TabPanel>
          ) : null}
          {tabs.includes("references") ? (
            <TabPanel value="references">
              {all ? (
                <div className="flex flex-col gap-4">
                  <ExportLibrary name="every country" />
                  <MatrixTab />
                </div>
              ) : shown.data ? (
                <References pack={shown.data} jurisdiction={country} />
              ) : shown.isError ? (
                <ErrorState error={shown.error} />
              ) : (
                <Loading />
              )}
            </TabPanel>
          ) : null}
        </Tabs>
      ) : null}
    </>
  );
}

function References({
  pack,
  jurisdiction,
}: {
  pack: Schemas["CompliancePack"];
  jurisdiction: string;
}) {
  return (
    <Tabs defaultValue="clocks">
      <div className="mb-4">
        <ExportLibrary jurisdiction={jurisdiction} name={pack.name} />
      </div>
      <TabList aria-label="References" className="mb-4">
        <Tab value="clocks">Deadlines</Tab>
        <Tab value="gaps">Open questions</Tab>
        <Tab value="matrix">Countries side by side</Tab>
        <Tab value="glossary">Glossary</Tab>
        <Tab value="sources">Sources</Tab>
        {pack.penalties?.length ? <Tab value="penalties">Penalties</Tab> : null}
        <Tab value="fx">Exchange rates</Tab>
      </TabList>
      <TabPanel value="clocks">
        <ClocksTab jurisdiction={jurisdiction} />
      </TabPanel>
      <TabPanel value="gaps">
        <GapsTab jurisdiction={jurisdiction} />
      </TabPanel>
      <TabPanel value="matrix">
        <MatrixTab />
      </TabPanel>
      <TabPanel value="glossary">
        <GlossaryTab jurisdiction={jurisdiction} />
      </TabPanel>
      {pack.penalties?.length ? (
        <TabPanel value="penalties">
          <PenaltyRegister pack={pack} />
        </TabPanel>
      ) : null}
      <TabPanel value="sources">
        <SourcesTab pack={pack} />
      </TabPanel>
      <TabPanel value="fx">
        <FXRatesTab />
      </TabPanel>
    </Tabs>
  );
}

/**
 * Rules → Your rules: the institution's own rules, each said in words, with how many are on and
 * what each has matched over the last 7 days.
 */
function CustomRules() {
  const q = useRules();
  const catalog = useCatalog();
  const stats = useRuleStats(7);
  const fields = catalog.data?.fields ?? [];
  const rules = q.data?.data ?? [];
  const statOf = (id: string) =>
    stats.data?.rules.find((x) => x.rule_id === id);
  const count = (label: string) =>
    rules.filter((r) => STATE(r).label === label).length;
  const total = (stats.data?.rules ?? []).reduce((n, x) => n + x.matches, 0);
  return (
    <>
      <div className="mb-5 grid grid-cols-2 gap-4 lg:grid-cols-4">
        <Metric label="Active" value={formatCount(count("Active"))} />
        <Metric label="Monitoring" value={formatCount(count("Monitoring"))} />
        <Metric label="Off" value={formatCount(count("Off"))} />
        <Metric
          label="Matches · last 7 days"
          value={stats.data ? formatCount(total) : "–"}
        />
      </div>
      <Panel className="overflow-hidden">
        <Table aria-label="Your rules">
          <thead>
            <tr>
              <Th>Rule</Th>
              <Th>Status</Th>
              <Th>Last 7 days</Th>
              <Th>Last match</Th>
              <Th className="text-right">Risk points</Th>
            </tr>
          </thead>
          <tbody>
            {q.isLoading ? <SkeletonRows columns={5} /> : null}
            {rules.map((r) => (
              <Tr key={r.id}>
                <Td className="max-w-2xl py-3">
                  <Link
                    href={`/rules/${r.id}` as "/rules"}
                    className="font-medium text-text-primary hover:text-accent"
                  >
                    {r.name}
                  </Link>
                  {fields.length ? (
                    <p className="text-body-sm text-text-secondary">
                      {ruleSentence(r, fields)}
                    </p>
                  ) : null}
                </Td>
                <Td className="whitespace-nowrap">
                  <StatusPill tone={STATE(r).tone}>{STATE(r).label}</StatusPill>
                  {r.pending_change_id ? (
                    <span className="mt-1 block text-label text-text-muted">
                      Waiting for approval
                    </span>
                  ) : null}
                </Td>
                <Td>
                  {statOf(r.id) ? (
                    <span className="flex items-center gap-2">
                      <Sparkline
                        daily={statOf(r.id)!.daily}
                        label={`${r.name}, matches per day`}
                      />
                      <span className="text-body-sm tabular-nums">
                        {formatCount(statOf(r.id)!.matches)}
                      </span>
                    </span>
                  ) : (
                    <span className="text-text-muted">–</span>
                  )}
                </Td>
                <DataCell className="text-text-muted">
                  {statOf(r.id)?.last_matched_at
                    ? formatAge(statOf(r.id)!.last_matched_at!)
                    : "Never"}
                </DataCell>
                <DataCell className="text-right">+{r.weight}</DataCell>
              </Tr>
            ))}
          </tbody>
        </Table>
        {q.isError ? <ErrorState error={q.error} className="m-4" /> : null}
        {q.isSuccess && rules.length === 0 ? (
          <Empty title="No rules of your own yet">
            The preset rules and built-in signals already check every event. Add
            a rule for what only your institution knows; start it in monitoring
            to see what it would catch.
          </Empty>
        ) : null}
      </Panel>
    </>
  );
}

export function NewRule() {
  const router = useRouter();
  return (
    <>
      <PageHeader
        title="Create rule"
        eyebrow={
          <Link href="/rules?tab=custom" className="hover:text-accent">
            Rules
          </Link>
        }
        description="Say when the rule looks, what must be true and how much risk it adds. New rules start by monitoring: they show what they would catch without changing any score."
      />
      <RuleBuilder
        initial={emptyDraft()}
        onSaved={(r) =>
          router.push(
            isChange(r) ? "/rules?tab=custom" : (`/rules/${r.id}` as "/rules"),
          )
        }
      />
    </>
  );
}

export function RuleDetail({ id }: { id: string }) {
  const q = useRule(id);
  const catalog = useCatalog();
  const { can } = useSession();
  if (q.isLoading || catalog.isLoading) return <Loading />;
  if (q.isError) return <ErrorState error={q.error} />;
  if (catalog.isError) return <ErrorState error={catalog.error} />;
  const r = q.data!;
  const fields = catalog.data?.fields ?? [];
  const draft = fromCondition(r.condition, fields);
  const writable = can("rules:write");
  return (
    <>
      <PageHeader
        title={r.name}
        eyebrow={
          <Link href="/rules?tab=custom" className="hover:text-accent">
            Rules
          </Link>
        }
        description={r.description || undefined}
        actions={<StatusPill tone={STATE(r).tone}>{STATE(r).label}</StatusPill>}
      />
      <PendingChangeNotice rule={r} />
      <Tabs defaultValue="rule">
        <TabList aria-label="Rule" className="mb-4">
          <Tab value="rule">Rule</Tab>
          <Tab value="activity">Activity</Tab>
          <Tab value="history">History</Tab>
        </TabList>
        <TabPanel value="rule">
          {writable && draft ? (
            <RuleBuilder
              key={r.version}
              rule={r}
              initial={draft}
              onSaved={() => undefined}
            />
          ) : (
            <Panel>
              <PanelHeader title="This rule says" />
              <PanelBody className="flex flex-col gap-3">
                <p className="text-body">{ruleSentence(r, fields)}</p>
                {writable && !draft ? (
                  <Notice>
                    This rule was made through the API with logic this page
                    cannot show as a form. Ask your developers to change it.
                  </Notice>
                ) : null}
              </PanelBody>
            </Panel>
          )}
        </TabPanel>
        <TabPanel value="activity">
          <RuleActivity ruleId={r.id} />
        </TabPanel>
        <TabPanel value="history">
          <RuleVersions rule={r} fields={fields} />
        </TabPanel>
      </Tabs>
    </>
  );
}

function RuleVersions({
  rule,
  fields,
}: {
  rule: Rule;
  fields: CatalogField[];
}) {
  const q = useRuleVersions(rule.id);
  const { can } = useSession();
  const userName = useUserName();
  const rollback = useAction(
    async (version: number) =>
      unwrap(
        await api().POST("/api/v1/rules/{rule_id}/rollback", {
          params: { path: { rule_id: rule.id } },
          body: { version },
        }),
      ),
    { invalidate: ["rule", "rules", "rule-versions", "rule-changes"] },
  );
  return (
    <Panel className="overflow-hidden">
      {rollback.error ? <Notice tone="error">{rollback.error}</Notice> : null}
      <Table aria-label="History">
        <thead>
          <tr>
            <Th>Version</Th>
            <Th>What it said</Th>
            <Th>Status</Th>
            <Th>By</Th>
            <Th>When</Th>
            <Th />
          </tr>
        </thead>
        <tbody>
          {q.isLoading ? <SkeletonRows columns={6} rows={3} /> : null}
          {[...(q.data?.data ?? [])].reverse().map((v) => (
            <Tr
              key={v.version}
              selected={v.version === q.data?.current_version}
            >
              <DataCell>
                {v.version}
                {v.restored_from ? (
                  <span className="block text-label text-text-muted">
                    restores {v.restored_from}
                  </span>
                ) : null}
              </DataCell>
              <Td className="max-w-xl text-body-sm text-text-secondary">
                {ruleSentence(v, fields)}
              </Td>
              <Td className="whitespace-nowrap">
                <StatusPill tone={STATE(v).tone}>{STATE(v).label}</StatusPill>
              </Td>
              <Td className="text-body-sm text-text-secondary">
                {userName(v.created_by)}
              </Td>
              <DataCell
                className="text-text-muted"
                title={formatTimestamp(v.created_at)}
              >
                {formatAge(v.created_at)}
              </DataCell>
              <Td className="text-right">
                {can("rules:write") &&
                !rule.pending_change_id &&
                v.version !== q.data?.current_version ? (
                  <Button
                    size="sm"
                    onClick={() => void rollback.run(v.version)}
                    disabled={rollback.busy}
                  >
                    Restore
                  </Button>
                ) : null}
              </Td>
            </Tr>
          ))}
        </tbody>
      </Table>
    </Panel>
  );
}
