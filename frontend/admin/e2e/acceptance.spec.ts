import {
  test,
  expect,
  request,
  type APIRequestContext,
} from "@playwright/test";
import { execFileSync } from "node:child_process";
import { resolve } from "node:path";
import { deflateSync } from "node:zlib";

const root = resolve(process.cwd(), "../..");
const credentials =
  process.env.ISLET_TEST_USERNAME && process.env.ISLET_TEST_PASSWORD
    ? {
        username: process.env.ISLET_TEST_USERNAME,
        password: process.env.ISLET_TEST_PASSWORD,
      }
    : JSON.parse(
        execFileSync(
          "powershell.exe",
          [
            "-NoProfile",
            "-Command",
            `Import-Module "$PSHOME/Modules/Microsoft.PowerShell.Security/Microsoft.PowerShell.Security.psd1"; $s=Get-Content -LiteralPath '${root.replace(/'/g, "''")}\\workspace\\acceptance.key' | ConvertTo-SecureString; [System.Net.NetworkCredential]::new('', $s).Password`,
          ],
          { encoding: "utf8" },
        ),
      );
const baseURL = process.env.ISLET_TEST_URL || "http://127.0.0.1:28091";
const publicURL = process.env.ISLET_TEST_PUBLIC_URL || "http://127.0.0.1:28090";
const suffix = Date.now().toString(36);
let owner: APIRequestContext,
  member: APIRequestContext,
  outsider: APIRequestContext;
let ownerImage: any, memberImage: any, album: any, group: any, memberID: number;

const png = (seed: number) => {
  const crc = (b: Buffer) => {
    let c = 0xffffffff;
    for (const x of b) {
      c ^= x;
      for (let n = 0; n < 8; n++) c = (c >>> 1) ^ (c & 1 ? 0xedb88320 : 0);
    }
    return (c ^ 0xffffffff) >>> 0;
  };
  const chunk = (type: string, data: Buffer) => {
    const b = Buffer.concat([Buffer.from(type), data]),
      len = Buffer.alloc(4),
      sum = Buffer.alloc(4);
    len.writeUInt32BE(data.length);
    sum.writeUInt32BE(crc(b));
    return Buffer.concat([len, b, sum]);
  };
  const w = 96,
    h = 64,
    head = Buffer.alloc(13);
  head.writeUInt32BE(w);
  head.writeUInt32BE(h, 4);
  head[8] = 8;
  head[9] = 2;
  const raw = Buffer.alloc((w * 3 + 1) * h);
  for (let y = 0; y < h; y++)
    for (let x = 0; x < w; x++) {
      const i = y * (w * 3 + 1) + 1 + x * 3;
      raw[i] = (x * 2 + seed) % 256;
      raw[i + 1] = (y * 3 + seed * 2) % 256;
      raw[i + 2] = (seed * 3 + 100) % 256;
    }
  return Buffer.concat([
    Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]),
    chunk("IHDR", head),
    chunk("IDAT", deflateSync(raw)),
    chunk("IEND", Buffer.alloc(0)),
  ]);
};
async function call(
  ctx: APIRequestContext,
  path: string,
  method = "GET",
  data?: any,
) {
  const r = await ctx.fetch(path, { method, data });
  const p = await r.json();
  expect(r.ok(), `${method} ${path}: ${p.message}`).toBeTruthy();
  expect(p.code, `${method} ${path}: ${p.message}`).toBe(0);
  return p.data;
}
async function login(ctx: APIRequestContext, username: string) {
  await call(ctx, "/api/v1/auth/local/login", "POST", {
    username,
    password: credentials.password,
  });
}
async function upload(
  ctx: APIRequestContext,
  seed: number,
  extra: Record<string, string> = {},
) {
  const r = await ctx.post("/api/v1/upload", {
    multipart: {
      files: {
        name: `画面-${seed}.png`,
        mimeType: "image/png",
        buffer: png(seed),
      },
      ...extra,
    },
  });
  const p = await r.json();
  expect(p.code, p.message).toBe(0);
  return p.data.items[0];
}

