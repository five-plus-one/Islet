<template>
  <n-layout has-sider style="height: 100vh">
    <n-layout-sider bordered collapse-mode="width" :collapsed-width="64" :width="220" v-model:collapsed="collapsed">
      <div class="brand" @click="router.push('/')">
        <span class="brand-mark">Is</span>
        <span v-if="!collapsed" class="brand-name">Islet 图床</span>
      </div>
      <n-menu :collapsed="collapsed" :options="menuOptions" :value="activeKey" @update:value="(key: string) => router.push({ name: key })" />
    </n-layout-sider>
    <n-layout>
      <n-layout-header bordered style="height: 52px; display: flex; align-items: center; justify-content: flex-end; gap: 14px; padding: 0 22px">
        <n-text depth="3">{{ auth.user?.username }}</n-text>
        <n-tag size="small" :type="roleTagType">{{ roleLabel }}</n-tag>
        <n-button quaternary size="small" @click="router.push({ name: 'profile' })">个人中心</n-button>
        <n-button quaternary size="small" @click="handleLogout">退出</n-button>
      </n-layout-header>
      <n-layout-content content-style="min-height: calc(100vh - 52px)">
        <router-view />
      </n-layout-content>
    </n-layout>
  </n-layout>
</template>

<script setup lang="ts">
import { computed, h, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { NButton, NLayout, NLayoutContent, NLayoutHeader, NLayoutSider, NMenu, NTag, NText, type MenuOption } from 'naive-ui';
import { useAuthStore } from '../stores/auth';

const collapsed = ref(false);
const route = useRoute();
const router = useRouter();
const auth = useAuthStore();

const menuOptions = computed<MenuOption[]>(() => {
  const isAdmin = auth.user?.role !== 'user';
  const items: MenuOption[] = [
    { label: '数据看板', key: 'dashboard' },
    { label: '图片库', key: 'images' },
    { label: '上传', key: 'upload' },
    { label: '相册', key: 'albums' },
    { label: 'API Token', key: 'tokens' },
    { label: '个人中心', key: 'profile' }
  ];
  if (isAdmin) {
    items.push(
      { label: '回收站', key: 'trash' },
      { label: '存储策略', key: 'policies' },
      { label: '用户分组', key: 'groups' },
      { label: '用户管理', key: 'users' },
      { label: '系统设置', key: 'settings' }
    );
  }
  return items;
});

const activeKey = computed(() => String(route.name ?? 'dashboard'));
const roleLabel = computed(() => ({ super_admin: '超级管理员', admin: '管理员', user: '用户' }[auth.user?.role ?? 'user'] ?? ''));
const roleTagType = computed(() => (auth.user?.role === 'user' ? 'default' : 'success'));

async function handleLogout() {
  await auth.logout();
  await router.push({ name: 'login' });
}
</script>

<style scoped>
.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 16px 18px;
  cursor: pointer;
  font-weight: 700;
  color: #173a27;
}
.brand-mark {
  background: #18a058;
  color: #fff;
  border-radius: 8px;
  width: 32px;
  height: 32px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: none;
}
</style>
