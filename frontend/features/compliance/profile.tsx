"use client";

import { useState, type FormEvent } from "react";

import { Button } from "@/components/ui/button";
import { Field, Select } from "@/components/ui/field";
import { Modal } from "@/components/ui/overlay";
import { Notice } from "@/components/ui/states";
import { useAction } from "@/lib/action";
import { api, unwrap, type Schemas } from "@/lib/api/client";

type Pack = Schemas["CompliancePack"];
type Profile = Schemas["ComplianceProfile"];

const label = (opts: Schemas["ComplianceOption"][], id: string) =>
  opts.find((o) => o.id === id)?.label ?? id;

/** "Commercial bank · Instant payments · Nigeria pack 2026.10.0". */
export function profileSummary(pack: Pack, p: Profile): string {
  return [
    label(pack.licence_types, p.licence_type),
    ...p.capabilities.map((c) => label(pack.capabilities, c)),
    `${p.jurisdiction} pack ${p.pack_version}`,
  ].join(" · ");
}

/** Sets the licence type and capabilities that decide which rules apply. Audited. */
export function ProfileDialog({
  open,
  onOpenChange,
  pack,
  profile,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  pack: Pack;
  profile: Profile | null;
}) {
  const [licence, setLicence] = useState(profile?.licence_type ?? "");
  const [caps, setCaps] = useState<string[]>(profile?.capabilities ?? []);
  const save = useAction(
    async () =>
      unwrap(
        await api().PUT("/api/v1/compliance/profile", {
          body: {
            jurisdiction: pack.jurisdiction,
            licence_type: licence,
            capabilities: caps,
          },
        }),
      ),
    { invalidate: ["compliance"], onSuccess: () => onOpenChange(false) },
  );
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (licence) void save.run();
  };
  const toggle = (id: string, on: boolean) =>
    setCaps((cur) => (on ? [...cur, id] : cur.filter((c) => c !== id)));
  return (
    <Modal
      open={open}
      onOpenChange={onOpenChange}
      title="Your institution"
      description="Decides which rules apply. The change is recorded in the audit log."
    >
      <form onSubmit={onSubmit} className="flex flex-col gap-4">
        {save.error ? <Notice tone="error">{save.error}</Notice> : null}
        <Field label="Licence type">
          {(p) => (
            <Select
              {...p}
              value={licence}
              onChange={(e) => setLicence(e.target.value)}
              required
            >
              <option value="" disabled>
                Choose…
              </option>
              {pack.licence_types.map((l) => (
                <option key={l.id} value={l.id}>
                  {l.label}
                </option>
              ))}
            </Select>
          )}
        </Field>
        {pack.capabilities.length ? (
          <fieldset className="flex flex-col gap-2">
            <legend className="mb-1 text-label text-text-primary">
              What you offer
            </legend>
            {pack.capabilities.map((c) => (
              <label
                key={c.id}
                className="inline-flex items-center gap-2 text-body-sm"
              >
                <input
                  type="checkbox"
                  className="size-4 accent-ink"
                  checked={caps.includes(c.id)}
                  onChange={(e) => toggle(c.id, e.target.checked)}
                />
                {c.label}
              </label>
            ))}
          </fieldset>
        ) : null}
        <div className="flex justify-end gap-2">
          <Button type="button" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            type="submit"
            variant="primary"
            disabled={save.busy || !licence}
          >
            Save
          </Button>
        </div>
      </form>
    </Modal>
  );
}