test.describe.serial("Account and storage API acceptance", () => {
  test.beforeAll(async () => {
    owner = await request.newContext({ baseURL });
    const status = await owner.get("/api/setup/status");
    if (status.ok()) {
      const p = await status.json();
      if (p.data?.status !== "ready") {
        const database = {
          mysql_host: "islet-review-db",
          mysql_port: "3306",
          mysql_database: "islet",
          mysql_user: "islet",
          mysql_password: credentials.database,
          redis_host: "",
        };
        await call(owner, "/api/setup/test/database", "POST", database);
        await call(owner, "/api/setup/verify/admin", "POST", {
          mode: "local",
          username: credentials.username,
          password: credentials.password,
        });
        await call(owner, "/api/setup/complete", "POST", {
          database,
          site_title: "Islet",
          public_base_url: publicURL,
        });
      }
    }
    await login(owner, credentials.username);
  });
  test("authentication, settings, groups and user isolation", async () => {
    const currentOwner = await call(owner, "/api/v1/me");
    const protectedKey = await call(owner, "/api/v1/me/tokens", "POST", {
      name: "protected-" + suffix,
    });
    for (const [field, value] of [
      ["status", "disabled"],
      ["role", "user"],
      ["group", 2],
    ] as const) {
      const payload =
        field === "group" ? { group_id: value } : { [field]: value };
      expect(
        (
          await owner.patch(
            `/api/admin/users/${currentOwner.user.id}/${field}`,
            { data: payload },
          )
        ).status(),
      ).toBe(403);
      expect((await owner.get("/api/v1/me")).status()).toBe(200);
      expect(
        (
          await owner.patch(`/api/admin/users/999999999/${field}`, {
            data: payload,
          })
        ).status(),
      ).toBe(404);
      expect(
        (
          await owner.patch(`/api/admin/users/not-a-number/${field}`, {
            data: payload,
          })
        ).status(),
      ).toBe(400);
    }
    const protectedTokens = await call(owner, "/api/v1/me/tokens");
    expect(
      protectedTokens.items.find((x: any) => x.id === protectedKey.item.id)
        .status,
    ).toBe("active");
    await call(owner, `/api/v1/me/tokens/${protectedKey.item.id}`, "DELETE");
    const anon = await request.newContext({ baseURL });
    expect((await anon.get("/api/admin/users")).status()).toBe(401);
    expect(
      (
        await anon.post("/api/v1/auth/local/login", {
          data: { username: credentials.username, password: "wrong-password" },
        })
      ).status(),
    ).toBe(401);
    expect(
      (
        await owner.post("/api/v1/albums", {
          headers: { Origin: "https://untrusted.example" },
          data: { name: "blocked" },
        })
      ).status(),
    ).toBe(403);
    expect(
      (
        await owner.put("/api/admin/settings", {
          data: { "auth.local_enabled": "false" },
        })
      ).status(),
    ).toBe(400);
    expect(
      (await anon.get("/api/admin/settings/login-defaults")).status(),
    ).toBe(401);
    const defaults = await call(owner, "/api/admin/settings/login-defaults");
    expect(Object.keys(defaults).sort()).toEqual([
      "auth.oidc_audience",
      "auth.oidc_client_id",
      "auth.oidc_issuer",
      "auth.oidc_scopes",
      "redirect_uri",
    ]);
    expect(defaults["auth.oidc_scopes"]).toBe("openid profile email");
    const original = await call(owner, "/api/admin/settings");
    const updated = await call(owner, "/api/admin/settings", "PUT", {
      "site.title": "Settings review",
    });
    for (const key of Object.keys(original).filter(
      (key) => key !== "site.title",
    ))
      expect(updated[key]).toBe(original[key]);
    await call(owner, "/api/admin/settings", "PUT", {
      "site.title": original["site.title"],
    });
    group = await call(owner, "/api/admin/user-groups", "POST", {
      name: "验收成员-" + suffix,
      quota_bytes: 10485760,
      max_file_bytes: 1048576,
      uploads_per_hour: 0,
      allowed_mimes: ["image/png"],
      policy_ids: [1],
      status: "active",
    });
    await call(owner, `/api/admin/user-groups/${group.ID}`, "PATCH", {
      name: group.Name,
      quota_bytes: 10485760,
      max_file_bytes: 1048576,
      uploads_per_hour: 0,
      allowed_mimes: ["image/png"],
      policy_ids: [1],
      status: "active",
    });
    const user = await call(owner, "/api/admin/users", "POST", {
      username: "member-" + suffix,
      password: credentials.password,
      group_id: group.ID,
    });
    memberID = user.id;
    await call(owner, `/api/admin/users/${memberID}/group`, "PATCH", {
      group_id: group.ID,
    });
    await call(owner, `/api/admin/users/${memberID}/role`, "PATCH", {
      role: "admin",
    });
    expect(
      (await call(owner, "/api/admin/users")).items.find(
        (x: any) => x.id === memberID,
      ).role,
    ).toBe("admin");
    await call(owner, `/api/admin/users/${memberID}/role`, "PATCH", {
      role: "user",
    });

    await call(owner, "/api/admin/users", "POST", {
      username: "outsider-" + suffix,
      password: credentials.password,
      group_id: group.ID,
    });
    member = await request.newContext({ baseURL });
    outsider = await request.newContext({ baseURL });
    await login(member, "member-" + suffix);
    await login(outsider, "outsider-" + suffix);
    for (const path of [
      "/api/admin/users",
      "/api/admin/settings",
      "/api/admin/settings/login-defaults",
      "/api/admin/storage-policies",
      "/api/admin/user-groups",
      "/api/admin/stats/overview",
      "/api/admin/recycle",
    ])
      expect((await member.get(path)).status(), path).toBe(403);
    expect((await member.post("/api/admin/recycle/purge-all")).status()).toBe(
      403,
    );
    const me = await call(member, "/api/v1/me");
    expect(me.policies[0].access_key).toBeUndefined();
    expect(me.policies).toHaveLength(1);
    ownerImage = await upload(owner, 1);
    album = await call(member, "/api/v1/albums", "POST", {
      name: "旅行-" + suffix,
      description: "照片整理",
    });
    memberImage = await upload(member, 2, {
      album_id: String(album.ID),
      tags: "海边,旅行",
    });
    const mine = await call(member, "/api/v1/images?scope=all&user_id=1");
    expect(mine.items).toHaveLength(1);
    expect(mine.items[0].id).toBe(memberImage.id);
    expect((await call(outsider, "/api/v1/images")).items).toHaveLength(0);
    for (const method of ["PATCH", "DELETE"])
      expect(
        (
          await outsider.fetch(`/api/v1/images/${memberImage.id}`, {
            method,
            data: method === "PATCH" ? { name: "changed" } : undefined,
          })
        ).status(),
      ).toBe(404);
    expect(
      (
        await outsider.post("/api/v1/upload", {
          multipart: {
            files: {
              name: "cross.png",
              mimeType: "image/png",
              buffer: png(33),
            },
            album_id: String(album.ID),
          },
        })
      ).status(),
    ).toBe(400);
  });
  test("image editing, stable pagination, recycle and storage lifecycle", async () => {
    const changed = await call(
      member,
      `/api/v1/images/${memberImage.id}`,
      "PATCH",
      { name: "海边日记.png", tags: ["海边", "收藏"], album_id: album.ID },
    );
    expect(changed.tags).toEqual(["收藏", "海边"]);
    const filtered = await call(
      member,
      `/api/v1/images?album_id=${album.ID}&keyword=${encodeURIComponent("海边")}`,
    );
    expect(filtered.items).toHaveLength(1);
    expect(
      (await member.delete(`/api/v1/images/${memberImage.id}/purge`)).status(),
    ).toBe(400);
    const duplicate = await upload(member, 2);
    expect(duplicate.id).toBe(memberImage.id);
    const more = await Promise.all([
      upload(member, 3),
      upload(member, 4),
      upload(member, 5),
    ]);
    const ids: number[] = [];
    let q = "";
    for (let i = 0; i < 8; i++) {
      const page = await call(member, "/api/v1/images?limit=2" + q);
      ids.push(...page.items.map((x: any) => x.id));
      if (!page.has_more) break;
      const last = page.items.at(-1);
      q = `&before=${encodeURIComponent(last.created_at)}&before_id=${last.id}`;
    }
    expect(new Set(ids).size).toBe(4);
    expect(ids.length).toBe(4);
    const thumb = await member.get(memberImage.thumb_url);
    expect(thumb.status()).toBe(200);
    const image = await member.get(memberImage.url);
    expect(image.status()).toBe(200);
    expect(image.headers()["content-type"]).toContain("image/png");
    await call(member, `/api/v1/images/${memberImage.id}`, "DELETE");
    expect(
      (await call(member, "/api/v1/images?recycled=1")).items,
    ).toHaveLength(1);
    expect(
      (
        await outsider.post(`/api/v1/images/${memberImage.id}/restore`)
      ).status(),
    ).toBe(404);
    await call(member, `/api/v1/images/${memberImage.id}/restore`, "POST");
    expect(
      (await call(member, "/api/v1/images?recycled=1")).items,
    ).toHaveLength(0);
    await call(member, `/api/v1/images/${more[0].id}`, "DELETE");
    await call(member, `/api/v1/images/${more[0].id}/purge`, "DELETE");
    expect((await member.get(more[0].url)).status()).toBe(404);
    expect((await member.delete(`/api/v1/albums/${album.ID}`)).status()).toBe(
      400,
    );
  });
  test("upload credentials, formats, quota and session revocation", async () => {
    const trend = await call(owner, "/api/admin/stats/trend?days=14");
    expect(trend.items.at(-1).count).toBeGreaterThan(0);
    const key = await call(member, "/api/v1/me/tokens", "POST", {
      name: "PicGo-" + suffix,
    });
    const client = await request.newContext({
      baseURL: publicURL,
      extraHTTPHeaders: { Authorization: `Bearer ${key.token}` },
    });
    const uploaded = await client.post("/api/v1/upload?picgo=1", {
      multipart: {
        files: { name: "picgo.png", mimeType: "image/png", buffer: png(9) },
      },
    });
    const result = await uploaded.json();
    expect(result.success).toBe(true);
    expect(result.result[0]).toContain("/i/");
    const tokenAdmin = await request.newContext({
      baseURL,
      extraHTTPHeaders: { Authorization: `Bearer ${key.token}` },
    });
    expect((await tokenAdmin.get("/api/v1/me/tokens")).status()).toBe(403);
    const piclist = await call(member, "/api/v1/client-configs/piclist");
    expect(piclist.uploader).toBe("advancedplist");
    expect(piclist.endpoint).toBe(publicURL + "/api/v1/upload");
    expect(piclist.formDataKey).toBe("files");
    expect(JSON.stringify(piclist)).not.toContain(key.token);
    expect(
      (await tokenAdmin.get("/api/v1/client-configs/piclist")).status(),
    ).toBe(403);
    const anonymous = await request.newContext({ baseURL });
    expect(
      (await anonymous.get("/api/v1/client-configs/piclist")).status(),
    ).toBe(401);
    await anonymous.dispose();
    const piclistUpload = await client.post(piclist.endpoint, {
      multipart: {
        [piclist.formDataKey]: {
          name: "piclist.png",
          mimeType: "image/png",
          buffer: png(10),
        },
      },
    });
    const piclistResult = await piclistUpload.json();
    const piclistURL = piclist.resDataPath
      .split(".")
      .reduce((value: any, part: string) => value[part], piclistResult);
    expect(typeof piclistURL).toBe("string");
    expect(piclistURL).toContain(publicURL + "/i/");
    expect((await client.get(piclistURL)).status()).toBe(200);
    expect(
      (
        await member.post("/api/v1/upload", {
          multipart: {
            files: {
              name: "fake.png",
              mimeType: "image/png",
              buffer: Buffer.from("not an image"),
            },
          },
        })
      ).status(),
    ).toBe(400);
    await call(member, `/api/v1/me/tokens/${key.item.id}`, "DELETE");
    expect(
      (
        await client.post("/api/v1/upload", {
          multipart: {
            files: {
              name: "revoked.png",
              mimeType: "image/png",
              buffer: png(11),
            },
          },
        })
      ).status(),
    ).toBe(401);
    await call(owner, `/api/admin/user-groups/${group.ID}`, "PATCH", {
      name: group.Name,
      quota_bytes: 1,
      max_file_bytes: 1048576,
      allowed_mimes: ["image/png"],
      policy_ids: [1],
      status: "active",
    });
    expect(
      (
        await member.post("/api/v1/upload", {
          multipart: {
            files: {
              name: "quota.png",
              mimeType: "image/png",
              buffer: png(12),
            },
          },
        })
      ).status(),
    ).toBe(400);
    await call(owner, `/api/admin/user-groups/${group.ID}`, "PATCH", {
      name: group.Name,
      quota_bytes: 10485760,
      max_file_bytes: 1048576,
      allowed_mimes: ["image/png"],
      policy_ids: [1],
      status: "active",
    });
    const disabledKey = await call(member, "/api/v1/me/tokens", "POST", {
      name: "disabled-" + suffix,
    });
    await call(owner, `/api/admin/users/${memberID}/status`, "PATCH", {
      status: "disabled",
    });
    expect(
      (await call(owner, "/api/admin/users")).items.find(
        (x: any) => x.id === memberID,
      ).status,
    ).toBe("disabled");
    expect((await member.get("/api/v1/me")).status()).toBe(401);
    await call(owner, `/api/admin/users/${memberID}/status`, "PATCH", {
      status: "active",
    });
    expect((await member.get("/api/v1/me")).status()).toBe(401);
    await login(member, "member-" + suffix);
    expect(
      (await call(member, "/api/v1/me/tokens")).items.find(
        (x: any) => x.id === disabledKey.item.id,
      ).status,
    ).toBe("disabled");
    const next = credentials.password + "x";
    await call(member, "/api/v1/me/password", "POST", {
      current_password: credentials.password,
      password: next,
    });
    expect((await member.get("/api/v1/me")).status()).toBe(401);
    await call(member, "/api/v1/auth/local/login", "POST", {
      username: "member-" + suffix,
      password: next,
    });
  });
  test("S3 managed access, policy restrictions and recoverable deletion", async () => {
    const config = {
      name: "对象存储-" + suffix,
      driver: "s3",
      url_mode: "proxy",
      naming: "date",
      status: "active",
      is_default: false,
      endpoint: "http://islet-review-storage:9000",
      region: "us-east-1",
      bucket: "islet-review",
      prefix: suffix,
      cdn: "",
      access_key: "isletreview",
      secret_key: credentials.database,
      path_style: true,
    };
    const policy = await call(
      owner,
      "/api/admin/storage-policies",
      "POST",
      config,
    );
    await call(owner, `/api/admin/storage-policies/${policy.id}/test`, "POST");
    const image = await upload(owner, 63, { policy_id: String(policy.id) });
    expect(image.url).toContain(publicURL + "/i/");
    expect((await owner.get(image.url)).status()).toBe(200);
    expect((await owner.get(image.thumb_url)).status()).toBe(200);
    await call(owner, `/api/admin/storage-policies/${policy.id}`, "PATCH", {
      ...config,
      url_mode: "direct",
      cdn: publicURL + "/source-only",
    });
    const direct = await upload(owner, 64, { policy_id: String(policy.id) });
    expect(direct.url).toContain(publicURL + "/i/");
    expect((await owner.get(direct.url)).headers()["content-type"]).toContain(
      "image/png",
    );
    await call(owner, `/api/admin/images/${direct.id}/moderation`, "PATCH", {
      blocked: true,
      reason: "对象存储审核验证",
    });
    expect((await owner.get(direct.url)).headers()["content-type"]).toContain(
      "image/svg+xml",
    );
    expect(
      (await owner.get(`/api/admin/images/${direct.id}/preview`)).headers()[
        "content-type"
      ],
    ).toContain("image/png");
    await call(owner, `/api/admin/images/${direct.id}/moderation`, "PATCH", {
      blocked: false,
    });
    expect((await owner.get(direct.url)).headers()["content-type"]).toContain(
      "image/png",
    );
    await call(owner, `/api/v1/images/${direct.id}`, "DELETE");
    await call(owner, `/api/v1/images/${direct.id}/purge`, "DELETE");
    await call(
      owner,
      `/api/admin/storage-policies/${policy.id}`,
      "PATCH",
      config,
    );
    expect(
      (
        await outsider.post("/api/v1/upload", {
          multipart: {
            files: {
              name: "restricted.png",
              mimeType: "image/png",
              buffer: png(65),
            },
            policy_id: String(policy.id),
          },
        })
      ).status(),
    ).toBe(400);
    expect(
      (
        await owner.patch(`/api/admin/storage-policies/${policy.id}`, {
          data: { ...config, prefix: "other" },
        })
      ).status(),
    ).toBe(400);
    await call(owner, `/api/admin/storage-policies/${policy.id}`, "PATCH", {
      ...config,
      status: "disabled",
    });
    expect((await owner.get(image.url)).status()).toBe(200);
    await call(owner, `/api/admin/storage-policies/${policy.id}`, "PATCH", {
      ...config,
      secret_key: "invalid-credentials-for-test",
    });
    await call(owner, `/api/v1/images/${image.id}`, "DELETE");
    expect(
      (await owner.delete(`/api/v1/images/${image.id}/purge`)).status(),
    ).toBe(400);
    expect(
      (await call(owner, "/api/v1/images?recycled=1")).items.some(
        (x: any) => x.id === image.id,
      ),
    ).toBe(true);
    await call(
      owner,
      `/api/admin/storage-policies/${policy.id}`,
      "PATCH",
      config,
    );
    await call(owner, `/api/v1/images/${image.id}/purge`, "DELETE");
    expect((await owner.get(image.url)).status()).toBe(404);
    await call(owner, `/api/admin/storage-policies/${policy.id}`, "DELETE");
  });
  test("image moderation protects public links and preserves originals", async () => {
    const anon = await request.newContext({ baseURL }),
      reviewer = await request.newContext({ baseURL }),
      uploader = await request.newContext({ baseURL });
    try {
      const u = await call(owner, "/api/admin/users", "POST", {
        username: "review-uploader-" + suffix,
        password: credentials.password,
        group_id: group.ID,
      });
      const a = await call(owner, "/api/admin/users", "POST", {
        username: "reviewer-" + suffix,
        password: credentials.password,
        group_id: group.ID,
      });
      await call(owner, `/api/admin/users/${a.id}/role`, "PATCH", {
        role: "admin",
      });
      await login(reviewer, "reviewer-" + suffix);
      await login(uploader, "review-uploader-" + suffix);
      const image = await upload(uploader, 79),
        original = await (await anon.get(image.url)).body();
      const privateStates = await call(
        uploader,
        `/api/v1/images/cache-sync?ids=${image.id},${ownerImage.id}`,
      );
      expect(privateStates[image.id]).toBe("complete");
      expect(privateStates[ownerImage.id]).toBeUndefined();
      expect(
        (await anon.get(`/api/v1/images/cache-sync?ids=${image.id}`)).status(),
      ).toBe(401);
      const key = new URL(image.url).pathname.slice(3),
        legacyURL = publicURL + "/legacy/" + key;
      expect(await (await anon.get(legacyURL)).body()).toEqual(original);
      expect(
        (await call(reviewer, `/api/v1/images?scope=all&user_id=${u.id}`))
          .items[0].id,
      ).toBe(image.id);
      expect(
        (await call(uploader, "/api/v1/images?scope=all")).items.every(
          (x: any) => x.user_id === u.id,
        ),
      ).toBe(true);
      const path = `/api/admin/images/${image.id}/moderation`,
        preview = `/api/admin/images/${image.id}/preview`;
      expect((await anon.get(preview)).status()).toBe(401);
      expect((await uploader.get(preview)).status()).toBe(403);
      expect(
        (
          await uploader.patch(path, {
            data: { blocked: true, reason: "越权" },
          })
        ).status(),
      ).toBe(403);
      for (const data of [
        {},
        { blocked: null },
        { blocked: "true" },
        { blocked: true, reason: " " },
        { blocked: true, reason: "字".repeat(501) },
      ]) {
        expect((await reviewer.patch(path, { data })).status()).toBe(400);
      }
      expect(
        (
          await reviewer.patch("/api/admin/images/not-a-number/moderation", {
            data: { blocked: true, reason: "测试" },
          })
        ).status(),
      ).toBe(400);
      expect(
        (
          await reviewer.patch("/api/admin/images/999999999/moderation", {
            data: { blocked: true, reason: "测试" },
          })
        ).status(),
      ).toBe(404);
      const before = await anon.get(image.url),
        etag = before.headers().etag;
      const blocked = await call(reviewer, path, "PATCH", {
        blocked: true,
        reason: "测试图片违反使用策略",
      });
      expect(blocked.blocked).toBe(true);
      expect(blocked.block_reason).toBe("测试图片违反使用策略");
      expect(blocked.moderated_at).toBeTruthy();
      for (const url of [
        image.url,
        image.thumb_url,
        legacyURL,
        legacyURL + ".thumb.png",
      ]) {
        const response = await anon.get(url, {
          headers: { "If-None-Match": etag },
        });
        expect(response.status()).toBe(200);
        expect(response.headers()["content-type"]).toContain("image/svg+xml");
        expect(response.headers()["cache-control"]).toContain("no-store");
        expect(await response.text()).toContain("图片违反使用策略");
        expect(await response.text()).not.toContain("测试图片");
        const head = await anon.head(url);
        expect(head.status()).toBe(200);
        expect(await head.body()).toHaveLength(0);
      }
      const protectedPreview = await reviewer.get(preview);
      expect(protectedPreview.headers()["cache-control"]).toBe(
        "private, no-store",
      );
      expect(await protectedPreview.body()).toEqual(original);
      expect(
        (await reviewer.get(preview + "?thumb=1")).headers()["content-type"],
      ).toContain("image/png");
      expect(
        (await call(uploader, "/api/v1/images?moderation=blocked")).items[0]
          .block_reason,
      ).toBe("测试图片违反使用策略");
      expect(
        (
          await call(
            reviewer,
            `/api/v1/images?scope=all&user_id=${u.id}&moderation=active`,
          )
        ).items,
      ).toHaveLength(0);
      expect((await upload(uploader, 79)).blocked).toBe(true);
      await call(uploader, `/api/v1/images/${image.id}`, "PATCH", {
        blocked: false,
        block_reason: "",
        name: "复核画面.png",
      });
      expect((await call(uploader, "/api/v1/images")).items[0].blocked).toBe(
        true,
      );
      await call(uploader, `/api/v1/images/${image.id}`, "DELETE");
      expect((await anon.get(image.url)).status()).toBe(404);
      expect((await reviewer.get(preview)).status()).toBe(404);
      await call(uploader, `/api/v1/images/${image.id}/restore`, "POST");
      expect((await anon.get(image.url)).headers()["content-type"]).toContain(
        "image/svg+xml",
      );
      const restored = await call(reviewer, path, "PATCH", { blocked: false });
      expect(restored.blocked).toBe(false);
      expect(restored.block_reason).toBe("");
      expect(await (await anon.get(image.url)).body()).toEqual(original);
      expect(await (await anon.get(legacyURL)).body()).toEqual(original);
      expect(
        (await anon.get(image.thumb_url)).headers()["content-type"],
      ).toContain("image/png");
      expect((await anon.head(image.url)).headers()["cache-control"]).toBe(
        "no-store",
      );
      const audit = (
        await call(owner, "/api/admin/audit-logs?limit=100")
      ).items.filter(
        (x: any) =>
          x.target_type === "image" &&
          x.target_id === String(image.id) &&
          ["image.block", "image.allow"].includes(x.action),
      );
      expect(audit.map((x: any) => x.action).sort()).toEqual([
        "image.allow",
        "image.block",
      ]);
      expect(
        JSON.parse(audit.find((x: any) => x.action === "image.block").detail)
          .reason,
      ).toBe("测试图片违反使用策略");
      const token = await call(owner, "/api/v1/me/tokens", "POST", {
        name: "moderation-isolation-" + suffix,
      });
      expect(
        (
          await anon.get(preview, {
            headers: { Authorization: `Bearer ${token.token}` },
          })
        ).status(),
      ).toBe(403);
      expect(
        (
          await anon.patch(path, {
            headers: { Authorization: `Bearer ${token.token}` },
            data: { blocked: true, reason: "Token 禁止审核" },
          })
        ).status(),
      ).toBe(403);
      await call(owner, `/api/v1/me/tokens/${token.item.id}`, "DELETE");
    } finally {
      await anon.dispose();
      await reviewer.dispose();
      await uploader.dispose();
    }
  });
  test.afterAll(async () => {
    await owner?.dispose();
    await member?.dispose();
    await outsider?.dispose();
  });
});
