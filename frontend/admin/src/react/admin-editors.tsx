import { useEffect, useRef, useState, type ReactNode } from "react";
import {
  useBeforeUnload,
  useBlocker,
  useNavigate,
  useParams,
} from "react-router-dom";
import {
  Button,
  Checkbox,
  Dialog,
  ManagementForm,
  WorkspaceField,
  WorkspaceSection,
} from "@fiveplusone/ui";
import { ArrowLeft } from "lucide-react";
import { api, type GroupView, type PolicyInput } from "../api";
import { errorText, groupLabel } from "../utils";
import { Failure, Loading, useLoad, useSession } from "./common";

const statusOptions = [
  { value: "active", label: "启用" },
  { value: "disabled", label: "停用" },
];

function EditorFrame({
  title,
  description,
  destination,
  tabs,
  section,
  setSection,
  dirty,
  busy,
  error,
  save,
  children,
}: {
  title: string;
  description: string;
  destination: string;
  tabs: { value: string; label: string }[];
  section: string;
  setSection: (s: string) => void;
  dirty: boolean;
  busy: boolean;
  error: string;
  save: () => Promise<boolean>;
  children: ReactNode;
}) {
  const navigate = useNavigate(),
    saved = useRef(false),
    submitting = useRef(false);
  const blocker = useBlocker(
    ({ nextLocation }) =>
      !saved.current && (dirty || busy) && nextLocation.pathname !== "/login",
  );
  useBeforeUnload((event) => {
    if (!saved.current && (dirty || busy)) event.preventDefault();
  });
  useEffect(() => {
    if (busy && blocker.state === "blocked") blocker.reset();
  }, [busy, blocker]);
  const feedback = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (error) feedback.current?.focus();
  }, [error, section]);
  return (
    <>
      <div>
        <Button
          variant="ghost"
          disabled={busy}
          onClick={() => navigate(destination)}
        >
          <ArrowLeft size={17} />
          返回{destination === "/groups" ? "用户分组" : "存储策略"}
        </Button>
      </div>
      <form
        className="islet-admin-editor"
        noValidate
        onSubmit={async (event) => {
          event.preventDefault();
          if (submitting.current) return;
          submitting.current = true;
          try {
            if (await save()) {
              saved.current = true;
              navigate(destination);
            }
          } finally {
            submitting.current = false;
          }
        }}
      >
        <ManagementForm
          title={title}
          description={description}
          items={tabs}
          value={section}
          onChange={(value) => {
            if (!busy) setSection(value);
          }}
          status={busy ? "正在保存…" : dirty ? "有未保存的修改" : "设置已载入"}
          actions={
            <>
              <Button
                variant="secondary"
                disabled={busy}
                onClick={() => navigate(destination)}
              >
                取消
              </Button>
              <Button type="submit" loading={busy}>
                保存{destination === "/groups" ? "分组" : "策略"}
              </Button>
            </>
          }
        >
          <div ref={feedback} tabIndex={-1}>
            <Failure error={error} />
          </div>
          <fieldset disabled={busy}>{children}</fieldset>
        </ManagementForm>
      </form>
      <Dialog
        open={blocker.state === "blocked" && !busy}
        title="放弃未保存的修改？"
        onClose={() => blocker.state === "blocked" && blocker.reset()}
        footer={
          <>
            <Button
              variant="secondary"
              onClick={() => blocker.state === "blocked" && blocker.reset()}
            >
              继续编辑
            </Button>
            <Button
              variant="danger"
              onClick={() => blocker.state === "blocked" && blocker.proceed()}
            >
              放弃更改
            </Button>
          </>
        }
      >
        <p>离开后，本次修改不会保存。</p>
      </Dialog>
    </>
  );
}

