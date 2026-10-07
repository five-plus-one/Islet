import { defineStore } from 'pinia';
import { api, clearSession, type AuthUser } from '../api';

export const useAuthStore = defineStore('auth', {
  state: () => ({
    user: null as AuthUser | null,
    initialized: true,
    checked: false
  }),
  actions: {
    async fetchUser(): Promise<AuthUser | null> {
      try {
        const status = await api.setupStatus();
        this.initialized = status.status === 'ready';
        if (!this.initialized) {
          this.checked = true;
          return null;
        }
      } catch {
        this.initialized = true; // 状态接口不可用时按已初始化处理
      }
      try {
        this.user = await api.me();
      } catch {
        this.user = null;
      }
      this.checked = true;
      return this.user;
    },
    async logout(): Promise<void> {
      try {
        await api.logout();
      } catch {
        // 忽略登出失败
      }
      clearSession();
      this.user = null;
    }
  }
});
