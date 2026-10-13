export function humanBytes(n: number): string {
  if (!Number.isFinite(n) || n <= 0) return "0 B";
  const i = Math.min(4, Math.floor(Math.log(n) / Math.log(1024)));
  return `${(n / 1024 ** i).toFixed(i ? 1 : 0)} ${["B", "KB", "MB", "GB", "TB"][i]}`;
}

export function groupLabel(name?: string): string {
  return (
    (
      { administrators: "管理员组", members: "默认成员组" } as Record<
        string,
        string
      >
    )[name || ""] ||
    name ||
    "—"
  );
}
export function dateLabel(value?: string | null): string {
  return value
    ? new Date(value).toLocaleString("zh-CN", { hour12: false })
    : "—";
}
export function errorText(error: unknown): string {
  return error instanceof Error ? error.message : "暂时无法完成，请稍后重试";
}
export type LinkFormat = "url" | "markdown" | "html" | "bbcode";
export const linkFormats = [
  { label: "直链", value: "url" },
  { label: "Markdown", value: "markdown" },
  { label: "HTML", value: "html" },
  { label: "BBCode", value: "bbcode" },
];
export function imageLink(
  item: { url: string; original_name: string },
  format: LinkFormat,
): string {
  const escape = (v: string) =>
    v
      .replace(/&/g, "&amp;")
      .replace(/"/g, "&quot;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;");
  if (format === "markdown")
    return `![${item.original_name.replace(/[\\\[\]]/g, "\\$&")}](<${item.url.replace(/>/g, "%3E")}>)`;
  if (format === "html")
    return `<img src="${escape(item.url)}" alt="${escape(item.original_name)}" />`;
  if (format === "bbcode") return `[img]${item.url}[/img]`;
  return item.url;
}
export async function copyText(value: string): Promise<void> {
  if (navigator.clipboard && window.isSecureContext) {
    await navigator.clipboard.writeText(value);
    return;
  }
  const field = document.createElement("textarea");
  field.value = value;
  field.style.cssText = "position:fixed;left:-9999px";
  document.body.appendChild(field);
  field.select();
  const ok = document.execCommand("copy");
  field.remove();
  if (!ok) throw new Error("复制未成功，请选择链接后手动复制");
}
