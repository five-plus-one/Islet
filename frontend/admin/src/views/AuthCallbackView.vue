<template>
  <div class="login-page">
    <div class="login-card" style="text-align: center">
      <template v-if="mode === 'working'">
        <n-spin size="medium" />
        <p>正在完成登录…</p>
      </template>
      <template v-else-if="mode === 'setup'">
        <n-result status="success" title="管理员身份已验证" :description="`欢迎，${username}`">
          <template #footer>
            <n-button type="primary" @click="router.push({ name: 'setup' })">返回初始化向导</n-button>
          </template>
        </n-result>
      </template>
      <template v-else>
        <n-result status="success" title="登录成功" :description="`欢迎，${username}`">
          <template #footer>
            <n-button type="primary" @click="router.push('/')">进入控制台</n-button>
          </template>
        </n-result>
      </template>
      <n-alert v-if="mode === 'error'" type="error">{{ errorMessage }}</n-alert>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { NAlert, NButton, NResult, NSpin, useMessage } from 'naive-ui';
import { api, consumeOIDCState, saveSession } from '../api';
import { useAuthStore } from '../stores/auth';

const mode = ref<'working' | 'done' | 'setup' | 'error'>('working');
const username = ref('');
const errorMessage = ref('');
const route = useRoute();
const router = useRouter();
const message = useMessage();
const auth = useAuthStore();

onMounted(async () => {
  const isSetup = route.meta.setup === true;
  const error = new URLSearchParams(window.location.search).get('error');
  if (error) {
    mode.value = 'error';
    errorMessage.value = `授权失败：${error}`;
    return;
  }
  const code = new URLSearchParams(window.location.search).get('code');
  const pkce = consumeOIDCState();
  if (!code || !pkce) {
    mode.value = 'error';
    errorMessage.value = '回调参数缺失或 state 校验失败，请重新发起登录';
    return;
  }
  try {
    const redirectUri = `${window.location.origin}${isSetup ? '/setup/auth/callback' : '/auth/callback'}`;
    const result = await api.oidcExchange(code, pkce.verifier, redirectUri);
    saveSession(result.access_token, result.id_token);
    username.value = result.username || 'admin';
    if (isSetup) {
      // 向导流程：把已验证的 token 提交给后端暂存管理员身份
      await api.setupVerifyAdmin({ mode: 'oidc', token: result.access_token });
      mode.value = 'setup';
      return;
    }
    await auth.fetchUser();
    if (auth.user?.status === 'pending') {
      mode.value = 'error';
      errorMessage.value = '账号待审核，请联系管理员开通后再登录';
      return;
    }
    mode.value = 'done';
    message.success('登录成功');
    const target = String(route.query.redirect ?? '/');
    setTimeout(() => router.push(target), 600);
  } catch (err) {
    mode.value = 'error';
    errorMessage.value = err instanceof Error ? err.message : '登录失败';
  }
});
</script>
