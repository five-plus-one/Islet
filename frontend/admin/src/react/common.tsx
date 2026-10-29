import {
  createContext,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import {
  Button,
  Field,
  Input,
  Spinner,
  SelectField,
  Dialog,
} from "@fiveplusone/ui";
import { api, type MeInfo } from "../api";
import { errorText } from "../utils";
export const Session = createContext<{
  me: MeInfo;
  refresh: () => Promise<void>;
  notify: (s: string) => void;
}>(null!);
export const useSession = () => useContext(Session);
export function useLoad<T>(fetcher: () => Promise<T>, deps: unknown[] = []) {
  const [data, setData] = useState<T>();
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    let alive = true;
    setLoading(true);
    setError("");
    fetcher()
      .then((x) => {
        if (alive) setData(x);
      })
      .catch((e) => {
        if (alive) setError(errorText(e));
      })
      .finally(() => {
        if (alive) setLoading(false);
      });
    return () => {
      alive = false;
    };
  }, [...deps, revision]);
  return {
    data,
    setData,
    error,
    loading,
    reload: () => setRevision((x) => x + 1),
  };
}
export function Failure({
  error,
  retry,
}: {
  error: string;
  retry?: () => void;
}) {
  return error ? (
    <div role="alert" className="islet-error">
      {error}
      {retry && (
        <Button variant="secondary" onClick={retry}>
          重新加载
        </Button>
      )}
    </div>
  ) : null;
}
export function Heading({
  title,
  description,
  actions,
}: {
  title: string;
  description: string;
  actions?: ReactNode;
}) {
  return (
    <header className="islet-heading">
      <div>
        <h1>{title}</h1>
        <p>{description}</p>
      </div>
      {actions && <div className="islet-heading-actions">{actions}</div>}
    </header>
  );
}
export function TextField({
  label,
  value,
  onChange,
  type = "text",
  required = false,
  hint,
}: {
  label: string;
  value: string | number;
  onChange: (v: string) => void;
  type?: string;
  required?: boolean;
  hint?: string;
}) {
  return (
    <Field label={label} hint={hint}>
      <Input
        type={type}
        value={value}
        required={required}
        onChange={(e) => onChange(e.target.value)}
      />
    </Field>
  );
}
export function Choice({
  label,
  value,
  onChange,
  options,
}: {
  label: string;
  value: string;
  onChange: (s: string) => void;
  options: { value: string; label: string }[];
}) {
  return (
    <Field label={label}>
      <SelectField
        label={label}
        value={value}
        onChange={onChange}
        options={options}
      />
    </Field>
  );
}
export function Confirm({
  title,
  description,
  onClose,
  onConfirm,
}: {
  title: string;
  description: string;
  onClose: () => void;
  onConfirm: () => Promise<void>;
}) {
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  return (
    <Dialog
      open
      title={title}
      onClose={() => !busy && onClose()}
      footer={
        <>
          <Button variant="secondary" disabled={busy} onClick={onClose}>
            取消
          </Button>
          <Button
            loading={busy}
            onClick={async () => {
              setBusy(true);
              try {
                await onConfirm();
                onClose();
              } catch (e) {
                setError(errorText(e));
              } finally {
                setBusy(false);
              }
            }}
          >
            确认{title}
          </Button>
        </>
      }
    >
      <p>{description}</p>
      <Failure error={error} />
    </Dialog>
  );
}
export function Loading() {
  return (
    <div className="islet-loading">
      <Spinner />
      <span>正在加载</span>
    </div>
  );
}
export const changed = () => window.dispatchEvent(new Event("islet:changed"));
