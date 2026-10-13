<script setup lang="ts">
import { computed, h, onMounted, ref } from "vue";
import {
  NAlert,
  NButton,
  NCard,
  NDataTable,
  NDatePicker,
  NForm,
  NFormItem,
  NInput,
  NModal,
  NSpace,
  NTag,
  useDialog,
  useMessage,
  type DataTableColumns,
} from "naive-ui";
import { api, type TokenView } from "../api";
import { copyText, dateLabel, errorText } from "../utils";
import PageHeader from "../components/PageHeader.vue";
import AppIcon from "../components/AppIcon.vue";
const tokens = ref<TokenView[]>([]),
  loading = ref(false),
  error = ref(""),
  show = ref(false),
  name = ref(""),
  expires = ref<number | null>(null),
  secret = ref(""),
  saving = ref(false),
  formError = ref(""),
  base = ref("");
const dialog = useDialog(),
  message = useMessage();
const uploadURL = computed(() =>
  base.value ? base.value.replace(/\/$/, "") + "/api/v1/upload?picgo=1" : "",
);
const state = (r: TokenView) =>
  r.status !== "active"
    ? "已撤销"
    : r.expires_at && new Date(r.expires_at).getTime() <= Date.now()
      ? "已过期"
      : "可用";
const columns: DataTableColumns<TokenView> = [
  { title: "名称", key: "name", width: 180, ellipsis: { tooltip: true } },
  {
    title: "状态",
    key: "status",
    width: 90,
    render: (r) =>
      h(
        NTag,
        {
          size: "small",
          bordered: false,
          type: state(r) === "可用" ? "success" : "default",
        },
        () => state(r),
      ),
  },
  {
    title: "创建时间",
    key: "created_at",
    width: 175,
    render: (r) => dateLabel(r.created_at),
  },
  {
    title: "到期时间",
    key: "expires_at",
    width: 175,
    render: (r) => (r.expires_at ? dateLabel(r.expires_at) : "长期有效"),
  },
  {
    title: "最后使用",
    key: "last_used_at",
    width: 175,
    render: (r) => dateLabel(r.last_used_at),
  },
  {
    title: "操作",
    key: "actions",
    width: 90,
    render: (r) =>
      r.status === "active"
        ? h(
            NButton,
            {
              text: true,
              type: "error",
              size: "small",
              onClick: () => revoke(r),
            },
            () => "撤销",
          )
        : "—",
  },
];
async function load() {
  loading.value = true;
  error.value = "";
  try {
    const [t, me] = await Promise.all([api.tokens(), api.meInfo()]);
    tokens.value = t.items;
    base.value = me.public_base_url;
  } catch (e) {
    error.value = errorText(e);
  } finally {
    loading.value = false;
  }
}
async function create() {
  if (saving.value) return;
  formError.value = "";
  if (!name.value.trim()) {
    formError.value = "请为密钥填写名称";
    return;
  }
  if (expires.value && expires.value <= Date.now()) {
    formError.value = "到期时间需要晚于当前时间";
    return;
  }
  saving.value = true;
  try {
    const result = await api.createToken(
      name.value.trim(),
      expires.value ? new Date(expires.value).toISOString() : undefined,
    );
    secret.value = result.token;
    show.value = false;
    await load();
  } catch (e) {
    formError.value = errorText(e);
  } finally {
    saving.value = false;
  }
}
async function copy(text: string) {
  try {
    await copyText(text);
    message.success("已复制");
  } catch (e) {
    message.error(errorText(e));
  }
}
function revoke(item: TokenView) {
  dialog.warning({
    title: "撤销上传密钥",
    content: `撤销「${item.name}」后，使用此密钥的客户端将无法继续上传。`,
    positiveText: "撤销密钥",
    negativeText: "取消",
    onPositiveClick: async () => {
      try {
        await api.revokeToken(item.id);
        await load();
        message.success("密钥已撤销");
      } catch (e) {
        message.error(errorText(e));
        return false;
      }
    },
  });
}
onMounted(load);
</script>
<template>
  <div class="page">
    <PageHeader
      title="上传密钥"
      description="连接 PicGo 等上传工具，把图片直接保存到 Islet。"
      eyebrow="CONNECTED WORKFLOW"
      ><n-button
        type="primary"
        size="large"
        @click="
          name = '';
          expires = null;
          formError = '';
          show = true;
        "
        ><template #icon><AppIcon name="plus" :size="18" /></template
        >创建密钥</n-button
      ></PageHeader
    ><n-alert v-if="error" type="error" style="margin-bottom: 18px"
      >{{ error }} <n-button text @click="load">重试</n-button></n-alert
    ><n-card :bordered="false"
      ><n-data-table
        :columns="columns"
        :data="tokens"
        :loading="loading"
        :bordered="false"
        :pagination="{ pageSize: 10 }"
        :scroll-x="885" /></n-card
    ><n-card title="连接 PicGo" :bordered="false" style="margin-top: 24px"
      ><p class="subtle" style="margin-top: 0">
        在支持自定义 HTTP 上传的插件中填写以下信息。
      </p>
      <div class="two-column">
        <div>
          <div class="detail-label">上传地址</div>
          <n-input
            :value="uploadURL"
            readonly
            placeholder="请联系管理员配置图床公开地址"
          /><n-button
            v-if="uploadURL"
            text
            type="primary"
            style="margin-top: 10px"
            @click="copy(uploadURL)"
            >复制上传地址</n-button
          >
        </div>
        <div>
          <dl class="metadata-list">
            <div>
              <dt>请求方法</dt>
              <dd>POST</dd>
            </div>
            <div>
              <dt>请求格式</dt>
              <dd>multipart/form-data</dd>
            </div>
            <div>
              <dt>文件字段</dt>
              <dd>files</dd>
            </div>
            <div>
              <dt>认证请求头</dt>
              <dd>Authorization: Bearer 你的密钥</dd>
            </div>
          </dl>
        </div>
      </div></n-card
    >
    <n-modal v-model:show="show" :mask-closable="!saving"
      ><n-card title="创建上传密钥" style="width: 460px" :bordered="false"
        ><n-alert v-if="formError" type="error" style="margin-bottom: 15px">{{
          formError
        }}</n-alert
        ><n-form label-placement="top"
          ><n-form-item label="名称" required
            ><n-input
              v-model:value="name"
              placeholder="如：MacBook · PicGo"
              :maxlength="64" /></n-form-item
          ><n-form-item label="到期时间"
            ><n-date-picker
              v-model:value="expires"
              type="datetime"
              clearable
              placeholder="长期有效"
              style="width: 100%" /></n-form-item></n-form
        ><template #footer
          ><n-space justify="end"
            ><n-button @click="show = false" :disabled="saving">取消</n-button
            ><n-button type="primary" :loading="saving" @click="create"
              >创建密钥</n-button
            ></n-space
          ></template
        ></n-card
      ></n-modal
    >
    <n-modal :show="!!secret" :mask-closable="false" :close-on-esc="false"
      ><n-card title="上传密钥已创建" style="width: 520px" :bordered="false"
        ><n-alert type="warning" :bordered="false" style="margin-bottom: 16px"
          >密钥只在这里显示一次，请复制并妥善保存。</n-alert
        ><n-input
          :value="secret"
          readonly
          type="textarea"
          :rows="3"
          aria-label="新建的上传密钥"
        /><template #footer
          ><n-space justify="end"
            ><n-button @click="secret = ''">我已保存</n-button
            ><n-button type="primary" @click="copy(secret)"
              >复制密钥</n-button
            ></n-space
          ></template
        ></n-card
      ></n-modal
    >
  </div>
</template>
