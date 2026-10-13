import { defineStore } from "pinia";
import { getAccessToken, type ImageItem } from "../api";
export interface UploadTask {
  id: number;
  file: File;
  preview: string;
  status: "waiting" | "uploading" | "done" | "error";
  progress: number;
  error: string;
  result?: ImageItem;
  album: number | null;
  policy: number | null;
  tags: string;
}
let serial = 0;
export const useUploadStore = defineStore("uploads", {
  state: () => ({ tasks: [] as UploadTask[], running: false }),
  getters: {
    pending: (state) =>
      state.tasks.some(
        (t) => t.status === "waiting" || t.status === "uploading",
      ),
  },
  actions: {
    reset() {
      for (const task of this.tasks) URL.revokeObjectURL(task.preview);
      this.tasks = [];
    },
    add(
      files: File[],
      options: {
        album: number | null;
        policy: number | null;
        tags: string;
        maxBytes: number;
        allowed: string[];
      },
    ) {
      for (const file of files) {
        const allowed =
          !options.allowed.length ||
          options.allowed.includes("image/*") ||
          options.allowed.includes(file.type);
        const error =
          !file.type.startsWith("image/") || file.type === "image/svg+xml"
            ? "请选择支持的图片文件"
            : !allowed
              ? "当前账户不支持此图片格式"
              : file.size > options.maxBytes
                ? "图片超过单文件大小限制"
                : "";
        this.tasks.push({
          id: ++serial,
          file,
          preview: file.type.startsWith("image/")
            ? URL.createObjectURL(file)
            : "",
          status: error ? "error" : "waiting",
          progress: 0,
          error,
          album: options.album,
          policy: options.policy,
          tags: options.tags,
        });
      }
      this.process();
    },
    async process() {
      if (this.running) return;
      this.running = true;
      try {
        let task: UploadTask | undefined;
        while ((task = this.tasks.find((t) => t.status === "waiting"))) {
          await this.send(task);
        }
      } finally {
        this.running = false;
        window.dispatchEvent(new Event("islet:changed"));
      }
    },
    send(task: UploadTask): Promise<void> {
      task.status = "uploading";
      task.progress = 0;
      task.error = "";
      return new Promise((resolve) => {
        const xhr = new XMLHttpRequest();
        xhr.open("POST", "/api/v1/upload");
        xhr.timeout = 180000;
        const token = getAccessToken();
        if (token) xhr.setRequestHeader("Authorization", `Bearer ${token}`);
        const form = new FormData();
        form.append("files", task.file);
        if (task.album) form.append("album_id", String(task.album));
        if (task.policy) form.append("policy_id", String(task.policy));
        if (task.tags) form.append("tags", task.tags);
        xhr.upload.onprogress = (e) => {
          if (e.lengthComputable)
            task.progress = Math.min(
              99,
              Math.round((e.loaded / e.total) * 100),
            );
        };
        const fail = (text: string) => {
          task.status = "error";
          task.error = text;
          resolve();
        };
        xhr.onerror = () => fail("网络连接中断，请重试");
        xhr.ontimeout = () => fail("上传超时，请重试");
        xhr.onabort = () => fail("上传已取消");
        xhr.onload = () => {
          try {
            const payload = JSON.parse(xhr.responseText);
            if (
              xhr.status < 200 ||
              xhr.status >= 300 ||
              payload.code !== 0 ||
              !payload.data?.items?.[0]
            ) {
              fail(payload.message || "上传失败，请重试");
              return;
            }
            task.result = payload.data.items[0];
            task.status = "done";
            task.progress = 100;
            resolve();
          } catch {
            fail("未能读取上传结果，请重试");
          }
        };
        xhr.send(form);
      });
    },
    retry(id?: number) {
      for (const task of this.tasks) {
        if (task.status === "error" && (id === undefined || task.id === id)) {
          task.status = "waiting";
          task.error = "";
        }
      }
      this.process();
    },
    clearCompleted() {
      this.tasks = this.tasks.filter((t) => {
        if (t.status === "done") {
          URL.revokeObjectURL(t.preview);
          return false;
        }
        return true;
      });
    },
    remove(id: number) {
      this.tasks = this.tasks.filter((t) => {
        if (t.id === id && t.status !== "uploading") {
          URL.revokeObjectURL(t.preview);
          return false;
        }
        return true;
      });
    },
  },
});
