import { useEffect, useRef, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import {
  Button,
  Card,
  Dialog,
  EmptyState,
  ResourceExplorer,
  UploadQueue,
  ActionToolbar,
  type ExplorerResource,
} from "@fiveplusone/ui";
import {
  Copy,
  RefreshCw,
  Upload,
  Trash2,
  FolderInput,
  RotateCcw,
  Plus,
  Image as ImageIcon,
} from "lucide-react";
import { api, type ImageItem, type AlbumView } from "../api";
import {
  humanBytes,
  dateLabel,
  copyText,
  imageLink,
  linkFormats,
  errorText,
  groupLabel,
  type LinkFormat,
} from "../utils";
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
import { uploads, useUploads } from "./uploads";
export function Gallery({ recycled = false }: { recycled?: boolean }) {
  const { me, notify } = useSession(),
    navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const [query, setQuery] = useState(params.get("q") || "");
  const [selected, setSelected] = useState<string[]>([]),
    [detail, setDetail] = useState<ImageItem>(),
    [format, setFormat] = useState<LinkFormat>("url"),
    [move, setMove] = useState(false),
    [destination, setDestination] = useState(""),
    [confirm, setConfirm] = useState<"delete" | "purge" | null>(null),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const albums = useLoad(api.albums);
  const seq = useRef(0);
  const [items, setItems] = useState<ImageItem[]>([]),
    [more, setMore] = useState(false),
    [loading, setLoading] = useState(true);
  const scope = params.get("scope") || "mine",
    album = params.get("album") || "",
    mime = params.get("mime") || "";
  async function load(append = false) {
    const id = ++seq.current;
    setLoading(true);
    setError("");
    try {
      const p = new URLSearchParams({ limit: "40" });
      if (recycled) p.set("recycled", "1");
      if (params.get("q")) p.set("keyword", params.get("q")!);
      if (album) p.set("album_id", album);
      if (mime) p.set("mime", mime);
      if (scope === "all") p.set("scope", "all");
      const last = items.at(-1);
      if (append && last) {
        p.set("before", last.created_at);
        p.set("before_id", String(last.id));
      }
      const res = await api.images("?" + p);
      if (id !== seq.current) return;
      setItems((prev) =>
        append
          ? [
              ...prev,
              ...res.items.filter((x) => !prev.some((y) => y.id === x.id)),
            ]
          : res.items,
      );
      setMore(res.has_more);
      if (!append) setSelected([]);
    } catch (e) {
      if (id === seq.current) setError(errorText(e));
    } finally {
      if (id === seq.current) setLoading(false);
    }
  }
  useEffect(() => {
    setQuery(params.get("q") || "");
    void load();
    return () => {
      seq.current++;
    };
  }, [params.toString(), recycled]);
  function filter(key: string, v: string) {
    const p = new URLSearchParams(params);
    v ? p.set(key, v) : p.delete(key);
    setParams(p);
  }
  const chosen = items.filter((x) => selected.includes(String(x.id)));
  const allowed = chosen.every(
    (x) => me.user.role === "super_admin" || x.user_id === me.user.id,
  );
  const canMove = chosen.every((x) => x.user_id === me.user.id);
  async function batch(fn: (i: ImageItem) => Promise<unknown>) {
    if (busy) return;
    setBusy(true);
    let done = 0;
    const errors: string[] = [];
    try {
      for (const i of chosen) {
        try {
          await fn(i);
          done++;
        } catch (e) {
          errors.push(`${i.original_name}：${errorText(e)}`);
        }
      }
      await load();
      changed();
      notify(`已完成 ${done} 项`);
      if (errors.length) setError(errors.join("；"));
    } finally {
      setBusy(false);
    }
  }
  async function copy(list: ImageItem[]) {
    try {
      await copyText(list.map((x) => imageLink(x, format)).join("\n"));
      notify(`已复制 ${list.length} 个链接`);
    } catch (e) {
      setError(errorText(e));
    }
  }
  const action = (name: string) => {
    if (name === "share") void copy(chosen);
    if (name === "rename" && chosen[0]) setDetail(chosen[0]);
    if (name === "move") setMove(true);
    if (name === "delete") setConfirm(recycled ? "purge" : "delete");
    if (name === "restore") void batch((i) => api.restoreImage(i.id));
  };
  return (
    <>
      <Heading
        title={recycled ? "回收站" : "图片库"}
        description={
          recycled
            ? "找回最近移入回收站的图片，或永久删除不再需要的内容。"
            : "把值得收藏的画面整理在一起，随时分享。"
        }
        actions={
          <Button onClick={() => navigate("/upload")}>
            <Upload size={18} />
            上传图片
          </Button>
        }
      />
      <Card>
        <form
          className="islet-filters"
          onSubmit={(e) => {
            e.preventDefault();
            filter("q", query);
          }}
        >
          <TextField label="搜索图片" value={query} onChange={setQuery} />
          <Button type="submit" variant="secondary">
            搜索
          </Button>
          <Choice
            label="相册"
            value={album}
            onChange={(v) => filter("album", v)}
            options={[
              { value: "", label: "全部相册" },
              ...(albums.data?.items || []).map((x) => ({
                value: String(x.id),
                label: x.name,
              })),
            ]}
          />
          <Choice
            label="格式"
            value={mime}
            onChange={(v) => filter("mime", v)}
            options={[
              { value: "", label: "全部格式" },
              ...["jpeg", "png", "gif", "webp", "avif"].map((x) => ({
                value: "image/" + x,
                label: x.toUpperCase(),
              })),
            ]}
          />
          {me.user.role !== "user" && (
            <Choice
              label="查看范围"
              value={scope}
              onChange={(v) => filter("scope", v)}
              options={[
                { value: "mine", label: "我的图片" },
                { value: "all", label: "全站图片" },
              ]}
            />
          )}
          <Choice
            label="复制格式"
            value={format}
            onChange={(v) => setFormat(v as LinkFormat)}
            options={linkFormats}
          />
        </form>
        <Failure error={error} retry={() => void load()} />
        <div className="islet-library-actions">
          <span>
            {selected.length
              ? `已选择 ${selected.length} 张`
              : `已加载 ${items.length} 张`}
          </span>
          <ActionToolbar
            label="图片操作"
            actions={[
              {
                id: "refresh",
                label: "刷新图片",
                icon: <RefreshCw size={18} />,
                onSelect: () => void load(),
                disabled: busy,
              },
              {
                id: "copy",
                label: "复制所选链接",
                icon: <Copy size={18} />,
                onSelect: () => void copy(chosen),
                disabled: !chosen.length,
              },
              {
                id: "move",
                label: "移动到相册",
                icon: <FolderInput size={18} />,
                onSelect: () => setMove(true),
                disabled: !chosen.length || !canMove || busy || recycled,
              },
              {
                id: "delete",
                label: recycled ? "永久删除" : "移入回收站",
                icon: <Trash2 size={18} />,
                onSelect: () => setConfirm(recycled ? "purge" : "delete"),
                danger: true,
                disabled: !chosen.length || !allowed || busy,
              },
              ...(recycled
                ? [
                    {
                      id: "restore",
                      label: "恢复图片",
                      icon: <RotateCcw size={18} />,
                      onSelect: () => void batch((i) => api.restoreImage(i.id)),
                      disabled: !chosen.length || !allowed || busy,
                    },
                  ]
                : []),
            ]}
          />
        </div>
        <div className="islet-explorer">
          <ResourceExplorer
            media
            defaultView="grid"
            showSearch={false}
            items={items.map((x) => ({
              id: String(x.id),
              kind: "file",
              name: x.original_name,
              size: x.size,
              modifiedAt: x.created_at,
              thumbnailUrl: x.thumb_url || x.url,
            }))}
            query=""
            onQueryChange={() => {}}
            folderId={null}
            onFolderChange={() => {}}
            selectedIds={selected}
            onSelectionChange={setSelected}
            onOpen={(x) => setDetail(items.find((i) => String(i.id) === x.id))}
            availableActions={[]}
            loading={loading && !items.length}
            error={error}
          />
        </div>
        {more && (
          <Button
            variant="secondary"
            loading={loading}
            onClick={() => void load(true)}
          >
            加载更多
          </Button>
        )}
      </Card>
      {detail && (
        <ImageDetails
          item={detail}
          editable={
            me.user.role === "super_admin" || detail.user_id === me.user.id
          }
          albums={albums.data?.items || []}
          format={format}
          recycled={recycled}
          onClose={() => setDetail(undefined)}
          onSaved={() => {
            void load();
            changed();
          }}
        />
      )}
      <Dialog
        open={move}
        title="移动到相册"
        onClose={() => !busy && setMove(false)}
        footer={
          <Button
            loading={busy}
            onClick={async () => {
              await batch((i) =>
                api.updateImage(i.id, { album_id: Number(destination) || 0 }),
              );
              setMove(false);
            }}
          >
            移动图片
          </Button>
        }
      >
        <Choice
          label="目标相册"
          value={destination}
          onChange={setDestination}
          options={[
            { value: "", label: "未分类" },
            ...(albums.data?.items || []).map((x) => ({
              value: String(x.id),
              label: x.name,
            })),
          ]}
        />
      </Dialog>
      {confirm && (
        <Confirm
          title={confirm === "purge" ? "永久删除" : "移入回收站"}
          description={
            confirm === "purge"
              ? `永久删除所选 ${chosen.length} 张图片，删除后无法恢复。`
              : `将所选 ${chosen.length} 张图片移入回收站，之后可以恢复。浏览器或 CDN 中的公开缓存可能仍可访问。`
          }
          onClose={() => setConfirm(null)}
          onConfirm={() =>
            batch((i) =>
              confirm === "purge"
                ? api.purgeImage(i.id)
                : api.recycleImage(i.id),
            )
          }
        />
      )}
    </>
  );
}
function ImageDetails({
  item,
  editable,
  albums,
  format,
  recycled,
  onClose,
  onSaved,
}: {
  item: ImageItem;
  editable: boolean;
  albums: AlbumView[];
  format: LinkFormat;
  recycled: boolean;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [name, setName] = useState(item.original_name),
    [album, setAlbum] = useState(String(item.album_id || "")),
    [tags, setTags] = useState(item.tags.join(", ")),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const { notify, me } = useSession();
  return (
    <Dialog
      drawer
      open
      title="图片详情"
      onClose={() => !busy && onClose()}
      footer={
        <>
          <Button
            variant="secondary"
            onClick={async () => {
              try {
                await copyText(imageLink(item, format));
                notify("链接已复制");
              } catch (e) {
                setError(errorText(e));
              }
            }}
          >
            复制链接
          </Button>
          {editable && !recycled && (
            <Button
              loading={busy}
              onClick={async () => {
                if (!name.trim()) {
                  setError("请输入图片名称");
                  return;
                }
                setBusy(true);
                try {
                  await api.updateImage(item.id, {
                    name: name.trim(),
                    album_id: Number(album) || 0,
                    tags: tags
                      .split(/[,，]/)
                      .map((x) => x.trim())
                      .filter(Boolean),
                  });
                  notify("图片信息已保存");
                  onSaved();
                  onClose();
                } catch (e) {
                  setError(errorText(e));
                } finally {
                  setBusy(false);
                }
              }}
            >
              保存更改
            </Button>
          )}
        </>
      }
    >
      <img className="islet-preview" src={item.url} alt={item.original_name} />
      <p className="fp-muted">
        {humanBytes(item.size)} · {item.width} × {item.height} ·{" "}
        {dateLabel(item.created_at)}
      </p>
      <Failure error={error} />
      {editable && !recycled ? (
        <div className="islet-form">
          <TextField label="图片名称" value={name} onChange={setName} />
          <Choice
            label="保存到相册"
            value={album}
            onChange={setAlbum}
            options={[
              { value: "", label: "未分类" },
              ...albums
                .filter(() => item.user_id === me.user.id)
                .map((x) => ({ value: String(x.id), label: x.name })),
            ]}
          />
          <TextField
            label="标签"
            value={tags}
            onChange={setTags}
            hint="多个标签用逗号分隔"
          />
        </div>
      ) : (
        <h3>{item.original_name}</h3>
      )}
    </Dialog>
  );
}
export function UploadPage() {
  const { me, notify } = useSession(),
    navigate = useNavigate(),
    tasks = useUploads(),
    albums = useLoad(api.albums);
  const input = useRef<HTMLInputElement>(null);
  const [album, setAlbum] = useState(""),
    [policy, setPolicy] = useState(""),
    [tags, setTags] = useState(""),
    [format, setFormat] = useState<LinkFormat>("url"),
    [error, setError] = useState("");
  const max = Math.min(
    me.upload_max_bytes,
    me.group.max_file_bytes || me.upload_max_bytes,
  );
  const add = (files: File[]) =>
    uploads.add(files, {
      album,
      policy,
      tags,
      max,
      allowed: me.group.allowed_mimes || [],
    });
  useEffect(() => {
    const paste = (e: ClipboardEvent) => {
      if (
        (e.target as HTMLElement)?.closest("input,textarea,[contenteditable]")
      )
        return;
      const files = Array.from(e.clipboardData?.files || []);
      if (files.length) {
        e.preventDefault();
        add(files);
      }
    };
    window.addEventListener("paste", paste);
    return () => window.removeEventListener("paste", paste);
  }, [album, policy, tags, max]);
  return (
    <>
      <Heading
        title="上传图片"
        description="选择、拖拽或粘贴图片，上传后即可分享。"
        actions={
          <Button variant="secondary" onClick={() => navigate("/images")}>
            查看图片库
          </Button>
        }
      />
      <div className="islet-upload-layout">
        <div>
          <Card>
            <div
              className="islet-drop"
              onDragOver={(e) => e.preventDefault()}
              onDrop={(e) => {
                e.preventDefault();
                add(Array.from(e.dataTransfer.files));
              }}
            >
              <Upload size={38} />
              <h2>将图片拖到这里</h2>
              <p>也可以点击选择，或直接粘贴图片</p>
              <Button onClick={() => input.current?.click()}>选择图片</Button>
              <input
                hidden
                type="file"
                multiple
                accept="image/*"
                ref={input}
                onChange={(e) => {
                  add(Array.from(e.target.files || []));
                  e.target.value = "";
                }}
              />
              <small>单张最大 {humanBytes(max)}</small>
            </div>
          </Card>
          <Card>
            <div className="islet-library-actions">
              <h2>上传队列</h2>
              <Button variant="ghost" onClick={() => uploads.clear()}>
                清除已完成
              </Button>
            </div>
            <UploadQueue
              tasks={tasks}
              onRetry={uploads.retry}
              onCancel={uploads.remove}
              onDismiss={uploads.remove}
            />
            <Failure error={error} />
            {tasks.some((x) => x.result) && (
              <Button
                onClick={async () => {
                  try {
                    await copyText(
                      tasks
                        .filter((x) => x.result)
                        .map((x) => imageLink(x.result!, format))
                        .join("\n"),
                    );
                    notify("链接已复制");
                  } catch (e) {
                    setError(errorText(e));
                  }
                }}
              >
                <Copy size={17} />
                复制本次上传链接
              </Button>
            )}
          </Card>
        </div>
        <Card>
          <h2>上传设置</h2>
          <div className="islet-form">
            <Choice
              label="保存到相册"
              value={album}
              onChange={setAlbum}
              options={[
                { value: "", label: "未分类" },
                ...(albums.data?.items || []).map((x) => ({
                  value: String(x.id),
                  label: x.name,
                })),
              ]}
            />
            <Choice
              label="存储位置"
              value={policy}
              onChange={setPolicy}
              options={[
                { value: "", label: "默认存储" },
                ...me.policies.map((x) => ({
                  value: String(x.id),
                  label: x.name,
                })),
              ]}
            />
            <TextField label="标签" value={tags} onChange={setTags} />
            <Choice
              label="复制链接格式"
              value={format}
              onChange={(v) => setFormat(v as LinkFormat)}
              options={linkFormats}
            />
            <p className="fp-muted">
              已使用 {humanBytes(me.quota.used)} · {groupLabel(me.group.name)}
            </p>
            <Failure error={albums.error} retry={albums.reload} />
          </div>
        </Card>
      </div>
    </>
  );
}
export function Albums() {
  const data = useLoad(api.albums),
    navigate = useNavigate(),
    { notify } = useSession();
  const [edit, setEdit] = useState<Partial<AlbumView> | null>(null),
    [name, setName] = useState(""),
    [description, setDescription] = useState(""),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [remove, setRemove] = useState<AlbumView>();
  function open(x: Partial<AlbumView>) {
    setEdit(x);
    setName(x.name || "");
    setDescription(x.description || "");
    setError("");
  }
  return (
    <>
      <Heading
        title="相册"
        description="按主题整理图片，让每次寻找都更轻松。"
        actions={
          <Button onClick={() => open({})}>
            <Plus size={18} />
            新建相册
          </Button>
        }
      />
      <Failure error={data.error} retry={data.reload} />
      {data.loading ? (
        <Loading />
      ) : !data.data?.items.length ? (
        <EmptyState
          title="创建你的第一本相册"
          description="把旅行、作品和日常灵感分开收藏。"
          action={<Button onClick={() => open({})}>新建相册</Button>}
        />
      ) : (
        <div className="islet-card-grid">
          {data.data.items.map((x) => (
            <Card key={x.id}>
              <Button
                variant="ghost"
                onClick={() => navigate("/images?album=" + x.id)}
              >
                <ImageIcon size={28} />
                <h2>{x.name}</h2>
              </Button>
              <p>{x.description || "留住每一份灵感"}</p>
              <small>{x.image_count || 0} 张图片</small>
              <div className="islet-row">
                <Button variant="secondary" onClick={() => open(x)}>
                  编辑
                </Button>
                <Button variant="ghost" onClick={() => setRemove(x)}>
                  删除相册
                </Button>
              </div>
            </Card>
          ))}
        </div>
      )}
      <Dialog
        open={!!edit}
        title={edit?.id ? "编辑相册" : "新建相册"}
        onClose={() => !busy && setEdit(null)}
        footer={
          <Button
            loading={busy}
            onClick={async () => {
              if (!name.trim()) {
                setError("请输入相册名称");
                return;
              }
              setBusy(true);
              try {
                edit?.id
                  ? await api.updateAlbum(edit.id, name.trim(), description)
                  : await api.createAlbum(name.trim(), description);
                setEdit(null);
                data.reload();
                notify("相册已保存");
              } catch (e) {
                setError(errorText(e));
              } finally {
                setBusy(false);
              }
            }}
          >
            {edit?.id ? "保存相册" : "创建相册"}
          </Button>
        }
      >
        <div className="islet-form">
          <TextField label="相册名称" value={name} onChange={setName} />
          <TextField
            label="描述"
            value={description}
            onChange={setDescription}
          />
          <Failure error={error} />
        </div>
      </Dialog>
      {remove && (
        <Confirm
          title="删除相册"
          description={`删除「${remove.name}」。含有图片的相册需先移动图片。`}
          onClose={() => setRemove(undefined)}
          onConfirm={async () => {
            await api.deleteAlbum(remove.id);
            data.reload();
          }}
        />
      )}
    </>
  );
}
export function Tokens() {
  const data = useLoad(api.tokens),
    { me, notify } = useSession();
  const [open, setOpen] = useState(false),
    [name, setName] = useState(""),
    [expiry, setExpiry] = useState(""),
    [secret, setSecret] = useState(""),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [remove, setRemove] = useState<number>();
  return (
    <>
      <Heading
        title="上传密钥"
        description="为 PicGo、PicList 和自动化工具创建独立上传凭证。"
        actions={
          <Button
            onClick={() => {
              setName("");
              setExpiry("");
              setError("");
              setOpen(true);
            }}
          >
            创建密钥
          </Button>
        }
      />
      <Failure error={data.error} retry={data.reload} />
      <div className="islet-card-grid">
        {data.data?.items.map((x) => (
          <Card key={x.id}>
            <h2>{x.name}</h2>
            <p>{x.status === "active" ? "使用中" : "已撤销"}</p>
            <p className="fp-muted">
              创建于 {dateLabel(x.created_at)}
              <br />
              到期：{x.expires_at ? dateLabel(x.expires_at) : "长期有效"}
            </p>
            {x.status === "active" && (
              <Button variant="secondary" onClick={() => setRemove(x.id)}>
                撤销密钥
              </Button>
            )}
          </Card>
        ))}
      </div>
      {data.loading && <Loading />}
      <Card>
        <h2>连接 PicGo / PicList</h2>
        <p>
          选择自定义上传，使用 POST 与 multipart/form-data，文件字段填写 files。
        </p>
        <TextField
          label="上传地址"
          value={
            (me.public_base_url || location.origin) + "/api/v1/upload?picgo=1"
          }
          onChange={() => {}}
        />
        <p>请求头 Authorization 填写 Bearer 加一个空格，再接你的上传密钥。</p>
      </Card>
      <Dialog
        open={open}
        title="创建上传密钥"
        onClose={() => !busy && setOpen(false)}
        footer={
          <Button
            loading={busy}
            onClick={async () => {
              if (!name.trim()) {
                setError("请输入名称");
                return;
              }
              if (expiry && new Date(expiry) <= new Date()) {
                setError("请选择未来的到期时间");
                return;
              }
              setBusy(true);
              try {
                const r = await api.createToken(
                  name.trim(),
                  expiry ? new Date(expiry).toISOString() : undefined,
                );
                setSecret(r.token);
                setOpen(false);
                data.reload();
              } catch (e) {
                setError(errorText(e));
              } finally {
                setBusy(false);
              }
            }}
          >
            创建密钥
          </Button>
        }
      >
        <div className="islet-form">
          <TextField label="密钥名称" value={name} onChange={setName} />
          <TextField
            label="到期时间（可选）"
            type="datetime-local"
            value={expiry}
            onChange={setExpiry}
          />
          <Failure error={error} />
        </div>
      </Dialog>
      <Dialog
        open={!!secret}
        title="保存你的上传密钥"
        onClose={() => setSecret("")}
        footer={
          <Button
            onClick={async () => {
              try {
                await copyText(secret);
                notify("密钥已复制");
              } catch (e) {
                setError(errorText(e));
              }
            }}
          >
            复制密钥
          </Button>
        }
      >
        <p>密钥只展示这一次，请妥善保存。任何持有者都可以使用你的上传权限。</p>
        <pre className="islet-secret">{secret}</pre>
        <Failure error={error} />
      </Dialog>
      {remove !== undefined && (
        <Confirm
          title="撤销密钥"
          description="使用此密钥的工具将无法继续上传。"
          onClose={() => setRemove(undefined)}
          onConfirm={async () => {
            await api.revokeToken(remove);
            data.reload();
          }}
        />
      )}
    </>
  );
}
export function Profile() {
  const { me } = useSession();
  const [current, setCurrent] = useState(""),
    [password, setPassword] = useState(""),
    [repeat, setRepeat] = useState(""),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const navigate = useNavigate();
  return (
    <>
      <Heading title="我的账户" description="管理账户信息与存储空间。" />
      <Card>
        <h2>{me.user.username}</h2>
        <p>{me.user.email}</p>
        <p>
          {groupLabel(me.group.name)} ·{" "}
          {me.user.auth_source === "local" ? "本地账户" : "五加一账户"}
        </p>
        <h3>已使用 {humanBytes(me.quota.used)}</h3>
        <p>总容量：{me.quota.limit ? humanBytes(me.quota.limit) : "不限"}</p>
      </Card>
      {me.user.auth_source === "local" && (
        <Card>
          <h2>修改密码</h2>
          <form
            className="islet-form islet-form-narrow"
            onSubmit={async (e) => {
              e.preventDefault();
              if (password !== repeat) {
                setError("两次输入的密码不一致");
                return;
              }
              setBusy(true);
              try {
                await api.changePassword(current, password);
                navigate("/login", { replace: true });
              } catch (e) {
                setError(errorText(e));
              } finally {
                setBusy(false);
              }
            }}
          >
            <TextField
              label="当前密码"
              type="password"
              value={current}
              onChange={setCurrent}
              required
            />
            <TextField
              label="新密码"
              type="password"
              value={password}
              onChange={setPassword}
              hint="10–72 字节"
              required
            />
            <TextField
              label="确认新密码"
              type="password"
              value={repeat}
              onChange={setRepeat}
              required
            />
            <Failure error={error} />
            <Button type="submit" loading={busy}>
              修改密码并重新登录
            </Button>
          </form>
        </Card>
      )}
    </>
  );
}
