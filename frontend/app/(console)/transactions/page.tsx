import type { Metadata } from "next";
import { Suspense } from "react";

import { Transactions } from "@/features/transactions/transactions";

export const metadata: Metadata = { title: "Transactions" };

export default function Page() {
  return (
    <Suspense>
      <Transactions />
    </Suspense>
  );
}
