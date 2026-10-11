"use client";

import { useInfiniteQuery, useQuery } from "@tanstack/react-query";

import { ApiError, api, unwrap, type Schemas } from "@/lib/api/client";
import { DEFAULT_THRESHOLDS, type Thresholds } from "@/lib/risk";
import { useSession } from "@/lib/session";

/** Query keys start with the environment: every reading belongs to one. */
function useKey(...parts: unknown[]) {
  const { environment } = useSession();
  return [environment, ...parts];
}

export function useRiskPolicy() {
  const key = useKey("risk-policy");
  return useQuery({
    queryKey: key,
    queryFn: async () => unwrap(await api().GET("/api/v1/risk/policy")),
    staleTime: 5 * 60_000,
  });
}

/** The environment's thresholds (defaults until the policy loads). */
export function useThresholds(): { thresholds: Thresholds; custom: boolean } {
  const q = useRiskPolicy();
  return {
    thresholds: q.data?.thresholds ?? DEFAULT_THRESHOLDS,
    custom: q.data?.custom_calibration ?? false,
  };
}

export function useCatalog() {
  const key = useKey("risk-catalog");
  return useQuery({
    queryKey: key,
    queryFn: async () => unwrap(await api().GET("/api/v1/risk/catalog")),
    staleTime: 5 * 60_000,
  });
}

/** Signal categories (feature sources) by code, for ReasonList; rules are "rule". */
export function useSignalCategories(): (code: string) => string {
  const q = useCatalog();
  const map = new Map((q.data?.signals ?? []).map((s) => [s.code, s.source]));
  return (code) => map.get(code) ?? "rule";
}

/** Filters of GET /api/v1/events (all optional). */
export type EventFilter = {
  type?: Schemas["EventType"];
  decision?: Schemas["Decision"];
  risk_level?: Schemas["RiskLevel"];
  user?: string;
  account?: string;
  device?: string;
};

export function useEvents(limit = 50, filter: EventFilter = {}) {
  const key = useKey("events", limit, filter);
  return useInfiniteQuery({
    queryKey: key,
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) =>
      unwrap(
        await api().GET("/api/v1/events", {
          params: { query: { limit, cursor: pageParam, ...filter } },
        }),
      ),
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
}

export type EntityKind = "user" | "account" | "device";

export function useEntity(kind: EntityKind, ref: string) {
  const key = useKey("entity", kind, ref);
  return useQuery({
    queryKey: key,
    retry: false,
    queryFn: async () =>
      unwrap(
        await api().GET("/api/v1/entities/{kind}/{ref}", {
          params: { path: { kind, ref } },
        }),
      ),
  });
}

export function useDenyList(kind?: Schemas["DenyKind"]) {
  const key = useKey("deny-list", kind ?? "all");
  return useQuery({
    queryKey: key,
    queryFn: async () =>
      unwrap(
        await api().GET("/api/v1/deny-list", {
          params: { query: kind ? { kind } : {} },
        }),
      ),
  });
}

export function useSSO() {
  return useQuery({
    queryKey: ["sso"],
    queryFn: async () => unwrap(await api().GET("/api/v1/sso")),
  });
}

export function useSSODomains() {
  return useQuery({
    queryKey: ["sso-domains"],
    queryFn: async () => unwrap(await api().GET("/api/v1/sso/domains")),
  });
}

export type AnalyticsWindow = "24h" | "7d" | "30d";

export function useAnalytics(window: AnalyticsWindow) {
  const key = useKey("analytics", window);
  return useQuery({
    queryKey: key,
    queryFn: async () =>
      unwrap(
        await api().GET("/api/v1/analytics", {
          params: { query: { window } },
        }),
      ),
    staleTime: 60_000,
  });
}

export function useEvent(id: string | undefined) {
  const key = useKey("event", id);
  return useQuery({
    queryKey: key,
    enabled: !!id,
    queryFn: async () =>
      unwrap(
        await api().GET("/api/v1/events/{event_id}", {
          params: { path: { event_id: id! } },
        }),
      ),
  });
}

export type AlertFilter = {
  status?: Schemas["AlertStatus"];
  kind?: Schemas["AlertKind"];
};

export function useAlerts(filter: AlertFilter) {
  const key = useKey("alerts", filter);
  return useInfiniteQuery({
    queryKey: key,
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) =>
      unwrap(
        await api().GET("/api/v1/alerts", {
          params: { query: { limit: 100, cursor: pageParam, ...filter } },
        }),
      ),
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    refetchInterval: 15_000,
  });
}

