"use client";

import { Tab, TabList, TabPanel, Tabs, CodeBlock } from "@/components/ui/misc";

/** The same example in several languages (curl, Node.js, Python, Go). */
export function CodeTabs({
  label,
  samples,
}: {
  label: string;
  samples: { lang: string; code: string }[];
}) {
  return (
    <Tabs defaultValue={samples[0]!.lang}>
      <TabList aria-label={`${label}: language`} className="mb-2">
        {samples.map((s) => (
          <Tab key={s.lang} value={s.lang}>
            {s.lang}
          </Tab>
        ))}
      </TabList>
      {samples.map((s) => (
        <TabPanel key={s.lang} value={s.lang}>
          <CodeBlock value={s.code} label={`${label} (${s.lang})`} />
        </TabPanel>
      ))}
    </Tabs>
  );
}
