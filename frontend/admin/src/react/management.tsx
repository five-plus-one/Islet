import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Button, Card, Dialog, Checkbox, EmptyState } from "@fiveplusone/ui";
import {
  api,
  type GroupView,
  type PolicyInput,
  type UserView,
  beginOIDCLogin,
} from "../api";
import { errorText, humanBytes, groupLabel, dateLabel } from "../utils";
import {
  Heading,
  TextField,
  Choice,
  Failure,
  Loading,
  Confirm,
  useLoad,
  useSession,
  changed,
} from "./common";
export function AdminPage({ kind }: { kind: string }) {
  return kind === "overview" ? (
    <Overview />
  ) : kind === "settings" ? (
    <Settings />
  ) : kind === "users" ? (
    <Users />
  ) : kind === "groups" ? (
    <Groups />
  ) : (
    <Policies />
  );
}
function Overview() {
  const data = useLoad(() =>
    Promise.all([api.overview(), api.trend(14), api.groupUsage()]),
  );
  return (
    <>
      <Heading
        title="运行概览"
        description="了解图片数量、存储用量与最近上传情况。"
        actions={
          <Button variant="secondary" onClick={data.reload}>
            刷新
          </Button>
        }
      />
      <Failure error={data.error} retry={data.reload} />
      {data.loading ? (
        <Loading />
      ) : (
        data.data && (
          <>
            <div className="islet-card-grid islet-stats">
              {[
                ["图片总数", "total_images"],
                ["存储用量", "total_bytes"],
                ["用户总数", "total_users"],
                ["待审核用户", "pending_users"],
              ].map(([label, key]) => (
                <Card key={key}>
                  <span className="fp-muted">{label}</span>
                  <h2>
                    {key === "total_bytes"
                      ? humanBytes(Number(data.data![0][key] || 0))
                      : String(data.data![0][key] ?? 0)}
                  </h2>
                </Card>
              ))}
            </div>
            <Card>
              <h2>最近 14 天上传</h2>
              <div className="islet-trend">
                {data.data[1].items.map((x) => (
                  <div key={x.date}>
                    <span>{x.count}</span>
                    <i
                      style={{
                        height: Math.max(
                          3,
                          (x.count /
                            Math.max(
                              1,
                              ...data.data![1].items.map((i) => i.count),
                            )) *
                            130,
                        ),
                      }}
                    />
                    <small>{x.date.slice(5)}</small>
                  </div>
                ))}
              </div>
            </Card>
            <Card>
              <h2>分组存储</h2>
              <div className="islet-table">
                <table>
                  <thead>
                    <tr>
                      <th>用户组</th>
                      <th>图片</th>
                      <th>已使用</th>
                    </tr>
                  </thead>
                  <tbody>
                    {data.data[2].items.map((x) => (
                      <tr key={x.group_id}>
                        <td>{groupLabel(x.name)}</td>
                        <td>{x.images}</td>
                        <td>{humanBytes(x.used_bytes)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </Card>
          </>
        )
      )}
    </>
  );
}
function Settings() {
  const data = useLoad(api.settings),
    { notify } = useSession();
  const [values, setValues] = useState<Record<string, string>>({}),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  useEffect(() => {
    if (data.data) setValues(data.data);
  }, [data.data]);
  const field = (key: string, label: string) => (
    <TextField
      key={key}
      label={label}
      value={values[key] || ""}
      onChange={(v) => setValues((x) => ({ ...x, [key]: v }))}
    />
  );
  return (
    <>
      <Heading
        title="系统设置"
        description="配置站点、登录方式和图片保留规则。"
      />
      <Failure error={data.error} retry={data.reload} />
      {data.loading ? (
        <Loading />
      ) : (
        data.data && (
          <form
            className="islet-form"
            onSubmit={async (e) => {
              e.preventDefault();
              setBusy(true);
              setError("");
              try {
                setValues(await api.saveSettings(values));
                notify("设置已保存");
                changed();
              } catch (e) {
                setError(errorText(e));
              } finally {
                setBusy(false);
              }
            }}
          >
            <div className="islet-card-grid">
              <Card>
                <h2>站点信息</h2>
                <div className="islet-form">
                  {field("site.title", "站点名称")}
                  {field("site.public_base_url", "图片访问地址")}
                  {field("brand.name", "品牌名称")}
                  {field("brand.logo_url", "品牌图标地址")}
                  {field("brand.oidc_label", "统一登录按钮名称")}
                </div>
              </Card>
              <Card>
                <h2>统一登录</h2>
                <div className="islet-form">
                  {field("auth.oidc_issuer", "身份提供方地址（Issuer）")}
                  {field("auth.oidc_client_id", "应用 ID")}
                  {field("auth.oidc_audience", "资源标识（可选）")}
                  {field("auth.oidc_scopes", "身份范围（Scopes）")}
                  <Checkbox
                    label="启用本地账户登录"
                    checked={values["auth.local_enabled"] !== "false"}
                    onChange={(e) =>
                      setValues((x) => ({
                        ...x,
                        "auth.local_enabled": String(e.target.checked),
                      }))
                    }
                  />
                  <Button
                    variant="secondary"
                    onClick={async () => {
                      try {
                        await api.adminTestOIDC(values["auth.oidc_issuer"]);
                        notify("登录服务连接正常");
                      } catch (e) {
                        setError(errorText(e));
                      }
                    }}
                  >
                    检查登录服务
                  </Button>
                </div>
              </Card>
              <Card>
                <h2>上传与注册</h2>
                <div className="islet-form">
                  <TextField
                    label="单张图片上限（MB）"
                    type="number"
                    value={
                      Number(values["upload.max_bytes"] || 104857600) / 1048576
                    }
                    onChange={(v) =>
                      setValues((x) => ({
                        ...x,
                        "upload.max_bytes": String(Number(v) * 1048576),
                      }))
                    }
                  />
                  <TextField
                    label="回收站保留天数"
                    type="number"
                    value={values["recycle.retention_days"] || "30"}
                    onChange={(v) =>
                      setValues((x) => ({ ...x, "recycle.retention_days": v }))
                    }
                  />
                  <Checkbox
                    label="自动通过新用户注册"
                    checked={values["users.auto_approve"] === "true"}
                    onChange={(e) =>
                      setValues((x) => ({
                        ...x,
                        "users.auto_approve": String(e.target.checked),
                      }))
                    }
                  />
                </div>
              </Card>
            </div>
            <Failure error={error} />
            <Button type="submit" loading={busy}>
              保存设置
            </Button>
          </form>
        )
      )}
    </>
  );
}
function Users() {
  const data = useLoad(api.users),
    groups = useLoad(api.groups),
    { me, notify } = useSession();
  const [query, setQuery] = useState(""),
    [open, setOpen] = useState(false),
    [name, setName] = useState(""),
    [password, setPassword] = useState(""),
    [group, setGroup] = useState("2"),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const superAdmin = me.user.role === "super_admin";
  async function change(fn: () => Promise<unknown>) {
    setBusy(true);
    try {
      await fn();
      data.reload();
      notify("用户信息已更新");
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <Heading
        title="用户管理"
        description="管理成员、访问权限与存储分组。"
        actions={
          superAdmin && (
            <Button
              onClick={() => {
                setName("");
                setPassword("");
                setOpen(true);
              }}
            >
              创建本地用户
            </Button>
          )
        }
      />
      <Failure
        error={data.error || groups.error}
        retry={() => {
          data.reload();
          groups.reload();
        }}
      />
      <Failure error={error} />
      <Card>
        <TextField label="搜索用户" value={query} onChange={setQuery} />
        {data.loading ? (
          <Loading />
        ) : (
          <div className="islet-table">
            <table>
              <thead>
                <tr>
                  <th>用户</th>
                  <th>分组</th>
                  <th>角色</th>
                  <th>状态</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {data.data?.items
                  .filter((x) =>
                    (x.username + " " + x.email)
                      .toLowerCase()
                      .includes(query.toLowerCase()),
                  )
                  .map((x) => (
                    <tr key={x.id}>
                      <td>
                        <strong>{x.username}</strong>
                        <small>{x.email}</small>
                      </td>
                      <td>
                        <Choice
                          label={`${x.username}的分组`}
                          value={String(x.group_id)}
                          onChange={(v) =>
                            !busy &&
                            void change(() => api.setUserGroup(x.id, Number(v)))
                          }
                          options={(groups.data?.items || []).map((g) => ({
                            value: String(g.id),
                            label: groupLabel(g.name),
                          }))}
                        />
                      </td>
                      <td>
                        {superAdmin && x.id !== me.user.id ? (
                          <Choice
                            label={`${x.username}的角色`}
                            value={x.role}
                            onChange={(v) =>
                              !busy &&
                              void change(() => api.setUserRole(x.id, v))
                            }
                            options={[
                              { value: "user", label: "成员" },
                              { value: "admin", label: "管理员" },
                              { value: "super_admin", label: "超级管理员" },
                            ]}
                          />
                        ) : x.role === "super_admin" ? (
                          "超级管理员"
                        ) : x.role === "admin" ? (
                          "管理员"
                        ) : (
                          "成员"
                        )}
                      </td>
                      <td>
                        {x.status === "active"
                          ? "正常"
                          : x.status === "pending"
                            ? "待审核"
                            : "已停用"}
                      </td>
                      <td>
                        {x.id !== me.user.id && (
                          <Button
                            variant="secondary"
                            disabled={
                              busy || (!superAdmin && x.role !== "user")
                            }
                            onClick={() =>
                              void change(() =>
                                api.setUserStatus(
                                  x.id,
                                  x.status === "active" ? "disabled" : "active",
                                ),
                              )
                            }
                          >
                            {x.status === "active" ? "停用" : "启用"}
                          </Button>
                        )}
                      </td>
                    </tr>
                  ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
      <Dialog
        open={open}
        title="创建本地用户"
        onClose={() => !busy && setOpen(false)}
        footer={
          <Button
            loading={busy}
            onClick={async () => {
              setBusy(true);
              try {
                await api.createUser(name, password, Number(group));
                setOpen(false);
                data.reload();
                notify("用户已创建");
              } catch (e) {
                setError(errorText(e));
              } finally {
                setBusy(false);
              }
            }}
          >
            创建用户
          </Button>
        }
      >
        <div className="islet-form">
          <TextField label="用户名" value={name} onChange={setName} />
          <TextField
            label="初始密码"
            type="password"
            value={password}
            onChange={setPassword}
          />
          <Choice
            label="用户分组"
            value={group}
            onChange={setGroup}
            options={(groups.data?.items || []).map((x) => ({
              value: String(x.id),
              label: groupLabel(x.name),
            }))}
          />
          <Failure error={error} />
        </div>
      </Dialog>
    </>
  );
}
function Groups() {
  const data = useLoad(api.groups),
    policies = useLoad(api.policies),
    { notify } = useSession();
  const [edit, setEdit] = useState<Partial<GroupView> | null>(null),
    [remove, setRemove] = useState<GroupView>(),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const update = (key: string, v: unknown) =>
    setEdit((x) => ({ ...x, [key]: v }));
  return (
    <>
      <Heading
        title="用户分组"
        description="为每位成员设置容量、上传限制和可用存储。"
        actions={
          <Button
            onClick={() => {
              setError("");
              setEdit({
                name: "",
                description: "",
                quota_bytes: 1073741824,
                max_file_bytes: 10485760,
                allowed_mimes: [
                  "image/jpeg",
                  "image/png",
                  "image/webp",
                  "image/gif",
                ],
                policy_ids: [],
                uploads_per_hour: 100,
                status: "active",
              });
            }}
          >
            新建分组
          </Button>
        }
      />
      <Failure error={data.error} retry={data.reload} />
      {data.loading ? (
        <Loading />
      ) : (
        <div className="islet-card-grid">
          {data.data?.items.map((x) => (
            <Card key={x.id}>
              <h2>{groupLabel(x.name)}</h2>
              <p>{x.description}</p>
              <p>
                每位成员容量：
                {x.quota_bytes ? humanBytes(x.quota_bytes) : "不限"}
              </p>
              <p>单张上限：{humanBytes(x.max_file_bytes)}</p>
              <div className="islet-row">
                <Button
                  variant="secondary"
                  onClick={() => {
                    setError("");
                    setEdit({ ...x, policy_ids: [...x.policy_ids] });
                  }}
                >
                  编辑
                </Button>
                {!x.built_in && (
                  <Button variant="ghost" onClick={() => setRemove(x)}>
                    删除分组
                  </Button>
                )}
              </div>
            </Card>
          ))}
        </div>
      )}
      <Dialog
        open={!!edit}
        title={edit?.id ? "编辑分组" : "新建分组"}
        onClose={() => !busy && setEdit(null)}
        footer={
          <Button
            loading={busy}
            onClick={async () => {
              setBusy(true);
              try {
                await api.saveGroup({ ...edit, name: edit?.name || "" });
                setEdit(null);
                data.reload();
                notify("分组已保存");
              } catch (e) {
                setError(errorText(e));
              } finally {
                setBusy(false);
              }
            }}
          >
            保存分组
          </Button>
        }
      >
        {edit && (
          <div className="islet-form">
            <TextField
              label="分组名称"
              value={edit.name || ""}
              onChange={(v) => update("name", v)}
            />
            <TextField
              label="描述"
              value={edit.description || ""}
              onChange={(v) => update("description", v)}
            />
            <TextField
              label="每位成员容量（MB，0 为不限）"
              type="number"
              value={(edit.quota_bytes || 0) / 1048576}
              onChange={(v) => update("quota_bytes", Number(v) * 1048576)}
            />
            <TextField
              label="单张上限（MB）"
              type="number"
              value={(edit.max_file_bytes || 0) / 1048576}
              onChange={(v) => update("max_file_bytes", Number(v) * 1048576)}
            />
            <TextField
              label="每小时上传数量"
              type="number"
              value={edit.uploads_per_hour || 0}
              onChange={(v) => update("uploads_per_hour", Number(v))}
            />
            <TextField
              label="允许的图片格式"
              value={(edit.allowed_mimes || []).join(", ")}
              onChange={(v) =>
                update(
                  "allowed_mimes",
                  v
                    .split(",")
                    .map((x) => x.trim())
                    .filter(Boolean),
                )
              }
              hint="填写 MIME 类型，多个用逗号分隔"
            />
            <fieldset>
              <legend>可用存储</legend>
              {policies.data?.items.map((x) => (
                <Checkbox
                  key={x.id}
                  label={x.name}
                  checked={edit.policy_ids?.includes(x.id) || false}
                  onChange={(e) =>
                    update(
                      "policy_ids",
                      e.target.checked
                        ? [...(edit.policy_ids || []), x.id]
                        : (edit.policy_ids || []).filter((id) => id !== x.id),
                    )
                  }
                />
              ))}
            </fieldset>
            <Choice
              label="状态"
              value={edit.status || "active"}
              onChange={(v) => update("status", v)}
              options={[
                { value: "active", label: "启用" },
                { value: "disabled", label: "停用" },
              ]}
            />
            <Failure error={error} />
          </div>
        )}
      </Dialog>
      {remove && (
        <Confirm
          title="删除分组"
          description={`删除「${groupLabel(remove.name)}」，已有成员的分组需要先转移成员。`}
          onClose={() => setRemove(undefined)}
          onConfirm={async () => {
            await api.deleteGroup(remove.id);
            data.reload();
          }}
        />
      )}
    </>
  );
}
function Policies() {
  const data = useLoad(api.policies),
    { notify } = useSession();
  const [edit, setEdit] = useState<PolicyInput | null>(null),
    [remove, setRemove] = useState<number>(),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const blank: PolicyInput = {
    name: "",
    driver: "local",
    url_mode: "proxy",
    naming: "date",
    is_default: false,
    status: "active",
    endpoint: "",
    region: "",
    bucket: "",
    prefix: "",
    cdn: "",
    access_key: "",
    secret_key: "",
    path_style: true,
  };
  const update = (key: string, v: unknown) =>
    setEdit((x) => ({ ...x!, [key]: v }));
  return (
    <>
      <Heading
        title="存储策略"
        description="连接本地磁盘或对象存储，为图片选择合适的位置。"
        actions={
          <Button
            onClick={() => {
              setError("");
              setEdit({ ...blank });
            }}
          >
            新建策略
          </Button>
        }
      />
      <Failure error={data.error} retry={data.reload} />
      <Failure error={error} />
      {data.loading ? (
        <Loading />
      ) : (
        <div className="islet-card-grid">
          {data.data?.items.map((x) => (
            <Card key={x.id}>
              <h2>
                {x.name}
                {x.is_default ? " · 默认" : ""}
              </h2>
              <p>
                {x.driver === "local" ? "本地磁盘" : "对象存储"} ·{" "}
                {x.status === "active" ? "启用" : "停用"}
              </p>
              <div className="islet-row">
                <Button
                  variant="secondary"
                  onClick={() => {
                    setError("");
                    setEdit({ ...blank, ...x, secret_key: "" });
                  }}
                >
                  编辑
                </Button>
                <Button
                  variant="ghost"
                  disabled={busy}
                  onClick={async () => {
                    setBusy(true);
                    try {
                      await api.testPolicyById(x.id);
                      notify("存储连接正常");
                    } catch (e) {
                      setError(errorText(e));
                    } finally {
                      setBusy(false);
                    }
                  }}
                >
                  测试连接
                </Button>
                <Button variant="ghost" onClick={() => setRemove(x.id)}>
                  删除
                </Button>
              </div>
            </Card>
          ))}
        </div>
      )}
      <Dialog
        open={!!edit}
        title={edit?.id ? "编辑存储策略" : "新建存储策略"}
        onClose={() => !busy && setEdit(null)}
        footer={
          <Button
            loading={busy}
            onClick={async () => {
              setBusy(true);
              try {
                await api.savePolicy(edit!);
                setEdit(null);
                data.reload();
                notify("存储策略已保存");
              } catch (e) {
                setError(errorText(e));
              } finally {
                setBusy(false);
              }
            }}
          >
            保存策略
          </Button>
        }
      >
        {edit && (
          <div className="islet-form">
            <TextField
              label="名称"
              value={edit.name}
              onChange={(v) => update("name", v)}
            />
            <Choice
              label="存储类型"
              value={edit.driver}
              onChange={(v) => update("driver", v)}
              options={[
                { value: "local", label: "本地磁盘" },
                { value: "s3", label: "S3 / 阿里云 OSS" },
              ]}
            />
            {edit.driver === "s3" && (
              <>
                {[
                  ["endpoint", "服务地址"],
                  ["region", "区域"],
                  ["bucket", "存储桶"],
                  ["prefix", "路径前缀"],
                  ["cdn", "CDN 地址（可选）"],
                  ["access_key", "访问密钥 ID"],
                  ["secret_key", "访问密钥（留空保留已有）"],
                ].map(([key, label]) => (
                  <TextField
                    key={key}
                    label={label}
                    value={String(edit[key as keyof PolicyInput] || "")}
                    type={key === "secret_key" ? "password" : "text"}
                    onChange={(v) => update(key, v)}
                  />
                ))}
                <Checkbox
                  label="使用路径风格访问"
                  checked={edit.path_style}
                  onChange={(e) => update("path_style", e.target.checked)}
                />
              </>
            )}
            <Choice
              label="命名方式"
              value={edit.naming}
              onChange={(v) => update("naming", v)}
              options={[
                { value: "date", label: "日期目录" },
                { value: "random", label: "随机名称" },
                { value: "keep", label: "保留文件名" },
              ]}
            />
            <Choice
              label="访问方式"
              value={edit.url_mode}
              onChange={(v) => update("url_mode", v)}
              options={[
                { value: "proxy", label: "通过本站访问" },
                { value: "direct", label: "直连存储 / CDN" },
              ]}
            />
            <Choice
              label="状态"
              value={edit.status}
              onChange={(v) => update("status", v)}
              options={[
                { value: "active", label: "启用" },
                { value: "disabled", label: "停用" },
              ]}
            />
            <Checkbox
              label="设为默认存储"
              checked={edit.is_default}
              onChange={(e) => update("is_default", e.target.checked)}
            />
            <Failure error={error} />
          </div>
        )}
      </Dialog>
      {remove !== undefined && (
        <Confirm
          title="删除存储策略"
          description="已被图片使用的策略需要保留，直到图片完成迁移或清除。"
          onClose={() => setRemove(undefined)}
          onConfirm={async () => {
            await api.deletePolicy(remove);
            data.reload();
          }}
        />
      )}
    </>
  );
}
export function Setup() {
  const status = useLoad(api.setupStatus),
    navigate = useNavigate();
  const [step, setStep] = useState(0),
    [db, setDb] = useState<Record<string, unknown>>({
      mysql_host: "127.0.0.1",
      mysql_port: "3306",
      mysql_database: "islet",
      mysql_user: "islet",
      mysql_password: "",
      redis_host: "",
      redis_port: "6379",
      redis_password: "",
    }),
    [name, setName] = useState(""),
    [password, setPassword] = useState(""),
    [site, setSite] = useState("Islet 图床"),
    [url, setUrl] = useState(location.origin),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  useEffect(() => {
    if (status.data?.status === "ready") navigate("/login", { replace: true });
    if (status.data?.defaults)
      setDb((x) => ({ ...x, ...status.data!.defaults }));
    if (status.data?.admin_verified) setStep(2);
  }, [status.data]);
  return (
    <main className="islet-standalone">
      <Card>
        <Heading
          title="欢迎使用 Islet"
          description={`第 ${step + 1} 步，共 3 步 · ${["连接数据服务", "创建管理员", "完成站点设置"][step]}`}
        />
        <Failure error={status.error} retry={status.reload} />
        <Failure error={error} />
        <form
          className="islet-form"
          onSubmit={async (e) => {
            e.preventDefault();
            setBusy(true);
            setError("");
            try {
              if (step === 0) {
                await api.setupTestDatabase(db);
                setStep(1);
              } else if (step === 1) {
                await api.setupVerifyAdmin({
                  mode: "local",
                  username: name,
                  password,
                });
                setStep(2);
              } else {
                await api.setupComplete(db, site, url);
                location.replace("/login");
              }
            } catch (e) {
              setError(errorText(e));
            } finally {
              setBusy(false);
            }
          }}
        >
          {step === 0 ? (
            [
              "mysql_host",
              "mysql_port",
              "mysql_database",
              "mysql_user",
              "mysql_password",
              "redis_host",
              "redis_port",
              "redis_password",
            ].map((key, i) => (
              <TextField
                key={key}
                label={
                  [
                    "MySQL 地址",
                    "MySQL 端口",
                    "数据库名称",
                    "数据库用户",
                    "数据库密码",
                    "Redis 地址（可选）",
                    "Redis 端口",
                    "Redis 密码（可选）",
                  ][i]
                }
                value={String(db[key] || "")}
                type={key.endsWith("password") ? "password" : "text"}
                onChange={(v) => setDb((x) => ({ ...x, [key]: v }))}
              />
            ))
          ) : step === 1 ? (
            <>
              <TextField
                label="管理员用户名"
                value={name}
                onChange={setName}
                required
              />
              <TextField
                label="管理员密码"
                type="password"
                value={password}
                onChange={setPassword}
                hint="10–72 字节"
                required
              />
            </>
          ) : (
            <>
              <TextField label="站点名称" value={site} onChange={setSite} />
              <TextField label="公开图片地址" value={url} onChange={setUrl} />
            </>
          )}
          <Button type="submit" loading={busy}>
            {step === 2 ? "完成初始化" : "继续"}
          </Button>
        </form>
      </Card>
    </main>
  );
}