export function GroupEditor() {
  const { id } = useParams(),
    data = useLoad(api.groups, [id]),
    policies = useLoad(api.policies);
  if (data.loading || policies.loading) return <Loading />;
  if (data.error || policies.error)
    return (
      <Failure
        error={data.error || policies.error}
        retry={() => {
          data.reload();
          policies.reload();
        }}
      />
    );
  const group = id
    ? data.data?.items.find((item) => String(item.id) === id)
    : undefined;
  if (id && !group) return <EditorMissing destination="/groups" name="分组" />;
  return (
    <GroupDraft
      key={id || "new"}
      group={group}
      policies={policies.data?.items || []}
    />
  );
}
function EditorMissing({
  destination,
  name,
}: {
  destination: string;
  name: string;
}) {
  const navigate = useNavigate();
  return (
    <WorkspaceSection
      title={`${name}不存在`}
      description={`请返回列表重新选择${name}。`}
    >
      <Button variant="secondary" onClick={() => navigate(destination)}>
        返回列表
      </Button>
    </WorkspaceSection>
  );
}
function GroupDraft({
  group,
  policies,
}: {
  group?: GroupView;
  policies: { id: number; name: string; status: string }[];
}) {
  const initial = useRef({
    name: group?.name || "",
    description: group?.description || "",
    status: group?.status || "active",
    quota: String(group ? group.quota_bytes / 1048576 : 1024),
    max: String(group ? group.max_file_bytes / 1048576 : 10),
    hourly: String(group?.uploads_per_hour ?? 100),
    mimes: group
      ? (group.allowed_mimes ?? []).join(", ")
      : "image/jpeg, image/png, image/webp, image/gif",
    policy_ids: [...(group?.policy_ids ?? [])],
    allPolicies: !group?.policy_ids?.length,
  }).current;
  const [draft, setDraft] = useState(initial),
    [section, setSection] = useState("basic"),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const { notify } = useSession();
  const update = <K extends keyof typeof draft>(
    key: K,
    value: (typeof draft)[K],
  ) => setDraft((current) => ({ ...current, [key]: value }));
  async function save() {
    setError("");
    if (!draft.name.trim() || draft.name.trim().length > 64) {
      setSection("basic");
      setError("请输入 1–64 字的分组名称");
      return false;
    }
    const quota = Number(draft.quota) * 1048576,
      max = Number(draft.max) * 1048576,
      hourly = Number(draft.hourly);
    if (
      [draft.quota, draft.max, draft.hourly].some((v) => !v.trim()) ||
      [quota, max, hourly].some((v) => !Number.isSafeInteger(v) || v < 0)
    ) {
      setSection("limits");
      setError("容量和上传数量须为有效的非负数，上传数量须为整数");
      return false;
    }
    const mimes = draft.mimes
      .split(/[,，]/)
      .map((s) => s.trim())
      .filter(Boolean);
    if (mimes.some((mime) => !/^image\/[a-zA-Z0-9.+*-]+$/.test(mime))) {
      setSection("limits");
      setError("图片格式请使用 image/jpeg 等格式，多个用逗号分隔");
      return false;
    }
    if (!draft.allPolicies && !draft.policy_ids.length) {
      setSection("storage");
      setError("请至少选择一个存储，或选择使用全部存储");
      return false;
    }
    setBusy(true);
    try {
      await api.saveGroup({
        id: group?.id,
        name: draft.name.trim(),
        description: draft.description.trim(),
        status: draft.status,
        quota_bytes: quota,
        max_file_bytes: max,
        uploads_per_hour: hourly,
        allowed_mimes: mimes,
        policy_ids: draft.allPolicies ? [] : draft.policy_ids,
      });
      notify("分组已保存");
      return true;
    } catch (e) {
      setError(errorText(e));
      return false;
    } finally {
      setBusy(false);
    }
  }
  return (
    <EditorFrame
      title={group ? `编辑分组 · ${groupLabel(group.name)}` : "新建分组"}
      description="设置成员的容量、上传规则与可用存储。"
      destination="/groups"
      tabs={[
        { value: "basic", label: "基本信息" },
        { value: "limits", label: "容量与上传" },
        { value: "storage", label: "可用存储" },
      ]}
      section={section}
      setSection={setSection}
      dirty={JSON.stringify(draft) !== JSON.stringify(initial)}
      busy={busy}
      error={error}
      save={save}
    >
      {section === "basic" && (
        <WorkspaceSection
          title="基本信息"
          description="用清楚的名称区分不同成员的使用规则。"
        >
          <>
            <WorkspaceField
              label="分组名称"
              value={draft.name}
              onChange={(v) => update("name", v)}
              required
            />
            <WorkspaceField
              label="描述"
              value={draft.description}
              onChange={(v) => update("description", v)}
              multiline
            />
            <WorkspaceField
              label="状态"
              value={draft.status}
              onChange={(v) => update("status", v)}
              options={statusOptions}
            />
          </>
        </WorkspaceSection>
      )}
      {section === "limits" && (
        <WorkspaceSection
          title="容量与上传"
          description="以下数值填写 0 表示不限，仍受站点允许的上传大小约束。"
        >
          <>
            <div className="fp-workspace-grid">
              <WorkspaceField
                label="每位成员容量（MB）"
                numeric
                value={draft.quota}
                onChange={(v) => update("quota", v)}
              />
              <WorkspaceField
                label="单张图片上限（MB）"
                numeric
                value={draft.max}
                onChange={(v) => update("max", v)}
              />
              <WorkspaceField
                label="每小时上传数量"
                numeric
                value={draft.hourly}
                onChange={(v) => update("hourly", v)}
              />
            </div>
            <WorkspaceField
              label="允许的图片格式"
              value={draft.mimes}
              onChange={(v) => update("mimes", v)}
              hint="例如 image/jpeg, image/png；留空允许全部受支持的图片格式。"
            />
          </>
        </WorkspaceSection>
      )}
      {section === "storage" && (
        <WorkspaceSection
          title="可用存储"
          description="成员上传图片时可以选择这里允许的存储。"
        >
          <>
            <Checkbox
              label="使用全部存储"
              checked={draft.allPolicies}
              onChange={(event) => update("allPolicies", event.target.checked)}
            />
            {draft.allPolicies ? (
              <p className="fp-muted">
                允许使用全部可用存储，包括以后新增的存储。
              </p>
            ) : (
              <>
                <p className="fp-muted">
                  选择此分组可以使用的存储，至少选择一项。
                </p>
                {policies.map((policy) => (
                  <Checkbox
                    key={policy.id}
                    label={
                      policy.name +
                      (policy.status === "active" ? "" : "（已停用）")
                    }
                    checked={draft.policy_ids.includes(policy.id)}
                    onChange={(event) =>
                      update(
                        "policy_ids",
                        event.target.checked
                          ? [...draft.policy_ids, policy.id]
                          : draft.policy_ids.filter(
                              (value) => value !== policy.id,
                            ),
                      )
                    }
                  />
                ))}
              </>
            )}
          </>
        </WorkspaceSection>
      )}
    </EditorFrame>
  );
}

