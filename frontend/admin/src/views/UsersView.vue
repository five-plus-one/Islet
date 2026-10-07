<template>
  <div class="page">
    <n-space justify="space-between" align="center" style="margin-bottom: 14px">
      <n-h2 style="margin: 0">用户管理</n-h2>
      <n-space>
        <n-select v-model:value="statusFilter" :options="statusOptions" style="width: 140px" @update:value="reload" />
        <n-input v-model:value="keyword" placeholder="搜索用户名 / 邮箱" style="width: 200px" @keyup.enter="reload" />
      </n-space>
    </n-space>

    <n-data-table :columns="columns" :data="users" :bordered="false" size="small" :row-key="(r: UserView) => r.id" />
  </div>
</template>

<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue';
import {
  NButton, NDataTable, NH2, NInput, NSelect, NSpace, NSwitch, NTag,
  useDialog, useMessage, type DataTableColumns, type SelectOption
} from 'naive-ui';
import { api, type GroupView, type UserView } from '../api';
import { useAuthStore } from '../stores/auth';

const users = ref<UserView[]>([]);
const groups = ref<GroupView[]>([]);
const keyword = ref('');
const statusFilter = ref<string | null>(null);
const message = useMessage();
const dialog = useDialog();
const auth = useAuthStore();

const isSuper = computed(() => auth.user?.role === 'super_admin');

const statusOptions: SelectOption[] = [
  { label: '全部状态', value: null as unknown as string },
  { label: '待审核', value: 'pending' },
  { label: '正常', value: 'active' },
  { label: '停用', value: 'disabled' }
];

const groupOptions = computed(() => groups.value.map((g) => ({ label: g.name, value: g.id })));

const columns = computed<DataTableColumns<UserView>>(() => [
  { title: 'ID', key: 'id', width: 60 },
  { title: '用户名', key: 'username' },
  { title: '邮箱', key: 'email', render: (row) => row.email || '—' },
  { title: '来源', key: 'auth_source', render: (row) => (row.auth_source === 'oidc' ? 'OIDC' : '本地') },
  {
    title: '角色',
    key: 'role',
    render: (row) =>
      isSuper.value && row.role !== 'super_admin'
        ? h(NSelect, {
            value: row.role,
            options: [
              { label: '管理员', value: 'admin' },
              { label: '用户', value: 'user' }
            ],
            size: 'tiny',
            style: 'width: 110px',
            onUpdateValue: (v: string) => handleRole(row, v)
          })
        : h(NTag, { size: 'small', type: row.role === 'super_admin' ? 'success' : 'default' }, () => ({ super_admin: '超管', admin: '管理员', user: '用户' }[row.role] ?? row.role))
  },
  {
    title: '分组',
    key: 'group_name',
    render: (row) =>
      h(NSelect, {
        value: row.group_id,
        options: groupOptions.value,
        size: 'tiny',
        style: 'width: 140px',
        onUpdateValue: (v: number) => handleGroup(row, v)
      })
  },
  {
    title: '状态',
    key: 'status',
    render: (row) => {
      if (row.role === 'super_admin') {
        return h(NTag, { size: 'small', type: 'success' }, () => ({ pending: '待审核', active: '正常', disabled: '停用' }[row.status] ?? row.status));
      }
      return h(NSwitch, {
        value: row.status === 'active',
        size: 'small',
        onUpdateValue: (v: boolean) => handleStatus(row, v ? 'active' : 'disabled')
      });
    }
  },
  { title: '最近登录', key: 'last_login_at', render: (row) => (row.last_login_at ? new Date(row.last_login_at).toLocaleString() : '—') }
]);

async function reload() {
  const params = new URLSearchParams();
  if (keyword.value) params.set('keyword', keyword.value);
  if (statusFilter.value) params.set('status', statusFilter.value);
  const [userData, groupData] = await Promise.all([api.users(`?${params.toString()}`), api.groups()]);
  users.value = userData.items;
  groups.value = groupData.items;
}

async function handleGroup(row: UserView, groupId: number) {
  try {
    await api.setUserGroup(row.id, groupId);
    row.group_id = groupId;
    message.success('已更新分组');
  } catch (err) {
    message.error(err instanceof Error ? err.message : '更新失败');
  }
}

async function handleRole(row: UserView, role: string) {
  try {
    await api.setUserRole(row.id, role);
    row.role = role;
    message.success('已更新角色');
  } catch (err) {
    message.error(err instanceof Error ? err.message : '更新失败');
  }
}

function handleStatus(row: UserView, status: string) {
  const action = status === 'disabled' ? '停用' : status === 'pending' ? '设为待审核' : '启用';
  dialog.warning({
    title: `${action}账号`,
    content: `确定${action}「${row.username}」？${status === 'disabled' ? '停用后其会话与 Token 全部失效。' : ''}`,
    positiveText: action,
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await api.setUserStatus(row.id, status);
        row.status = status;
        message.success(`已${action}`);
      } catch (err) {
        message.error(err instanceof Error ? err.message : '操作失败');
      }
    }
  });
}

onMounted(reload);
</script>
