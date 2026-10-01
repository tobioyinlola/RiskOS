"use client";

import { useInfiniteQuery, useQuery } from "@tanstack/react-query";

import { api, unwrap, type Schemas } from "@/lib/api/client";
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
