import { useSyncExternalStore } from "react";
import { getAccessToken, type ImageItem } from "../api";
export interface Task {
  id: string;
  file: File;
  preview?: string;
  name: string;
  status: "waiting" | "uploading" | "done" | "error";
  progress: number;
  error?: string;
  result?: ImageItem;
  album: string;
  policy: string;
  tags: string;
  blocked?: boolean;
}
let tasks: Task[] = [];
let running = false;
let active: XMLHttpRequest | undefined;
let generation = 0;
let serial = 0;
const listeners = new Set<() => void>();
const emit = () => {
  tasks = [...tasks];
  listeners.forEach((x) => x());
};
export const uploads = {
  add(
    files: File[],
    o: {
      album: string;
      policy: string;
      tags: string;
      max: number;
      allowed: string[];
    },
  ) {
    for (const file of files) {
      const error =
        !file.type.startsWith("image/") || file.type === "image/svg+xml"
          ? "请选择支持的图片"
          : file.size > o.max
            ? "图片超过单文件大小限制"
            : o.allowed.length &&
                !o.allowed.includes("image/*") &&
                !o.allowed.includes(file.type)
              ? "当前账户不支持此格式"
              : "";
      tasks.push({
        id: String(++serial),
        file,
        preview: !error ? URL.createObjectURL(file) : undefined,
        name: file.name,
        status: error ? "error" : "waiting",
        progress: 0,
        error,
        blocked: !!error,
        ...o,
      });
    }
    emit();
    void process();
  },
  retry(id: string) {
    const t = tasks.find((x) => x.id === id);
    if (t && !t.blocked) {
      t.status = "waiting";
      emit();
      void process();
    }
  },
  remove(id: string) {
    const t = tasks.find((x) => x.id === id);
    if (t?.status === "uploading") active?.abort();
    if (t?.preview) URL.revokeObjectURL(t.preview);
    tasks = tasks.filter((x) => x.id !== id);
    emit();
  },
  reset() {
    generation++;
    tasks.forEach((t) => t.preview && URL.revokeObjectURL(t.preview));
    tasks = [];
    active?.abort();
    emit();
  },
  pending() {
    return tasks.some(
      (x) => x.status === "waiting" || x.status === "uploading",
    );
  },
  clear() {
    tasks
      .filter((t) => t.status === "done")
      .forEach((t) => t.preview && URL.revokeObjectURL(t.preview));
    tasks = tasks.filter((x) => x.status !== "done");
    emit();
  },
};
export function useUploads() {
  return useSyncExternalStore(
    (cb) => {
      listeners.add(cb);
      return () => {
        listeners.delete(cb);
      };
    },
    () => tasks,
  );
}
async function process() {
  if (running) return;
  running = true;
  const current = generation;
  try {
    let t;
    while (
      current === generation &&
      (t = tasks.find((x) => x.status === "waiting"))
    ) {
      await send(t);
    }
  } finally {
    running = false;
    window.dispatchEvent(new Event("islet:changed"));
    if (tasks.some((x) => x.status === "waiting")) void process();
  }
}
function send(t: Task) {
  return new Promise<void>((resolve) => {
    t.status = "uploading";
    t.error = "";
    emit();
    const xhr = new XMLHttpRequest();
    active = xhr;
    xhr.open("POST", "/api/v1/upload");
    xhr.timeout = 180000;
    const token = getAccessToken();
    if (token) xhr.setRequestHeader("Authorization", `Bearer ${token}`);
    const body = new FormData();
    body.append("files", t.file);
    if (t.album) body.append("album_id", t.album);
    if (t.policy) body.append("policy_id", t.policy);
    if (t.tags) body.append("tags", t.tags);
    const end = (error?: string) => {
      t.status = error ? "error" : "done";
      t.error = error;
      if (!error) t.progress = 100;
      active = undefined;
      emit();
      resolve();
    };
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) {
        t.progress = Math.min(99, Math.round((e.loaded / e.total) * 100));
        emit();
      }
    };
    xhr.onerror = () => end("网络连接中断，请重试");
    xhr.ontimeout = () => end("上传超时，请重试");
    xhr.onabort = () => end("上传已取消");
    xhr.onload = () => {
      try {
        const p = JSON.parse(xhr.responseText);
        if (xhr.status === 401)
          window.dispatchEvent(new Event("islet:session-expired"));
        if (
          xhr.status < 200 ||
          xhr.status >= 300 ||
          p.code !== 0 ||
          !p.data?.items?.[0]
        )
          return end(p.message || "上传失败，请重试");
        t.result = p.data.items[0];
        end();
      } catch {
        end("未能读取上传结果，请重试");
      }
    };
    xhr.send(body);
  });
}