export function useAlert(id: string | undefined) {
  const key = useKey("alert", id);
  return useQuery({
    queryKey: key,
    enabled: !!id,
    queryFn: async () =>
      unwrap(
        await api().GET("/api/v1/alerts/{alert_id}", {
          params: { path: { alert_id: id! } },
        }),
      ),
  });
}

export function useCases(status?: Schemas["CaseStatus"]) {
  const key = useKey("cases", status);
  return useInfiniteQuery({
    queryKey: key,
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) =>
      unwrap(
        await api().GET("/api/v1/cases", {
          params: { query: { limit: 50, cursor: pageParam, status } },
        }),
      ),
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
}

export function useCase(id: string) {
  const key = useKey("case", id);
  return useQuery({
    queryKey: key,
    queryFn: async () =>
      unwrap(
        await api().GET("/api/v1/cases/{case_id}", {
          params: { path: { case_id: id } },
        }),
      ),
  });
}

export function useRules() {
  const key = useKey("rules");
  return useQuery({
    queryKey: key,
    queryFn: async () => unwrap(await api().GET("/api/v1/rules")),
  });
}

export function useRule(id: string | undefined) {
  const key = useKey("rule", id);
  return useQuery({
    queryKey: key,
    enabled: !!id,
    queryFn: async () =>
      unwrap(
        await api().GET("/api/v1/rules/{rule_id}", {
          params: { path: { rule_id: id! } },
        }),
      ),
  });
}

export function useRuleVersions(id: string | undefined) {
  const key = useKey("rule-versions", id);
  return useQuery({
    queryKey: key,
    enabled: !!id,
    queryFn: async () =>
      unwrap(
        await api().GET("/api/v1/rules/{rule_id}/versions", {
          params: { path: { rule_id: id! } },
        }),
      ),
  });
}

export function useRuleChanges(
  status: "pending" | "approved" | "rejected" | "all" = "pending",
) {
  const key = useKey("rule-changes", status);
  return useQuery({
    queryKey: key,
    queryFn: async () =>
      unwrap(
        await api().GET("/api/v1/rule-changes", {
          params: { query: { status } },
        }),
      ),
  });
}

export function useRuleChange(id: string | null | undefined) {
  const key = useKey("rule-changes", "one", id);
  return useQuery({
    queryKey: key,
    enabled: !!id,
    queryFn: async () =>
      unwrap(
        await api().GET("/api/v1/rule-changes/{change_id}", {
          params: { path: { change_id: id! } },
        }),
      ),
  });
}

export function useRiskSettings() {
  const key = useKey("risk-settings");
  return useQuery({
    queryKey: key,
    queryFn: async () => unwrap(await api().GET("/api/v1/risk/settings")),
  });
}

export function useApiKeys() {
  const key = useKey("api-keys");
  return useQuery({
    queryKey: key,
    queryFn: async () =>
      unwrap(
        await api().GET("/api/v1/api-keys", {
          params: { query: { limit: 100 } },
        }),
      ),
  });
}

export function useWebhooks() {
  const key = useKey("webhooks");
  return useQuery({
    queryKey: key,
    queryFn: async () => unwrap(await api().GET("/api/v1/webhooks")),
  });
}

export function useWebhook(id: string) {
  const key = useKey("webhook", id);
  return useQuery({
    queryKey: key,
    queryFn: async () =>
      unwrap(
        await api().GET("/api/v1/webhooks/{webhook_id}", {
          params: { path: { webhook_id: id } },
        }),
      ),
  });
}

export function useDeliveries(webhookId: string) {
  const key = useKey("deliveries", webhookId);
  return useInfiniteQuery({
    queryKey: key,
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) =>
      unwrap(
        await api().GET("/api/v1/webhooks/{webhook_id}/deliveries", {
          params: {
            path: { webhook_id: webhookId },
            query: { limit: 50, cursor: pageParam },
          },
        }),
      ),
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    refetchInterval: 10_000,
  });
}

