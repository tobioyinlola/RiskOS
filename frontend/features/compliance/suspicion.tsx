"use client";

import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Field, Select, Textarea } from "@/components/ui/field";
import { Modal } from "@/components/ui/overlay";
import { Notice } from "@/components/ui/states";
import { useAction } from "@/lib/action";
import { api, unwrap } from "@/lib/api/client";
import { useActivations } from "@/lib/queries";
import { useSession } from "@/lib/session";

/** Each country's suspicious transaction report: its authority and deadline. */
const STR_RULE: Record<string, { rule: string; to: string }> = {
  NG: { rule: "REG-14", to: "the NFIU" },
  GH: { rule: "GH-REG-06", to: "Ghana\u2019s FIC" },
  RW: { rule: "RW-REG-04", to: "Rwanda\u2019s FIC" },
};

/**
 * A compliance officer confirms suspicion on a case: a suspicious transaction report opens to the
 * chosen country's authority, due within its deadline. Shown only to compliance officers (GOV-09).
 */
export function ConfirmSuspicion({ caseId }: { caseId: string }) {
  const { can } = useSession();
  const acts = useActivations();
  const [open, setOpen] = useState(false);
  const [note, setNote] = useState("");
  const [done, setDone] = useState<string>();
  const list = acts.data?.data ?? [];
  const countries = [...new Set(list.map((a) => a.jurisdiction))];
  const fallback = list.find((a) => a.fallback)?.jurisdiction ?? "NG";
  const [country, setCountry] = useState("");
  const chosen = country || fallback;
  const str = STR_RULE[chosen] ?? { rule: "", to: "the authority" };
  const confirm = useAction(
    async () =>
      unwrap(
        await api().POST("/api/v1/compliance/suspicion", {
          body: {
            case_id: caseId,
            jurisdiction: chosen,
            ...(note.trim() ? { note: note.trim() } : {}),
          },
        }),
      ),
    {
      invalidate: ["compliance"],
      onSuccess: () => {
        setOpen(false);
        setDone(
          `Suspicion confirmed. The suspicious transaction report to ${str.to} is in the work queue with its deadline.`,
        );
      },
    },
  );
  if (!can("compliance:sensitive")) return null;
  return (
    <>
      {done ? <Notice tone="ok">{done}</Notice> : null}
      <Button onClick={() => setOpen(true)} disabled={!!done}>
        Confirm suspicion
      </Button>
      <Modal
        open={open}
        onOpenChange={setOpen}
        title="Confirm suspicion?"
        description={`This opens a suspicious transaction report to ${str.to}, due within its legal deadline (${str.rule}). Only compliance officers see it; never tell the customer.`}
      >
        <div className="flex flex-col gap-3">
          {confirm.error ? <Notice tone="error">{confirm.error}</Notice> : null}
          {countries.length > 1 ? (
            <Field
              label="Country"
              hint="The country whose law the report is owed under: where the customer is booked."
            >
              {(f) => (
                <Select
                  {...f}
                  value={chosen}
                  onChange={(e) => setCountry(e.target.value)}
                >
                  {countries.map((c) => (
                    <option key={c} value={c}>
                      {c}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
          ) : null}
          <Field
            label="What you found"
            hint="The start of the report's narrative."
          >
            {(f) => (
              <Textarea
                {...f}
                value={note}
                maxLength={4000}
                onChange={(e) => setNote(e.target.value)}
              />
            )}
          </Field>
          <div className="flex justify-end gap-2">
            <Button onClick={() => setOpen(false)}>Cancel</Button>
            <Button
              variant="primary"
              disabled={confirm.busy}
              onClick={() => void confirm.run()}
            >
              Confirm suspicion
            </Button>
          </div>
        </div>
      </Modal>
    </>
  );
}
