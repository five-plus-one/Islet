<template>
  <div class="login-page">
    <div class="login-card">
      <div class="login-brand">
        <img v-if="config?.brand_logo" :src="config.brand_logo" alt="logo" style="height: 32px" />
        <span>{{ config?.brand_name || 'Islet 图床' }}</span>
      </div>
      <n-alert v-if="errorMessage" type="error" size="small">{{ errorMessage }}</n-alert>

      <template v-if="config && config.authorization_endpoint">
        <n-button block size="large" type="primary" secondary @click="handleOIDCLogin">
          <span style="display: inline-flex; align-items: center; gap: 8px">
            <img v-if="config.oidc_logo" :src="config.oidc_logo" style="height: 18px" alt="" />
            {{ config.oidc_label }}
          </span>
        </n-button>
        <n-divider v-if="config.local_enabled" style="margin: 4px 0">
          <n-text depth="3" style="font-size: 12px">或使用本地账号</n-text>
        </n-divider>
      </template>

      <template v-if="config?.local_enabled">
        <n-input v-model:value="username" placeholder="用户名" @keyup.enter="handleLocalLogin" />
        <n-input v-model:value="password" type="password" show-password-on="click" placeholder="密码" @keyup.enter="handleLocalLogin" />
        <n-button type="primary" block :loading="loading" @click="handleLocalLogin">登录</n-button>
      </template>

      <n-text v-if="errorMessage?.includes('待审核')" type="warning" style="font-size: 12px; text-align: center">
        账号待审核，请联系管理员开通
      </n-text>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { NAlert, NButton, NDivider, NInput, NText, useMessage } from 'naive-ui';
import { api, beginOIDCLogin, type AuthConfig } from '../api';
import { useAuthStore } from '../stores/auth';

const config = ref<AuthConfig | null>(null);
const username = ref('');
const password = ref('');
const loading = ref(false);
const errorMessage = ref('');
const route = useRoute();
const router = useRouter();
const message = useMessage();
const auth = useAuthStore();

onMounted(async () => {
  try {
    config.value = await api.authConfig();
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '加载配置失败';
  }
});

async function handleLocalLogin() {
  if (!username.value || !password.value) {
    errorMessage.value = '请输入用户名和密码';
    return;
  }
  loading.value = true;
  errorMessage.value = '';
  try {
    const user = await api.localLogin(username.value, password.value);
    await auth.fetchUser();
    message.success(`欢迎，${user.username}`);
    await router.push(String(route.query.redirect ?? '/'));
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '登录失败';
  } finally {
    loading.value = false;
  }
}

async function handleOIDCLogin() {
  if (!config.value) return;
  try {
    await beginOIDCLogin(config.value);
  } catch (err) {
    errorMessage.value = err instanceof Error ? err.message : '无法发起 OIDC 登录';
  }
}
</script>
