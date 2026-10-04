import type { Metadata } from "next";
import { Suspense } from "react";

import { RulesHome } from "@/features/rules/rules";

export const metadata: Metadata = { title: "Rules" };

export default function Page() {
  return (
    <Suspense>
      <RulesHome />
    </Suspense>
  );
}