const blankPolicy: PolicyInput = {
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
  cdn_refresh: false,
  access_key: "",
  secret_key: "",
  path_style: true,
};
export function PolicyEditor() {
  const { id } = useParams(),
    data = useLoad(api.policies, [id]);
  if (data.loading) return <Loading />;
  if (data.error) return <Failure error={data.error} retry={data.reload} />;
  const policy = id
    ? data.data?.items.find((item) => String(item.id) === id)
    : undefined;
  if (id && !policy)
    return <EditorMissing destination="/policies" name="存储策略" />;
  return (
    <PolicyDraft
      key={id || "new"}
      initial={{ ...blankPolicy, ...policy, secret_key: "" }}
      secretConfigured={!!policy?.secret_configured}
    />
  );
}
function PolicyDraft({
  initial: source,
  secretConfigured,
}: {
  initial: PolicyInput;
  secretConfigured: boolean;
}) {
  const initial = useRef(source).current,
    [draft, setDraft] = useState(initial),
    [section, setSection] = useState("basic"),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const { notify } = useSession();
  const update = <K extends keyof PolicyInput>(key: K, value: PolicyInput[K]) =>
    setDraft((current) => ({ ...current, [key]: value }));
  async function save() {
    setError("");
    if (!draft.name.trim() || draft.name.trim().length > 64) {
      setSection("basic");
      setError("请输入 1–64 字的策略名称");
      return false;
    }
    if (
      draft.driver === "s3" &&
      (!draft.endpoint.trim() ||
        !draft.bucket.trim() ||
        !draft.access_key.trim() ||
        (!draft.secret_key && !secretConfigured))
    ) {
      setSection("connection");
      setError("请填写服务地址、存储桶和访问密钥");
      return false;
    }
    if (
      draft.driver === "s3" &&
      draft.cdn_refresh &&
      !/^https:\/\/[^/?#:@]+\/?$/.test(draft.cdn.trim())
    ) {
      setSection("delivery");
      setError("请填写完整的 HTTPS 加速域名，例如 https://img.example.com");
      return false;
    }
    setBusy(true);
    try {
      await api.savePolicy({ ...draft, name: draft.name.trim() });
      notify("存储策略已保存");
      return true;
    } catch (e) {
      setError(errorText(e));
      return false;
    } finally {
      setBusy(false);
    }
  }
  return (
    <EditorFrame
      title={initial.id ? `编辑存储策略 · ${initial.name}` : "新建存储策略"}
      description="连接存储位置，设置图片的保存与访问方式。"
      destination="/policies"
      tabs={[
        { value: "basic", label: "基本信息" },
        { value: "connection", label: "存储连接" },
        { value: "delivery", label: "图片访问" },
      ]}
      section={section}
      setSection={setSection}
      dirty={JSON.stringify(draft) !== JSON.stringify(initial)}
      busy={busy}
      error={error}
      save={save}
    >
      {section === "basic" && (
        <WorkspaceSection
          title="基本信息"
          description="默认存储会用于未指定位置的新上传。"
        >
          <>
            <WorkspaceField
              label="策略名称"
              value={draft.name}
              onChange={(v) => update("name", v)}
              required
            />
            <div className="fp-workspace-grid">
              <WorkspaceField
                label="存储类型"
                value={draft.driver}
                onChange={(v) => update("driver", v)}
                options={[
                  { value: "local", label: "本地磁盘" },
                  { value: "s3", label: "S3 / 阿里云 OSS" },
                ]}
              />
              <WorkspaceField
                label="状态"
                value={draft.status}
                onChange={(v) => update("status", v)}
                options={statusOptions}
              />
            </div>
            <Checkbox
              label="设为默认存储"
              checked={draft.is_default}
              onChange={(event) => update("is_default", event.target.checked)}
            />
          </>
        </WorkspaceSection>
      )}
      {section === "connection" && (
        <WorkspaceSection
          title="存储连接"
          description={
            draft.driver === "local"
              ? "图片保存在服务器本地，由站点统一提供访问。"
              : "填写对象存储提供商给出的连接信息。"
          }
        >
          {draft.driver === "local" ? (
            <p>本地存储由部署时配置，无需填写连接信息。</p>
          ) : (
            <>
              <WorkspaceField
                label="服务地址"
                value={draft.endpoint}
                onChange={(v) => update("endpoint", v)}
                hint="例如 https://oss-cn-shanghai.aliyuncs.com"
              />
              <div className="fp-workspace-grid">
                <WorkspaceField
                  label="区域"
                  value={draft.region}
                  onChange={(v) => update("region", v)}
                />
                <WorkspaceField
                  label="存储桶"
                  value={draft.bucket}
                  onChange={(v) => update("bucket", v)}
                />
              </div>
              <WorkspaceField
                label="路径前缀"
                value={draft.prefix}
                onChange={(v) => update("prefix", v)}
                hint="可选，例如 images/"
              />
              <div className="fp-workspace-grid">
                <WorkspaceField
                  label="访问密钥 ID"
                  value={draft.access_key}
                  onChange={(v) => update("access_key", v)}
                />
                <WorkspaceField
                  label="访问密钥"
                  secret
                  value={draft.secret_key}
                  onChange={(v) => update("secret_key", v)}
                  hint={
                    secretConfigured
                      ? "已保存密钥，留空继续使用。"
                      : "填写存储提供商的访问密钥。"
                  }
                />
              </div>
              <Checkbox
                label="使用路径风格访问"
                checked={draft.path_style}
                onChange={(event) => update("path_style", event.target.checked)}
              />
            </>
          )}
        </WorkspaceSection>
      )}
      {section === "delivery" && (
        <WorkspaceSection
          title="图片访问"
          description="设置新上传图片的命名和分享方式。"
        >
          <>
            <WorkspaceField
              label="命名方式"
              value={draft.naming}
              onChange={(v) => update("naming", v)}
              options={[
                { value: "date", label: "日期目录" },
                { value: "random", label: "随机名称" },
                { value: "keep", label: "保留文件名" },
              ]}
            />
            {draft.driver === "s3" && (
              <>
                <WorkspaceField
                  label="图片加速地址"
                  value={draft.cdn}
                  onChange={(v) => update("cdn", v)}
                  hint="可选，例如 https://img.example.com"
                />
                <Checkbox
                  label="启用阿里云 CDN 缓存同步"
                  checked={!!draft.cdn_refresh}
                  onChange={(event) =>
                    update("cdn_refresh", event.target.checked)
                  }
                />
                <p className="fp-muted">
                  开启后分享链接默认使用加速域名。加速域名须回源至本站；密钥须有该域名的缓存刷新与查询权限，审核、恢复和回收图片后会自动同步。
                </p>
                <p className="fp-muted">
                  对象存储请设为私有读取，让图片访问继续遵循本站的审核规则。
                </p>
              </>
            )}
          </>
        </WorkspaceSection>
      )}
    </EditorFrame>
  );
}
