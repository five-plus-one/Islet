<template>
  <div class="page">
    <n-h2>系统设置</n-h2>
    <n-grid :cols="2" :x-gap="14">
      <n-gi>
        <n-card title="OIDC 认证" size="small" style="margin-bottom: 14px">
          <n-alert v-if="!oidcIssuer" type="warning" size="small" style="margin-bottom: 10px">
            未配置 OIDC 时登录页仅显示本地账号
          </n-alert>
          <n-form label-placement="left" label-width="150">
            <n-form-item label="Issuer">
              <n-input v-model:value="oidcIssuer" placeholder="https://sso.example.com" />
            </n-form-item>
            <n-form-item label="Client ID">
              <n-input v-model:value="oidcClientID" />
            </n-form-item>
            <n-form-item label="Audience（可选）">
              <n-input v-model:value="oidcAudience" placeholder="留空则使用 Client ID" />
            </n-form-item>
            <n-form-item label="Scopes">
              <n-input v-model:value="oidcScopes" placeholder="openid profile email" />
            </n-form-item>
            <n-form-item label="本地登录">
              <n-switch v-model:value="localEnabled" />
              <n-text depth="3" style="margin-left: 8px; font-size: 12px">关闭后登录页仅显示 OIDC 按钮</n-text>
            </n-form-item>
          </n-form>
          <n-alert v-if="oidcTestResult" :type="oidcTestOk ? 'success' : 'error'" size="small" closable @close="oidcTestResult = ''">
            {{ oidcTestResult }}
          </n-alert>
          <template #footer>
            <n-space justify="space-between">
              <n-button :loading="testingOIDC" @click="handleTestOIDC">检查 Discovery</n-button>
              <n-text depth="3" style="font-size: 12px">保存后即时生效，无需重启</n-text>
            </n-space>
          </template>
        </n-card>
        <n-card title="品牌与登录" size="small">
          <n-form label-placement="left" label-width="150">
            <n-form-item label="品牌名称"><n-input v-model:value="values['brand.name']" /></n-form-item>
            <n-form-item label="品牌 Logo URL"><n-input v-model:value="values['brand.logo_url']" placeholder="https://..." /></n-form-item>
            <n-form-item label="OIDC 按钮名称"><n-input v-model:value="values['brand.oidc_label']" placeholder="OIDC 登录" /></n-form-item>
            <n-form-item label="OIDC 按钮图标">
              <n-input v-model:value="values['brand.oidc_logo_url']" placeholder="https://... 图标 URL" />
            </n-form-item>
          </n-form>
        </n-card>
      </n-gi>
      <n-gi>
        <n-card title="站点与上传" size="small">
          <n-form label-placement="left" label-width="150">
            <n-form-item label="站点名称"><n-input v-model:value="values['site.title']" /></n-form-item>
            <n-form-item label="图床公开地址">
              <n-input v-model:value="values['site.public_base_url']" placeholder="https://img.example.com" />
            </n-form-item>
            <n-form-item label="OIDC 自动通过">
              <n-switch v-model:value="autoApprove" />
            </n-form-item>
            <n-form-item label="回收站保留天数">
              <n-input-number v-model:value="retentionDays" :min="0" :max="3650" style="width: 100%">
                <template #suffix>0 = 永久保留</template>
              </n-input-number>
            </n-form-item>
            <n-form-item label="单文件上限 (MB)">
              <n-input-number v-model:value="maxBytesMB" :min="1" :max="4096" style="width: 100%" />
            </n-form-item>
          </n-form>
        </n-card>
      </n-gi>
    </n-grid>

    <n-space justify="space-between" style="margin-top: 18px">
      <n-popconfirm @positive-click="handleReset">
        <template #trigger>
          <n-button type="error" ghost>恢复初始化…</n-button>
        </template>
        <n-space vertical>
          <span>将清除全部配置、账号状态并回到初始化向导（图片数据保留）。输入 RESET 确认：</span>
          <n-input v-model:value="resetConfirm" size="small" />
          <n-input v-if="isLocalSuper" v-model:value="resetPassword" type="password" size="small" placeholder="当前管理员密码" />
        </n-space>
      </n-popconfirm>
      <n-button type="primary" :loading="saving" @click="handleSave">保存设置</n-button>
    </n-space>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { NAlert, NButton, NCard, NForm, NFormItem, NGrid, NGi, NH2, NInput, NInputNumber, NPopconfirm, NSpace, NSwitch, NText, useMessage } from 'naive-ui';
