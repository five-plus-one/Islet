<template>
  <div class="page">
    <n-space justify="space-between" align="center" style="margin-bottom: 14px">
      <n-h2 style="margin: 0">存储策略</n-h2>
      <n-button type="primary" @click="openCreate">新建策略</n-button>
    </n-space>

    <n-data-table :columns="columns" :data="policies" :bordered="false" size="small" />

    <n-modal :show="showEditor" @update:show="(v: boolean) => (showEditor = v)">
      <n-card style="width: 560px" :title="form.id ? '编辑策略' : '新建策略'" :bordered="false">
        <n-form label-placement="left" label-width="120">
          <n-form-item label="名称"><n-input v-model:value="form.name" /></n-form-item>
          <n-form-item label="驱动">
            <n-radio-group v-model:value="form.driver" :disabled="!!form.id">
              <n-radio-button value="local">本地磁盘</n-radio-button>
              <n-radio-button value="s3">S3 / OSS</n-radio-button>
            </n-radio-group>
          </n-form-item>

          <template v-if="form.driver === 's3'">
            <n-form-item label="Endpoint"><n-input v-model:value="form.endpoint" placeholder="https://oss-cn-hangzhou.aliyuncs.com" /></n-form-item>
            <n-form-item label="Region"><n-input v-model:value="form.region" placeholder="cn-hangzhou" /></n-form-item>
            <n-form-item label="Bucket"><n-input v-model:value="form.bucket" /></n-form-item>
            <n-form-item label="Prefix"><n-input v-model:value="form.prefix" placeholder="islet/" /></n-form-item>
            <n-form-item label="CDN / 直链域"><n-input v-model:value="form.cdn" placeholder="https://cdn.example.com（可空走代理）" /></n-form-item>
            <n-form-item label="Access Key"><n-input v-model:value="form.access_key" /></n-form-item>
            <n-form-item label="Secret Key">
              <n-input v-model:value="form.secret_key" type="password" show-password-on="click" :placeholder="form.id && secretConfigured ? '留空保留原值' : ''" />
            </n-form-item>
            <n-form-item label="Path Style"><n-switch v-model:value="form.path_style" /></n-form-item>
            <n-form-item label="访问模式">
              <n-radio-group v-model:value="form.url_mode">
                <n-radio-button value="direct">直链 / 302</n-radio-button>
                <n-radio-button value="proxy">经应用代理</n-radio-button>
              </n-radio-group>
            </n-form-item>
          </template>

          <n-form-item label="命名规则">
            <n-radio-group v-model:value="form.naming">
              <n-radio-button value="keep">保留原名</n-radio-button>
              <n-radio-button value="random">随机串</n-radio-button>
              <n-radio-button value="date">日期 + 随机</n-radio-button>
            </n-radio-group>
          </n-form-item>
          <n-form-item label="设为默认"><n-switch v-model:value="form.is_default" /></n-form-item>
          <n-form-item label="状态">
            <n-switch v-model:value="statusActive" checked-value="active" unchecked-value="disabled" />
          </n-form-item>
        </n-form>
        <n-alert v-if="editorError" type="error" size="small">{{ editorError }}</n-alert>
        <template #footer>
          <n-space justify="space-between">
            <n-button :loading="testing" @click="handleTest">测试连接</n-button>
            <n-space>
              <n-button @click="showEditor = false">取消</n-button>
              <n-button type="primary" :loading="saving" @click="handleSave">保存</n-button>
            </n-space>
          </n-space>
        </template>
      </n-card>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue';
import {
  NAlert, NButton, NCard, NDataTable, NForm, NFormItem, NH2, NInput, NModal,
  NRadio, NRadioButton, NRadioGroup, NSpace, NSwitch, NTag, useMessage, useDialog, type DataTableColumns
} from 'naive-ui';
import { api, type PolicyInput, type PolicyView } from '../api';

const policies = ref<PolicyView[]>([]);
const showEditor = ref(false);
const saving = ref(false);
const testing = ref(false);
const editorError = ref('');
const secretConfigured = ref(false);
const message = useMessage();
const dialog = useDialog();

const emptyForm = (): PolicyInput => ({
  name: '', driver: 'local', url_mode: 'direct', naming: 'date', is_default: false,
  status: 'active', endpoint: '', region: '', bucket: '', prefix: '', cdn: '',
  access_key: '', secret_key: '', path_style: false
});
const form = ref<PolicyInput>(emptyForm());
const statusActive = computed({
  get: () => form.value.status === 'active',
  set: (v: boolean) => (form.value.status = v ? 'active' : 'disabled')
});

const columns: DataTableColumns<PolicyView> = [
  { title: '名称', key: 'name', render: (row) => `${row.name}${row.is_default ? '（默认）' : ''}` },
  {
    title: '驱动',
    key: 'driver',
    render: (row) => h(NTag, { size: 'small' }, () => (row.driver === 'local' ? '本地磁盘' : 'S3 / OSS'))
  },
  { title: '命名', key: 'naming', render: (row) => ({ keep: '保留原名', random: '随机', date: '日期+随机' }[row.naming]) },
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
        h(NButton, { size: 'tiny', text: true, type: 'primary', onClick: () => handleTestById(row) }, () => '测试'),
        h(NButton, { size: 'tiny', text: true, onClick: () => openEdit(row) }, () => '编辑'),
        row.id !== 1
          ? h(NButton, { size: 'tiny', text: true, type: 'error', onClick: () => handleDelete(row) }, () => '删除')
          : null
      ])
  }
];

async function reload() {
  policies.value = (await api.policies()).items;
}

function openCreate() {
  form.value = emptyForm();
  secretConfigured.value = false;
  editorError.value = '';
  showEditor.value = true;
}

function openEdit(row: PolicyView) {
  form.value = {
    id: row.id, name: row.name, driver: row.driver, url_mode: row.url_mode, naming: row.naming,
    is_default: row.is_default, status: row.status,
    endpoint: row.endpoint ?? '', region: row.region ?? '', bucket: row.bucket ?? '',
    prefix: row.prefix ?? '', cdn: row.cdn ?? '', access_key: row.access_key ?? '',
    secret_key: '', path_style: row.path_style ?? false
  };
  secretConfigured.value = row.secret_configured;
  editorError.value = '';
  showEditor.value = true;
}

async function handleTest() {
  testing.value = true;
  editorError.value = '';
  try {
    const result = await api.testPolicy(form.value);
    message.success(result.message);
  } catch (err) {
    editorError.value = err instanceof Error ? err.message : '测试失败';
  } finally {
    testing.value = false;
  }
}

async function handleTestById(row: PolicyView) {
  try {
    const result = await api.testPolicyById(row.id);
    message.success(`${row.name}: ${result.message}`);
  } catch (err) {
    message.error(`${row.name}: ${err instanceof Error ? err.message : '测试失败'}`);
  }
}

async function handleSave() {
  saving.value = true;
  editorError.value = '';
  try {
    await api.savePolicy(form.value);
    showEditor.value = false;
    message.success('已保存');
    await reload();
  } catch (err) {
    editorError.value = err instanceof Error ? err.message : '保存失败';
  } finally {
    saving.value = false;
  }
}

function handleDelete(row: PolicyView) {
  dialog.warning({
    title: '删除存储策略',
    content: `确定删除策略「${row.name}」？已有图片不受影响。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await api.deletePolicy(row.id);
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
