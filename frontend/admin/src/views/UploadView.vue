<template>
  <div class="page">
    <n-h2>上传</n-h2>
    <div
      class="upload-drop"
      :class="{ dragover: dragover }"
      @click="fileInput?.click()"
      @dragover.prevent="dragover = true"
      @dragleave="dragover = false"
      @drop.prevent="handleDrop"
    >
      <p style="font-size: 16px; margin: 0 0 6px">点击选择 / 拖拽 / 粘贴图片上传</p>
      <n-text depth="3">支持批量；上传后自动生成链接与缩略图</n-text>
      <input ref="fileInput" type="file" accept="image/*" multiple hidden @change="handlePick" />
    </div>

    <n-space align="center" style="margin-top: 14px" :wrap="false">
      <n-select v-model:value="albumTarget" :options="albumOptions" placeholder="上传到相册（可选）" clearable style="width: 200px" />
      <n-input v-model:value="tagsInput" placeholder="标签，逗号分隔（可选）" style="width: 240px" />
    </n-space>

    <n-progress v-if="uploading" type="line" :percentage="progress" style="margin-top: 14px" indicator-placement="inside" />

    <n-alert v-if="errors.length" type="error" closable style="margin-top: 14px">
      <div v-for="(err, i) in errors" :key="i">{{ err }}</div>
    </n-alert>

    <template v-if="uploaded.length">
      <n-h3>上传成功（{{ uploaded.length }}）</n-h3>
      <n-space vertical size="small">
        <n-space v-for="item in uploaded" :key="item.id" align="center" :wrap="false" style="width: 100%">
          <img :src="item.thumb_url || item.url" style="width: 48px; height: 48px; object-fit: cover; border-radius: 6px" />
          <n-input :value="item.url" readonly size="small" style="flex: 1" />
          <n-button size="small" type="primary" @click="copyURL(item.url)">复制</n-button>
        </n-space>
      </n-space>
      <n-button style="margin-top: 10px" @click="copyAll">复制全部</n-button>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { NAlert, NButton, NH2, NH3, NInput, NProgress, NSelect, NSpace, NText, useMessage } from 'naive-ui';
import { api, getAccessToken, type AlbumView, type ImageItem } from '../api';

const fileInput = ref<HTMLInputElement>();
const dragover = ref(false);
const uploading = ref(false);
const progress = ref(0);
const albumTarget = ref<number | null>(null);
const tagsInput = ref('');
const albums = ref<AlbumView[]>([]);
const uploaded = ref<ImageItem[]>([]);
const errors = ref<string[]>([]);
const message = useMessage();

const albumOptions = computed(() => albums.value.map((a) => ({ label: a.name, value: a.id })));

async function uploadFiles(files: File[]) {
  const images = files.filter((f) => f.type.startsWith('image/'));
  if (!images.length) return;
  uploading.value = true;
  errors.value = [];
  progress.value = 0;
  for (const file of images) {
    const form = new FormData();
    form.append('files', file);
    if (albumTarget.value) form.append('album_id', String(albumTarget.value));
    if (tagsInput.value.trim()) form.append('tags', tagsInput.value.trim());
    try {
      const headers = new Headers();
      const token = getAccessToken();
      if (token) headers.set('Authorization', `Bearer ${token}`);
      const response = await fetch('/api/v1/upload', { method: 'POST', headers, body: form });
      const payload = await response.json();
      if (payload.code !== 0) throw new Error(payload.message ?? '上传失败');
      uploaded.value.push(...payload.data.items);
    } catch (err) {
      errors.value.push(`${file.name}: ${err instanceof Error ? err.message : '上传失败'}`);
    }
    progress.value = Math.round(((images.indexOf(file) + 1) / images.length) * 100);
  }
  uploading.value = false;
  message.success(`上传完成 ${images.length - errors.value.length}/${images.length}`);
}

function handlePick(event: Event) {
  const target = event.target as HTMLInputElement;
  if (target.files) uploadFiles(Array.from(target.files));
  target.value = '';
}

function handleDrop(event: DragEvent) {
  dragover.value = false;
  if (event.dataTransfer?.files) uploadFiles(Array.from(event.dataTransfer.files));
}

function handlePaste(event: ClipboardEvent) {
  const files = Array.from(event.clipboardData?.files ?? []);
  if (files.length) uploadFiles(files);
}

function copyURL(url: string) {
  navigator.clipboard.writeText(url);
  message.success('已复制');
}

function copyAll() {
  navigator.clipboard.writeText(uploaded.value.map((i) => i.url).join('\n'));
  message.success('已复制全部');
}

onMounted(async () => {
  const result = await api.albums();
  albums.value = result.items;
  window.addEventListener('paste', handlePaste);
});
onUnmounted(() => window.removeEventListener('paste', handlePaste));
</script>
