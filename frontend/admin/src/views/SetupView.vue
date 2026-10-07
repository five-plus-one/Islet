<template>
  <div class="login-page">
    <div class="login-card" style="width: 560px">
      <div class="login-brand"><span class="brand-dot"></span>Islet 初始化向导</div>

      <n-alert v-if="alreadyInstalled" type="success">
        系统已完成初始化。<n-button text type="primary" @click="router.push({ name: 'login' })">前往登录</n-button>
      </n-alert>

      <n-steps v-else :current="step" size="small">
        <n-step title="数据服务" />
        <n-step title="管理员身份" />
        <n-step title="站点设置" />
        <n-step title="完成" />
      </n-steps>

      <template v-if="!alreadyInstalled && step === 1">
        <n-form label-placement="left" label-width="120">
          <n-form-item label="MySQL Host"><n-input v-model:value="db.mysql_host" /></n-form-item>
          <n-form-item label="端口"><n-input v-model:value="db.mysql_port" /></n-form-item>
          <n-form-item label="数据库"><n-input v-model:value="db.mysql_database" /></n-form-item>
          <n-form-item label="用户"><n-input v-model:value="db.mysql_user" /></n-form-item>
          <n-form-item label="密码"><n-input v-model:value="db.mysql_password" type="password" show-password-on="click" /></n-form-item>
          <n-divider style="margin: 8px 0" />
          <n-form-item label="Redis Host"><n-input v-model:value="db.redis_host" /></n-form-item>
          <n-form-item label="端口"><n-input v-model:value="db.redis_port" /></n-form-item>
          <n-form-item label="密码"><n-input v-model:value="db.redis_password" type="password" show-password-on="click" /></n-form-item>
        </n-form>
        <n-alert v-if="dbError" type="error" size="small">{{ dbError }}</n-alert>
        <n-button type="primary" block :loading="testing" @click="handleTestDatabase">测试连接并继续</n-button>
      </template>

      <template v-if="!alreadyInstalled && step === 2">
        <n-radio-group v-model:value="authMode" style="margin-bottom: 12px">
          <n-radio-button value="oidc">OIDC 单点登录</n-radio-button>
          <n-radio-button value="local">本地账号</n-radio-button>
        </n-radio-group>

        <template v-if="authMode === 'oidc'">
          <n-form label-placement="left" label-width="120">
            <n-form-item label="Issuer"><n-input v-model:value="oidcIssuer" placeholder="https://sso.example.com" /></n-form-item>
            <n-form-item label="Client ID"><n-input v-model:value="oidcClientID" /></n-form-item>
          </n-form>
          <n-alert v-if="oidcError" type="error" size="small">{{ oidcError }}</n-alert>
          <n-alert v-if="authConfigPayload?.authorization_endpoint" type="info" size="small">
            Discovery 校验通过。回调地址：<code>{{ authConfigPayload?.redirect_uri }}</code>
            （请加入 IdP 的允许列表，setup 用 /setup/auth/callback）
          </n-alert>
          <n-space vertical size="small">
            <n-button block :loading="testingOIDC" @click="handleTestOIDC">检查 Discovery</n-button>
            <n-button
              type="primary"
              block
              :disabled="!authConfigPayload?.authorization_endpoint || !oidcClientID"
              @click="handleOIDCVerify"
            >用 OIDC 登录验证管理员身份</n-button>
            <n-alert v-if="adminVerified" type="success" size="small">管理员身份已验证 ✓</n-alert>
          </n-space>
        </template>

        <template v-else>
          <n-form label-placement="left" label-width="120">
            <n-form-item label="用户名"><n-input v-model:value="localUsername" /></n-form-item>
            <n-form-item label="密码"><n-input v-model:value="localPassword" type="password" show-password-on="click" placeholder="至少 10 位" /></n-form-item>
            <n-form-item label="确认密码"><n-input v-model:value="localPassword2" type="password" show-password-on="click" /></n-form-item>
          </n-form>
          <n-alert v-if="localError" type="error" size="small">{{ localError }}</n-alert>
          <n-button type="primary" block @click="handleLocalVerify">保存管理员账号</n-button>
        </template>

        <n-button quaternary block style="margin-top: 10px" @click="step = 1">上一步</n-button>
      </template>

      <template v-if="!alreadyInstalled && step === 3">
        <n-form label-placement="left" label-width="140" class="form-narrow">
          <n-form-item label="站点名称"><n-input v-model:value="siteTitle" /></n-form-item>
          <n-form-item label="图床公开地址">
            <n-input v-model:value="publicBaseUrl" placeholder="https://img.example.com" />
          </n-form-item>
          <n-form-item label="默认缩略图">
            <n-text depth="3">最长边 480px，可稍后在系统设置调整</n-text>
          </n-form-item>
        </n-form>
        <n-space>
          <n-button quaternary @click="step = 2">上一步</n-button>
          <n-button type="primary" :loading="completing" @click="handleComplete">完成初始化</n-button>
        </n-space>
      </template>

      <template v-if="!alreadyInstalled && step === 4">
        <n-result status="success" title="初始化完成" :description="`管理员 ${ownerName} 已创建，业务已启用`">
          <template #footer>
            <n-button type="primary" @click="router.push({ name: 'login' })">前往登录</n-button>
          </template>
        </n-result>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import {
  NAlert, NButton, NDivider, NForm, NFormItem, NInput, NRadioButton, NRadioGroup,
  NResult, NStep, NSteps, NSpace, useMessage
} from 'naive-ui';
import { api, beginOIDCLogin, type AuthConfig } from '../api';

