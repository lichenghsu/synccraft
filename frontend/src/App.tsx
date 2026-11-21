import { ConnectionsPage } from "./features/connections/ConnectionsPage";

export default function App() {
  return (
    <main className="shell">
      <header className="topbar">
        <h1>
          Sync<span>Craft</span>
        </h1>
        <p>Database schema &amp; data compare</p>
      </header>
      <ConnectionsPage />
    </main>
  );
}