export function useDelivery(webhookId: string, id: string | undefined) {
  const key = useKey("delivery", webhookId, id);
  return useQuery({
    queryKey: key,
    enabled: !!id,
    queryFn: async () =>
      unwrap(
        await api().GET(
          "/api/v1/webhooks/{webhook_id}/deliveries/{delivery_id}",
          {
            params: { path: { webhook_id: webhookId, delivery_id: id! } },
          },
        ),
      ),
  });
}

export function useAuditLogs() {
  const key = useKey("audit-logs");
  return useInfiniteQuery({
    queryKey: key,
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) =>
      unwrap(
        await api().GET("/api/v1/audit-logs", {
          params: { query: { limit: 100, cursor: pageParam } },
        }),
      ),
    getNextPageParam: (last) => last.next_cursor ?? undefined,
  });
}

export function useTeam() {
  const key = useKey("team");
  const { can } = useSession();
  return useQuery({
    queryKey: key,
    enabled: can("team:read"),
    queryFn: async () => unwrap(await api().GET("/api/v1/team/members")),
    staleTime: 60_000,
  });
}

/** Resolves a user ID (as stored in assignee, created_by) to a display name. */
export function useUserName(): (ref: string | null | undefined) => string {
  const q = useTeam();
  const map = new Map((q.data?.data ?? []).map((m) => [m.id, m.name]));
  return (ref) => (ref ? (map.get(ref) ?? ref) : "Unassigned");
}

export function useInvites() {
  const key = useKey("invites");
  return useQuery({
    queryKey: key,
    queryFn: async () => unwrap(await api().GET("/api/v1/team/invites")),
  });
}

/*
 * The compliance rules library (ADR-0006). The pack belongs to the workspace, not to an
 * environment, and a loaded pack version never changes, so it is cached for long.
 */
const COMPLIANCE_STALE = 10 * 60_000;

/** The workspace's pack or, with a jurisdiction, that jurisdiction's pack to browse. */
export function useCompliancePack(jurisdiction?: string) {
  return useQuery({
    queryKey: ["compliance", "pack", jurisdiction ?? "own"],
    staleTime: COMPLIANCE_STALE,
    retry: false,
    queryFn: async () =>
      unwrap(
        await api().GET("/api/v1/compliance/pack", {
          params: { query: jurisdiction ? { jurisdiction } : {} },
        }),
      ),
  });
}

/** Every loaded jurisdiction's pack. */
export function useCompliancePacks() {
  return useQuery({
    queryKey: ["compliance", "packs"],
    staleTime: COMPLIANCE_STALE,
    queryFn: async () => unwrap(await api().GET("/api/v1/compliance/packs")),
  });
}

/** How often each of the workspace's rules matched, per day, over the last days. */
export function useRuleStats(days = 7) {
  const key = useKey("rules", "stats", days);
  return useQuery({
    queryKey: key,
    refetchInterval: 60_000,
    queryFn: async () =>
      unwrap(
        await api().GET("/api/v1/rules/stats", {
          params: { query: { days } },
        }),
      ),
  });
}

/** Every rule of the pack (lists never carry engineering logic). */
export function useComplianceRules(jurisdiction?: string) {
  const key = useKey("compliance", "rules", jurisdiction ?? "own");
  return useQuery({
    queryKey: key,
    staleTime: COMPLIANCE_STALE,
    queryFn: async () =>
      unwrap(
        await api().GET("/api/v1/compliance/rules", {
          params: { query: jurisdiction ? { jurisdiction } : {} },
        }),
      ),
  });
}

/**
 * One rule; the engineering logic is present only when the caller may see it. Without a
 * jurisdiction, a country rule ID (GH-REG-01) names its own; others are the fallback pack's.
 */