import { api } from '../api';
import { useAuthStore } from '../stores/auth';
import { useRouter } from 'vue-router';

const values = ref<Record<string, string>>({});
const saving = ref(false);
const resetConfirm = ref('');
const resetPassword = ref('');
const testingOIDC = ref(false);
const oidcTestResult = ref('');
const oidcTestOk = ref(false);
const message = useMessage();
const auth = useAuthStore();
const router = useRouter();

const oidcIssuer = computed({
  get: () => values.value['auth.oidc_issuer'] ?? '',
  set: (v: string) => (values.value['auth.oidc_issuer'] = v)
});
const oidcClientID = computed({
  get: () => values.value['auth.oidc_client_id'] ?? '',
  set: (v: string) => (values.value['auth.oidc_client_id'] = v)
});
const oidcAudience = computed({
  get: () => values.value['auth.oidc_audience'] ?? '',
  set: (v: string) => (values.value['auth.oidc_audience'] = v)
});
const oidcScopes = computed({
  get: () => values.value['auth.oidc_scopes'] ?? '',
  set: (v: string) => (values.value['auth.oidc_scopes'] = v)
});
const localEnabled = computed({
  get: () => (values.value['auth.local_enabled'] ?? 'true') !== 'false',
  set: (v: boolean) => (values.value['auth.local_enabled'] = String(v))
});

async function handleTestOIDC() {
  if (!oidcIssuer.value.trim()) {
    oidcTestOk.value = false;
    oidcTestResult.value = '请先填写 Issuer';
    return;
  }
  testingOIDC.value = true;
  try {
    const meta = await api.adminTestOIDC(oidcIssuer.value.trim());
    oidcTestOk.value = true;
    oidcTestResult.value = `Discovery 通过：${meta.issuer}（授权端点 ${new URL(meta.authorization_endpoint).host}）`;
  } catch (err) {
    oidcTestOk.value = false;
    oidcTestResult.value = err instanceof Error ? err.message : 'Discovery 检查失败';
  } finally {
    testingOIDC.value = false;
  }
}

const autoApprove = computed({
  get: () => values.value['users.auto_approve'] === 'true',
  set: (v: boolean) => (values.value['users.auto_approve'] = String(v))
});
const retentionDays = computed({
  get: () => Number(values.value['recycle.retention_days'] ?? 30),
  set: (v: number | null) => (values.value['recycle.retention_days'] = String(v ?? 30))
});
const maxBytesMB = computed({
  get: () => Math.round(Number(values.value['upload.max_bytes'] ?? 104857600) / (1024 * 1024)),
  set: (v: number | null) => (values.value['upload.max_bytes'] = String(Math.round((v ?? 100) * 1024 * 1024)))
});
const isLocalSuper = computed(() => auth.user?.auth_source === 'local');

async function handleSave() {
  saving.value = true;
  try {
    values.value = await api.saveSettings(values.value);
    message.success('已保存，即时生效');
  } catch (err) {
    message.error(err instanceof Error ? err.message : '保存失败');
  } finally {
    saving.value = false;
  }
}

async function handleReset() {
  if (resetConfirm.value !== 'RESET') {
    message.error('请输入 RESET 确认');
    return;
  }
  try {
    await api.resetSystem('RESET', resetPassword.value);
    message.success('已恢复初始化，请重新完成向导');
    setTimeout(() => router.push({ name: 'setup' }), 800);
  } catch (err) {
    message.error(err instanceof Error ? err.message : '重置失败');
  }
}

onMounted(async () => {
  values.value = await api.settings();
});
</script>
