<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import {
  NAlert,
  NButton,
  NDivider,
  NForm,
  NFormItem,
  NInput,
  NSkeleton,
} from "naive-ui";
import { api, beginOIDCLogin, clearSession, type AuthConfig } from "../api";
import { useAuthStore } from "../stores/auth";
import { errorText } from "../utils";
import AppIcon from "../components/AppIcon.vue";
const config = ref<AuthConfig | null>(null),
  username = ref(""),
  password = ref(""),
  loading = ref(false),
  configLoading = ref(true),
  error = ref("");
const route = useRoute(),
  router = useRouter(),
  auth = useAuthStore();
async function load() {
  configLoading.value = true;
  error.value = "";
  try {
    config.value = await api.authConfig();
  } catch (e) {
    error.value = errorText(e);
  } finally {
    configLoading.value = false;
  }
}
async function login() {
  if (loading.value) return;
  if (!username.value.trim() || !password.value) {
    error.value = "请输入用户名和密码";
    return;
  }
  loading.value = true;
  error.value = "";
  try {
    clearSession();
    await api.localLogin(username.value.trim(), password.value);
    await auth.fetchUser();
    const redirect = String(route.query.redirect || "/images");
    await router.replace(
      redirect.startsWith("/") && !redirect.startsWith("//")
        ? redirect
        : "/images",
    );
  } catch (e) {
    error.value = errorText(e);
  } finally {
    loading.value = false;
  }
}
async function oidc() {
  if (!config.value) return;
  try {
    await beginOIDCLogin(config.value);
  } catch (e) {
    error.value = errorText(e);
  }
}
onMounted(load);
</script>
<template>
  <div class="login-page">
    <section class="login-art">
      <span class="login-mark"><AppIcon name="image" :size="30" /></span>
      <h2>每一张图片，<br />都有归处。</h2>
      <p>让画面留存，让灵感流动。<br />你的图片空间，从这里开始。</p>
      <div class="login-illustration">
        <div class="art-mountain one" />
        <div class="art-mountain two" />
        <div class="art-sun" />
      </div>
    </section>
    <div class="login-card">
      <div class="login-brand">
        <img
          v-if="config?.brand_logo"
          :src="config.brand_logo"
          alt=""
          style="width: 34px; height: 34px; object-fit: contain"
        /><span>{{ config?.brand_name || "Islet" }}</span>
      </div>
      <h1 style="font-size: 22px; font-weight: 600; margin: 9px 0">欢迎回来</h1>
      <p class="login-caption">登录你的账户，继续整理与分享。</p>
      <n-alert v-if="error" type="error" :bordered="false"
        >{{ error
        }}<n-button v-if="!config" text @click="load"
          >重新加载</n-button
        ></n-alert
      ><template v-if="configLoading"
        ><n-skeleton height="40px" /><n-skeleton height="40px" /></template
      ><template v-else
        ><template v-if="config?.authorization_endpoint"
          ><n-button
            block
            size="large"
            secondary
            type="primary"
            @click="oidc"
            >{{ config.oidc_label || "统一登录" }}</n-button
          ><n-divider
            v-if="config.local_enabled"
            style="margin: 0; font-size: 12px; color: var(--muted)"
            >或使用本地账户</n-divider
          ></template
        >
        <form v-if="config?.local_enabled" @submit.prevent="login">
          <n-form label-placement="top"
            ><n-form-item label="用户名"
              ><n-input
                v-model:value="username"
                placeholder="输入用户名"
                autocomplete="username"
                size="large"
                :input-props="{ 'aria-label': '用户名' }" /></n-form-item
            ><n-form-item label="密码"
              ><n-input
                v-model:value="password"
                type="password"
                show-password-on="click"
                placeholder="输入密码"
                autocomplete="current-password"
                size="large"
                :input-props="{ 'aria-label': '密码' }" /></n-form-item></n-form
          ><n-button
            attr-type="submit"
            type="primary"
            block
            size="large"
            :loading="loading"
            >登录工作台<AppIcon
              name="arrow"
              :size="17"
              style="margin-left: 10px"
          /></n-button>
        </form>
        <n-alert
          v-if="
            config && !config.local_enabled && !config.authorization_endpoint
          "
          type="warning"
          >登录服务暂时不可用，请联系管理员。</n-alert
        ></template
      >
      <div class="login-footer">Islet · 你的图片，井然有序</div>
    </div>
  </div>
</template>
<style scoped>
.login-illustration {
  position: relative;
  width: 300px;
  height: 185px;
  margin-top: 45px;
  border: 8px solid #fff;
  border-radius: 20px;
  background: #d9e8fb;
  box-shadow: 0 20px 50px #2e5c9120;
  overflow: hidden;
  transform: rotate(-5deg);
}
.art-mountain {
  position: absolute;
  width: 240px;
  height: 150px;
  border-radius: 50% 50% 0 0;
  bottom: -40px;
  left: -40px;
  transform: rotate(-15deg);
  background: #8fbbeb;
}
.art-mountain.two {
  left: 130px;
  bottom: -10px;
  background: #bdd7f4;
  transform: rotate(20deg);
}
.art-sun {
  width: 35px;
  height: 35px;
  background: #fff7d2;
  border-radius: 50%;
  position: absolute;
  right: 70px;
  top: 27px;
}
</style>
