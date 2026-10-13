<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import {
  NAlert,
  NButton,
  NCard,
  NDescriptions,
  NDescriptionsItem,
  NForm,
  NFormItem,
  NInput,
  NProgress,
  NSpace,
  NTag,
  useMessage,
} from "naive-ui";
import { api, type MeInfo } from "../api";
import { useAuthStore } from "../stores/auth";
import { errorText, humanBytes, groupLabel } from "../utils";
import PageHeader from "../components/PageHeader.vue";
const info = ref<MeInfo | null>(null),
  error = ref(""),
  current = ref(""),
  password = ref(""),
  confirm = ref(""),
  saving = ref(false),
  passwordError = ref("");
const auth = useAuthStore(),
  router = useRouter(),
  message = useMessage();
const percent = computed(() =>
  info.value?.quota.limit
    ? Math.min(100, (info.value.quota.used / info.value.quota.limit) * 100)
    : 0,
);
const role = computed(
  () =>
    ({ super_admin: "超级管理员", admin: "管理员", user: "普通用户" })[
      info.value?.user.role || "user"
    ],
);
async function load() {
  error.value = "";
  try {
    info.value = await api.meInfo();
  } catch (e) {
    error.value = errorText(e);
  }
}
async function changePassword() {
  if (saving.value) return;
  passwordError.value = "";
  if (password.value !== confirm.value) {
    passwordError.value = "两次输入的新密码不一致";
    return;
  }
  if (!current.value || new TextEncoder().encode(password.value).length < 10) {
    passwordError.value = "请填写当前密码，新密码至少需要 10 字节";
    return;
  }
  saving.value = true;
  try {
    await api.changePassword(current.value, password.value);
    await auth.logout();
    message.success("密码已修改，请重新登录");
    router.replace("/login");
  } catch (e) {
    passwordError.value = errorText(e);
  } finally {
    saving.value = false;
  }
}
onMounted(load);
</script>
<template>
  <div class="page">
    <PageHeader
      title="个人中心"
      description="管理你的账户，了解图片空间的使用情况。"
      eyebrow="YOUR ACCOUNT"
    /><n-alert v-if="error" type="error"
      >{{ error }} <n-button text @click="load">重试</n-button></n-alert
    >
    <div class="two-column">
      <n-card title="账户信息" :bordered="false"
        ><n-descriptions
          :column="1"
          label-placement="left"
          :label-style="{ width: '100px', color: '#748298' }"
          ><n-descriptions-item label="用户名">{{
            info?.user.username || "—"
          }}</n-descriptions-item
          ><n-descriptions-item label="邮箱">{{
            info?.user.email || "未设置"
          }}</n-descriptions-item
          ><n-descriptions-item label="角色">{{ role }}</n-descriptions-item
          ><n-descriptions-item label="登录方式">{{
            info?.user.auth_source === "oidc" ? "统一登录" : "本地账户"
          }}</n-descriptions-item
          ><n-descriptions-item label="所属分组"
            ><n-tag size="small" :bordered="false" type="info">{{
              groupLabel(info?.group.name)
            }}</n-tag></n-descriptions-item
          ></n-descriptions
        ></n-card
      ><n-card title="图片空间" :bordered="false"
        ><div style="font-size: 32px; font-weight: 600; margin-bottom: 18px">
          {{ info ? humanBytes(info.quota.used) : "—" }}
          <span class="subtle">已使用</span>
        </div>
        <n-progress
          type="line"
          :percentage="percent"
          :show-indicator="false"
          :height="6"
        />
        <p class="subtle">
          容量上限：{{
            info?.quota.limit ? humanBytes(info.quota.limit) : "不限"
          }}
        </p>
        <n-descriptions :column="1" label-placement="left" size="small"
          ><n-descriptions-item label="单文件上限">{{
            info
              ? humanBytes(
                  Math.min(
                    info.upload_max_bytes,
                    auth.user?.role === "super_admin"
                      ? Infinity
                      : info.group.max_file_bytes || Infinity,
                  ),
                )
              : "—"
          }}</n-descriptions-item
          ><n-descriptions-item label="每小时上传">{{
            auth.user?.role === "super_admin" || !info?.group.uploads_per_hour
              ? "不限"
              : `${info.group.uploads_per_hour} 张`
          }}</n-descriptions-item
          ><n-descriptions-item label="支持格式">{{
            info?.group.allowed_mimes
              ?.map((x) => x.replace("image/", "").toUpperCase())
              .join("、") || "常见图片格式"
          }}</n-descriptions-item></n-descriptions
        ></n-card
      ><n-card
        v-if="info?.user.auth_source === 'local'"
        title="修改密码"
        :bordered="false"
        ><n-alert
          v-if="passwordError"
          type="error"
          style="margin-bottom: 15px"
          >{{ passwordError }}</n-alert
        ><n-form label-placement="top"
          ><n-form-item label="当前密码"
            ><n-input
              v-model:value="current"
              type="password"
              autocomplete="current-password"
              show-password-on="click" /></n-form-item
          ><n-form-item label="新密码"
            ><n-input
              v-model:value="password"
              type="password"
              autocomplete="new-password"
              show-password-on="click"
              placeholder="至少 10 字节" /></n-form-item
          ><n-form-item label="确认新密码"
            ><n-input
              v-model:value="confirm"
              type="password"
              autocomplete="new-password"
              show-password-on="click" /></n-form-item></n-form
        ><n-space justify="space-between" align="center"
          ><span class="subtle">修改后需要重新登录</span
          ><n-button type="primary" :loading="saving" @click="changePassword"
            >更新密码</n-button
          ></n-space
        ></n-card
      >
    </div>
  </div>
</template>
