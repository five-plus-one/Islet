<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import {
  NAlert,
  NButton,
  NCard,
  NForm,
  NFormItem,
  NInput,
  NModal,
  NSpace,
  NSkeleton,
  useDialog,
  useMessage,
} from "naive-ui";
import { api, type AlbumView } from "../api";
import { errorText } from "../utils";
import AppIcon from "../components/AppIcon.vue";
import PageHeader from "../components/PageHeader.vue";
import EmptyState from "../components/EmptyState.vue";
const router = useRouter(),
  message = useMessage(),
  dialog = useDialog();
const albums = ref<AlbumView[]>([]),
  loading = ref(true),
  error = ref(""),
  show = ref(false),
  saving = ref(false),
  formError = ref("");
const editing = ref<number | null>(null),
  form = ref({ name: "", description: "" });
async function load() {
  loading.value = true;
  error.value = "";
  try {
    albums.value = (await api.albums()).items;
  } catch (e) {
    error.value = errorText(e);
  } finally {
    loading.value = false;
  }
}
function open(item?: AlbumView) {
  editing.value = item?.id || null;
  form.value = { name: item?.name || "", description: item?.description || "" };
  formError.value = "";
  show.value = true;
}
async function save() {
  if (saving.value) return;
  if (!form.value.name.trim()) {
    formError.value = "请填写相册名称";
    return;
  }
  saving.value = true;
  try {
    if (editing.value)
      await api.updateAlbum(
        editing.value,
        form.value.name.trim(),
        form.value.description,
      );
    else await api.createAlbum(form.value.name.trim(), form.value.description);
    show.value = false;
    message.success(editing.value ? "相册已更新" : "相册已创建");
    await load();
  } catch (e) {
    formError.value = errorText(e);
  } finally {
    saving.value = false;
  }
}
function remove(item: AlbumView) {
  dialog.warning({
    title: "删除相册",
    content: `确定删除「${item.name}」？相册内的图片需要先移出。`,
    positiveText: "删除相册",
    negativeText: "取消",
    onPositiveClick: async () => {
      try {
        await api.deleteAlbum(item.id);
        message.success("相册已删除");
        await load();
      } catch (e) {
        message.error(errorText(e));
        return false;
      }
    },
  });
}
onMounted(load);
</script>
<template>
  <div class="page">
    <PageHeader
      title="相册"
      description="把相关的图片放在一起，给灵感留一处归所。"
      eyebrow="YOUR COLLECTIONS"
      ><n-button type="primary" size="large" @click="open()"
        ><template #icon><AppIcon name="plus" :size="18" /></template
        >新建相册</n-button
      ></PageHeader
    ><n-alert v-if="error" type="error"
      >{{ error }} <n-button text @click="load">重试</n-button></n-alert
    >
    <div v-if="loading" class="album-grid">
      <n-skeleton v-for="n in 3" :key="n" height="260px" />
    </div>
    <div v-else-if="albums.length" class="album-grid">
      <article
        v-for="album in albums"
        :key="album.id"
        class="surface album-card"
      >
        <button
          class="album-cover"
          style="border: 0; width: 100%"
          :aria-label="'打开相册 ' + album.name"
          @click="router.push({ name: 'images', query: { album: album.id } })"
        >
          <AppIcon name="album" :size="43" />
        </button>
        <h3>{{ album.name }}</h3>
        <p>{{ album.description || "记录这一刻的画面" }}</p>
        <div class="album-footer">
          <span class="subtle">{{ album.image_count || 0 }} 张图片</span
          ><n-space :size="12"
            ><n-button text size="small" type="primary" @click="open(album)"
              >编辑</n-button
            ><n-button text size="small" @click="remove(album)"
              >删除</n-button
            ></n-space
          >
        </div>
      </article>
    </div>
    <EmptyState
      v-else-if="!error"
      title="为图片创建一个相册"
      description="按项目、主题或旅程整理，查找时更轻松。"
      icon="album"
      ><n-button type="primary" @click="open()">新建相册</n-button></EmptyState
    ><n-modal v-model:show="show" :mask-closable="!saving"
      ><n-card
        style="width: 460px"
        :title="editing ? '编辑相册' : '新建相册'"
        :bordered="false"
        ><n-alert v-if="formError" type="error" style="margin-bottom: 15px">{{
          formError
        }}</n-alert
        ><n-form label-placement="top"
          ><n-form-item label="相册名称" required
            ><n-input
              v-model:value="form.name"
              placeholder="如：旅行日记"
              :maxlength="64"
              @keyup.enter="save" /></n-form-item
          ><n-form-item label="描述"
            ><n-input
              v-model:value="form.description"
              type="textarea"
              placeholder="关于这个相册，想记住些什么？"
              :maxlength="255"
              :rows="3" /></n-form-item></n-form
        ><template #footer
          ><n-space justify="end"
            ><n-button :disabled="saving" @click="show = false">取消</n-button
            ><n-button type="primary" :loading="saving" @click="save">{{
              editing ? "保存更改" : "创建相册"
            }}</n-button></n-space
          ></template
        ></n-card
      ></n-modal
    >
  </div>
</template>
