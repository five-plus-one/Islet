import { useEffect, useRef, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import {
  Button,
  Card,
  Dialog,
  EmptyState,
  ActionToolbar,
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
  ShieldBan,
  ShieldCheck,
} from "lucide-react";
import { api, getAccessToken, type ImageItem, type AlbumView } from "../api";
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
import { PicListConfig } from "./PicListConfig";
import {
  MediaDropzone,
  MediaUploadResults,
  MediaAlbumGrid,
  MediaGallery,
  ConnectionPanel,
  ConnectionValue,
} from "@fiveplusone/ui/media-workspace";
function imagePreviewURL(url: string, item: ImageItem) {
  if (!item.moderated_at) return url;
  return (
    url +
    (url.includes("?") ? "&" : "?") +
    "review=" +
    encodeURIComponent(item.moderated_at + "-" + item.cache_sync)
  );
}
export function Gallery({
  recycled = false,
  review = false,
}: {
  recycled?: boolean;
  review?: boolean;
}) {
  const { me, notify } = useSession(),
    navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const [query, setQuery] = useState(params.get("q") || "");
  const [filters, setFilters] = useState(false);
  const [selected, setSelected] = useState<string[]>([]),
    [detail, setDetail] = useState<ImageItem>(),
    [format, setFormat] = useState<LinkFormat>("url"),
    [move, setMove] = useState(false),
    [destination, setDestination] = useState(""),
    [confirm, setConfirm] = useState<"delete" | "purge" | null>(null),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const [moderation, setModeration] = useState<{
    items: ImageItem[];
    blocked: boolean;
  }>();
  const albums = useLoad(api.albums);
  const users = useLoad(
    () => (review ? api.users() : Promise.resolve({ items: [] })),
    [review],
  );
  const seq = useRef(0);
  const [items, setItems] = useState<ImageItem[]>([]),
    [more, setMore] = useState(false),
    [loading, setLoading] = useState(true);
  const scope = review ? "all" : params.get("scope") || "mine",
    album = params.get("album") || "",
    mime = params.get("mime") || "",
    status = params.get("status") || "",
    owner = params.get("owner") || "";
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
      if (status) p.set("moderation", status);
      if (review && owner) p.set("user_id", owner);
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
      setDetail((prev) =>
        prev ? res.items.find((x) => x.id === prev.id) || prev : prev,
      );
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
  }, [params.toString(), recycled, review]);
  useEffect(() => {
    if (
      !items.some(
        (x) => x.cache_sync === "pending" || x.cache_sync === "retrying",
      )
    )
      return;
    let alive = true,
      checking = false;
    const ids = items
      .filter((x) => x.cache_sync !== "complete")
      .slice(0, 100)
      .map((x) => x.id);
    const timer = window.setInterval(async () => {
      if (document.hidden || busy || checking) return;
      checking = true;
      try {
        const states = await api.imageCacheStatus(ids);
        if (!alive) return;
        setItems((prev) =>
          prev.some((x) => states[x.id] && states[x.id] !== x.cache_sync)
            ? prev.map((x) =>
                states[x.id] ? { ...x, cache_sync: states[x.id] } : x,
              )
            : prev,
        );
        setDetail((prev) =>
          prev && states[prev.id]
            ? { ...prev, cache_sync: states[prev.id] }
            : prev,
        );
      } catch (e) {
        if (alive) setError(errorText(e));
      } finally {
        checking = false;
      }
    }, 10000);
    return () => {
      alive = false;
      window.clearInterval(timer);
    };
  }, [items, params.toString(), busy]);
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
  return (
    <>
      {album && !review && !recycled && (
        <div className="fp-media-breadcrumb">
          <Button variant="ghost" onClick={() => navigate("/albums")}>
            返回相册
          </Button>
          <span>/</span>
          <span>
            {albums.data?.items.find((x) => String(x.id) === album)?.name ||
              "相册"}
          </span>
        </div>
      )}
      <Heading
        title={
          review
            ? "图片审核"
            : recycled
              ? "回收站"
              : albums.data?.items.find((x) => String(x.id) === album)?.name ||
                "图片库"
        }
        description={
          review
            ? "查看全站上传内容，管理图片的公开访问。"
            : recycled
              ? "找回最近移入回收站的图片，或永久删除不再需要的内容。"
              : album
                ? "浏览相册中的图片，选择图片可复制链接或移动位置。"
                : "把值得收藏的画面整理在一起，随时分享。"
        }
        actions={
          !review ? (
            <Button
              onClick={() =>
                navigate(
                  album && !recycled ? `/upload?album=${album}` : "/upload",
                )
              }
            >
              <Upload size={18} />
              上传图片
            </Button>
          ) : undefined
        }
      />
      <section className="fp-media-gallery-workspace">
        <form
          className="fp-media-gallery-filters"
          onSubmit={(e) => {
            e.preventDefault();
            filter("q", query);
          }}
        >
          <TextField label="搜索图片" value={query} onChange={setQuery} />
          <Button type="submit" variant="secondary">
            搜索
          </Button>
          {!review && (
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
          )}
          {review && (
            <Choice
              label="上传者"
              value={owner}
              onChange={(v) => filter("owner", v)}
              options={[
                { value: "", label: "全部上传者" },
                ...(users.data?.items || []).map((x) => ({
                  value: String(x.id),
                  label: x.username,
                })),
              ]}
            />
          )}
          {review ? (
            <Choice
              label="访问状态"
              value={status}
              onChange={(v) => filter("status", v)}
              options={[
                { value: "", label: "全部状态" },
                { value: "active", label: "正常访问" },
                { value: "blocked", label: "已禁止访问" },
              ]}
            />
          ) : (
            <Choice
              label="链接格式"
              value={format}
              onChange={(v) => setFormat(v as LinkFormat)}
              options={linkFormats}
            />
          )}
          <Button
            variant="ghost"
            aria-expanded={filters}
            onClick={() => setFilters(!filters)}
          >
            {filters
              ? "收起筛选"
              : `更多筛选${mime || (!review && (status || scope === "all")) ? " · 已启用" : ""}`}
          </Button>
          {filters && (
            <div className="fp-media-gallery-filter-extra">
              <Choice
                label="图片格式"
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
              {me.user.role !== "user" && !review && (
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
              {!review && (
                <Choice
                  label="访问状态"
                  value={status}
                  onChange={(v) => filter("status", v)}
                  options={[
                    { value: "", label: "全部状态" },
                    { value: "active", label: "正常访问" },
                    { value: "blocked", label: "已禁止访问" },
                  ]}
                />
              )}
            </div>
          )}
        </form>
        <Failure error={error} retry={() => void load()} />
        {items.some(
          (x) => x.cache_sync === "pending" || x.cache_sync === "retrying",
        ) && (
          <p role="status" className="fp-muted">
            {items.some((x) => x.cache_sync === "retrying")
              ? "部分图片的访问状态正在自动重试同步。"
              : "正在同步图片访问状态，完成后自动更新。"}
          </p>
        )}
        {review && <Failure error={users.error} retry={users.reload} />}
        <div className="fp-media-gallery-actions">
          <label>
            <input
              type="checkbox"
              aria-label="选择当前已加载的全部图片"
              checked={!!items.length && selected.length === items.length}
              disabled={!items.length}
              onChange={(e) =>
                setSelected(
                  e.target.checked ? items.map((x) => String(x.id)) : [],
                )
              }
            />
            {selected.length
              ? `已选择 ${selected.length} 张`
              : `已加载 ${items.length} 张`}
          </label>
          <ActionToolbar
            label="图片操作"
            actions={[
              {
                id: "refresh",
                label: "刷新图片",
                icon: (
                  <>
                    <RefreshCw size={16} />
                    <span>刷新</span>
                  </>
                ),
                onSelect: () => void load(),
                disabled: busy,
              },
              ...(me.user.role !== "user" && !recycled
                ? [
                    {
                      id: "block",
                      label: "禁止访问",
                      icon: (
                        <>
                          <ShieldBan size={16} />
                          <span>禁止访问</span>
                        </>
                      ),
                      danger: true,
                      onSelect: () =>
                        setModeration({
                          items: chosen.filter((x) => !x.blocked),
                          blocked: true,
                        }),
                      disabled: !chosen.some((x) => !x.blocked) || busy,
                    },
                    {
                      id: "allow",
                      label: "恢复访问",
                      icon: (
                        <>
                          <ShieldCheck size={16} />
                          <span>恢复访问</span>
                        </>
                      ),
                      onSelect: () =>
                        setModeration({
                          items: chosen.filter((x) => x.blocked),
                          blocked: false,
                        }),
                      disabled: !chosen.some((x) => x.blocked) || busy,
                    },
                  ]
                : []),
              ...(!review
                ? [
                    {
                      id: "copy",
                      label: "复制所选链接",
                      icon: (
                        <>
                          <Copy size={16} />
                          <span>复制链接</span>
                        </>
                      ),
                      onSelect: () => void copy(chosen),
                      disabled: !chosen.length,
                    },
                    {
                      id: "move",
                      label: "移动到相册",
                      icon: (
                        <>
                          <FolderInput size={16} />
                          <span>移动</span>
                        </>
                      ),
                      onSelect: () => setMove(true),
                      disabled: !chosen.length || !canMove || busy || recycled,
                    },
                    {
                      id: "delete",
                      label: recycled ? "永久删除" : "移入回收站",
                      icon: (
                        <>
                          <Trash2 size={16} />
                          <span>删除</span>
                        </>
                      ),
                      onSelect: () => setConfirm(recycled ? "purge" : "delete"),
                      danger: true,
                      disabled: !chosen.length || !allowed || busy,
                    },
                  ]
                : []),
              ...(recycled
                ? [
                    {
                      id: "restore",
                      label: "恢复图片",
                      icon: (
                        <>
                          <RotateCcw size={16} />
                          <span>恢复</span>
                        </>
                      ),
                      onSelect: () => void batch((i) => api.restoreImage(i.id)),
                      disabled: !chosen.length || !allowed || busy,
                    },
                  ]
                : []),
            ]}
          />
        </div>
        <MediaGallery
          items={items.map((x) => ({
            id: String(x.id),
            name: x.original_name,
            preview: imagePreviewURL(x.thumb_url || x.url, x),
            description: `${humanBytes(x.size)} · ${x.width} × ${x.height}${review && x.username ? ` · ${x.username}` : ""}`,
            blocked: x.blocked,
            status:
              x.cache_sync !== "complete"
                ? "正在同步访问状态"
                : x.blocked
                  ? "已禁止访问"
                  : undefined,
          }))}
          selected={selected}
          onSelectionChange={setSelected}
          onOpen={(id) => setDetail(items.find((x) => String(x.id) === id))}
          onCopy={
            !review && !recycled
              ? (id) => {
                  const item = items.find((x) => String(x.id) === id);
                  if (item) void copy([item]);
                }
              : undefined
          }
          empty={
            loading ? (
              <Loading />
            ) : (
              <EmptyState
                title={
                  params.get("q") || mime || status || owner
                    ? "没有找到图片"
                    : recycled
                      ? "回收站是空的"
                      : album
                        ? "相册还没有图片"
                        : "开始收藏你的第一张图片"
                }
                description={
                  params.get("q") || mime || status || owner
                    ? "试试其他关键词或筛选条件。"
                    : recycled
                      ? "移入回收站的图片会显示在这里。"
                      : "选择、拖入或粘贴图片，上传后即可分享。"
                }
                action={
                  !review && !recycled ? (
                    <Button
                      onClick={() =>
                        navigate(album ? `/upload?album=${album}` : "/upload")
                      }
                    >
                      上传图片
                    </Button>
                  ) : undefined
                }
              />
            )
          }
        />
        {more && (
          <Button
            variant="secondary"
            loading={loading}
            onClick={() => void load(true)}
          >
            加载更多
          </Button>
        )}
      </section>
      {detail && (
        <ImageDetails
          item={detail}
          editable={
            !review &&
            (me.user.role === "super_admin" || detail.user_id === me.user.id)
          }
          albums={albums.data?.items || []}
          format={format}
          recycled={recycled}
          onClose={() => setDetail(undefined)}
          onSaved={() => {
            void load();
            changed();
          }}
          onModerate={
            me.user.role !== "user" && !recycled
              ? (blocked) => {
                  setModeration({ items: [detail], blocked });
                  setDetail(undefined);
                }
              : undefined
          }
        />
      )}
      {moderation && (
        <ModerationDialog
          items={moderation.items}
          blocked={moderation.blocked}
          onClose={() => setModeration(undefined)}
          onChanged={() => {
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
  onModerate,
}: {
  item: ImageItem;
  editable: boolean;
  albums: AlbumView[];
  format: LinkFormat;
  recycled: boolean;
  onClose: () => void;
  onSaved: () => void;
  onModerate?: (blocked: boolean) => void;
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
          {onModerate && (
            <Button
              variant="secondary"
              disabled={busy}
              onClick={() => onModerate(!item.blocked)}
            >
              {item.blocked ? (
                <ShieldCheck size={17} />
              ) : (
                <ShieldBan size={17} />
              )}
              {item.blocked ? "恢复访问" : "禁止访问"}
            </Button>
          )}
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
      <img
        className="islet-preview"
        src={imagePreviewURL(item.url, item)}
        alt={item.original_name}
      />
      <p>
        <strong>{item.blocked ? "已禁止访问" : "正常访问"}</strong>
        {item.username && <> · 上传者：{item.username}</>}
      </p>
      {item.blocked && <p>违规原因：{item.block_reason}</p>}
      {item.cache_sync !== "complete" && (
        <p role="status">
          {item.cache_sync === "retrying"
            ? "访问状态同步暂未完成，正在自动重试。"
            : "正在同步访问状态，部分地区可能仍显示之前的图片。"}
        </p>
      )}
      {item.cache_sync === "complete" && item.moderated_at && (
        <p className="fp-muted">访问状态已同步</p>
      )}
      {item.moderated_at && (
        <p className="fp-muted">最近审核：{dateLabel(item.moderated_at)}</p>
      )}
      {onModerate && item.blocked && (
        <OriginalPreview id={item.id} name={item.original_name} />
      )}
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
function OriginalPreview({ id, name }: { id: number; name: string }) {
  const [open, setOpen] = useState(false),
    [url, setURL] = useState(""),
    [error, setError] = useState("");
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    if (!open) return;
    const controller = new AbortController();
    let objectURL = "";
    setError("");
    setURL("");
    const token = getAccessToken();
    void fetch(`/api/admin/images/${id}/preview`, {
      signal: controller.signal,
      cache: "no-store",
      headers: token ? { Authorization: `Bearer ${token}` } : {},
    })
      .then(async (response) => {
        if (!response.ok) throw new Error("无法查看原图，请检查登录状态后重试");
        const blob = await response.blob();
        if (controller.signal.aborted) return;
        objectURL = URL.createObjectURL(blob);
        setURL(objectURL);
      })
      .catch((e) => {
        if (!controller.signal.aborted) setError(errorText(e));
      });
    return () => {
      controller.abort();
      if (objectURL) URL.revokeObjectURL(objectURL);
    };
  }, [id, open, revision]);
  return (
    <>
      <Button variant="secondary" onClick={() => setOpen((v) => !v)}>
        {open ? "收起原图" : "查看原图"}
      </Button>
      {open && (
        <>
          <p className="fp-muted">原图仅供管理员复核，公开访问仍受限制。</p>
          <Failure error={error} retry={() => setRevision((v) => v + 1)} />
          {!url && !error && <Loading />}
          {url && (
            <img className="islet-preview" src={url} alt={`${name} 原图`} />
          )}
        </>
      )}
    </>
  );
}

function ModerationDialog({
  items,
  blocked,
  onClose,
  onChanged,
}: {
  items: ImageItem[];
  blocked: boolean;
  onClose: () => void;
  onChanged: () => void;
}) {
  const { notify } = useSession();
  const [pending, setPending] = useState(items),
    [reason, setReason] = useState(""),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const title = blocked ? "禁止访问" : "恢复访问";
  const submit = async () => {
    if (busy) return;
    if (blocked && (!reason.trim() || [...reason.trim()].length > 500)) {
      setError("请填写违规原因，最多 500 字");
      return;
    }
    setBusy(true);
    setError("");
    const failed: ImageItem[] = [],
      errors: string[] = [];
    let syncing = false;
    for (const item of pending) {
      try {
        const result = await api.moderateImage(item.id, blocked, reason.trim());
        if (result.cache_sync !== "complete") syncing = true;
      } catch (e) {
        failed.push(item);
        errors.push(`${item.original_name}：${errorText(e)}`);
      }
    }
    const count = pending.length - failed.length;
    setPending(failed);
    setBusy(false);
    if (count) {
      notify(
        syncing
          ? `已保存 ${count} 张图片的审核结果，正在同步访问状态`
          : `已${title} ${count} 张图片`,
      );
      onChanged();
    }
    if (failed.length) setError(errors.join("；"));
    else onClose();
  };
  return (
    <Dialog
      open
      title={`${title} · ${pending.length} 张图片`}
      onClose={() => !busy && onClose()}
      footer={
        <>
          <Button variant="secondary" disabled={busy} onClick={onClose}>
            取消
          </Button>
          <Button loading={busy} onClick={() => void submit()}>
            确认{title}
          </Button>
        </>
      }
    >
      <div className="islet-form">
        <p>
          {blocked
            ? "公开链接将显示违反使用策略的提示图片，原文件保留，可在复核后恢复访问。"
            : "所选图片的公开链接将重新显示原图。"}
        </p>
        <p className="fp-muted">
          使用 CDN 的图片会自动同步至各地区，通常需要 5–6 分钟。
        </p>
        <p>
          {pending
            .slice(0, 3)
            .map((x) => x.original_name)
            .join("、")}
          {pending.length > 3 ? ` 等 ${pending.length} 张图片` : ""}
        </p>
        {blocked && (
          <TextField
            label="违规原因"
            required
            value={reason}
            onChange={setReason}
            hint="向上传者说明限制原因，最多 500 字"
          />
        )}
        <Failure error={error} />
      </div>
    </Dialog>
  );
}

export function UploadPage() {
  const { me, notify } = useSession(),
    navigate = useNavigate(),
    tasks = useUploads(),
    albums = useLoad(api.albums);
  const [params] = useSearchParams();
  const [preview, setPreview] = useState<ImageItem>();
  const [album, setAlbum] = useState(params.get("album") || ""),
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
  async function copy(ids?: string[]) {
    const done = tasks.filter((t) => t.result && (!ids || ids.includes(t.id)));
    if (!done.length) return;
    try {
      await copyText(done.map((t) => imageLink(t.result!, format)).join("\n"));
      setError("");
      notify(`已复制 ${done.length} 个链接`);
    } catch (e) {
      setError(errorText(e));
    }
  }
  return (
    <>
      <Heading
        title="上传图片"
        description="上传、取链，一次完成。支持拖拽和粘贴多张图片。"
        actions={
          <Button
            variant="secondary"
            onClick={() =>
              navigate(album ? `/images?album=${album}` : "/images")
            }
          >
            查看{album ? "相册" : "图片库"}
          </Button>
        }
      />
      <section className="fp-media-upload-options" aria-label="图片保存位置">
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
            ...me.policies.map((x) => ({ value: String(x.id), label: x.name })),
          ]}
        />
        <details>
          <summary>添加标签</summary>
          <TextField
            label="标签"
            value={tags}
            onChange={setTags}
            hint="多个标签用逗号分隔，应用于接下来上传的图片"
          />
        </details>
      </section>
      <Failure error={albums.error} retry={albums.reload} />
      <MediaDropzone
        onFiles={add}
        limit={humanBytes(max)}
        compact={tasks.length > 0}
      />
      <Failure error={error} />
      <MediaUploadResults
        items={tasks.map((t) => ({
          id: t.id,
          name: t.name,
          size: humanBytes(t.file.size),
          preview: t.preview,
          status: t.status,
          progress: t.progress,
          error: t.error,
          canRetry: !t.blocked,
          link: t.result ? imageLink(t.result, format) : undefined,
        }))}
        formatControl={
          <Choice
            label="链接格式"
            value={format}
            onChange={(v) => setFormat(v as LinkFormat)}
            options={linkFormats}
          />
        }
        onCopy={(id) => void copy([id])}
        onCopyAll={() => void copy()}
        onRetry={uploads.retry}
        onRemove={uploads.remove}
        onClear={uploads.clear}
        onOpen={(id) => {
          const t = tasks.find((t) => t.id === id);
          if (t?.result) setPreview(t.result);
        }}
      />
      {preview && (
        <ConnectionPanel
          title={preview.original_name}
          onClose={() => setPreview(undefined)}
          footer={<Button onClick={() => setPreview(undefined)}>完成</Button>}
        >
          <img
            className="islet-preview"
            src={preview.url}
            alt={preview.original_name}
          />
          <ConnectionValue
            label="图片链接"
            value={imageLink(preview, format)}
            onCopy={async () => {
              try {
                await copyText(imageLink(preview, format));
                notify("链接已复制");
              } catch (e) {
                setError(errorText(e));
              }
            }}
          />
        </ConnectionPanel>
      )}
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
        <MediaAlbumGrid
          albums={data.data.items.map((x) => ({
            id: String(x.id),
            name: x.name,
            description: x.description,
            count: x.image_count || 0,
            cover: x.cover_url,
          }))}
          onOpen={(id) => navigate("/images?album=" + id)}
          onEdit={(id) => {
            const x = data.data?.items.find((x) => String(x.id) === id);
            if (x) open(x);
          }}
          onDelete={(id) =>
            setRemove(data.data?.items.find((x) => String(x.id) === id))
          }
        />
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
  const [picList, setPicList] = useState<{ key: string }>();
  const [picGo, setPicGo] = useState(false);
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
      <section className="fp-media-client-card">
        <div>
          <h2>连接上传工具</h2>
          <p>为每个客户端创建独立密钥，随时管理它的上传权限。</p>
        </div>
        <div>
          <Button variant="secondary" onClick={() => setPicList({ key: "" })}>
            连接 PicList
          </Button>
          <Button variant="ghost" onClick={() => setPicGo(true)}>
            连接 PicGo
          </Button>
        </div>
      </section>
      {data.data?.items.length ? (
        <section className="fp-media-token-list" aria-label="上传密钥列表">
          {data.data.items.map((x) => (
            <article className="fp-media-token-row" key={x.id}>
              <div>
                <h2>{x.name}</h2>
                <p>
                  创建于 {dateLabel(x.created_at)} ·{" "}
                  {x.expires_at
                    ? `到期于 ${dateLabel(x.expires_at)}`
                    : "长期有效"}
                </p>
              </div>
              <div className="fp-media-token-meta">
                <span>{x.status === "active" ? "使用中" : "已撤销"}</span>
                {x.status === "active" && (
                  <Button variant="secondary" onClick={() => setRemove(x.id)}>
                    撤销密钥
                  </Button>
                )}
              </div>
            </article>
          ))}
        </section>
      ) : (
        !data.loading && (
          <EmptyState
            title="创建第一个上传密钥"
            description="连接 PicList、PicGo 或其他上传工具。"
            action={
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
        )
      )}
      {data.loading && <Loading />}
      {picGo && (
        <ConnectionPanel
          title="连接 PicGo"
          onClose={() => setPicGo(false)}
          footer={<Button onClick={() => setPicGo(false)}>完成</Button>}
        >
          <div className="fp-media-connect-intro">
            <div>
              <h3>添加自定义上传图床</h3>
              <p>
                使用 multipart/form-data 上传，Authorization 填写
                Bearer、一个空格和你的上传密钥。
              </p>
            </div>
          </div>
          <div className="fp-media-config-values">
            <ConnectionValue
              label="上传地址"
              value={
                (me.public_base_url || location.origin) +
                "/api/v1/upload?picgo=1"
              }
              onCopy={async () => {
                try {
                  await copyText(
                    (me.public_base_url || location.origin) +
                      "/api/v1/upload?picgo=1",
                  );
                  notify("上传地址已复制");
                } catch (e) {
                  setError(errorText(e));
                }
              }}
            />
            <ConnectionValue label="请求方法" value="POST" />
            <ConnectionValue label="文件字段" value="files" />
          </div>
          <Failure error={error} />
        </ConnectionPanel>
      )}
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
          <>
            <Button
              variant="secondary"
              onClick={() => {
                setPicList({ key: secret });
                setSecret("");
              }}
            >
              配置 PicList
            </Button>
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
          </>
        }
      >
        <p>密钥只展示这一次，请妥善保存。任何持有者都可以使用你的上传权限。</p>
        <pre className="islet-secret">{secret}</pre>
        <Failure error={error} />
      </Dialog>
      {picList && (
        <PicListConfig
          initialKey={picList.key}
          onClose={() => setPicList(undefined)}
        />
      )}
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
