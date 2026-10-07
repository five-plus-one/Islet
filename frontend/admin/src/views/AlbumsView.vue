<template>
  <div class="page">
    <n-space justify="space-between" align="center" style="margin-bottom: 14px">
      <n-h2 style="margin: 0">相册</n-h2>
      <n-button type="primary" @click="showCreate = true">新建相册</n-button>
    </n-space>

    <n-grid :cols="3" :x-gap="14" :y-gap="14">
      <n-gi v-for="album in albums" :key="album.id">
        <n-card :title="album.name" size="small">
          <n-text depth="3">{{ album.description || '暂无描述' }}</n-text>
          <n-space justify="space-between" style="margin-top: 10px">
            <n-text depth="3">{{ album.image_count ?? 0 }} 张图片</n-text>
            <n-space size="small">
              <n-button text size="small" type="primary" @click="router.push({ name: 'images', query: { album: album.id } })">查看</n-button>
              <n-button text size="small" @click="startEdit(album)">编辑</n-button>
              <n-button text size="small" type="error" @click="handleDelete(album)">删除</n-button>
            </n-space>
          </n-space>
        </n-card>
      </n-gi>
    </n-grid>
    <n-empty v-if="!albums.length" description="还没有相册" style="margin-top: 80px" />

    <n-modal :show="showCreate" @update:show="(v: boolean) => (showCreate = v)">
      <n-card style="width: 420px" :title="editing ? '编辑相册' : '新建相册'" :bordered="false">
        <n-form label-placement="left" label-width="70">
          <n-form-item label="名称"><n-input v-model:value="form.name" /></n-form-item>
          <n-form-item label="描述"><n-input v-model:value="form.description" type="textarea" :rows="2" /></n-form-item>
        </n-form>
        <template #footer>
          <n-space justify="end">
            <n-button @click="showCreate = false">取消</n-button>
            <n-button type="primary" @click="handleSave">保存</n-button>
          </n-space>
        </template>
      </n-card>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { NButton, NCard, NEmpty, NForm, NFormItem, NGrid, NGi, NH2, NInput, NModal, NSpace, NText, useMessage } from 'naive-ui';
import { api, type AlbumView } from '../api';
import { useRouter } from 'vue-router';

const albums = ref<AlbumView[]>([]);
const showCreate = ref(false);
const editing = ref<AlbumView | null>(null);
const form = ref({ name: '', description: '' });
const message = useMessage();
const router = useRouter();

async function reload() {
  albums.value = (await api.albums()).items;
}

function startEdit(album: AlbumView) {
  editing.value = album;
  form.value = { name: album.name, description: album.description };
  showCreate.value = true;
}

async function handleSave() {
  try {
    if (editing.value) {
      await api.updateAlbum(editing.value.id, form.value.name, form.value.description);
    } else {
      await api.createAlbum(form.value.name, form.value.description);
    }
    showCreate.value = false;
    editing.value = null;
    form.value = { name: '', description: '' };
    message.success('已保存');
    await reload();
  } catch (err) {
    message.error(err instanceof Error ? err.message : '保存失败');
  }
}

async function handleDelete(album: AlbumView) {
  try {
    await api.deleteAlbum(album.id);
    message.success('已删除');
    await reload();
  } catch (err) {
    message.error(err instanceof Error ? err.message : '删除失败');
  }
}

onMounted(reload);
</script>
