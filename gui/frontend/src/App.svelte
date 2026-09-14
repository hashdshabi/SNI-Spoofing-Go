<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { _, locale } from "svelte-i18n";
  import { GetDefaultConfig, LoadConfig, SaveConfig, UTLSPresets, InjectorModes, Start, Stop, Status, RunTest } from "../wailsjs/go/main/App.js";
  import { EventsOn, EventsOff } from "../wailsjs/runtime/runtime.js";
  import type { main } from "../wailsjs/go/models";
  import { appendLog, resetLogIds, type LogEntry } from "./logs";

  type ProxyConfig = main.ProxyConfig;
  type ProxyStatus = main.ProxyStatus;
  type TestResult = main.TestResult;
  type TestSummary = main.TestSummary;
  type Preset = { fakeSni: string; upstream: string };

  let cfg: ProxyConfig | null = null;
  let utlsList: string[] = [];
  let injectorList: string[] = [];
  let presets: Preset[] = [];
  let selectedPreset = -1;
  let status: ProxyStatus = { running: false, testing: false, listenAddr: "" };
  let logs: LogEntry[] = [];
  let testResults: TestResult[] = [];
  let testSummary: TestSummary | null = null;
  let busy = false;
  let saving = false;
  let rightPanelTab: "logs" | "results" = "logs";

  function pushLog(entry: Omit<LogEntry, "id">) { logs = appendLog(logs, entry); }
  function pushError(err: unknown) { pushLog({ ts: Date.now(), level: "error", message: err instanceof Error ? err.message : String(err) }); }

  onMount(async () => {
    try {
      const saved = await LoadConfig();
      cfg = saved.config;
      presets = saved.presets || [];
      if (presets.length) selectedPreset = 0;
    } catch (err) {
      pushError(err);
      cfg = await GetDefaultConfig();
    }
    utlsList = await UTLSPresets();
    injectorList = await InjectorModes();
    status = await Status();
    EventsOn("log", (e: { level: string; message: string }) => pushLog({ ts: Date.now(), level: e.level, message: e.message }));
    EventsOn("status", (s: ProxyStatus) => status = s);
    EventsOn("test_result", (row: TestResult) => testResults = [...testResults, row]);
  });

  onDestroy(() => { EventsOff("log"); EventsOff("status"); EventsOff("test_result"); });

  function applyPreset(index: number) {
    if (!cfg || !presets[index]) return;
    selectedPreset = index;
    cfg.fakeSni = presets[index].fakeSni;
    cfg.connect = presets[index].upstream.includes(":") ? presets[index].upstream : `${presets[index].upstream}:443`;
  }

  function presetKey(p: Preset) { return `${p.fakeSni.toLowerCase()}\x00${p.upstream.toLowerCase()}`; }
  function normalizePresets(list: Preset[]) {
    const seen = new Set<string>();
    return list.filter((p) => {
      p.fakeSni = p.fakeSni.trim(); p.upstream = p.upstream.trim();
      if (!p.fakeSni || !p.upstream) return false;
      const key = presetKey(p); if (seen.has(key)) return false; seen.add(key); return true;
    });
  }
  function addPreset() {
    const sni = cfg?.fakeSni?.trim() || "developers.cloudflare.com";
    const upstream = (cfg?.connect?.trim() || "104.16.2.189:443").replace(/:443$/, "");
    const next = normalizePresets([...presets, { fakeSni: sni, upstream }]);
    if (next.length === presets.length) { pushLog({ ts: Date.now(), level: "warn", message: "This Fake SNI + upstream preset already exists." }); return; }
    presets = next; selectedPreset = presets.length - 1;
  }
  function updateSelectedPreset() {
    if (selectedPreset < 0 || !cfg || !presets[selectedPreset]) return;
    const upstream = cfg.connect.trim().replace(/:443$/, "");
    presets[selectedPreset] = { fakeSni: cfg.fakeSni.trim(), upstream };
    presets = normalizePresets(presets);
  }
  function deletePreset() {
    if (selectedPreset < 0) return;
    presets = presets.filter((_, i) => i !== selectedPreset);
    selectedPreset = presets.length ? Math.min(selectedPreset, presets.length - 1) : -1;
    if (selectedPreset >= 0) applyPreset(selectedPreset);
  }

  async function onSave() {
    if (!cfg) return;
    saving = true;
    try {
      updateSelectedPreset();
      await SaveConfig(cfg, normalizePresets(presets));
      pushLog({ ts: Date.now(), level: "info", message: "Configuration saved to config.ini" });
    } catch (err) { pushError(err); }
    finally { saving = false; }
  }

  async function onStart() {
    if (!cfg) return; busy = true; rightPanelTab = "logs";
    try { await Start(cfg); } catch (err) { pushError(err); } finally { busy = false; }
  }
  async function onStop() {
    busy = true; try { await Stop(); } catch (err) { pushError(err); } finally { busy = false; }
  }
  function partialSummaryFromResults(results: TestResult[]): TestSummary {
    let passed = 0, failed = 0; for (const r of results) r.pass ? passed++ : failed++;
    return { preflight: testSummary?.preflight ?? {}, results, passed, failed };
  }
  async function onRunTest() {
    if (!cfg) return; busy = true; testResults = []; testSummary = null; rightPanelTab = "results";
    try {
      testSummary = await RunTest(cfg);
      if (testSummary.results && testSummary.results.length > testResults.length) testResults = testSummary.results;
    } catch (err) {
      if (testResults.length) testSummary = partialSummaryFromResults(testResults); else { pushError(err); testSummary = null; }
    } finally { busy = false; }
  }
  function onLocaleChange(ev: Event) { locale.set((ev.target as HTMLSelectElement).value); }
  function clearLogs() { resetLogIds(); logs = []; }
  const logTimeFormatter = new Intl.DateTimeFormat("en", { hour: "2-digit", minute: "2-digit", second: "2-digit" });
  function formatTime(ts: number) { return logTimeFormatter.format(new Date(ts)); }
