<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import {
  NAlert,
  NButton,
  NForm,
  NFormItem,
  NInput,
  NProgress,
  NSelect,
  NSpace,
  NTag,
  useMessage,
} from "naive-ui";
import { api, type AlbumView, type MeInfo } from "../api";
import { useUploadStore } from "../stores/uploads";
import { useAuthStore } from "../stores/auth";
import {
  copyText,
  errorText,
  humanBytes,
  groupLabel,
  imageLink,
  linkFormats,
  type LinkFormat,
} from "../utils";
import PageHeader from "../components/PageHeader.vue";
import AppIcon from "../components/AppIcon.vue";
const route = useRoute(),
  router = useRouter(),
  store = useUploadStore(),
  auth = useAuthStore(),
  message = useMessage();
const input = ref<HTMLInputElement>(),
  dragging = ref(false),
  info = ref<MeInfo | null>(null),
  albums = ref<AlbumView[]>([]),
  error = ref("");
const album = ref<number | null>(Number(route.query.album) || null),
  policy = ref<number | null>(null),
  tags = ref(""),
  format = ref<LinkFormat>("url");
const albumOptions = computed(() =>
  albums.value.map((a) => ({ label: a.name, value: a.id })),
);
const policyOptions = computed(
  () =>
    info.value?.policies.map((p) => ({
      label: p.name + (p.is_default ? " · 默认" : ""),
      value: p.id,
    })) || [],
);
const maxBytes = computed(() =>
  Math.min(
    info.value?.upload_max_bytes || 100 * 1024 * 1024,
    auth.user?.role === "super_admin"
      ? Infinity
      : info.value?.group.max_file_bytes || Infinity,
  ),
);
const completed = computed(() =>
  store.tasks.filter((t) => t.status === "done" && t.result),
);
const failed = computed(() => store.tasks.filter((t) => t.status === "error"));
async function init() {
  error.value = "";
  try {
    const [me, a] = await Promise.all([api.meInfo(), api.albums()]);
    info.value = me;
    albums.value = a.items;
    policy.value =
      me.policies.find((p) => p.is_default)?.id || me.policies[0]?.id || null;
  } catch (e) {
    error.value = errorText(e);
  }
}
function add(files: File[]) {
  if (!info.value || !policy.value) {
    message.warning("请先选择可用的存储策略");
    return;
  }
  store.add(files, {
    album: album.value,
    policy: policy.value,
    tags: tags.value.trim(),
    maxBytes: maxBytes.value,
    allowed:
      auth.user?.role === "super_admin"
        ? []
        : info.value.group.allowed_mimes || [],
  });
}
function pick(e: Event) {
  const el = e.target as HTMLInputElement;
  if (el.files) add([...el.files]);
  el.value = "";
}
function drop(e: DragEvent) {
  dragging.value = false;
  if (e.dataTransfer?.files) add([...e.dataTransfer.files]);
}
function paste(e: ClipboardEvent) {
  if (e.clipboardData?.files.length) {
    e.preventDefault();
    add([...e.clipboardData.files]);
  }
}
async function copyAll() {
  try {
    await copyText(
      completed.value.map((t) => imageLink(t.result!, format.value)).join("\n"),
    );
    message.success(`已复制 ${completed.value.length} 个链接`);
  } catch (e) {
    message.error(errorText(e));
  }
}
async function copyOne(id: number) {
  const task = store.tasks.find((t) => t.id === id);
  if (!task?.result) return;
  try {
    await copyText(imageLink(task.result, format.value));
    message.success("链接已复制");
  } catch (e) {
    message.error(errorText(e));
  }
}
onMounted(() => {
  init();
  window.addEventListener("paste", paste);
});
onUnmounted(() => window.removeEventListener("paste", paste));
</script>
<template>
  <div class="page">
    <PageHeader
      title="上传图片"
      description="把精彩的画面留在这里，随时分享给任何人。"
      eyebrow="UPLOAD & SHARE"
      ><n-button @click="router.push('/images')"
        >查看图片库<AppIcon
          name="arrow"
          :size="16"
          style="margin-left: 8px" /></n-button
    ></PageHeader>
    <n-alert v-if="error" type="error" style="margin-bottom: 20px"
      >{{ error }} <n-button text @click="init">重试</n-button></n-alert
    >
    <div class="upload-layout">
      <div>
        <div
          class="upload-drop"
          :class="{ dragover: dragging }"
          role="button"
          tabindex="0"
          aria-label="选择图片上传"
          @click="input?.click()"
          @keydown.enter.prevent="input?.click()"
          @keydown.space.prevent="input?.click()"
          @dragover.prevent="dragging = true"
          @dragleave.prevent="dragging = false"
          @drop.prevent="drop"
        >
          <span class="empty-icon"><AppIcon name="upload" :size="30" /></span>
          <h3>将图片拖到这里</h3>
          <p>也可以点击选择，或直接粘贴图片</p>
          <n-button
            type="primary"
            style="margin: 22px 0 16px"
            :disabled="!info || !policy"
            tabindex="-1"
            >选择图片</n-button
          >
          <p>单张最大 {{ humanBytes(maxBytes) }} · 支持批量上传</p>
          <input
            ref="input"
            type="file"
            accept="image/*"
            multiple
            hidden
            @change="pick"
          />
        </div>
        <div
          v-if="store.tasks.length"
          class="surface"
          style="padding: 22px; margin-top: 24px"
        >
          <div class="section-head">
            <h2 class="section-title">
              上传记录
              <span class="subtle"
                >{{ completed.length }} / {{ store.tasks.length }} 已完成</span
              >
            </h2>
            <n-button
              v-if="failed.length"
              text
              type="primary"
              :disabled="store.running"
              @click="store.retry()"
              >重试失败项</n-button
            >
          </div>
          <div v-for="task in store.tasks" :key="task.id" class="upload-task">
            <img v-if="task.preview" :src="task.preview" alt="" /><AppIcon
              v-else
              name="image"
              :size="36"
            />
            <div class="upload-task-body">
              <div class="upload-task-name" :title="task.file.name">
                {{ task.file.name }}
              </div>
              <div class="subtle" style="margin-top: 5px">
                {{ humanBytes(task.file.size) }}
                <span v-if="task.status === 'error'" style="color: #c44343"
                  >· {{ task.error }}</span
                ><span v-else-if="task.status === 'waiting'">· 等待上传</span
                ><span v-else-if="task.status === 'uploading'"
                  >·
                  {{
                    task.progress === 99
                      ? "正在处理图片"
                      : `正在上传 ${task.progress}%`
                  }}</span
                >
              </div>
              <n-progress
                v-if="task.status === 'uploading'"
                type="line"
                :percentage="task.progress"
                :show-indicator="false"
                :height="3"
                style="margin-top: 9px"
              />
            </div>
            <n-button
              v-if="task.status === 'done'"
              text
              type="primary"
              size="small"
              @click="copyOne(task.id)"
              >复制链接</n-button
            ><n-button
              v-if="task.status === 'error'"
              text
              type="primary"
              size="small"
              @click="store.retry(task.id)"
              >重试</n-button
            ><n-button
              v-if="task.status === 'error' || task.status === 'waiting'"
              quaternary
              circle
              size="small"
              :aria-label="'移除 ' + task.file.name"
              @click="store.remove(task.id)"
              ><AppIcon name="close" :size="16" /></n-button
            ><AppIcon
              v-if="task.status === 'done'"
              name="check"
              :size="17"
              style="color: #389681"
            />
          </div>
          <n-space
            v-if="completed.length"
            style="margin-top: 20px"
            justify="space-between"
            ><n-button size="small" quaternary @click="store.clearCompleted()"
              >清除已完成记录</n-button
            ><n-button size="small" type="primary" @click="copyAll"
              >复制本次全部链接</n-button
            ></n-space
          >
        </div>
        <div
          v-else
          style="
            padding: 28px 8px;
            color: var(--muted);
            font-size: 12px;
            line-height: 1.9;
          "
        >
          <strong style="color: #66778e; font-weight: 500"
            >上传后，即刻可用</strong
          ><br />图片会自动保存到你的图库。完成后复制链接，即可用于文章、笔记或网站。
        </div>
      </div>
      <aside class="surface upload-options">
        <h2 class="section-title" style="margin-bottom: 22px">上传设置</h2>
        <n-form label-placement="top"
          ><n-form-item label="保存到相册"
            ><n-select
              v-model:value="album"
              :options="albumOptions"
              placeholder="未分类"
              clearable /></n-form-item
          ><n-form-item label="存储位置"
            ><n-select
              v-model:value="policy"
              :options="policyOptions"
              placeholder="选择存储策略" /></n-form-item
          ><n-form-item label="标签"
            ><n-input
              v-model:value="tags"
              placeholder="如：旅行, 灵感"
              :maxlength="255" /></n-form-item
          ><n-form-item label="复制链接格式"
            ><n-select
              v-model:value="format"
              :options="linkFormats" /></n-form-item
        ></n-form>
        <div
          v-if="info"
          style="border-top: 1px solid var(--border); padding-top: 16px"
        >
          <div class="subtle">我的容量</div>
          <p style="font-size: 13px">
            {{ humanBytes(info.quota.used) }}
            <span class="subtle"
              >/
              {{
                info.quota.limit ? humanBytes(info.quota.limit) : "不限"
              }}</span
            >
          </p>
          <n-tag size="small" :bordered="false" type="info">{{
            groupLabel(info.group.name)
          }}</n-tag>
        </div>
      </aside>
    </div>
  </div>
</template>
