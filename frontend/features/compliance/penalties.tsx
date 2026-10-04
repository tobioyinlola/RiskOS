"use client";

import { Panel, PanelHeader } from "@/components/ui/panel";
import { StatusPill } from "@/components/ui/pill";
import { Notice } from "@/components/ui/states";
import { Table, Td, Th, Tr } from "@/components/ui/table";
import type { Schemas } from "@/lib/api/client";

/**
 * A country's penalty exposure register (ADR-0007), as the library lists it. Estimates in the
 * local currency that rest on an assumption (Ghana's penalty unit at GHS 12) say so.
 */
export function PenaltyRegister({
  pack,
  compact = false,
}: {
  pack: Schemas["CompliancePack"];
  compact?: boolean;
}) {
  const rows = pack.penalties ?? [];
  if (!rows.length) return null;
  const inferred = rows.some((r) => r.inference);
  return (
    <Panel className="overflow-hidden">
      <PanelHeader
        title={compact ? "Penalty exposure" : `${pack.name}: penalties`}
        legend="what a missed duty can cost, as the library lists it"
      />
      {inferred ? (
        <div className="px-5 pb-3">
          <Notice tone="warn">
            Amounts marked as estimates are the library&apos;s inference, not a
            stated figure: for Ghana, one penalty unit is taken as GHS 12 from a
            tax authority page whose effective date is unverified.
          </Notice>
        </div>
      ) : null}
      <Table aria-label="Penalties">
        <thead>
          <tr>
            <Th>Provision</Th>
            <Th>Penalty</Th>
            <Th>In {pack.currency ?? "local currency"}</Th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <Tr key={r.provision}>
              <Td className="py-2.5 text-body-sm">{r.provision}</Td>
              <Td className="py-2.5 text-body-sm text-text-secondary">
                {r.penalty}
              </Td>
              <Td className="py-2.5 text-body-sm whitespace-nowrap">
                {r.estimate ? (
                  <span className="flex flex-col items-start gap-1">
                    {r.estimate}
                    {r.inference ? (
                      <StatusPill tone="warning">Estimate</StatusPill>
                    ) : null}
                  </span>
                ) : (
                  <span className="text-text-muted">–</span>
                )}
              </Td>
            </Tr>
          ))}
        </tbody>
      </Table>
    </Panel>
  );
}
