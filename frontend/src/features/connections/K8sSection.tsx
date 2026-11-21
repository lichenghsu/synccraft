import { useEffect, useState } from "react";
import {
  api,
  CNPGUser,
  DBType,
  errMsg,
  FileKind,
  type K8sContexts,
  type K8sForward,
  type K8sTargets,
} from "../../api/wails";
import { FileField } from "./FileField";
import { DEFAULT_PORT, type K8sTarget } from "./model";

type Props = {
  dbType: DBType;
  k8s: K8sForward;
  target: K8sTarget;
  onChange: (k8s: K8sForward) => void;
  onTargetChange: (t: K8sTarget) => void;
};

const TARGETS: { value: K8sTarget; label: string; pgOnly?: boolean }[] = [
  { value: "cnpg", label: "CloudNativePG", pgOnly: true },
  { value: "service", label: "Service" },
  { value: "pod", label: "Pod" },
];

// Kubernetes route: kubeconfig, context, namespace and what to forward to.
// Lists are loaded from the cluster; every field can also be typed in, since
// RBAC often forbids listing namespaces.
export function K8sSection({
  dbType,
  k8s,
  target,
  onChange,
  onTargetChange,
}: Props) {
  const [contexts, setContexts] = useState<K8sContexts | null>(null);
  const [namespaces, setNamespaces] = useState<string[]>([]);
  const [targets, setTargets] = useState<K8sTargets | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  const set = (patch: Partial<K8sForward>) => onChange({ ...k8s, ...patch });
  const kubeconfig = k8s.kubeconfigPath ?? "";
  const context = k8s.context ?? "";

  useEffect(() => {
    let live = true;
    api
      .k8sContexts(kubeconfig)
      .then((c) => live && (setContexts(c), setError("")))
      .catch((e) => live && (setContexts(null), setError(errMsg(e))));
    return () => {
      live = false;
    };
  }, [kubeconfig]);

  const load = async (fn: () => Promise<void>) => {
    setError("");
    setLoading(true);
    try {
      await fn();
    } catch (e) {
      setError(errMsg(e));
    } finally {
      setLoading(false);
    }
  };

  const loadNamespaces = () =>
    load(async () =>
      setNamespaces(await api.k8sNamespaces(kubeconfig, context)),
    );
  const loadTargets = () =>
    load(async () =>
      setTargets(await api.k8sTargets(kubeconfig, context, k8s.namespace)),
    );

  const clusters = targets?.cnpgClusters ?? [];
  const services = targets?.services ?? [];
  const available = TARGETS.filter(
    (t) => !t.pgOnly || dbType === DBType.PostgreSQL,
  );

  return (
    <div className="section ssh-fields">
      <FileField
        label="kubeconfig (default: $KUBECONFIG or ~/.kube/config)"
        optional
        value={k8s.kubeconfigPath}
        choices={[{ kind: FileKind.FileKubeconfig, label: "Choose…" }]}
        onChange={(kubeconfigPath) => set({ kubeconfigPath })}
      />

      <div className="grid2">
        <label className="field grow">
          <span className="label">Context</span>
          <select
            value={context}
            onChange={(e) => set({ context: e.target.value })}
          >
            <option value="">
              current{contexts?.current ? ` (${contexts.current})` : ""}
            </option>
            {(contexts?.contexts ?? []).map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </select>
        </label>
        <label className="field grow">
          <span className="label">Namespace</span>
          <div className="row">
            <input
              list="k8s-namespaces"
              value={k8s.namespace}
              onChange={(e) => set({ namespace: e.target.value })}
              required
            />
            <datalist id="k8s-namespaces">
              {namespaces.map((n) => (
                <option key={n} value={n} />
              ))}
            </datalist>
            <button
              type="button"
              className="ghost"
              onClick={() => void loadNamespaces()}
              disabled={loading}
            >
              List
            </button>
          </div>
        </label>
      </div>

      <div className="field">
        <span className="label">Target</span>
        <div className="row">
          <div
            className="segmented"
            role="radiogroup"
            aria-label="Kubernetes target"
          >
            {available.map((t) => (
              <button
                key={t.value}
                type="button"
                role="radio"
                aria-checked={target === t.value}
                className={target === t.value ? "on" : ""}
                onClick={() => onTargetChange(t.value)}
              >
                {t.label}
              </button>
            ))}
          </div>
          <button
            type="button"
            className="ghost"
            onClick={() => void loadTargets()}
            disabled={loading || !k8s.namespace.trim()}
          >
            Find in namespace
          </button>
        </div>
      </div>

      {target === "cnpg" && (
        <>
          <div className="grid2">
            <label className="field grow">
              <span className="label">Cluster</span>
              <input
                list="k8s-clusters"
                value={k8s.cnpgCluster ?? ""}
                onChange={(e) => set({ cnpgCluster: e.target.value })}
                required
              />
              <datalist id="k8s-clusters">
                {clusters.map((c) => (
                  <option key={c.name} value={c.name}>
                    {`${c.readyInstances}/${c.instances} ready · ${c.phase}`}
                  </option>
                ))}
              </datalist>
            </label>
            <label className="field grow">
              <span className="label">Log in as</span>
              <select
                value={k8s.cnpgUser || CNPGUser.CNPGUserApp}
                onChange={(e) => set({ cnpgUser: e.target.value as CNPGUser })}
              >
                <option value={CNPGUser.CNPGUserApp}>
                  app owner (&lt;cluster&gt;-app)
                </option>
                <option value={CNPGUser.CNPGUserSuperuser}>
                  superuser (&lt;cluster&gt;-superuser)
                </option>
              </select>
            </label>
          </div>
          <p className="hint">
            Connects to the <code>-rw</code> (primary) service. User, password
            and database come from the cluster's secret and TLS is verified with
            its <code>-ca</code> secret at connect time; nothing is stored on
            this computer.
          </p>
        </>
      )}

      {target !== "cnpg" && (
        <div className="grid2">
          {target === "service" ? (
            <label className="field grow">
              <span className="label">Service</span>
              <input
                list="k8s-services"
                value={k8s.service ?? ""}
                onChange={(e) => set({ service: e.target.value })}
                required
              />
              <datalist id="k8s-services">
                {services.map((s) => (
                  <option key={s.name} value={s.name}>
                    {(s.ports ?? []).join(", ")}
                  </option>
                ))}
              </datalist>
            </label>
          ) : (
            <label className="field grow">
              <span className="label">Pod</span>
              <input
                value={k8s.pod ?? ""}
                onChange={(e) => set({ pod: e.target.value })}
                required
              />
            </label>
          )}
          <label className="field port">
            <span className="label">Port</span>
            <input
              type="number"
              min={0}
              max={65535}
              value={k8s.port || ""}
              placeholder={String(DEFAULT_PORT[dbType])}
              onChange={(e) => set({ port: Number(e.target.value) || 0 })}
            />
          </label>
        </div>
      )}

      {error && <p className="err">{error}</p>}
    </div>
  );
}
