<template>
  <div class="page">
    <n-space justify="space-between" align="center" style="margin-bottom: 14px">
      <n-h2 style="margin: 0">用户分组</n-h2>
      <n-button type="primary" @click="openCreate">新建分组</n-button>
    </n-space>

    <n-data-table :columns="columns" :data="groups" :bordered="false" size="small" />

    <n-modal :show="showEditor" @update:show="(v: boolean) => (showEditor = v)">
      <n-card style="width: 560px" :title="form.id ? '编辑分组' : '新建分组'" :bordered="false">
        <n-form label-placement="left" label-width="130">
          <n-form-item label="名称"><n-input v-model:value="form.name" /></n-form-item>
          <n-form-item label="描述"><n-input v-model:value="form.description" type="textarea" :rows="2" /></n-form-item>
          <n-form-item label="总配额 (MB)">
            <n-input-number v-model:value="quotaMB" :min="0" style="width: 100%">
              <template #suffix>0 = 不限</template>
            </n-input-number>
          </n-form-item>
          <n-form-item label="单文件上限 (MB)">
            <n-input-number v-model:value="maxFileMB" :min="0" style="width: 100%">
              <template #suffix>0 = 不限</template>
            </n-input-number>
          </n-form-item>
          <n-form-item label="每小时上传次数">
            <n-input-number v-model:value="form.uploads_per_hour" :min="0" style="width: 100%">
              <template #suffix>0 = 不限</template>
            </n-input-number>
          </n-form-item>
          <n-form-item label="允许格式">
            <n-select
              v-model:value="form.allowed_mimes"
              multiple
              tag
              :options="mimeOptions"
              placeholder="空 = 全部图片格式"
            />
          </n-form-item>
          <n-form-item label="可用存储策略">
            <n-select v-model:value="form.policy_ids" multiple :options="policyOptions" placeholder="空 = 全部策略" />
          </n-form-item>
          <n-form-item label="状态">
            <n-switch v-model:value="statusActive" checked-value="active" unchecked-value="disabled" />
          </n-form-item>
        </n-form>
        <template #footer>
          <n-space justify="end">
            <n-button @click="showEditor = false">取消</n-button>
            <n-button type="primary" @click="handleSave">保存</n-button>
          </n-space>
        </template>
      </n-card>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue';
import {
  NButton, NCard, NDataTable, NForm, NFormItem, NH2, NInput, NInputNumber, NModal,
  NSelect, NSpace, NSwitch, NTag, useDialog, useMessage, type DataTableColumns
} from 'naive-ui';
import { api, type GroupView, type PolicyView } from '../api';

const groups = ref<GroupView[]>([]);
const policies = ref<PolicyView[]>([]);
const showEditor = ref(false);
const form = ref<Partial<GroupView> & { name: string }>({ name: '' });
const message = useMessage();
const dialog = useDialog();

const mimeOptions = ['image/png', 'image/jpeg', 'image/gif', 'image/webp', 'image/*'].map((m) => ({ label: m, value: m }));
const policyOptions = computed(() => policies.value.map((p) => ({ label: p.name, value: p.id })));

const quotaMB = computed({
  get: () => Math.round((form.value.quota_bytes ?? 0) / (1024 * 1024)),
  set: (v: number | null) => (form.value.quota_bytes = Math.round((v ?? 0) * 1024 * 1024))
});
const maxFileMB = computed({
  get: () => Math.round((form.value.max_file_bytes ?? 0) / (1024 * 1024)),
  set: (v: number | null) => (form.value.max_file_bytes = Math.round((v ?? 0) * 1024 * 1024))
});
const statusActive = computed({
  get: () => (form.value.status ?? 'active') === 'active',
  set: (v: boolean) => (form.value.status = v ? 'active' : 'disabled')
});

const columns: DataTableColumns<GroupView> = [
  { title: '分组', key: 'name', render: (row) => `${row.name}${row.built_in ? '（内置）' : ''}` },
  { title: '成员', key: 'members' },
  { title: '配额', key: 'quota', render: (row) => (row.quota_bytes > 0 ? humanBytes(row.quota_bytes) : '不限') },
  { title: '单文件', key: 'max', render: (row) => (row.max_file_bytes > 0 ? humanBytes(row.max_file_bytes) : '不限') },
  { title: '频率/时', key: 'rate', render: (row) => (row.uploads_per_hour > 0 ? String(row.uploads_per_hour) : '不限') },
  {
    title: '状态',
    key: 'status',
    render: (row) => h(NTag, { size: 'small', type: row.status === 'active' ? 'success' : 'default' }, () => (row.status === 'active' ? '启用' : '停用'))
  },
  {
    title: '操作',
    key: 'actions',
    render: (row) =>
      h(NSpace, { size: 'small' }, () => [
        h(NButton, { size: 'tiny', text: true, type: 'primary', onClick: () => openEdit(row) }, () => '编辑'),
        !row.built_in ? h(NButton, { size: 'tiny', text: true, type: 'error', onClick: () => handleDelete(row) }, () => '删除') : null
      ])
  }
];

function humanBytes(n: number): string {
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let value = n;
  let i = 0;
  while (value >= 1024 && i < units.length - 1) {
    value /= 1024;
    i++;
  }
  return `${value.toFixed(i > 0 ? 1 : 0)} ${units[i]}`;
}

async function reload() {
  const [groupData, policyData] = await Promise.all([api.groups(), api.policies()]);
  groups.value = groupData.items;
  policies.value = policyData.items;
}

function openCreate() {
  form.value = { name: '', description: '', quota_bytes: 0, max_file_bytes: 0, uploads_per_hour: 0, allowed_mimes: [], policy_ids: [], status: 'active' };
  showEditor.value = true;
}

function openEdit(row: GroupView) {
  form.value = { ...row };
  showEditor.value = true;
}

async function handleSave() {
  try {
    await api.saveGroup(form.value);
    showEditor.value = false;
    message.success('已保存');
    await reload();
  } catch (err) {
    message.error(err instanceof Error ? err.message : '保存失败');
  }
}

function handleDelete(row: GroupView) {
  dialog.warning({
    title: '删除分组',
    content: `确定删除分组「${row.name}」？`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await api.deleteGroup(row.id);
        message.success('已删除');
        await reload();
      } catch (err) {
        message.error(err instanceof Error ? err.message : '删除失败');
      }
    }
  });
}

onMounted(reload);
</script>