export function useComplianceRule(ruleId: string, jurisdiction?: string) {
  const key = useKey("compliance", "rule", ruleId, jurisdiction ?? "own");
  return useQuery({
    queryKey: key,
    staleTime: COMPLIANCE_STALE,
    retry: false,
    queryFn: async () =>
      unwrap(
        await api().GET("/api/v1/compliance/rules/{rule_id}", {
          params: {
            path: { rule_id: ruleId },
            query: jurisdiction ? { jurisdiction } : {},
          },
        }),
      ),
  });
}

const byCountry = (jurisdiction?: string) => ({
  params: { query: jurisdiction ? { jurisdiction } : {} },
});

/** A country's clocks (the fallback pack's without one). */
export function useComplianceClocks(jurisdiction?: string) {
  return useQuery({
    queryKey: ["compliance", "clocks", jurisdiction ?? "own"],
    staleTime: COMPLIANCE_STALE,
    queryFn: async () =>
      unwrap(
        await api().GET("/api/v1/compliance/clocks", byCountry(jurisdiction)),
      ),
  });
}

/** A country's open questions. */
export function useComplianceGaps(jurisdiction?: string) {
  return useQuery({
    queryKey: ["compliance", "gaps", jurisdiction ?? "own"],
    staleTime: COMPLIANCE_STALE,
    queryFn: async () =>
      unwrap(
        await api().GET("/api/v1/compliance/gaps", byCountry(jurisdiction)),
      ),
  });
}

/** A country's glossary. */
export function useComplianceGlossary(jurisdiction?: string) {
  return useQuery({
    queryKey: ["compliance", "glossary", jurisdiction ?? "own"],
    staleTime: COMPLIANCE_STALE,
    queryFn: async () =>
      unwrap(
        await api().GET("/api/v1/compliance/glossary", byCountry(jurisdiction)),
      ),
  });
}

/** The countries with a pack, and whether the workspace activated each. */
export function useJurisdictions() {
  return useQuery({
    queryKey: ["compliance", "jurisdictions"],
    staleTime: COMPLIANCE_STALE,
    queryFn: async () =>
      unwrap(await api().GET("/api/v1/compliance/jurisdictions")),
  });
}

/** The workspace's country activations, the fallback first. */
export function useActivations() {
  return useQuery({
    queryKey: ["compliance", "activations"],
    queryFn: async () =>
      unwrap(await api().GET("/api/v1/compliance/activations")),
  });
}

/** The library's side-by-side comparison of the countries' clocks and thresholds. */
export function useClockMatrix() {
  return useQuery({
    queryKey: ["compliance", "clock-matrix"],
    staleTime: COMPLIANCE_STALE,
    queryFn: async () =>
      unwrap(await api().GET("/api/v1/compliance/clock-matrix")),
  });
}

/** The workspace's compliance profile, or null until one is set. */
export function useComplianceProfile() {
  return useQuery({
    queryKey: ["compliance", "profile"],
    retry: false,
    queryFn: async () => {
      try {
        return unwrap(await api().GET("/api/v1/compliance/profile"));
      } catch (e) {
        if (e instanceof ApiError && e.status === 404) return null;
        throw e;
      }
    },
  });
}

/**
 * A rule's configuration in the current environment, with any change awaiting approval, in a
 * country's pack (an inherited rule such as ATO-04 is configured apart in each country).
 */
export function useComplianceConfig(ruleId: string, jurisdiction?: string) {
  const key = useKey("compliance", "config", ruleId, jurisdiction ?? "own");
  return useQuery({
    queryKey: key,
    queryFn: async () =>
      unwrap(
        await api().GET("/api/v1/compliance/rules/{rule_id}/config", {
          params: {
            path: { rule_id: ruleId },
            ...byCountry(jurisdiction).params,
          },
        }),
      ),
  });
}

/** A rule's configuration versions in the current environment, newest first. */
export function useComplianceHistory(ruleId: string, jurisdiction?: string) {
  const key = useKey("compliance", "history", ruleId, jurisdiction ?? "own");
  return useQuery({
    queryKey: key,
    queryFn: async () =>
      unwrap(
        await api().GET("/api/v1/compliance/rules/{rule_id}/history", {
          params: {
            path: { rule_id: ruleId },
            ...byCountry(jurisdiction).params,
          },
        }),
      ),
  });
}