</script>

<header class="topbar">
  <div class="brand"><div class="brand-title">{$_("app.title")}</div><div class="brand-subtitle">{$_("app.subtitle")}</div></div>
  <div class="topbar-spacer"></div>
  <div class="status-pill" class:running={status.running} class:testing={status.testing}><span class="dot"></span>{#if status.testing}{$_("status.testing")}{:else if status.running}{$_("status.running")}{:else}{$_("status.stopped")}{/if}{#if status.running && status.listenAddr}<span class="status-detail">{status.listenAddr}</span>{/if}</div>
  <div class="lang-switch"><label for="lang">{$_("lang.label")}</label><select id="lang" value={$locale} on:change={onLocaleChange}><option value="en">English</option><option value="fa">فارسی</option></select></div>
</header>

<main class="layout">
  <section class="panel">
    {#if cfg}
      <div class="section-title">Connection & Presets</div>
      <div class="preset-box">
        <label><span>Fake SNI preset</span><select value={selectedPreset} on:change={(e) => applyPreset(Number((e.target as HTMLSelectElement).value))}>
          <option value={-1}>Manual configuration</option>
          {#each presets as p, i}<option value={i}>{p.fakeSni} → {p.upstream}</option>{/each}
        </select></label>
        <div class="preset-actions"><button class="btn" on:click={addPreset}>Add</button><button class="btn" on:click={updateSelectedPreset} disabled={selectedPreset < 0}>Update</button><button class="btn danger" on:click={deletePreset} disabled={selectedPreset < 0}>Delete</button></div>
        <small>Each preset is identified by Fake SNI + upstream IP, so duplicate SNI values are allowed.</small>
      </div>
      <div class="grid-2">
        <label><span>{$_("form.listen")}</span><input type="text" bind:value={cfg.listen} placeholder="127.0.0.1:40443" /><small>{$_("form.listen_help")}</small></label>
        <label><span>{$_("form.connect")}</span><input type="text" bind:value={cfg.connect} placeholder="host:443" /><small>{$_("form.connect_help")}</small></label>
        <label><span>{$_("form.fake_sni")}</span><input type="text" bind:value={cfg.fakeSni} placeholder="hcaptcha.com" /><small>{$_("form.fake_sni_help")}</small></label>
        <label><span>{$_("form.utls")}</span><select bind:value={cfg.utls}>{#each utlsList as p}<option value={p}>{p}</option>{/each}</select></label>
      </div>

      <div class="section-title">{$_("form.section_injection")}</div>
      <div class="grid-2">
        <label><span>{$_("form.injector")}</span><select bind:value={cfg.injector}>{#each injectorList as m}<option value={m}>{m}</option>{/each}</select></label>
        <label><span>{$_("form.fake_repeat")}</span><input type="number" min="1" bind:value={cfg.fakeRepeat} /></label>
        <label><span>{$_("form.fake_delay")}</span><input type="number" min="0" bind:value={cfg.fakeDelayMs} /></label>
        <label><span>{$_("form.ack_timeout")}</span><input type="number" min="1" bind:value={cfg.ackTimeoutMs} /></label>
      </div>
      <div class="section-title">{$_("form.section_fragmentation")}</div>
      <div class="grid-2">
        <label class="checkbox-row"><input type="checkbox" bind:checked={cfg.enableFragment} /><span>{$_("form.enable_fragment")}</span></label><span></span>
        <label><span>{$_("form.fragment_delay")}</span><input type="number" min="0" bind:value={cfg.fragmentDelayMs} disabled={!cfg.enableFragment} /></label>
        <label><span>{$_("form.sni_chunk")}</span><input type="number" min="0" bind:value={cfg.sniChunk} disabled={!cfg.enableFragment} /></label>
      </div>
      <div class="actions">
        {#if status.running || status.testing}<button class="btn danger" on:click={onStop} disabled={busy && !status.testing}>{status.testing ? $_("actions.cancel_test") : $_("actions.stop")}</button>{:else}<button class="btn primary" on:click={onStart} disabled={busy}>{$_("actions.start")}</button>{/if}
        <button class="btn" on:click={onRunTest} disabled={busy || status.running || status.testing}>{$_("actions.test")}</button>
        <button class="btn save" on:click={onSave} disabled={saving || busy}>{saving ? "Saving…" : "Save"}</button>
      </div>
    {/if}
  </section>

  <section class="panel side-panel">
    <div class="panel-header"><div class="tab-bar" role="tablist" aria-label={$_("logs.title")}>
      <button type="button" class="tab" class:active={rightPanelTab === "logs"} on:click={() => rightPanelTab = "logs"}>{$_("panel.tab_logs")}</button>
      <button type="button" class="tab" class:active={rightPanelTab === "results"} on:click={() => rightPanelTab = "results"}>{$_("panel.tab_results")}{#if testResults.length > 0}<span class="tab-badge">{testResults.length}</span>{/if}</button>
    </div>{#if rightPanelTab === "logs"}<button class="btn-link" on:click={clearLogs}>{$_("actions.clear_logs")}</button>{/if}</div>
    {#if rightPanelTab === "logs"}
      <div class="tab-panel log-list" role="tabpanel" dir="ltr">{#if logs.length === 0}<div class="empty">{$_("logs.empty")}</div>{:else}{#each logs as line (line.id)}<div class="log-line log-{line.level}"><span class="log-time">{formatTime(line.ts)}</span><span class="log-msg">{line.message}</span></div>{/each}{/if}</div>
    {:else}
      <div class="tab-panel test-results" role="tabpanel">{#if testSummary}<div class="test-preflight">{#if testSummary.preflight.externalIp}<span>{$_("test.external_ip")}: {testSummary.preflight.externalIp}</span>{/if}{#if testSummary.preflight.internalIp}<span>{$_("test.internal_ip")}: {testSummary.preflight.internalIp}</span>{/if}{#if testSummary.preflight.warning}<span class="test-preflight-note">{testSummary.preflight.warning}</span>{/if}<span class="test-summary-counts">{$_("test.pass")}: {testSummary.passed} / {$_("test.fail")}: {testSummary.failed}</span></div>{/if}{#if testResults.length === 0}<div class="empty">{$_("test.empty")}</div>{:else}<table class="test-table"><thead><tr><th>{$_("test.col_utls")}</th><th>{$_("test.col_repeat")}</th><th>{$_("test.col_fragment")}</th><th>{$_("test.col_result")}</th></tr></thead><tbody>{#each testResults as r}<tr><td>{r.utls}</td><td>{r.fakeRepeat}</td><td>{r.enableFragment ? $_("test.on") : $_("test.off")}</td><td class:pass={r.pass} class:fail={!r.pass}>{r.pass ? $_("test.pass") : $_("test.fail")}</td></tr>{/each}</tbody></table>{/if}</div>
    {/if}
  </section>
</main>

<style>
  .topbar{display:flex;align-items:center;gap:16px;padding-inline:20px;padding-block:14px;border-block-end:1px solid var(--border);background:var(--panel)}
  .brand-title{font-weight:700;font-size:16px}.brand-subtitle{color:var(--muted);font-size:12px}.topbar-spacer{flex:1}
  .status-pill{display:inline-flex;align-items:center;gap:8px;padding-inline:12px;padding-block:6px;border-radius:999px;background:var(--panel-2);border:1px solid var(--border);font-size:13px}.status-pill .dot{width:8px;height:8px;border-radius:50%;background:var(--muted)}.status-pill.running .dot{background:var(--ok);box-shadow:0 0 0 3px rgba(61,220,151,.18)}.status-pill.testing .dot{background:var(--warn);box-shadow:0 0 0 3px rgba(255,214,107,.18)}.status-detail{color:var(--muted);margin-inline-start:6px}
  .lang-switch{display:flex;align-items:center;gap:8px}.lang-switch label{color:var(--muted);font-size:12px}.lang-switch select{background:var(--panel-2);color:var(--text);border:1px solid var(--border);border-radius:var(--radius);padding-inline:10px;padding-block:6px}
  .layout{display:grid;grid-template-columns:minmax(380px,1fr) minmax(360px,1fr);gap:16px;padding:16px;flex:1;min-height:0}.panel{background:var(--panel);border:1px solid var(--border);border-radius:var(--radius);padding:18px;overflow:auto}.panel.side-panel{display:flex;flex-direction:column;overflow:hidden}
  .panel-header{display:flex;align-items:center;justify-content:space-between;gap:12px;margin-block-end:8px}.tab-bar{display:flex;gap:4px}.tab{display:inline-flex;align-items:center;gap:6px;background:transparent;color:var(--muted);border:1px solid transparent;border-radius:var(--radius);padding-inline:12px;padding-block:6px;font-size:12px;font-weight:600}.tab:hover{color:var(--text);background:var(--panel-2)}.tab.active{color:var(--text);background:var(--panel-2);border-color:var(--border)}.tab-badge{min-width:18px;padding-inline:5px;border-radius:999px;background:var(--accent-strong);color:white;font-size:10px;line-height:16px;text-align:center}
  .tab-panel{flex:1;min-height:0;overflow-y:auto}.test-results{background:var(--bg);border:1px solid var(--border);border-radius:var(--radius);padding:8px}.section-title{font-weight:600;color:var(--muted);text-transform:uppercase;letter-spacing:.06em;font-size:11px;margin-block:14px 8px}.grid-2{display:grid;grid-template-columns:1fr 1fr;gap:12px 16px}
  label{display:flex;flex-direction:column;gap:4px;font-size:13px}label>span{color:var(--muted);font-size:12px}label small{color:var(--muted);font-size:11px}input[type="text"],input[type="number"],select{background:var(--panel-2);border:1px solid var(--border);border-radius:var(--radius);padding-inline:10px;padding-block:8px;color:var(--text);outline:none}input:focus,select:focus{border-color:var(--accent)}input:disabled{opacity:.5}.checkbox-row{flex-direction:row;align-items:center;gap:8px}
  .preset-box{background:var(--bg);border:1px solid var(--border);border-radius:var(--radius);padding:12px;margin-block-end:12px}.preset-actions{display:flex;gap:8px;margin-block-start:8px}.preset-box>small{display:block;color:var(--muted);font-size:11px;margin-block-start:8px}.actions{display:flex;gap:10px;margin-block-start:18px}.btn{background:var(--panel-2);color:var(--text);border:1px solid var(--border);border-radius:var(--radius);padding-inline:14px;padding-block:8px;font-weight:600}.btn:hover:not(:disabled){border-color:var(--accent)}.btn:disabled{opacity:.5;cursor:not-allowed}.btn.primary{background:var(--accent-strong);border-color:var(--accent-strong);color:white}.btn.save{border-color:var(--accent);color:var(--accent)}.btn.danger{background:#5a2030;border-color:#7a2a40;color:#ffd0d0}.btn-link{background:transparent;border:none;color:var(--accent);font-size:12px;padding:0}
  .log-list{direction:ltr;text-align:left;unicode-bidi:isolate;background:var(--bg);border:1px solid var(--border);border-radius:var(--radius);padding:8px;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,"Liberation Mono","Courier New",monospace;font-size:12px}.empty{color:var(--muted);text-align:center;padding-block:12px}.log-line{display:flex;flex-direction:row;gap:10px;padding-block:2px}.log-time{color:var(--muted);flex-shrink:0}.log-error .log-msg{color:var(--err)}.log-warn .log-msg{color:var(--warn)}.log-debug .log-msg{color:var(--muted)}
  .test-preflight{display:flex;flex-wrap:wrap;gap:8px 16px;font-size:12px;color:var(--muted);margin-block:8px}.test-preflight-note{color:var(--warn)}.test-summary-counts{margin-inline-start:auto;color:var(--text)}.test-table{width:100%;border-collapse:collapse;font-size:12px}.test-table th,.test-table td{text-align:start;padding:6px 8px;border-block-end:1px solid var(--border)}.test-table th{color:var(--muted);font-weight:600}td.pass{color:var(--ok);font-weight:600}td.fail{color:var(--err);font-weight:600}
</style>
