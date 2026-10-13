<template>
  <div class="page">
    <PageHeader
      title="用户管理"
      description="管理团队成员的访问权限与图片空间。"
      eyebrow="PEOPLE"
      ><n-button
        v-if="isSuper"
        type="primary"
        size="large"
        @click="
          createForm = { username: '', password: '', group_id: 2 };
          createError = '';
          createOpen = true;
        "
        >创建本地用户</n-button
      ></PageHeader
    >
    <div class="toolbar surface">
      <n-select
        v-model:value="statusFilter"
        :options="statusOptions"
        style="width: 140px"
        @update:value="reload"
      />
      <n-input
        v-model:value="keyword"
        placeholder="搜索用户名 / 邮箱"
        class="search-input"
        @keyup.enter="reload"
      />
      <n-button @click="reload">搜索</n-button>
    </div>

    <n-alert v-if="loadError" type="error" style="margin-bottom: 16px"
      >{{ loadError }} <n-button text @click="reload">重试</n-button></n-alert
    >
    <n-card :bordered="false"
      ><n-data-table
        :columns="columns"
        :data="users"
        :loading="loading"
        :bordered="false"
        :pagination="{ pageSize: 15 }"
        :scroll-x="950"
        :row-key="(r: UserView) => r.id"
    /></n-card>
    <n-modal v-model:show="createOpen" :mask-closable="!creating"
      ><n-card title="创建本地用户" style="width: 460px" :bordered="false"
        ><n-alert v-if="createError" type="error" style="margin-bottom: 15px">{{
          createError
        }}</n-alert
        ><n-form label-placement="top"
          ><n-form-item label="用户名"
            ><n-input
              v-model:value="createForm.username"
              :maxlength="64"
              autocomplete="off" /></n-form-item
          ><n-form-item label="初始密码"
            ><n-input
              v-model:value="createForm.password"
              type="password"
              show-password-on="click"
              autocomplete="new-password"
              placeholder="至少 10 字节" /></n-form-item
          ><n-form-item label="用户分组"
            ><n-select
              v-model:value="createForm.group_id"
              :options="groupOptions" /></n-form-item></n-form
        ><template #footer
          ><n-space justify="end"
            ><n-button :disabled="creating" @click="createOpen = false"
              >取消</n-button
            ><n-button type="primary" :loading="creating" @click="createLocal"
              >创建用户</n-button
            ></n-space
          ></template
        ></n-card
      ></n-modal
    >
  </div>
</template>

<script setup lang="ts">
import { computed, h, onMounted, ref } from "vue";
import PageHeader from "../components/PageHeader.vue";
import {
  NAlert,
  NCard,
  NModal,
  NForm,
  NFormItem,
  NButton,
  NDataTable,
  NH2,
  NInput,
  NSelect,
  NSpace,
  NSwitch,
  NTag,
  useDialog,
  useMessage,
  type DataTableColumns,
  type SelectOption,
} from "naive-ui";
import { api, type GroupView, type UserView } from "../api";
import { useAuthStore } from "../stores/auth";

const users = ref<UserView[]>([]);
const loading = ref(false),
  loadError = ref(""),
  createOpen = ref(false),
  creating = ref(false),
  createError = ref("");
const createForm = ref({ username: "", password: "", group_id: 2 });
async function createLocal() {
  if (creating.value) return;
  creating.value = true;
  createError.value = "";
  try {
    await api.createUser(
      createForm.value.username,
      createForm.value.password,
      createForm.value.group_id,
    );
    createForm.value.password = "";
    createOpen.value = false;
    message.success("本地用户已创建");
    await reload();
  } catch (e) {
    createError.value = e instanceof Error ? e.message : "创建失败";
  } finally {
    creating.value = false;
  }
}
const groups = ref<GroupView[]>([]);
const keyword = ref("");
const statusFilter = ref<string | null>(null);
const message = useMessage();
const dialog = useDialog();
const auth = useAuthStore();

const isSuper = computed(() => auth.user?.role === "super_admin");

