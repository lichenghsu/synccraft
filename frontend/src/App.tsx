import { useState } from "react";
import { CountTick } from "@events";
import type { CountTick as CountTickData } from "@bindings/internal/app";
import { api, errMsg, isCancel } from "./api/wails";
import { useAsync } from "./hooks/useAsync";
import { useCancellable } from "./hooks/useCancellable";
import { useWailsEvent } from "./hooks/useWailsEvent";

export default function App() {
  return (
    <main className="shell">
      <header className="topbar">
        <h1>
          Sync<span>Craft</span>
        </h1>
        <p>Database schema &amp; data compare · M0</p>
      </header>
      <div className="grid">
        <HelloCard />
        <CountCard />
      </div>
    </main>
  );
}

function HelloCard() {
  const [name, setName] = useState("");
  const hello = useAsync(api.hello);

  return (
    <section className="card">
      <h2>1. 呼叫與錯誤</h2>
      <p className="hint">Go 回傳 error 時，前端的 Promise 會被 reject。留空試試看。</p>
      <form
        className="row"
        onSubmit={(e) => {
          e.preventDefault();
          void hello.run(name);
        }}
      >
        <input value={name} onChange={(e) => setName(e.target.value)} placeholder="你的名字" />
        <button disabled={hello.state.status === "loading"}>Hello</button>
      </form>
      {hello.state.status === "success" && <p className="ok">{hello.state.data}</p>}
      {hello.state.status === "error" && <p className="err">{hello.state.error}</p>}
    </section>
  );
}

function CountCard() {
  const total = 50;
  const [tick, setTick] = useState<CountTickData | null>(null);
  const [result, setResult] = useState<string>("");
  const count = useCancellable(api.count);

  useWailsEvent(CountTick, setTick);

  const start = async () => {
    setTick(null);
    setResult("");
    try {
      const n = await count.run(total, 80);
      setResult(`完成，共 ${n} 步`);
    } catch (e) {
      setResult(isCancel(e) ? "已取消" : `錯誤：${errMsg(e)}`);
    }
  };

  const pct = tick ? Math.round((tick.current / tick.total) * 100) : 0;

  return (
    <section className="card">
      <h2>2. 可取消的呼叫 + 3. 事件</h2>
      <p className="hint">按「取消」會讓 Go 端的 context 被取消，迴圈立即結束。</p>
      <div className="row">
        <button onClick={start} disabled={count.running}>
          開始
        </button>
        <button className="ghost" onClick={count.cancel} disabled={!count.running}>
          取消
        </button>
      </div>
      <div className="progress" aria-label="progress">
        <div style={{ width: `${pct}%` }} />
      </div>
      <p className="mono">
        {tick ? `${tick.current} / ${tick.total}` : "—"} {result && `· ${result}`}
      </p>
    </section>
  );
}
