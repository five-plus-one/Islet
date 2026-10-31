import { useState } from "react";
import { Button, Dialog, Field, Input } from "@fiveplusone/ui";
import { api } from "../api";
import { copyText, errorText } from "../utils";
import {
  Choice,
  Failure,
  Loading,
  TextField,
  useLoad,
  useSession,
} from "./common";

export function PicListConfig({
  initialKey,
  onClose,
}: {
  initialKey: string;
  onClose: () => void;
}) {
  const { me, notify } = useSession();
  const config = useLoad(api.picListConfig);
  const albums = useLoad(api.albums);
  const [key, setKey] = useState(initialKey);
  const [policy, setPolicy] = useState("");
  const [album, setAlbum] = useState("");
  const [error, setError] = useState("");
  const body = JSON.stringify({
    ...(policy ? { policy_id: policy } : {}),
    ...(album ? { album_id: album } : {}),
  });
  const headers = JSON.stringify({ Authorization: `Bearer ${key.trim()}` });
  async function copy(value: string, needsKey = false) {
    if (needsKey && !key.trim()) {
      setError("请先填写上传密钥");
      return;
    }
    try {
      await copyText(value);
      setError("");
      notify("配置已复制");
    } catch (e) {
      setError(errorText(e));
    }
  }
  function item(
    label: string,
    value: string,
    actual = value,
    needsKey = false,
  ) {
    return (
      <div className="islet-row" style={{ alignItems: "end" }}>
        <div style={{ flex: "1 1 220px", minWidth: 0 }}>
          <Field label={label}>
            <Input readOnly value={value} />
          </Field>
        </div>
        <Button
          variant="secondary"
          aria-label={"复制" + label}
          onClick={() => copy(actual, needsKey)}
        >
          复制
        </Button>
      </div>
    );
  }
  return (
    <Dialog
      open
      title="配置 PicList"
      onClose={onClose}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            完成
          </Button>
          <Button
            disabled={!config.data}
            onClick={() =>
              config.data &&
              copy(
                JSON.stringify(
                  {
                    picBed: {
                      uploader: "advancedplist",
                      current: "advancedplist",
                      advancedplist: {
                        ...config.data,
                        uploader: undefined,
                        headers,
                        body,
                      },
                    },
                  },
                  null,
                  2,
                ),
                true,
              )
            }
          >
            复制配置片段
          </Button>
        </>
      }
    >
      <div className="islet-form">
        <p>
          在 PicList
          的“图床”中选择“高级自定义”，新建配置并填写以下内容，保存后设为默认图床。
        </p>
        <TextField
          label="上传密钥"
          type="password"
          value={key}
          onChange={setKey}
          hint="填写本人创建的密钥；关闭后清除本次填写内容。"
        />
        <Failure error={config.error} retry={config.reload} />
        {config.loading && <Loading />}
        {config.data && (
          <>
            {item("请求地址", config.data.endpoint)}
            {item("请求方法", config.data.method)}
            {item("文件字段", config.data.formDataKey)}
            <Choice
              label="存储策略"
              value={policy}
              onChange={setPolicy}
              options={[
                { value: "", label: "默认存储策略" },
                ...me.policies.map((p) => ({
                  value: String(p.id),
                  label: p.name,
                })),
              ]}
            />
            <Choice
              label="保存到相册"
              value={album}
              onChange={setAlbum}
              options={[
                { value: "", label: "不指定相册" },
                ...(albums.data?.items || []).map((a) => ({
                  value: String(a.id),
                  label: a.name,
                })),
              ]}
            />
            <Failure error={albums.error} retry={albums.reload} />
            {item(
              "请求头",
              '{"Authorization":"Bearer ••••••••"}',
              headers,
              true,
            )}
            {item("请求参数", body)}
            {item("返回链接路径", config.data.resDataPath)}
            <p className="fp-muted">
              脚本、链接前缀和网站路径留空。配置片段含有上传密钥，请妥善保存。
            </p>
          </>
        )}
        <Failure error={error} />
      </div>
    </Dialog>
  );
}
