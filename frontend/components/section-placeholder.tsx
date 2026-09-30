import { getSection, type SectionHref } from "@/lib/navigation";

/**
 * Honest placeholder for a console section that has not been built. It states what the section
 * will contain and never renders sample or fabricated data.
 */
export function SectionPlaceholder({ href }: { href: SectionHref }) {
  const section = getSection(href);

  return (
    <section aria-labelledby="section-title" className="max-w-3xl space-y-4">
      <h1 id="section-title" className="text-2xl font-semibold tracking-tight">
        {section.label}
      </h1>
      <p className="text-muted-foreground">{section.description}</p>
      <div className="rounded-lg border border-dashed p-6 text-sm">
        <p className="font-medium">Not built yet</p>
        <p className="mt-1 text-muted-foreground">
          This section is delivered in the frontend phase of the implementation
          plan, once the API it depends on exists. No data is shown until then.
        </p>
      </div>
    </section>
  );
}
