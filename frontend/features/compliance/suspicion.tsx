"use client";

import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Field, Textarea } from "@/components/ui/field";
import { Modal } from "@/components/ui/overlay";
import { Notice } from "@/components/ui/states";
import { useAction } from "@/lib/action";
import { api, unwrap } from "@/lib/api/client";
import { useSession } from "@/lib/session";

/**
 * A compliance officer confirms suspicion on a case: a suspicious transaction report opens, due
 * within REG-14's deadline. Shown only to compliance officers (GOV-09).
 */
export function ConfirmSuspicion({ caseId }: { caseId: string }) {
  const { can } = useSession();
  const [open, setOpen] = useState(false);
  const [note, setNote] = useState("");
  const [done, setDone] = useState<string>();
  const confirm = useAction(
    async () =>
      unwrap(
        await api().POST("/api/v1/compliance/suspicion", {
          body: {
            case_id: caseId,
            ...(note.trim() ? { note: note.trim() } : {}),
          },
        }),
      ),
    {
      invalidate: ["compliance"],
      onSuccess: () => {
        setOpen(false);
        setDone(
          "Suspicion confirmed. The suspicious transaction report is in the work queue, due within 24 hours.",
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
        description="This opens a suspicious transaction report, due within 24 hours (REG-14). Only compliance officers see it; never tell the customer."
      >
        <div className="flex flex-col gap-3">
          {confirm.error ? <Notice tone="error">{confirm.error}</Notice> : null}
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
