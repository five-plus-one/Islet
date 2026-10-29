import { useEffect, useState } from "react";
import {
  Navigate,
  Route,
  Routes,
  useLocation,
  useNavigate,
} from "react-router-dom";
import {
  Button,
  Card,
  ThemeToggle,
  AccountMenu,
  ProductNavigation,
  Dialog,
  Toast,
} from "@fiveplusone/ui";
import { AppEntryLayout } from "@fiveplusone/ui/app-surfaces";
import {
  Images,
  Upload,
  Folder,
  Trash2,
  KeyRound,
  ChartNoAxesCombined,
  HardDrive,
  Users,
  Settings,
  UserRound,
  Menu,
  LogOut,
  Layers,
} from "lucide-react";
import {
  api,
  ApiError,
  beginOIDCLogin,
  consumeOIDCState,
  saveSession,
  clearSession,
  type MeInfo,
} from "../api";
import { humanBytes, errorText } from "../utils";
import {
  Session,
  TextField,
  Failure,
  Loading,
  useLoad,
  Confirm,
} from "./common";
import { uploads } from "./uploads";
import { Gallery, UploadPage, Albums, Tokens, Profile } from "./library";
import { AdminPage, Setup } from "./management";
const navigation = [
  ["/images", "图片库", Images],
  ["/upload", "上传图片", Upload],
  ["/albums", "相册", Folder],
  ["/trash", "回收站", Trash2],
  ["/tokens", "上传密钥", KeyRound],
  ["/profile", "我的账户", UserRound],
  ["/overview", "运行概览", ChartNoAxesCombined],
  ["/policies", "存储策略", HardDrive],
  ["/groups", "用户分组", Layers],
  ["/users", "用户管理", Users],
  ["/settings", "系统设置", Settings],
] as const;
export function App() {
  const installation = useLoad(api.setupStatus);
  const current = useLocation();
  const [dark, setDark] = useState(
    document.documentElement.dataset.theme === "dark",
  );
  const theme = (
    <ThemeToggle
      dark={dark}
      onChange={(v) => {
        setDark(v);
        document.documentElement.dataset.theme = v ? "dark" : "light";
        localStorage.setItem("islet-theme", v ? "dark" : "light");
      }}
    />
  );
  if (installation.loading) return <Loading />;
  if (installation.error)
    return <Failure error={installation.error} retry={installation.reload} />;
  if (
    installation.data?.status !== "ready" &&
    !current.pathname.startsWith("/setup")
  )
    return <Navigate to="/setup" replace />;
  return (
    <Routes>
      <Route path="/login" element={<Login theme={theme} />} />
      <Route path="/auth/callback" element={<Callback />} />
      <Route path="/setup/auth/callback" element={<Callback setup />} />
      <Route path="/setup" element={<Setup />} />
      <Route path="/*" element={<Workspace theme={theme} />} />
    </Routes>
  );
}
function Login({ theme }: { theme: React.ReactNode }) {
  const config = useLoad(api.authConfig);
  const [username, setUsername] = useState(""),
    [password, setPassword] = useState(""),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const navigate = useNavigate();
  const next =
    new URLSearchParams(location.search).get("redirect") || "/images";
  const destination =
    next.startsWith("/") && !next.startsWith("//") ? next : "/images";
  return (
    <AppEntryLayout
      brand={<strong>Islet · 五加一图床</strong>}
      eyebrow="IMAGE LIBRARY"
      title={
        <>
          每一张图片，
          <br />
          都有自己的位置。
        </>
      }
      description="收集、整理、分享。让灵感随时可用。"
      benefits={[
        { icon: <Images />, label: "集中整理你的图片" },
        { icon: <Upload />, label: "上传后，即刻分享" },
        { icon: <HardDrive />, label: "存储与访问由你掌握" },
      ]}
      actions={theme}
    >
      <h2>欢迎回来</h2>
      <p className="fp-muted">登录你的图片工作台</p>
      <Failure error={config.error} retry={config.reload} />
      <Failure error={error} />
      {config.loading ? (
        <Loading />
      ) : (
        config.data && (
          <div className="islet-form">
            {config.data.client_id && config.data.authorization_endpoint && (
              <Button
                loading={busy}
                onClick={async () => {
                  setBusy(true);
                  try {
                    sessionStorage.setItem("islet-return", destination);
                    await beginOIDCLogin(config.data!);
                  } catch (e) {
                    setError(errorText(e));
                    setBusy(false);
                  }
                }}
              >
                {config.data.oidc_label || "使用五加一账户登录"}
              </Button>
            )}
            {config.data.local_enabled && (
              <form
                className="islet-form"
                onSubmit={async (e) => {
                  e.preventDefault();
                  setBusy(true);
                  setError("");
                  try {
                    clearSession();
                    await api.localLogin(username, password);
                    navigate(destination, { replace: true });
                  } catch (e) {
                    setError(errorText(e));
                  } finally {
                    setBusy(false);
                  }
                }}
              >
                <TextField
                  label="用户名"
                  value={username}
                  onChange={setUsername}
                  required
                />
                <TextField
                  label="密码"
                  type="password"
                  value={password}
                  onChange={setPassword}
                  required
                />
                <Button type="submit" loading={busy}>
                  登录工作台
                </Button>
              </form>
            )}
          </div>
        )
      )}
    </AppEntryLayout>
  );
}
function Callback({ setup = false }: { setup?: boolean }) {
  const [error, setError] = useState("");
  const navigate = useNavigate();
  useEffect(() => {
    let stopped = false;
    async function finish() {
      try {
        const params = new URLSearchParams(location.search);
        if (params.has("error")) throw new Error("登录未完成，请重新尝试");
        const state = consumeOIDCState(),
          code = params.get("code");
        if (!state || !code) throw new Error("登录请求已过期，请重新登录");
        const result = await api.oidcExchange(
          code,
          state.verifier,
          location.origin + location.pathname,
        );
        if (stopped) return;
        clearSession();
        saveSession(result.access_token, result.id_token);
        if (setup) {
          await api.setupVerifyAdmin({
            mode: "oidc",
            token: result.id_token || result.access_token,
          });
          navigate("/setup", { replace: true });
        } else {
          await api.me();
          const next = sessionStorage.getItem("islet-return") || "/images";
          sessionStorage.removeItem("islet-return");
          navigate(
            next.startsWith("/") && !next.startsWith("//") ? next : "/images",
            { replace: true },
          );
        }
      } catch (e) {
        if (!stopped) setError(errorText(e));
      }
    }
    void finish();
    return () => {
      stopped = true;
    };
  }, []);
  return (
    <main className="islet-standalone">
      <Card>
        <h1>正在登录</h1>
        {error ? (
          <>
            <Failure error={error} />
            <Button onClick={() => navigate("/login")}>返回登录</Button>
          </>
        ) : (
          <Loading />
        )}
      </Card>
    </main>
  );
}
function Workspace({ theme }: { theme: React.ReactNode }) {
  const [me, setMe] = useState<MeInfo>(),
    [error, setError] = useState(""),
    [message, setMessage] = useState(""),
    [mobile, setMobile] = useState(false),
    [logout, setLogout] = useState(false);
  const navigate = useNavigate(),
    loc = useLocation();
  async function refresh() {
    try {
      setMe(await api.meInfo());
      setError("");
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        clearSession();
        uploads.reset();
        navigate(
          "/login?redirect=" + encodeURIComponent(loc.pathname + loc.search),
          { replace: true },
        );
      } else setError(errorText(e));
    }
  }
  useEffect(() => {
    void refresh();
    const update = () => void refresh();
    const expired = () => {
      clearSession();
      uploads.reset();
      navigate("/login", { replace: true });
    };
    const leave = (e: BeforeUnloadEvent) => {
      if (uploads.pending()) {
        e.preventDefault();
        e.returnValue = "";
      }
    };
    window.addEventListener("islet:changed", update);
    window.addEventListener("islet:session-expired", expired);
    window.addEventListener("beforeunload", leave);
    return () => {
      window.removeEventListener("islet:changed", update);
      window.removeEventListener("islet:session-expired", expired);
      window.removeEventListener("beforeunload", leave);
    };
  }, []);
  if (!me)
    return (
      <main className="islet-standalone">
        {error ? (
          <Failure error={error} retry={() => void refresh()} />
        ) : (
          <Loading />
        )}
      </main>
    );
  const admin = me.user.role !== "user";
  const items = navigation
    .filter((_, i) => admin || i < 6)
    .map(([id, label, Icon]) => ({ id, label, icon: <Icon size={19} /> }));
  const nav = (
    <>
      <div className="islet-brand">
        <Images />
        <strong>Islet</strong>
        <span>五加一图床</span>
      </div>
      <ProductNavigation
        variant="sidebar"
        items={items}
        value={loc.pathname}
        onChange={(path) => {
          navigate(path);
          setMobile(false);
        }}
      />
      <footer className="islet-capacity">
        <span>我的空间</span>
        <strong>
          {humanBytes(me.quota.used)}{" "}
          <small>
            / {me.quota.limit ? humanBytes(me.quota.limit) : "不限"}
          </small>
        </strong>
        {!!me.quota.limit && (
          <progress value={me.quota.used} max={me.quota.limit} />
        )}
      </footer>
    </>
  );
  return (
    <Session.Provider value={{ me, refresh, notify: setMessage }}>
      <div className="islet-shell">
        <aside className="islet-sidebar">{nav}</aside>
        <div className="islet-workspace">
          <header className="islet-topbar">
            <Button
              variant="ghost"
              className="islet-mobile-nav"
              aria-label="打开导航"
              onClick={() => setMobile(true)}
            >
              <Menu size={20} />
            </Button>
            <span>
              {items.find((x) => x.id === loc.pathname)?.label || "图片工作台"}
            </span>
            <div>
              {theme}
              <AccountMenu
                name={me.user.username || me.user.email || "我的账户"}
                description={
                  me.user.role === "super_admin"
                    ? "超级管理员"
                    : me.user.role === "admin"
                      ? "管理员"
                      : "成员"
                }
                items={[
                  {
                    id: "account",
                    label: "我的账户",
                    icon: <UserRound size={17} />,
                  },
                  {
                    id: "logout",
                    label: "退出登录",
                    icon: <LogOut size={17} />,
                    separator: true,
                  },
                ]}
                onSelect={(id) =>
                  id === "account" ? navigate("/profile") : setLogout(true)
                }
              />
            </div>
          </header>
          <main className="islet-content">
            <Failure error={error} retry={() => void refresh()} />
            <Routes>
              <Route path="/" element={<Navigate to="/images" replace />} />
              <Route path="/images" element={<Gallery />} />
              <Route path="/trash" element={<Gallery recycled />} />
              <Route path="/upload" element={<UploadPage />} />
              <Route path="/albums" element={<Albums />} />
              <Route path="/tokens" element={<Tokens />} />
              <Route path="/profile" element={<Profile />} />
              {["overview", "policies", "groups", "users", "settings"].map(
                (path) => (
                  <Route
                    key={path}
                    path={"/" + path}
                    element={
                      admin ? (
                        <AdminPage kind={path} />
                      ) : (
                        <Navigate to="/images" replace />
                      )
                    }
                  />
                ),
              )}
              <Route path="*" element={<Navigate to="/images" replace />} />
            </Routes>
          </main>
        </div>
      </div>
      <Dialog
        drawer
        open={mobile}
        title="图片工作台"
        onClose={() => setMobile(false)}
      >
        {nav}
      </Dialog>
      {message && <Toast message={message} onClose={() => setMessage("")} />}
      {logout && (
        <Confirm
          title="退出登录"
          description={
            uploads.pending()
              ? "仍有图片正在上传，退出会取消未完成的上传。"
              : "退出当前账户，返回登录页。"
          }
          onClose={() => setLogout(false)}
          onConfirm={async () => {
            await api.logout();
            clearSession();
            uploads.reset();
            navigate("/login", { replace: true });
          }}
        />
      )}
    </Session.Provider>
  );
}