/** Activation changes: each activation's history and those awaiting approval. */
export function useActivationChanges(
  state?: Schemas["ActivationChange"]["state"],
) {
  return useQuery({
    queryKey: ["compliance", "activation-changes", state ?? "all"],
    queryFn: async () =>
      unwrap(
        await api().GET("/api/v1/compliance/activation-changes", {
          params: { query: state ? { state } : {} },
        }),
      ),
  });
}

/** What must be settled before an activation goes live, in the current environment. */
export function useActivationReadiness(id: string | undefined) {
  const key = useKey("compliance", "readiness", id ?? "");
  return useQuery({
    queryKey: key,
    enabled: !!id,
    queryFn: async () =>
      unwrap(
        await api().GET(
          "/api/v1/compliance/activations/{activation_id}/readiness",
          { params: { path: { activation_id: id! } } },
        ),
      ),
  });
}

/** Every rule running for the workspace's live and shadow activations. */
export function useActiveRules() {
  const key = useKey("compliance", "active-rules");
  return useQuery({
    queryKey: key,
    queryFn: async () =>
      unwrap(await api().GET("/api/v1/compliance/active-rules")),
  });
}

/** Configuration changes in the current environment. */
export function useComplianceChanges(
  status?: Schemas["PackRuleChange"]["status"],
) {
  const key = useKey("compliance", "changes", status ?? "all");
  return useQuery({
    queryKey: key,
    queryFn: async () =>
      unwrap(
        await api().GET("/api/v1/compliance/changes", {
          params: { query: status ? { status } : {} },
        }),
      ),
  });
}

/** The institution's exchange rates. */
export function useFXRates() {
  return useQuery({
    queryKey: ["compliance", "fx-rates"],
    queryFn: async () => unwrap(await api().GET("/api/v1/compliance/fx-rates")),
  });
}

/** The compliance work queue of the current environment, of one country or all. */
export function useComplianceTasks(
  status?: Schemas["ComplianceTaskStatus"],
  jurisdiction?: string,
) {
  const key = useKey(
    "compliance",
    "tasks",
    status ?? "all",
    jurisdiction ?? "all",
  );
  return useQuery({
    queryKey: key,
    queryFn: async () =>
      unwrap(
        await api().GET("/api/v1/compliance/tasks", {
          params: {
            query: {
              ...(status ? { status } : {}),
              ...(jurisdiction ? { jurisdiction } : {}),
            },
          },
        }),
      ),
  });
}

/** The work running against each legal clock now, in the current environment. */
export function useClockLoad() {
  const key = useKey("compliance", "clock-load");
  return useQuery({
    queryKey: key,
    queryFn: async () =>
      unwrap(await api().GET("/api/v1/compliance/clock-load")),
  });
}

/** Events that started a clock outside the country that governed them. */
export function useCrossBorder() {
  const key = useKey("compliance", "cross-border");
  return useQuery({
    queryKey: key,
    queryFn: async () =>
      unwrap(await api().GET("/api/v1/compliance/cross-border")),
  });
}

/** The retention in force for each record type across the activated countries. */
export function useRetention() {
  const key = useKey("compliance", "retention");
  return useQuery({
    queryKey: key,
    queryFn: async () =>
      unwrap(await api().GET("/api/v1/compliance/retention")),
  });
}

/** The public holidays the workspace entered for a country (working-day clocks). */
export function useHolidays(jurisdiction: string) {
  return useQuery({
    queryKey: ["compliance", "holidays", jurisdiction],
    queryFn: async () =>
      unwrap(
        await api().GET(
          "/api/v1/compliance/jurisdictions/{jurisdiction}/holidays",
          { params: { path: { jurisdiction } } },
        ),
      ),
  });
}

/** Compliance-only signals (compliance officers only, GOV-09). */
export function useRestrictedSignals(enabled: boolean) {
  const key = useKey("compliance", "restricted-signals");
  return useQuery({
    queryKey: key,
    enabled,
    queryFn: async () =>
      unwrap(await api().GET("/api/v1/compliance/restricted-signals")),
  });
}
