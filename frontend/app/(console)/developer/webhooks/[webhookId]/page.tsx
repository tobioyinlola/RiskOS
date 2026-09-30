import type { Metadata } from "next";

import { WebhookDetail } from "@/features/developer/developer";

export const metadata: Metadata = { title: "Webhook endpoint" };

export default async function Page({
  params,
}: PageProps<"/developer/webhooks/[webhookId]">) {
  const { webhookId } = await params;
  return <WebhookDetail id={webhookId} />;
}