const statusOptions: SelectOption[] = [
  { label: "全部状态", value: "" },
  { label: "待审核", value: "pending" },
  { label: "正常", value: "active" },
  { label: "停用", value: "disabled" },
];

const groupOptions = computed(() =>
  groups.value.map((g) => ({ label: g.name, value: g.id })),
);

const columns = computed<DataTableColumns<UserView>>(() => [
  { title: "用户名", key: "username", width: 160, ellipsis: { tooltip: true } },
  { title: "邮箱", key: "email", render: (row) => row.email || "—" },
  {
    title: "来源",
    key: "auth_source",
    render: (row) => (row.auth_source === "oidc" ? "OIDC" : "本地"),
  },
  {
    title: "角色",
    key: "role",
    render: (row) =>
      isSuper.value && row.role !== "super_admin"
        ? h(NSelect, {
            value: row.role,
            options: [
              { label: "管理员", value: "admin" },
              { label: "用户", value: "user" },
            ],
            size: "tiny",
            style: "width: 110px",
            onUpdateValue: (v: string) => handleRole(row, v),
          })
        : h(
            NTag,
            {
              size: "small",
              type: row.role === "super_admin" ? "success" : "default",
            },
            () =>
              ({ super_admin: "超管", admin: "管理员", user: "用户" })[
                row.role
              ] ?? row.role,
          ),
  },
  {
    title: "分组",
    key: "group_name",
    render: (row) =>
      h(NSelect, {
        value: row.group_id,
        disabled: row.role === "super_admin",
        options: groupOptions.value,
        size: "tiny",
        style: "width: 140px",
        onUpdateValue: (v: number) => handleGroup(row, v),
      }),
  },
  {
    title: "状态",
    key: "status",
    render: (row) => {
      if (row.role === "super_admin" || !isSuper.value) {
        return h(
          NTag,
          { size: "small", type: "success" },
          () =>
            ({ pending: "待审核", active: "正常", disabled: "停用" })[
              row.status
            ] ?? row.status,
        );
      }
      return h(NSwitch, {
        value: row.status === "active",
        size: "small",
        onUpdateValue: (v: boolean) =>
          handleStatus(row, v ? "active" : "disabled"),
      });
    },
  },
  {
    title: "最近登录",
    key: "last_login_at",
    render: (row) =>
      row.last_login_at ? new Date(row.last_login_at).toLocaleString() : "—",
  },
]);

async function reload() {
  loading.value = true;
  loadError.value = "";
  try {
    const params = new URLSearchParams();
    if (keyword.value) params.set("keyword", keyword.value);
    if (statusFilter.value) params.set("status", statusFilter.value);
    const [userData, groupData] = await Promise.all([
      api.users(`?${params.toString()}`),
      api.groups(),
    ]);
    users.value = userData.items;
    groups.value = groupData.items;
  } catch (e) {
    loadError.value = e instanceof Error ? e.message : "加载失败，请重试";
  } finally {
    loading.value = false;
  }
}

async function handleGroup(row: UserView, groupId: number) {
  try {
    await api.setUserGroup(row.id, groupId);
    row.group_id = groupId;
    message.success("已更新分组");
  } catch (err) {
    message.error(err instanceof Error ? err.message : "更新失败");
  }
}

async function handleRole(row: UserView, role: string) {
  try {
    await api.setUserRole(row.id, role);
    row.role = role;
    message.success("已更新角色");
  } catch (err) {
    message.error(err instanceof Error ? err.message : "更新失败");
  }
}

function handleStatus(row: UserView, status: string) {
  const action =
    status === "disabled"
      ? "停用"
      : status === "pending"
        ? "设为待审核"
        : "启用";
  dialog.warning({
    title: `${action}账号`,
    content: `确定${action}「${row.username}」？${status === "disabled" ? "停用后其会话与 Token 全部失效。" : ""}`,
    positiveText: action,
    negativeText: "取消",
    onPositiveClick: async () => {
      try {
        await api.setUserStatus(row.id, status);
        row.status = status;
        message.success(`已${action}`);
      } catch (err) {
        message.error(err instanceof Error ? err.message : "操作失败");
      }
    },
  });
}

onMounted(reload);
</script>
