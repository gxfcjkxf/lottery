import type { AgentChildUpdate } from "./agent-api";
import type { AgentNode } from "./agent-api";

export interface AgentUpdateIntent {
  brandId: string;
  memberId: string;
  childId: string;
  body: Readonly<AgentChildUpdate>;
  idempotencyKey: string;
}

const intents = new Map<string, AgentUpdateIntent>();
const receipts = new Map<string, AgentNode>();

function intentKey(brandId: string, memberId: string, childId: string): string {
  return `${brandId}:${memberId}:${childId}`;
}
function receiptKey(brandId: string, memberId: string, childId: string, idempotencyKey: string): string {
  return `${intentKey(brandId, memberId, childId)}:${idempotencyKey}`;
}

export function rememberAgentUpdate(intent: AgentUpdateIntent): AgentUpdateIntent {
  const key = intentKey(intent.brandId, intent.memberId, intent.childId);
  const existing = intents.get(key);
  if (existing) return existing;
  const body = Object.freeze({ ...intent.body });
  const retained = Object.freeze({ ...intent, body });
  intents.set(key, retained);
  return retained;
}

export function getPendingAgentUpdate(
  brandId: string,
  memberId: string,
  childId: string,
): AgentUpdateIntent | null {
  return intents.get(intentKey(brandId, memberId, childId)) ?? null;
}

export function getPendingAgentUpdatesForScope(
  brandId: string,
  memberId: string,
): AgentUpdateIntent[] {
  return [...intents.values()].filter(
    (intent) => intent.brandId === brandId && intent.memberId === memberId,
  );
}

export function clearPendingAgentUpdate(intent: Pick<AgentUpdateIntent, "brandId" | "memberId" | "childId" | "idempotencyKey">): boolean {
  const key = intentKey(intent.brandId, intent.memberId, intent.childId);
  if (intents.get(key)?.idempotencyKey !== intent.idempotencyKey) return false;
  intents.delete(key);
  return true;
}

/** Preserve the first acknowledged receipt; it is historical evidence, not live state. */
export function cacheAgentUpdateReceipt(
  intent: Pick<AgentUpdateIntent, "brandId" | "memberId" | "childId" | "idempotencyKey">,
  receipt: AgentNode,
): AgentNode {
  const key = receiptKey(intent.brandId, intent.memberId, intent.childId, intent.idempotencyKey);
  const existing = receipts.get(key);
  if (existing) return existing;
  const firstReceipt = Object.freeze({ ...receipt, path: Object.freeze([...receipt.path]), config: Object.freeze({ ...receipt.config }) }) as AgentNode;
  receipts.set(key, firstReceipt);
  return firstReceipt;
}

export function getAgentUpdateReceipt(
  brandId: string,
  memberId: string,
  childId: string,
  idempotencyKey: string,
): AgentNode | null {
  return receipts.get(receiptKey(brandId, memberId, childId, idempotencyKey)) ?? null;
}

export function clearAgentUpdateReceipt(
  intent: Pick<AgentUpdateIntent, "brandId" | "memberId" | "childId" | "idempotencyKey">,
): void {
  receipts.delete(receiptKey(intent.brandId, intent.memberId, intent.childId, intent.idempotencyKey));
}

export function clearAgentUpdateScope(brandId: string, memberId: string): void {
  for (const [key, intent] of intents) {
    if (intent.brandId === brandId && intent.memberId === memberId) {
      intents.delete(key);
      receipts.delete(key);
    }
  }
  for (const [key, receipt] of receipts) {
    if (receipt.brand_id === brandId && receipt.member_id === memberId) receipts.delete(key);
  }
}

export function clearAllAgentUpdates(): void {
  intents.clear();
  receipts.clear();
}
