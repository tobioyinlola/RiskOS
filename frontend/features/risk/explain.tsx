"use client";

import { ReasonList, type Reason } from "@/components/instrument/reason-list";
import { RiskScore } from "@/components/instrument/risk-score";
import { Notice } from "@/components/instrument/states";
import type { Schemas } from "@/lib/api/client";
import { humanize } from "@/lib/format";
import { useSignalCategories, useThresholds } from "@/lib/queries";
import type { Decision, RiskBand } from "@/lib/risk";

/**
 * A decision as the console shows it everywhere: the reading, then the reasons
 * (docs/design-system.md, "Readings, then reasons").
 */
export function DecisionReading({
  decision,
}: {
  decision: Schemas["RiskDecision"];
}) {
  const { thresholds, custom } = useThresholds();
  const category = useSignalCategories();
  const reasons: Reason[] = decision.signals.map((s) => ({
    label: s.label,
    category: category(s.code),
    contribution: s.weight,
  }));
  return (
    <div className="flex flex-col gap-5">
      <RiskScore
        score={decision.risk_score}
        band={decision.risk_level as RiskBand}
        decision={decision.decision as Decision}
        engineVersion={decision.engine_version}
        thresholds={thresholds}
        customCalibration={custom}
      />
      {decision.degraded_signals.length ? (
        <Notice tone="warn">
          Evaluated with missing signals:{" "}
          {decision.degraded_signals.map(humanize).join(", ")}. The decision
          followed the fail policy.
        </Notice>
      ) : null}
      <ReasonList reasons={reasons} />
    </div>
  );
}
