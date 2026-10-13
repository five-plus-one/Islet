<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { NButton, NProgress, useMessage } from "naive-ui";
import { useAuthStore } from "../stores/auth";
import { useUploadStore } from "../stores/uploads";
import { api, type MeInfo } from "../api";
import { humanBytes, errorText } from "../utils";
import AppIcon from "../components/AppIcon.vue";
const route = useRoute();
const router = useRouter();
const auth = useAuthStore();
const message = useMessage();
const menuOpen = ref(false);
const info = ref<MeInfo | null>(null);
const nav = [
  { name: "images", label: "图片库", icon: "image" },
  { name: "upload", label: "上传图片", icon: "upload" },
  { name: "albums", label: "相册", icon: "album" },
  { name: "trash", label: "回收站", icon: "trash" },
  { name: "tokens", label: "上传密钥", icon: "key" },
];
const management = [
  { name: "dashboard", label: "运行概览", icon: "chart" },
  { name: "policies", label: "存储策略", icon: "storage" },
  { name: "groups", label: "用户分组", icon: "users" },
  { name: "users", label: "用户管理", icon: "user" },
  { name: "settings", label: "系统设置", icon: "settings" },
];
const isAdmin = computed(() =>
  ["admin", "super_admin"].includes(auth.user?.role || ""),
);
const currentTitle = computed(
  () =>
    [...nav, ...management, { name: "profile", label: "个人中心" }].find(
      (x) => x.name === route.name,
    )?.label || "工作区",
);
const percent = computed(() =>
  info.value?.quota.limit
    ? Math.min(100, (info.value.quota.used / info.value.quota.limit) * 100)
    : 0,
);
async function refreshQuota() {
  try {
    info.value = await api.meInfo();
  } catch {
    /* 页面中保留具体错误反馈。 */
  }
}
async function logout() {
  if (useUploadStore().pending) {
    message.warning("图片正在上传，请完成后再退出");
    return;
  }
  try {
    await auth.logout();
    await router.replace("/login");
  } catch (e) {
    message.error(errorText(e));
  }
}
function navigate(name: string) {
  menuOpen.value = false;
  router.push({ name });
}
function onKey(e: KeyboardEvent) {
  if (e.key === "Escape") menuOpen.value = false;
}
onMounted(() => {
  refreshQuota();
  window.addEventListener("islet:changed", refreshQuota);
  window.addEventListener("keydown", onKey);
});
onUnmounted(() => {
  window.removeEventListener("islet:changed", refreshQuota);
  window.removeEventListener("keydown", onKey);
});
</script>
<template>
  <div class="app-shell">
    <button
      v-if="menuOpen"
      class="sidebar-scrim"
      aria-label="关闭导航"
      @click="menuOpen = false"
    />
    <aside class="sidebar" :class="{ open: menuOpen }">
      <a class="brand" href="/images" @click.prevent="navigate('images')"
        ><span class="brand-symbol"><AppIcon name="image" :size="24" /></span
        ><span>Islet<span class="brand-caption">图片工作台</span></span></a
      >
      <div class="nav-scroll">
        <div class="nav-label">工作区</div>
        <nav aria-label="工作区">
          <a
            v-for="item in nav"
            :key="item.name"
            :href="router.resolve({ name: item.name }).href"
            class="nav-item"
            :class="{ active: route.name === item.name }"
            :aria-current="route.name === item.name ? 'page' : undefined"
            @click.prevent="navigate(item.name)"
            ><AppIcon :name="item.icon" :size="18" /><span>{{
              item.label
            }}</span
            ><span v-if="item.name === 'upload'" class="nav-plus">+</span></a
          >
        </nav>
        <template v-if="isAdmin"
          ><div class="nav-label management-label">管理</div>
          <nav aria-label="管理">
            <a
              v-for="item in management"
              :key="item.name"
              :href="router.resolve({ name: item.name }).href"
              class="nav-item"
              :class="{ active: route.name === item.name }"
              :aria-current="route.name === item.name ? 'page' : undefined"
              @click.prevent="navigate(item.name)"
              ><AppIcon :name="item.icon" :size="18" /><span>{{
                item.label
              }}</span></a
            >
          </nav></template
        >
      </div>
      <div class="sidebar-bottom">
        <div v-if="info" class="capacity">
          <div>
            <span>我的存储</span
            ><span>{{
              info.quota.limit ? Math.round(percent) + "%" : "不限容量"
            }}</span>
          </div>
          <n-progress
            type="line"
            :percentage="percent"
            :show-indicator="false"
            :height="4"
          />
          <p>
            {{ humanBytes(info.quota.used)
            }}<template v-if="info.quota.limit">
              / {{ humanBytes(info.quota.limit) }}</template
            >
          </p>
        </div>
        <button class="account" @click="navigate('profile')">
          <span class="avatar">{{
            auth.user?.username?.slice(0, 1).toUpperCase()
          }}</span
          ><span class="account-info"
            ><strong>{{ auth.user?.username }}</strong
            ><small>{{
              auth.user?.role === "super_admin"
                ? "超级管理员"
                : isAdmin
                  ? "管理员"
                  : "个人账户"
            }}</small></span
          ><AppIcon name="arrow" :size="16" />
        </button>
      </div>
    </aside>
    <div class="main-column">
      <header class="topbar">
        <div class="topbar-left">
          <n-button
            class="mobile-menu"
            quaternary
            circle
            aria-label="打开导航"
            @click="menuOpen = true"
            ><AppIcon name="menu" /></n-button
          ><span class="breadcrumb-root">工作台</span
          ><span class="breadcrumb-separator">/</span
          ><strong>{{ currentTitle }}</strong>
        </div>
        <div class="topbar-actions">
          <span class="topbar-username">{{ auth.user?.username }}</span
          ><n-button
            quaternary
            circle
            aria-label="退出登录"
            title="退出登录"
            @click="logout"
            ><AppIcon name="logout" :size="18"
          /></n-button>
        </div>
      </header>
      <main id="main-content">
        <router-view v-slot="{ Component }"
          ><component :is="Component"
        /></router-view>
      </main>
      <footer class="app-footer">
        Islet <span>·</span> 每一张图片，都有归处
      </footer>
    </div>
  </div>
