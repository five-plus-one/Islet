import { test } from "node:test";
import assert from "node:assert/strict";
import { api, ApiError } from "../src/api.ts";

test("historical group arrays preserve unrestricted and restricted meanings", async () => {
  const originalFetch = globalThis.fetch;
  const storage = Object.getOwnPropertyDescriptor(globalThis, "localStorage");
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    value: { getItem: () => null },
  });
  try {
    globalThis.fetch = async () =>
      Response.json({
        code: 0,
        data: {
          items: [
            { id: 1, policy_ids: null, allowed_mimes: null },
            {
              id: 2,
              policy_ids: [3, 9],
              allowed_mimes: ["image/png", "image/webp"],
            },
          ],
        },
      });
    const { items } = await api.groups();
    assert.deepEqual(items[0].policy_ids, []);
    assert.deepEqual(items[0].allowed_mimes, []);
    assert.deepEqual(items[1].policy_ids, [3, 9]);
    assert.deepEqual(items[1].allowed_mimes, ["image/png", "image/webp"]);

    let saved: Record<string, unknown> | undefined;
    globalThis.fetch = async (url, options) => {
      assert.equal(url, "/api/admin/user-groups/2");
      assert.equal(options?.method, "PATCH");
      saved = JSON.parse(String(options?.body));
      return Response.json({ code: 0, data: saved });
    };
    await api.saveGroup({ ...items[1], name: "members" });
    assert.deepEqual(saved?.policy_ids, [3, 9]);
    assert.deepEqual(saved?.allowed_mimes, ["image/png", "image/webp"]);

    globalThis.fetch = async () =>
      Response.json({ code: 500, message: "暂时无法保存" }, { status: 500 });
    await assert.rejects(
      api.saveGroup({ ...items[1], name: "members" }),
      (error: unknown) =>
        error instanceof ApiError && error.message === "暂时无法保存",
    );
  } finally {
    globalThis.fetch = originalFetch;
    if (storage) Object.defineProperty(globalThis, "localStorage", storage);
    else Reflect.deleteProperty(globalThis, "localStorage");
  }
});
