import type { Metadata } from "next";

import { Developer } from "@/features/developer/developer";

export const metadata: Metadata = { title: "Developer" };

export default function Page() {
  return <Developer />;
}
