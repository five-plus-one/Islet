import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router';
import { api, clearSession, getAccessToken } from './api';
import { useAuthStore } from './stores/auth';

const routes: RouteRecordRaw[] = [
  { path: '/setup', name: 'setup', component: () => import('./views/SetupView.vue'), meta: { public: true } },
  { path: '/setup/auth/callback', name: 'setup-callback', component: () => import('./views/AuthCallbackView.vue'), meta: { public: true, setup: true } },
  { path: '/login', name: 'login', component: () => import('./views/LoginView.vue'), meta: { public: true } },
  { path: '/auth/callback', name: 'callback', component: () => import('./views/AuthCallbackView.vue'), meta: { public: true } },
  {
    path: '/',
    component: () => import('./layouts/AdminLayout.vue'),
    children: [
      { path: '', name: 'dashboard', component: () => import('./views/DashboardView.vue'), meta: { admin: true } },
      { path: 'images', name: 'images', component: () => import('./views/ImagesView.vue') },
      { path: 'upload', name: 'upload', component: () => import('./views/UploadView.vue') },
      { path: 'albums', name: 'albums', component: () => import('./views/AlbumsView.vue') },
      { path: 'trash', name: 'trash', component: () => import('./views/TrashView.vue'), meta: { admin: true } },
      { path: 'tokens', name: 'tokens', component: () => import('./views/TokensView.vue') },
      { path: 'policies', name: 'policies', component: () => import('./views/PoliciesView.vue'), meta: { admin: true } },
      { path: 'groups', name: 'groups', component: () => import('./views/GroupsView.vue'), meta: { admin: true } },
      { path: 'users', name: 'users', component: () => import('./views/UsersView.vue'), meta: { admin: true } },
      { path: 'settings', name: 'settings', component: () => import('./views/SettingsView.vue'), meta: { admin: true } },
      { path: 'profile', name: 'profile', component: () => import('./views/ProfileView.vue') }
    ]
  },
  { path: '/:pathMatch(.*)*', redirect: '/' }
];

export const router = createRouter({
  history: createWebHistory(),
  routes
});

router.beforeEach(async (to) => {
  // 初始化向导与回调直接放行
  if (to.meta.public) return true;

  // 管理页面：先确认已初始化，否则进向导
  const auth = useAuthStore();
  try {
    if (!auth.checked) await auth.fetchUser();
  } catch {
    // 未初始化 / 未登录
  }
  if (!auth.initialized) {
    clearSession();
    if (to.name !== 'setup') return { name: 'setup' };
    return true;
  }
  if (!auth.user) {
    clearSession();
    return to.name === 'login' ? true : { name: 'login', query: { redirect: to.fullPath } };
  }
  if (to.meta.admin && auth.user.role === 'user') {
    return { name: 'images' };
  }
  return true;
});

// 预取健康状态辅助
export async function isInstalled(): Promise<boolean> {
  try {
    const status = await api.setupStatus();
    return status.status === 'ready';
  } catch {
    return false;
  }
}

export function hasSession(): boolean {
  return getAccessToken() !== '';
}
