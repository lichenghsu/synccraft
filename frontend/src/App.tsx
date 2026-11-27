import { useState } from "react";
import { BrowsePage } from "./features/browse/BrowsePage";
import { ConnectionsPage } from "./features/connections/ConnectionsPage";

type Page = "connections" | "browse";

export default function App() {
  const [page, setPage] = useState<Page>("connections");
  return (
    <main className="shell">
      <header className="topbar row between">
        <div>
          <h1>
            Sync<span>Craft</span>
          </h1>
          <p>Database schema &amp; data compare</p>
        </div>
        <nav className="segmented">
          <button
            type="button"
            className={page === "connections" ? "on" : ""}
            onClick={() => setPage("connections")}
          >
            Connections
          </button>
          <button
            type="button"
            className={page === "browse" ? "on" : ""}
            onClick={() => setPage("browse")}
          >
            Browse
          </button>
        </nav>
      </header>
      {page === "connections" ? <ConnectionsPage /> : <BrowsePage />}
    </main>
  );
}
