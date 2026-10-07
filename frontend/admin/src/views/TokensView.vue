<template>
  <div class="page">
    <n-space justify="space-between" align="center" style="margin-bottom: 14px">
      <n-h2 style="margin: 0">API Token</n-h2>
      <n-button type="primary" @click="showCreate = true">新建 Token</n-button>
    </n-space>

    <n-alert type="info" style="margin-bottom: 14px">
      PicGo 自定义 Web 上传地址：<code>{{ origin }}/api/v1/upload?picgo=1</code>，POST JSON
      <code>{ body: multipart, headers: { Authorization: "Bearer isl_xxx" } }</code>
    </n-alert>

    <n-data-table :columns="columns" :data="tokens" :bordered="false" size="small" />

    <n-modal :show="showCreate" @update:show="(v: boolean) => (showCreate = v)">
      <n-card style="width: 420px" title="新建 Token" :bordered="false">
        <n-form label-placement="left" label-width="80">
          <n-form-item label="名称"><n-input v-model:value="name" placeholder="如：PicGo-笔记本" /></n-form-item>
          <n-form-item label="过期时间">
            <n-date-picker v-model:value="expireTs" type="datetime" clearable style="width: 100%" />
          </n-form-item>
        </n-form>
        <template #footer>
          <n-space justify="end">
            <n-button @click="showCreate = false">取消</n-button>
            <n-button type="primary" @click="handleCreate">创建</n-button>
          </n-space>
        </template>
      </n-card>
    </n-modal>

    <n-modal :show="!!createdToken" @update:show="(v: boolean) => (v || (createdToken = ''))">
      <n-card style="width: 520px" title="Token 已创建" :bordered="false">
        <n-alert type="warning" style="margin-bottom: 10px">仅此一次展示，请立即复制保存</n-alert>
        <n-input :value="createdToken" readonly type="textarea" :rows="2" />
        <template #footer>
          <n-space justify="end">
            <n-button type="primary" @click="copyCreated">复制</n-button>
          </n-space>
        </template>
      </n-card>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { h, onMounted, ref } from 'vue';
import { NAlert, NButton, NCard, NDataTable, NDatePicker, NForm, NFormItem, NH2, NInput, NModal, NSpace, NTag, useMessage, type DataTableColumns } from 'naive-ui';
import { api, type TokenView } from '../api';

const tokens = ref<TokenView[]>([]);
const showCreate = ref(false);
const name = ref('');
const expireTs = ref<number | null>(null);
const createdToken = ref('');
const message = useMessage();

const origin = window.location.origin;

const columns: DataTableColumns<TokenView> = [
  { title: '名称', key: 'name' },
  {
    title: '状态',
    key: 'status',
    render: (row) => h(NTag, { size: 'small', type: row.status === 'active' ? 'success' : 'default' }, () => (row.status === 'active' ? '启用' : '已吊销'))
  },
  { title: '创建时间', key: 'created_at', render: (row) => new Date(row.created_at).toLocaleString() },
  { title: '最后使用', key: 'last_used_at', render: (row) => (row.last_used_at ? new Date(row.last_used_at).toLocaleString() : '—') },
  {
    title: '操作',
    key: 'actions',
    render: (row) =>
      row.status === 'active'
        ? h(NButton, { size: 'tiny', type: 'error', text: true, onClick: () => handleRevoke(row) }, () => '吊销')
        : '—'
  }
];

async function reload() {
  tokens.value = (await api.tokens()).items;
}

async function handleCreate() {
  try {
    const result = await api.createToken(name.value, expireTs.value ? new Date(expireTs.value).toISOString() : undefined);
    createdToken.value = result.token;
    showCreate.value = false;
    name.value = '';
    expireTs.value = null;
    await reload();
  } catch (err) {
    message.error(err instanceof Error ? err.message : '创建失败');
  }
}

async function handleRevoke(token: TokenView) {
  await api.revokeToken(token.id);
  message.success('已吊销');
  await reload();
}

function copyCreated() {
  navigator.clipboard.writeText(createdToken.value);
  message.success('已复制');
  createdToken.value = '';
}

onMounted(reload);
</script>
