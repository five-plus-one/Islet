import { useState, useRef } from "react";
import { Button } from "@fiveplusone/ui";
import {
  ConnectionPanel,
  ConnectionValue,
} from "@fiveplusone/ui/media-workspace";
import { Link2 } from "lucide-react";
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
  const keyField = useRef<HTMLDivElement>(null);
  const body = JSON.stringify({
    ...(policy ? { policy_id: policy } : {}),
    ...(album ? { album_id: album } : {}),
  });
  const headers = JSON.stringify({ Authorization: `Bearer ${key.trim()}` });
  async function copy(value: string, needsKey = false) {
    if (needsKey && !key.trim()) {
      setError("请先填写上传密钥");
      keyField.current?.querySelector("input")?.focus();
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
  const template = config.data;
  return (
    <ConnectionPanel
      title="连接 PicList"
      onClose={onClose}
      footer={
        <>
          <Button
            variant="ghost"
            disabled={!template}
            data-fp-tooltip="复制供 PicList Core 使用的 JSON 配置，包含上传密钥"
            onClick={() =>
              template &&
              copy(
                JSON.stringify(
                  {
                    picBed: {
                      uploader: "advancedplist",
                      current: "advancedplist",
                      advancedplist: {
                        ...template,
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
            复制 Core 配置
          </Button>
          <Button onClick={onClose}>完成</Button>
        </>
      }
    >
      <div className="fp-media-connect-intro">
        <span>
          <Link2 size={20} />
        </span>
        <div>
          <h3>在 PicList 中添加「高级自定义」图床</h3>
          <p>填写下面的配置，保存后设为默认图床，即可开始上传。</p>
        </div>
      </div>
      <div className="fp-media-connect-fields">
        <div ref={keyField} className="fp-media-connect-key">
          <TextField
            label="上传密钥"
            type="password"
            value={key}
            onChange={(v) => {
              setKey(v);
              setError("");
            }}
            hint="粘贴你创建的上传密钥，用于生成请求头"
          />
        </div>
        <Choice
          label="保存到相册"
          value={album}
          onChange={setAlbum}
          options={[
            { value: "", label: "未分类" },
            ...(albums.data?.items || []).map((a) => ({
              value: String(a.id),
              label: a.name,
            })),
          ]}
        />
        <Choice
          label="存储位置"
          value={policy}
          onChange={setPolicy}
          options={[
            { value: "", label: "默认存储" },
            ...me.policies.map((p) => ({ value: String(p.id), label: p.name })),
          ]}
        />
      </div>
      <Failure error={config.error} retry={config.reload} />
      <Failure error={albums.error} retry={albums.reload} />
      <Failure error={error} />
      {config.loading && <Loading />}
      {template && (
        <>
          <div className="fp-media-config-values">
            <ConnectionValue
              label="请求地址"
              value={template.endpoint}
              onCopy={() => void copy(template.endpoint)}
            />
            <ConnectionValue label="请求方法" value={template.method} />
            <ConnectionValue
              label="文件字段"
              value={template.formDataKey}
              onCopy={() => void copy(template.formDataKey)}
            />
            <ConnectionValue
              label="请求头"
              hint="JSON 格式"
              value={
                key.trim()
                  ? '{"Authorization":"Bearer ••••••••"}'
                  : "请先填写上方的上传密钥"
              }
              onCopy={() => void copy(headers, true)}
            />
            <ConnectionValue
              label="请求参数"
              hint="JSON 格式"
              value={body}
              onCopy={() => void copy(body)}
            />
            <ConnectionValue
              label="返回链接路径"
              value={template.resDataPath}
              onCopy={() => void copy(template.resDataPath)}
            />
          </div>
          <p className="fp-media-connect-note">
            脚本、链接前缀和网站路径留空。请求头与 Core
            配置包含上传密钥，请妥善保存。
          </p>
        </>
      )}
    </ConnectionPanel>
  );
}
