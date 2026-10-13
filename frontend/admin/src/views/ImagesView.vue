<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import {
  NAlert,
  NButton,
  NCheckbox,
  NDrawer,
  NDrawerContent,
  NDynamicTags,
  NForm,
  NFormItem,
  NInput,
  NModal,
  NCard,
  NSelect,
  NSkeleton,
  NSpace,
  NTag,
  useDialog,
  useMessage,
} from "naive-ui";
import { api, type AlbumView, type ImageItem } from "../api";
import { useAuthStore } from "../stores/auth";
import {
  copyText,
  dateLabel,
  errorText,
  humanBytes,
  imageLink,
  linkFormats,
  type LinkFormat,
} from "../utils";
import PageHeader from "../components/PageHeader.vue";
import EmptyState from "../components/EmptyState.vue";
import AppIcon from "../components/AppIcon.vue";
import ImageAsset from "../components/ImageAsset.vue";
const props = withDefaults(defineProps<{ recycled?: boolean }>(), {
  recycled: false,
});
const router = useRouter(),
  route = useRoute(),
  auth = useAuthStore(),
  message = useMessage(),
  dialog = useDialog();
const items = ref<ImageItem[]>([]),
  albums = ref<AlbumView[]>([]),
  loading = ref(false),
  loadingMore = ref(false),
  error = ref(""),
  hasMore = ref(false);
const keyword = ref(String(route.query.q || "")),
  album = ref<number | null>(Number(route.query.album) || null),
  mime = ref<string | null>(String(route.query.mime || "") || null),
  scope = ref(String(route.query.scope || "mine"));
const selected = ref<number[]>([]),
  detail = ref<ImageItem | null>(null),
  format = ref<LinkFormat>("url"),
  busy = ref(false),
  saving = ref(false),
  formError = ref("");
const edit = ref({
  name: "",
  album_id: null as number | null,
  tags: [] as string[],
});
const moveOpen = ref(false),
  moveAlbum = ref<number | null>(null);
let requestId = 0;
const albumOptions = computed(() =>
  albums.value.map((x) => ({ label: x.name, value: x.id })),
);
const canAdmin = computed(() =>
  ["admin", "super_admin"].includes(auth.user?.role || ""),
);
const canModify = (item: ImageItem) =>
  item.user_id === auth.user?.id || auth.user?.role === "super_admin";
