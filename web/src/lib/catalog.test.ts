import { beforeEach, expect, it, vi } from "vitest";
import { webcrypto } from "node:crypto";
import {
  document,
  operationKey,
  submitOperation,
  type OperationInput,
} from "./catalog";
const input: OperationInput = {
  resource_type_id: "fixture.database",
  action_id: "create",
  target_kind: "scope",
  target_id: "binding",
  input: document({ name: "db" }),
};
beforeEach(() => {
  sessionStorage.clear();
  vi.stubGlobal("crypto", webcrypto);
  vi.restoreAllMocks();
});
it("keeps one key across retries and changes it only for a different request", async () => {
  const key = await operationKey(input);
  expect(await operationKey(input)).toBe(key);
  expect(
    await operationKey({ ...input, input: document({ name: "other" }) }),
  ).not.toBe(key);
});
it("replays an unknown operation with the same idempotency key", async () => {
  const keys: string[] = [];
  vi.spyOn(globalThis, "fetch").mockImplementation(async (_, init) => {
    keys.push(new Headers(init?.headers).get("Idempotency-Key")!);
    return new Response(
      JSON.stringify({ id: "operation", status: "unknown" }),
      { headers: { "content-type": "application/json" } },
    );
  });
  expect((await submitOperation(input)).status).toBe("unknown");
  expect((await submitOperation(input)).id).toBe("operation");
  expect(keys[0]).toBe(keys[1]);
});

it("never stores product secret inputs in browser storage", async () => {
  await operationKey({
    ...input,
    input: document({ name: "db", password: "product-secret" }),
  });
  for (let i = 0; i < sessionStorage.length; i++) {
    const key = sessionStorage.key(i)!;
    expect(key + sessionStorage.getItem(key)).not.toContain("product-secret");
    expect(key).not.toContain("password");
  }
});
