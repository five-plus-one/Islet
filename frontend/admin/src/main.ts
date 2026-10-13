import { createApp } from "vue";
import { createPinia } from "pinia";
import App from "./App.vue";
import { router } from "./router";
import "./styles/base.css";
import { useUploadStore } from "./stores/uploads";
import { useAuthStore } from "./stores/auth";
import { clearSession } from "./api";

const app = createApp(App);
app.use(createPinia());
app.use(router);
app.mount("#app");

window.addEventListener("islet:session-expired", () => {
  if (router.currentRoute.value.meta.public) return;
  const redirect = router.currentRoute.value.fullPath;
  clearSession();
  useAuthStore().user = null;
  useUploadStore().reset();
  router.replace({ name: "login", query: { redirect } });
});

window.addEventListener("beforeunload", (event) => {
  if (useUploadStore().pending) {
    event.preventDefault();
    event.returnValue = "";
  }
});
window.addEventListener("vite:preloadError", (event) => {
  event.preventDefault();
  const notice = document.createElement("div");
  notice.style.cssText =
    "position:fixed;inset:0;z-index:9999;background:#f5f7fb;display:grid;place-content:center;text-align:center;color:#172b46;padding:24px";
  const title = document.createElement("h2");
  title.textContent = "页面已更新";
  const description = document.createElement("p");
  description.textContent = "请重新加载页面后继续使用。";
  const button = document.createElement("button");
  button.textContent = "重新加载";
  button.onclick = () => location.reload();
  button.style.cssText =
    "border:0;border-radius:8px;background:#2675df;color:white;padding:12px 24px";
  notice.append(title, description, button);
  document.body.append(notice);
});