const selectedItems = computed(() =>
  items.value.filter((x) => selected.value.includes(x.id)),
);
const selectable = computed(() => items.value.filter(canModify));
const allSelected = computed(
  () =>
    selectable.value.length > 0 &&
    selectable.value.every((x) => selected.value.includes(x.id)),
);
const filtered = computed(() => !!(keyword.value || album.value || mime.value));
const title = computed(() =>
  props.recycled
    ? "回收站"
    : album.value
      ? albums.value.find((a) => a.id === album.value)?.name || "相册图片"
      : "图片库",
);
const mimeOptions = [
  { label: "JPEG", value: "image/jpeg" },
  { label: "PNG", value: "image/png" },
  { label: "GIF", value: "image/gif" },
  { label: "WebP", value: "image/webp" },
  { label: "AVIF", value: "image/avif" },
];
function query(more = false) {
  const p = new URLSearchParams({ limit: "40" });
  if (props.recycled) p.set("recycled", "1");
  if (keyword.value.trim()) p.set("keyword", keyword.value.trim());
  if (album.value) p.set("album_id", String(album.value));
  if (mime.value) p.set("mime", mime.value);
  if (scope.value === "all") p.set("scope", "all");
  const last = items.value.at(-1);
  if (more && last) {
    p.set("before", last.created_at);
    p.set("before_id", String(last.id));
  }
  return "?" + p.toString();
}
async function load(more = false) {
  if (more && (loading.value || loadingMore.value)) return;
  const id = ++requestId;
  if (more) loadingMore.value = true;
  else {
    loading.value = true;
    selected.value = [];
  }
  error.value = "";
  try {
    const result = await api.images(query(more));
    if (id !== requestId) return;
    items.value = more
      ? [
          ...items.value,
          ...result.items.filter(
            (x) => !items.value.some((i) => i.id === x.id),
          ),
        ]
      : result.items;
    hasMore.value = result.has_more;
  } catch (e) {
    if (id === requestId) error.value = errorText(e);
  } finally {
    if (id === requestId) {
      loading.value = false;
      loadingMore.value = false;
    }
  }
}
function applyFilters() {
  const q: Record<string, string> = {};
  if (keyword.value.trim()) q.q = keyword.value.trim();
  if (album.value) q.album = String(album.value);
  if (mime.value) q.mime = mime.value;
  if (scope.value === "all") q.scope = "all";
  if (JSON.stringify(q) === JSON.stringify(route.query)) load();
  else router.replace({ query: q });
}
function resetFilters() {
  keyword.value = "";
  album.value = null;
  mime.value = null;
  applyFilters();
}
watch(
  () => route.fullPath,
  () => {
    keyword.value = String(route.query.q || "");
    album.value = Number(route.query.album) || null;
    mime.value = String(route.query.mime || "") || null;
    scope.value = String(route.query.scope || "mine");
    load();
  },
);
function toggle(id: number, value: boolean) {
  selected.value = value
    ? [...new Set([...selected.value, id])]
    : selected.value.filter((x) => x !== id);
}
function openDetail(item: ImageItem) {
  detail.value = item;
  edit.value = {
    name: item.original_name,
    album_id: item.album_id || null,
    tags: [...item.tags],
  };
  formError.value = "";
}
async function copy(itemsToCopy: ImageItem[]) {
  try {
    await copyText(
      itemsToCopy.map((x) => imageLink(x, format.value)).join("\n"),
    );
    message.success(`已复制 ${itemsToCopy.length} 个链接`);
  } catch (e) {
    message.error(errorText(e));
  }
}
async function save() {
  if (!detail.value || saving.value) return;
  if (!edit.value.name.trim()) {
    formError.value = "请输入图片名称";
    return;
  }
  saving.value = true;
  formError.value = "";
  try {
    const updated = await api.updateImage(detail.value.id, {
      name: edit.value.name.trim(),
      album_id: edit.value.album_id || 0,
      tags: edit.value.tags,
    });
    items.value = items.value.map((x) => (x.id === updated.id ? updated : x));
    detail.value = updated;
    message.success("图片信息已保存");
    window.dispatchEvent(new Event("islet:changed"));
  } catch (e) {
    formError.value = errorText(e);
  } finally {
    saving.value = false;
  }
}
async function execute(
  targets: ImageItem[],
  action: "recycle" | "restore" | "purge" | "move",
) {
  if (busy.value) return;
  busy.value = true;
  let count = 0;
  const failures: string[] = [];
  for (const item of targets) {
    try {
      if (action === "recycle") await api.recycleImage(item.id);
      else if (action === "restore") await api.restoreImage(item.id);
      else if (action === "purge") await api.purgeImage(item.id);
      else await api.updateImage(item.id, { album_id: moveAlbum.value || 0 });
      count++;
    } catch (e) {
      failures.push(`${item.original_name}：${errorText(e)}`);
    }
  }
  busy.value = false;
  moveOpen.value = false;
  if (count) {
    message.success(
      `已${{ recycle: "移入回收站", restore: "恢复", purge: "永久删除", move: "整理" }[action]} ${count} 张图片`,
    );
    detail.value = null;
    await load();
    window.dispatchEvent(new Event("islet:changed"));
  }
  if (failures.length) {
    error.value = failures.join("；");
  }
}
function confirmAction(
  targets: ImageItem[],
  action: "recycle" | "restore" | "purge",
) {
  if (!targets.length) return;
  if (action === "restore") {
    execute(targets, action);
    return;
  }
  dialog.warning({
    title: action === "purge" ? "永久删除图片" : "移入回收站",
    content:
      action === "purge"
        ? `永久删除这 ${targets.length} 张图片后将无法恢复，已使用的链接可能失效。`
        : `将这 ${targets.length} 张图片移入回收站？已发布的图片链接可能受到影响，缓存中的图片可能仍可访问。`,
    positiveText: action === "purge" ? "永久删除" : "移入回收站",
    negativeText: "取消",
    onPositiveClick: () => execute(targets, action),
  });
}
onMounted(async () => {
  await Promise.all([
    load(),
    api
      .albums()
      .then((r) => (albums.value = r.items))
      .catch((e) => message.error(errorText(e))),
  ]);
});
</script>
<template>
  <div class="page">
    <PageHeader
      :title="title"
      :description="
        recycled
          ? '找回需要的图片，或释放不再使用的空间。'
          : '收藏、整理与分享，让每一张图片随时可用。'
      "
      :eyebrow="recycled ? 'RECYCLE BIN' : 'YOUR IMAGE SPACE'"
    >
      <n-button
        v-if="!recycled"
        type="primary"
        size="large"
        @click="
          router.push({
            name: 'upload',
            query: album ? { album: String(album) } : {},
          })
        "
        ><template #icon><AppIcon name="plus" :size="18" /></template
        >上传图片</n-button
      >
    </PageHeader>
    <div class="toolbar surface">
      <n-input
        v-model:value="keyword"
        class="search-input"
        placeholder="搜索图片名称或链接"
        clearable
        :input-props="{ 'aria-label': '搜索图片' }"
        @keyup.enter="applyFilters"
        @clear="
          keyword = '';
          applyFilters();
        "
        ><template #prefix><AppIcon name="search" :size="17" /></template
      ></n-input>
      <n-select
        v-model:value="album"
        class="filter-input"
        :options="albumOptions"
        placeholder="全部相册"
        clearable
        aria-label="按相册筛选"
        @update:value="applyFilters"
      />
      <n-select
        v-model:value="mime"
        class="filter-input"
        :options="mimeOptions"
        placeholder="全部格式"
        clearable
        aria-label="按格式筛选"
        @update:value="applyFilters"
      />
      <n-button @click="applyFilters">搜索</n-button>
      <div class="toolbar-spacer" />
      <n-select
        v-if="canAdmin"
        v-model:value="scope"
        style="width: 130px"
        :options="[
          { label: '我的图片', value: 'mine' },
          { label: '全站图片', value: 'all' },
        ]"
        aria-label="图片范围"
        @update:value="applyFilters"
      />
      <n-button
        quaternary
        circle
        :loading="loading"
        aria-label="刷新图片"
        @click="load()"
        ><AppIcon name="refresh" :size="18"
      /></n-button>
    </div>
    <n-alert v-if="error" type="error" class="error-panel" title="暂时未能完成"
      ><div>{{ error }}</div>
      <n-button size="small" style="margin-top: 8px" @click="load()"
        >重新加载</n-button
      ></n-alert
    >
    <div v-if="loading" class="image-grid">
      <div v-for="n in 8" :key="n" class="surface" style="padding: 12px">
        <n-skeleton height="160px" /><n-skeleton
          text
          style="margin-top: 16px"
        /><n-skeleton text :width="'60%'" />
      </div>
    </div>
    <template v-else-if="items.length">
      <div class="section-head">
        <n-checkbox
          :checked="allSelected"
          :indeterminate="selected.length > 0 && !allSelected"
          :disabled="busy"
          @update:checked="
            (v) => (selected = v ? selectable.map((x) => x.id) : [])
          "
          >选择当前已加载图片</n-checkbox
        ><span class="subtle">已显示 {{ items.length }} 张</span>
      </div>
      <div class="image-grid">
        <article
          v-for="item in items"
          :key="item.id"
          class="image-card"
          :class="{ selected: selected.includes(item.id) }"
        >
          <div v-if="canModify(item)" class="image-check">
            <n-checkbox
              :checked="selected.includes(item.id)"
              :disabled="busy"
              :aria-label="'选择 ' + item.original_name"
              @update:checked="(v) => toggle(item.id, v)"
            />
          </div>
          <span class="image-kind">{{
            item.mime.split("/")[1]?.toUpperCase()
          }}</span>
          <button
            class="image-preview"
            :aria-label="'查看 ' + item.original_name"
            @click="openDetail(item)"
          >
            <ImageAsset
              :src="item.thumb_url || item.url"
              :fallback="item.url"
              :alt="item.original_name"
              loading="lazy"
            />
          </button>
          <div class="image-meta">
            <span class="image-name" :title="item.original_name">{{
              item.original_name
            }}</span>
            <div class="image-detail">
              <span>{{ humanBytes(item.size) }}</span
              ><span>{{
                item.width && item.height
                  ? `${item.width} × ${item.height}`
                  : "原始图片"
              }}</span>
            </div>
            <div class="image-actions">
              <template v-if="recycled"
                ><n-button
                  text
                  size="tiny"
                  type="primary"
                  :disabled="busy || !canModify(item)"
                  @click="confirmAction([item], 'restore')"
                  >恢复</n-button
                ><n-button
                  text
                  size="tiny"
                  :disabled="busy || !canModify(item)"
                  @click="confirmAction([item], 'purge')"
                  >永久删除</n-button
                ></template
              ><template v-else
                ><n-button text size="tiny" type="primary" @click="copy([item])"
                  ><template #icon><AppIcon name="copy" :size="13" /></template
                  >复制链接</n-button
                ><n-button text size="tiny" @click="openDetail(item)"
                  >详情</n-button
                ></template
              >
            </div>
          </div>
        </article>
      </div>
      <div class="load-more">
        <n-button v-if="hasMore" :loading="loadingMore" @click="load(true)"
          >加载更多图片</n-button
        ><span v-else class="subtle">已显示全部图片</span>
      </div>
      <div v-if="selected.length" class="selection-bar">
        <strong style="font-size: 13px">已选择 {{ selected.length }} 张</strong
        ><n-select
          v-if="!recycled"
          v-model:value="format"
          :options="linkFormats"
          style="width: 130px"
          aria-label="链接格式"
        /><n-button
          v-if="!recycled"
          :disabled="busy"
          @click="copy(selectedItems)"
          >复制链接</n-button
        ><n-button
          v-if="!recycled"
          :disabled="busy"
          @click="
            moveAlbum = null;
            moveOpen = true;
          "
          >移入相册</n-button
        ><n-button
          v-if="recycled"
          :loading="busy"
          @click="confirmAction(selectedItems, 'restore')"
          >恢复所选</n-button
        ><n-button
          :loading="busy"
          type="error"
          secondary
          @click="confirmAction(selectedItems, recycled ? 'purge' : 'recycle')"
          >{{ recycled ? "永久删除" : "移入回收站" }}</n-button
        ><n-button quaternary :disabled="busy" @click="selected = []"
          >取消选择</n-button
        >
      </div>
    </template>
    <EmptyState
      v-else-if="!error"
      :title="
        filtered
          ? '没有找到匹配的图片'
          : recycled
            ? '回收站是空的'
            : '从第一张图片开始'
      "
      :description="
        filtered
          ? '试试其他关键词，或清除筛选条件。'
          : recycled
            ? '移入回收站的图片会出现在这里。'
            : '上传照片、截图或设计素材，马上获得可分享的链接。'
      "
      :icon="recycled ? 'trash' : 'image'"
      ><n-button v-if="filtered" @click="resetFilters">清除筛选</n-button
      ><n-button
        v-else-if="!recycled"
        type="primary"
        @click="router.push('/upload')"
        >上传图片</n-button
      ></EmptyState
    >
    <n-drawer
      :show="!!detail"
      :width="520"
      :auto-focus="true"
      style="max-width: 100vw"
      @update:show="
        (v) => {
          if (!v && !saving) detail = null;
        }
      "
      ><n-drawer-content title="图片详情" closable
        ><template v-if="detail"
          ><div class="detail-preview">
            <img :src="detail.url" :alt="detail.original_name" />
          </div>
          <n-space justify="space-between" align="center"
            ><n-tag size="small" :bordered="false">{{ detail.mime }}</n-tag
            ><a :href="detail.url" target="_blank" rel="noopener noreferrer"
              >打开原图 ↗</a
            ></n-space
          >
          <dl class="metadata-list">
            <div>
              <dt>文件大小</dt>
              <dd>{{ humanBytes(detail.size) }}</dd>
            </div>
            <div>
              <dt>图片尺寸</dt>
              <dd>
                {{
                  detail.width && detail.height
                    ? `${detail.width} × ${detail.height}`
                    : "—"
                }}
              </dd>
            </div>
            <div>
              <dt>上传时间</dt>
              <dd>{{ dateLabel(detail.created_at) }}</dd>
            </div>
            <div>
              <dt>{{ recycled ? "删除时间" : "所有者" }}</dt>
              <dd>
                {{
                  recycled
                    ? dateLabel(detail.deleted_at)
                    : detail.username || auth.user?.username
                }}
              </dd>
            </div>
          </dl>
          <div class="detail-label">分享链接</div>
          <n-select
            v-model:value="format"
            :options="linkFormats"
            aria-label="分享链接格式" /><n-input
            :value="imageLink(detail, format)"
            readonly
            type="textarea"
            :rows="2"
            style="margin-top: 10px"
            aria-label="图片链接" /><n-button
            block
            secondary
            type="primary"
            style="margin-top: 10px"
            @click="copy([detail])"
            >复制链接</n-button
          >
          <template v-if="!recycled && canModify(detail)"
            ><div class="detail-label">整理图片</div>
            <n-alert
              v-if="formError"
              type="error"
              style="margin-bottom: 12px"
              >{{ formError }}</n-alert
            ><n-form label-placement="top"
              ><n-form-item label="名称"
                ><n-input
                  v-model:value="edit.name"
                  :maxlength="255" /></n-form-item
              ><n-form-item v-if="detail.user_id === auth.user?.id" label="相册"
                ><n-select
                  v-model:value="edit.album_id"
                  :options="albumOptions"
                  placeholder="未分类"
                  clearable /></n-form-item
              ><n-form-item label="标签"
                ><n-dynamic-tags
                  v-model:value="edit.tags"
                  :max="16" /></n-form-item></n-form></template
        ></template>
        <template #footer
          ><n-space v-if="detail" justify="space-between" style="width: 100%"
            ><n-button
              v-if="canModify(detail)"
              secondary
              type="error"
              :disabled="saving || busy"
              @click="confirmAction([detail], recycled ? 'purge' : 'recycle')"
              >{{ recycled ? "永久删除" : "移入回收站" }}</n-button
            ><n-button
              v-if="!recycled && detail && canModify(detail)"
              type="primary"
              :loading="saving"
              @click="save"
              >保存更改</n-button
            ><n-button
              v-if="recycled && detail && canModify(detail)"
              type="primary"
              :loading="busy"
              @click="confirmAction([detail], 'restore')"
              >恢复图片</n-button
            ></n-space
          ></template
        >
      </n-drawer-content></n-drawer
    >
    <n-modal v-model:show="moveOpen"
      ><n-card style="width: 420px" title="移入相册" :bordered="false"
        ><n-select
          v-model:value="moveAlbum"
          :options="albumOptions"
          placeholder="未分类"
          clearable
        />
        <p class="subtle">
          将所选 {{ selected.length }} 张图片整理到同一个相册。
        </p>
        <template #footer
          ><n-space justify="end"
            ><n-button :disabled="busy" @click="moveOpen = false">取消</n-button
            ><n-button
              type="primary"
              :loading="busy"
              @click="execute(selectedItems, 'move')"
              >确认移动</n-button
            ></n-space
          ></template
        ></n-card
      ></n-modal
    >
  </div>
</template>
