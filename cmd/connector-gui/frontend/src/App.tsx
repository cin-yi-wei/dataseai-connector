import { useState, useEffect, useCallback } from 'react';
import './App.css';
import {
  GetStatus,
  SaveConfig,
  InstallAndStart,
  Start,
  Stop,
  Restart,
  CopyDiagnostics,
} from '../wailsjs/go/main/App';
import { main } from '../wailsjs/go/models';

type Status = main.GUIStatus;

const DEFAULT_SERVER = 'wss://dataseai.conray.top/agent';

function statusBadgeClass(s: string) {
  if (s === 'running') return 'badge badge-green';
  if (s === 'stopped') return 'badge badge-red';
  if (s === 'not_installed') return 'badge badge-gray';
  return 'badge badge-yellow';
}

function App() {
  const [status, setStatus] = useState<Status | null>(null);
  const [token, setToken] = useState('');
  const [server, setServer] = useState(DEFAULT_SERVER);
  const [executor, setExecutor] = useState('mysql');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [copied, setCopied] = useState(false);
  const [showAdvanced, setShowAdvanced] = useState(false);

  const fetchStatus = useCallback(() => {
    GetStatus()
      .then((s) => {
        setStatus(s);
        if (s.server) setServer(s.server);
        if (s.executor) setExecutor(s.executor);
      })
      .catch((e) => setError(String(e)));
  }, []);

  useEffect(() => {
    fetchStatus();
    const id = setInterval(fetchStatus, 5000);
    return () => clearInterval(id);
  }, [fetchStatus]);

  async function run(fn: () => Promise<unknown>) {
    setLoading(true);
    setError('');
    try {
      await fn();
      fetchStatus();
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }

  const handleInstallAndStart = () =>
    run(() => InstallAndStart(token, server, executor));

  const handleStart = () => run(() => Start());
  const handleStop = () => run(() => Stop());
  const handleRestart = () => run(() => Restart());

  const handleSaveConfig = () =>
    run(() => SaveConfig(token, server, executor));

  const handleCopyDiagnostics = async () => {
    setError('');
    try {
      const diag = await CopyDiagnostics();
      await navigator.clipboard.writeText(diag);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (e) {
      setError(String(e));
    }
  };

  const svcStatus = status?.service_status ?? '—';
  const disabled = loading;

  return (
    <div id="App">
      <h1>DataseAI Connector</h1>

      {/* Status row */}
      <div className="status-row">
        <span>Service:</span>
        <span className={statusBadgeClass(svcStatus)}>{svcStatus}</span>
        {status?.agent_status && (
          <>
            <span>Agent:</span>
            <span className={statusBadgeClass(status.agent_status)}>{status.agent_status}</span>
          </>
        )}
      </div>

      {/* Error banner */}
      {error && (
        <div className="error-banner">
          <strong>Error:</strong> {error}
        </div>
      )}

      {/* Config form */}
      <div className="form-section">
        <label htmlFor="token">Agent Token</label>
        <input
          id="token"
          type="password"
          value={token}
          placeholder={status?.token_masked || 'Paste agent token here'}
          onChange={(e) => setToken(e.target.value)}
          className="input"
          autoComplete="off"
        />
        <div className="advanced-toggle" onClick={() => setShowAdvanced(!showAdvanced)}>
          {showAdvanced ? '▾' : '▸'} Advanced
        </div>
        {showAdvanced && (
          <div className="advanced-fields">
            <label htmlFor="server">Server URL</label>
            <input
              id="server"
              type="text"
              value={server}
              onChange={(e) => setServer(e.target.value)}
              className="input"
            />
            <label htmlFor="executor">Executor</label>
            <input
              id="executor"
              type="text"
              value={executor}
              onChange={(e) => setExecutor(e.target.value)}
              className="input"
            />
          </div>
        )}
      </div>

      {/* Action buttons */}
      <div className="button-row">
        <button className="btn btn-primary" onClick={handleInstallAndStart} disabled={disabled || !token}>
          Install &amp; Start
        </button>
        <button className="btn" onClick={handleSaveConfig} disabled={disabled || !token}>
          Save Config
        </button>
        <button className="btn" onClick={handleRestart} disabled={disabled}>
          Restart
        </button>
        <button className="btn btn-danger" onClick={handleStop} disabled={disabled}>
          Stop
        </button>
        <button className="btn" onClick={handleStart} disabled={disabled}>
          Start
        </button>
        <button className="btn btn-secondary" onClick={handleCopyDiagnostics} disabled={loading}>
          {copied ? 'Copied!' : 'Copy Diagnostics'}
        </button>
      </div>

      {/* Config path */}
      {status?.config_path && (
        <div className="config-path">Config: {status.config_path}</div>
      )}

      {/* Log panel */}
      <div className="log-panel">
        <div className="log-header">Logs</div>
        <pre className="log-body">
          {status?.log_lines?.length
            ? status.log_lines.join('\n')
            : 'No logs yet.'}
        </pre>
      </div>
    </div>
  );
}

export default App;
