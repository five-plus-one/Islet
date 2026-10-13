import {
  test,
  expect,
  request,
  type APIRequestContext,
} from "@playwright/test";
import { execFileSync } from "node:child_process";
import { resolve } from "node:path";
import { deflateSync } from "node:zlib";
import { mkdirSync } from "node:fs";

const root = resolve(process.cwd(), "../..");
const credentials = JSON.parse(
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
const publicURL = "http://127.0.0.1:28090";
const suffix = Date.now().toString(36);
let owner: APIRequestContext,
  member: APIRequestContext,
  outsider: APIRequestContext;
let ownerImage: any, memberImage: any, album: any, group: any, memberID: number;
let screenshotImages: Buffer[] = [];
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

test.describe.serial("Local account production acceptance", () => {
  test.beforeAll(async () => {
    mkdirSync(resolve(root, "workspace/screenshots"), { recursive: true });
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
    await call(owner, `/api/admin/users/${memberID}/status`, "PATCH", {
      status: "disabled",
    });
    expect((await member.get("/api/v1/me")).status()).toBe(401);
    await call(owner, `/api/admin/users/${memberID}/status`, "PATCH", {
      status: "active",
    });
    await login(member, "member-" + suffix);
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
  test("desktop UI, upload, editing, albums and navigation", async ({
    page,
  }) => {
    const errors: string[] = [];
    page.on("pageerror", (e) => errors.push(e.message));
    await page.goto("/login");
    await expect(page.getByRole("heading", { name: "欢迎回来" })).toBeVisible();
    await page.screenshot({
      path: resolve(root, "workspace/screenshots/login-desktop.png"),
    });
    await page
      .getByRole("textbox", { name: "用户名", exact: true })
      .fill(credentials.username);
    await page
      .getByRole("textbox", { name: "密码", exact: true })
      .fill(credentials.password);
    await page.getByRole("button", { name: "登录工作台" }).click();
    await expect(page).toHaveURL(/\/images$/);
    await page.getByRole("link", { name: "相册", exact: true }).click();
    await page
      .getByRole("button", { name: "新建相册", exact: true })
      .first()
      .click();
    await page.locator(".n-modal input").fill("灵感片段-" + suffix);
    await page.getByRole("button", { name: "创建相册", exact: true }).click();
    await expect(
      page.locator(".album-card").filter({ hasText: "灵感片段-" + suffix }),
    ).toBeVisible();
    await page.getByRole("link", { name: "上传图片" }).click();
    const sample = await page.evaluate(() => {
      const c = document.createElement("canvas");
      c.width = 800;
      c.height = 600;
      const ctx = c.getContext("2d")!;
      const out: string[] = [];
      const themes = [
        ["#b7d7e7", "#2f7388", "#e0ccb0"],
        ["#eac2a8", "#aa7877", "#535d7a"],
        ["#d6dfd5", "#638173", "#a9b69d"],
        ["#b4c7e0", "#6f89b7", "#c3d3e9"],
        ["#e4d1b2", "#b8906c", "#657b76"],
        ["#e3d8e7", "#9c8fac", "#6e789b"],
      ];
      themes.forEach((t, i) => {
        ctx.fillStyle = t[0];
        ctx.fillRect(0, 0, 800, 600);
        ctx.fillStyle = "#fff9e5";
        ctx.beginPath();
        ctx.arc(580, 130, 46, 0, Math.PI * 2);
        ctx.fill();
        ctx.fillStyle = t[1];
        ctx.beginPath();
        ctx.moveTo(0, 400);
        ctx.bezierCurveTo(180, 160 + i * 12, 380, 550, 800, 240);
        ctx.lineTo(800, 600);
        ctx.lineTo(0, 600);
        ctx.fill();
        ctx.fillStyle = t[2];
        ctx.beginPath();
        ctx.moveTo(0, 480);
        ctx.bezierCurveTo(300, 370, 480, 600, 800, 420);
        ctx.lineTo(800, 600);
        ctx.lineTo(0, 600);
        ctx.fill();
        out.push(c.toDataURL("image/png").split(",")[1]);
      });
      return out;
    });
    screenshotImages = sample.map((x) => Buffer.from(x, "base64"));
    await page.locator("input[type=file]").setInputFiles(
      screenshotImages.map((buffer, i) => ({
        name:
          [
            "海岸清晨",
            "落日余温",
            "山间来信",
            "远方的蓝",
            "沙丘漫步",
            "暮色温柔",
          ][i] + ".png",
        mimeType: "image/png",
        buffer,
      })),
    );
    await expect(
      page.locator(".upload-task").filter({ hasText: "复制链接" }),
    ).toHaveCount(6, { timeout: 30000 });
    await page.getByRole("button", { name: "查看图片库" }).click();
    await expect(page.locator(".image-card")).toHaveCount(7);
    await expect
      .poll(() =>
        page
          .locator(".image-preview img")
          .evaluateAll((imgs) =>
            imgs.every((img) => (img as HTMLImageElement).naturalWidth > 0),
          ),
      )
      .toBe(true);
    await expect(page.locator(".n-message")).toHaveCount(0);
    await page.screenshot({
      path: resolve(root, "workspace/screenshots/gallery-desktop.png"),
      fullPage: true,
    });
    await page.getByRole("button", { name: /查看 海岸清晨/ }).click();
    await expect(page.getByText("图片详情", { exact: true })).toBeVisible();
    await page.screenshot({
      path: resolve(root, "workspace/screenshots/image-detail.png"),
    });
    const nameInput = page
      .locator(".n-drawer input")
      .filter({ hasNot: page.locator("[readonly]") });
    await page
      .locator(".n-drawer .n-form input")
      .first()
      .fill("海岸清晨 · 收藏.png");
    await page.getByRole("button", { name: "保存更改", exact: true }).click();
    await expect(page.getByText("图片信息已保存")).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(page.locator(".n-drawer")).toBeHidden();
    await page
      .getByRole("textbox", { name: "搜索图片", exact: true })
      .fill("海岸清晨");
    await page.getByRole("button", { name: "搜索", exact: true }).click();
    await expect(page).toHaveURL(/q=/);
    await expect(page.locator(".image-card")).toHaveCount(1);
    await page
      .getByRole("button", { name: "查看 海岸清晨 · 收藏.png" })
      .click();
    await page.locator(".n-drawer .n-form input").first().fill("未保存");
    await page.keyboard.press("Escape");
    await expect(page.locator(".image-name")).toHaveText("海岸清晨 · 收藏.png");
    for (const [label, title] of [
      ["运行概览", "运行概览"],
      ["存储策略", "存储策略"],
      ["用户分组", "用户分组"],
      ["用户管理", "用户管理"],
      ["系统设置", "系统设置"],
      ["上传密钥", "上传密钥"],
      ["回收站", "回收站"],
    ] as const) {
      await page.getByRole("link", { name: label, exact: true }).click();
      await expect(
        page.getByRole("heading", { name: title, exact: true }),
      ).toBeVisible();
      await expect
        .poll(() =>
          page.evaluate(
            () => document.documentElement.scrollWidth <= window.innerWidth,
          ),
        )
        .toBe(true);
    }
    expect(errors).toEqual([]);
  });
  test("mobile layout, drawer, menu, recovery and expired session", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto("/login");
    await page
      .getByRole("textbox", { name: "用户名", exact: true })
      .fill(credentials.username);
    await page
      .getByRole("textbox", { name: "密码", exact: true })
      .fill(credentials.password);
    await page.getByRole("button", { name: "登录工作台" }).click();
    await expect(page).toHaveURL(/\/images$/);
    await expect(page.locator(".image-card").first()).toBeVisible();
    await page.screenshot({
      path: resolve(root, "workspace/screenshots/gallery-mobile.png"),
      fullPage: true,
    });
    await expect
      .poll(() =>
        page.evaluate(
          () => document.documentElement.scrollWidth <= window.innerWidth,
        ),
      )
      .toBe(true);
    await page.getByRole("button", { name: "打开导航" }).click();
    await page.getByRole("link", { name: "上传图片" }).click();
    await expect(
      page.getByRole("heading", { name: "上传图片", exact: true }),
    ).toBeVisible();
    await expect(page.getByRole("button", { name: "关闭导航" })).toHaveCount(0);
    await page.screenshot({
      path: resolve(root, "workspace/screenshots/upload-mobile.png"),
      fullPage: true,
    });
    await expect
      .poll(() =>
        page.evaluate(
          () => document.documentElement.scrollWidth <= window.innerWidth,
        ),
      )
      .toBe(true);
    await page.getByRole("button", { name: "查看图片库" }).click();
    await page.route("**/api/v1/images?*", (route) =>
      route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ code: 503, message: "连接暂时不可用" }),
      }),
    );
    await page.getByRole("button", { name: "刷新图片" }).click();
    await expect(page.getByText("连接暂时不可用")).toBeVisible();
    await page.unroute("**/api/v1/images?*");
    await page.getByRole("button", { name: "重新加载" }).click();
    await expect(page.locator(".image-card").first()).toBeVisible();
    for (const width of [390, 768]) {
      await page.setViewportSize({ width, height: 900 });
      for (const path of [
        "/overview",
        "/policies",
        "/groups",
        "/users",
        "/settings",
        "/tokens",
        "/albums",
        "/profile",
        "/trash",
      ]) {
        await page.goto(path);
        await expect(page.locator("h1")).toBeVisible();
        await expect
          .poll(() =>
            page.evaluate(
              () => document.documentElement.scrollWidth <= window.innerWidth,
            ),
          )
          .toBe(true);
      }
    }
  });
  test("S3 proxy, policy restrictions and recoverable deletion", async () => {
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
  test("upload retry, queue retention, clipboard and batch recycling", async ({
    page,
    context,
  }) => {
    await context.grantPermissions(["clipboard-read", "clipboard-write"]);
    await page.goto("/login");
    await page
      .getByRole("textbox", { name: "用户名", exact: true })
      .fill(credentials.username);
    await page
      .getByRole("textbox", { name: "密码", exact: true })
      .fill(credentials.password);
    await page.getByRole("button", { name: "登录工作台" }).click();
    await page.getByRole("link", { name: "上传图片" }).click();
    await page.route(
      "**/api/v1/upload",
      (r) =>
        r.fulfill({
          status: 503,
          contentType: "application/json",
          body: JSON.stringify({ code: 503, message: "上传连接中断" }),
        }),
      { times: 1 },
    );
    await page.locator("input[type=file]").setInputFiles({
      name: "重试图片.png",
      mimeType: "image/png",
      buffer: png(181),
    });
    await expect(page.getByText(/上传连接中断/)).toBeVisible();
    await page.getByRole("button", { name: "重试", exact: true }).click();
    await expect(
      page.locator(".upload-task").getByRole("button", { name: "复制链接" }),
    ).toBeVisible();
    await page.getByRole("button", { name: "查看图片库" }).click();
    await page.getByRole("link", { name: "上传图片" }).click();
    await expect(page.locator(".upload-task")).toHaveCount(1);
    await page.getByRole("button", { name: "复制本次全部链接" }).click();
    expect(await page.evaluate(() => navigator.clipboard.readText())).toContain(
      publicURL + "/i/",
    );
    await page.getByRole("button", { name: "查看图片库" }).click();
    await page
      .getByRole("textbox", { name: "搜索图片", exact: true })
      .fill("重试图片");
    await page.getByRole("button", { name: "搜索", exact: true }).click();
    await expect(page.locator(".image-card")).toHaveCount(1);
    await page.getByRole("checkbox", { name: "选择当前已加载图片" }).check();
    await page.getByRole("button", { name: "移入回收站", exact: true }).click();
    await page
      .locator(".n-dialog")
      .getByRole("button", { name: "移入回收站", exact: true })
      .click();
    await expect(page.locator(".image-card")).toHaveCount(0);
    await page.getByRole("link", { name: "回收站", exact: true }).click();
    const card = page.locator(".image-card").filter({ hasText: "重试图片" });
    await expect(card).toBeVisible();
    await card.getByRole("button", { name: "恢复", exact: true }).click();
    await expect(card).toHaveCount(0);
    const record = (
      await call(
        owner,
        "/api/v1/images?keyword=" + encodeURIComponent("重试图片"),
      )
    ).items[0];
    await call(owner, `/api/v1/images/${record.id}`, "DELETE");
    await call(owner, `/api/v1/images/${record.id}/purge`, "DELETE");
    await page.request.post("/api/v1/auth/logout");
    await page.getByRole("button", { name: "刷新图片" }).click();
    await expect(page).toHaveURL(/\/login/);
  });
  test.afterAll(async () => {
    await owner?.dispose();
    await member?.dispose();
    await outsider?.dispose();
  });
});