const router = useRouter();
const message = useMessage();

const step = ref(1);
const alreadyInstalled = ref(false);
const db = ref({
  mysql_host: '127.0.0.1', mysql_port: '3306', mysql_database: 'islet',
  mysql_user: 'islet', mysql_password: '', redis_host: '127.0.0.1', redis_port: '6379', redis_password: ''
});
const testing = ref(false);
const dbError = ref('');

const authMode = ref<'oidc' | 'local'>('oidc');
const oidcIssuer = ref('');
const oidcClientID = ref('');
const testingOIDC = ref(false);
const oidcError = ref('');
const authConfigPayload = ref<AuthConfig | null>(null);
const adminVerified = ref(false);

const localUsername = ref('');
const localPassword = ref('');
const localPassword2 = ref('');
const localError = ref('');

const siteTitle = ref('Islet 图床');
const publicBaseUrl = ref(window.location.origin.replace(/:\d+$/, ':18090'));
const completing = ref(false);
const ownerName = ref('');

onMounted(async () => {
  try {
    const status = await api.setupStatus();
    alreadyInstalled.value = status.status === 'ready';
    adminVerified.value = status.admin_verified;
    if (status.defaults) {
      db.value.mysql_host = status.defaults.mysql_host || db.value.mysql_host;
      db.value.mysql_port = status.defaults.mysql_port || db.value.mysql_port;
      db.value.mysql_database = status.defaults.mysql_database || db.value.mysql_database;
      db.value.mysql_user = status.defaults.mysql_user || db.value.mysql_user;
      db.value.redis_host = status.defaults.redis_host || db.value.redis_host;
      db.value.redis_port = status.defaults.redis_port || db.value.redis_port;
    }
    if (status.admin_verified) step.value = 3;
  } catch {
    // 状态接口异常时按未初始化处理
  }
});

async function handleTestDatabase() {
  testing.value = true;
  dbError.value = '';
  try {
    const result = await api.setupTestDatabase({ ...db.value });
    if (result.mysql) {
      message.success('MySQL 连接成功');
      step.value = 2;
    }
  } catch (err) {
    dbError.value = err instanceof Error ? err.message : '连接失败';
  } finally {
    testing.value = false;
  }
}

async function handleTestOIDC() {
  testingOIDC.value = true;
  oidcError.value = '';
  try {
    await api.setupTestOIDC(oidcIssuer.value, oidcClientID.value);
    authConfigPayload.value = await api.authConfig();
    message.success('Discovery 校验通过');
  } catch (err) {
    oidcError.value = err instanceof Error ? err.message : 'Discovery 校验失败';
    authConfigPayload.value = null;
  } finally {
    testingOIDC.value = false;
  }
}

async function handleOIDCVerify() {
  if (!authConfigPayload.value) return;
  // setup 流程用 /setup/auth/callback 回调，真实登录验证 IdP 配置
  const setupConfig: AuthConfig = {
    ...authConfigPayload.value,
    redirect_uri: `${window.location.origin}/setup/auth/callback`
  };
  sessionStorage.setItem('islet_setup_oidc_issuer', oidcIssuer.value);
  sessionStorage.setItem('islet_setup_oidc_client', oidcClientID.value);
  await beginOIDCLogin(setupConfig, setupConfig.redirect_uri);
}

async function handleLocalVerify() {
  localError.value = '';
  if (localPassword.value !== localPassword2.value) {
    localError.value = '两次密码不一致';
    return;
  }
  try {
    const result = await api.setupVerifyAdmin({ mode: 'local', username: localUsername.value, password: localPassword.value });
    message.success(`管理员 ${result.username} 已暂存`);
    adminVerified.value = true;
    step.value = 3;
  } catch (err) {
    localError.value = err instanceof Error ? err.message : '保存失败';
  }
}

async function handleComplete() {
  completing.value = true;
  try {
    // 数据库凭据只在浏览器内存中，最后一步随 complete 一并提交落盘
    const database = { ...db.value };
    const result = await api.setupComplete(database, siteTitle.value, publicBaseUrl.value);
    ownerName.value = result.owner;
    step.value = 4;
  } catch (err) {
    message.error(err instanceof Error ? err.message : '初始化失败');
  } finally {
    completing.value = false;
  }
}
</script>

<style scoped>
.brand-dot {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  background: #18a058;
  display: inline-block;
}
</style>