</template>
<style scoped>
.app-shell {
  display: flex;
  min-height: 100vh;
}
.sidebar {
  width: 232px;
  background: #fff;
  border-right: 1px solid var(--border);
  position: fixed;
  inset: 0 auto 0 0;
  display: flex;
  flex-direction: column;
  z-index: 30;
}
.brand {
  display: flex;
  align-items: center;
  gap: 11px;
  padding: 30px 25px 32px;
  color: var(--text);
  font-size: 24px;
  font-weight: 650;
  letter-spacing: -0.6px;
}
.brand-symbol {
  width: 39px;
  height: 39px;
  display: grid;
  place-items: center;
  border-radius: 12px;
  background: var(--primary);
  color: #fff;
  box-shadow: 0 4px 12px #2675df30;
}
.brand-caption {
  display: block;
  font-size: 10px;
  color: var(--muted);
  font-weight: 400;
  letter-spacing: 1px;
  margin-top: 1px;
}
.nav-scroll {
  flex: 1;
  overflow-y: auto;
  padding: 0 15px;
}
.nav-label {
  padding: 0 14px 10px;
  color: #9ba7b8;
  font-size: 10px;
  letter-spacing: 1.5px;
}
.management-label {
  margin-top: 25px;
}
.nav-item {
  display: flex;
  align-items: center;
  gap: 12px;
  color: #63728a;
  height: 42px;
  border-radius: 8px;
  padding: 0 14px;
  margin-bottom: 5px;
  font-size: 13px;
  transition:
    background 0.16s,
    color 0.16s;
}
.nav-item:hover {
  background: #f5f8fc;
  color: var(--primary);
}
.nav-item.active {
  background: #edf4ff;
  color: #216acb;
  font-weight: 600;
}
.nav-plus {
  margin-left: auto;
  font-size: 18px;
  color: #8ca4c7;
}
.sidebar-bottom {
  padding: 20px 22px 18px;
}
.capacity {
  padding: 14px 0 20px;
}
.capacity > div:first-child {
  display: flex;
  justify-content: space-between;
  font-size: 10px;
  color: var(--muted);
  margin-bottom: 10px;
}
.capacity p {
  font-size: 11px;
  color: #8493a9;
  margin: 8px 0 0;
}
.account {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  border: 0;
  border-top: 1px solid var(--border);
  padding: 17px 0 0;
  background: transparent;
  text-align: left;
  color: var(--muted);
}
.avatar {
  width: 34px;
  height: 34px;
  display: grid;
  place-items: center;
  border-radius: 50%;
  background: #e8f0fe;
  color: #3979d4;
  font-size: 13px;
  font-weight: 600;
}
.account-info {
  flex: 1;
  min-width: 0;
}
.account-info strong {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  font-size: 12px;
  font-weight: 550;
  color: var(--text);
}
.account-info small {
  font-size: 10px;
  color: var(--muted);
}
.main-column {
  margin-left: 232px;
  width: calc(100% - 232px);
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.topbar {
  height: 64px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0 32px;
  border-bottom: 1px solid var(--border);
  background: #ffffffeb;
  position: sticky;
  top: 0;
  z-index: 15;
  backdrop-filter: blur(14px);
}
.topbar-left,
.topbar-actions {
  display: flex;
  align-items: center;
  gap: 15px;
  font-size: 12px;
}
.topbar-left strong {
  font-weight: 500;
}
.breadcrumb-root,
.topbar-username {
  color: var(--muted);
}
.breadcrumb-separator {
  color: #c5cfdb;
}
.mobile-menu {
  display: none;
}
.app-footer {
  margin-top: auto;
  text-align: center;
  color: #a4aebc;
  font-size: 10px;
  padding: 14px;
}
.app-footer span {
  padding: 0 8px;
}
.sidebar-scrim {
  display: none;
}
main {
  min-width: 0;
}
@media (max-width: 768px) {
  .sidebar {
    transform: translateX(-100%);
    transition: transform 0.2s;
    visibility: hidden;
  }
  .sidebar.open {
    transform: translateX(0);
    visibility: visible;
  }
  .main-column {
    margin-left: 0;
    width: 100%;
  }
  .topbar {
    height: 56px;
    padding: 0 16px;
  }
  .mobile-menu {
    display: flex;
  }
  .breadcrumb-root,
  .breadcrumb-separator,
  .topbar-username {
    display: none;
  }
  .sidebar-scrim {
    display: block;
    position: fixed;
    inset: 0;
    background: #172b4655;
    border: 0;
    z-index: 25;
    backdrop-filter: blur(2px);
  }
  .brand {
    padding-top: 24px;
    padding-bottom: 24px;
  }
  .sidebar-bottom {
    padding-top: 5px;
  }
}
</style>
