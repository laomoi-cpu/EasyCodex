package server

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"easycodex-agent/internal/config"
	"easycodex-agent/internal/netinfo"
)

func (s *Server) homePage(w http.ResponseWriter, r *http.Request) {
	if !isLocalRequest(r) {
		writeError(w, http.StatusForbidden, fmt.Errorf("EasyCodex console is only available from localhost"))
		return
	}
	http.Redirect(w, r, "/pairing", http.StatusFound)
}

func (s *Server) settingsPage(w http.ResponseWriter, r *http.Request) {
	if !isLocalRequest(r) {
		writeError(w, http.StatusForbidden, fmt.Errorf("settings page is only available from localhost"))
		return
	}
	lang := s.updateUILanguageFromSettings(r)
	writeHTML(w, settingsPageHTML(lang))
}

func (s *Server) connectionsPage(w http.ResponseWriter, r *http.Request) {
	if !isLocalRequest(r) {
		writeError(w, http.StatusForbidden, fmt.Errorf("connections page is only available from localhost"))
		return
	}
	lang := normalizeUILang(s.configSnapshot().UILanguage)
	writeHTML(w, connectionsPageHTML(lang))
}

func (s *Server) terminalPage(w http.ResponseWriter, r *http.Request) {
	lang := normalizeUILang(s.configSnapshot().UILanguage)
	writeHTML(w, terminalPageHTML(lang))
}

func (s *Server) statusPage(w http.ResponseWriter, r *http.Request) {
	if !isLocalRequest(r) {
		writeError(w, http.StatusForbidden, fmt.Errorf("status page is only available from localhost"))
		return
	}
	cfg := s.configSnapshot()
	lang := normalizeUILang(cfg.UILanguage)
	network := netinfo.Inspect(cfg.Listen)
	body := fmt.Sprintf(`
<section class="hero compact">
  <div>
    <p class="eyebrow">%s</p>
    <h1>%s</h1>
    <p class="lead">%s</p>
  </div>
  <div class="status-card">
    <span class="status-dot"></span>
    <strong>%s</strong>
    <small>%s</small>
  </div>
</section>
<section class="panel-grid two">
  <article class="panel">
    <h2>%s</h2>
    <dl class="kv">
      <dt>%s</dt><dd>%s</dd>
      <dt>%s</dt><dd>%s</dd>
      <dt>%s</dt><dd>%s</dd>
      <dt>%s</dt><dd>%s</dd>
    </dl>
  </article>
  <article class="panel">
    <h2>%s</h2>
    <dl class="kv">
      <dt>%s</dt><dd>%s</dd>
      <dt>%s</dt><dd>%s</dd>
      <dt>%s</dt><dd>%s</dd>
    </dl>
  </article>
</section>
<section class="panel pair-section">
  <div class="panel-title-row">
    <h2>%s</h2>
    <button id="runNetworkTests" type="button">%s</button>
  </div>
  <p class="pair-hint">%s</p>
  <div id="networkTestState" class="muted-text">%s</div>
  <div id="networkTestResults" class="test-list"></div>
</section>`,
		html.EscapeString(lang.t("agentStatus")),
		html.EscapeString(lang.t("statusRunning")),
		html.EscapeString(lang.t("statusLead")),
		html.EscapeString(lang.t("online")),
		html.EscapeString(timeNow()),
		html.EscapeString(lang.t("network")),
		html.EscapeString(lang.t("listen")),
		html.EscapeString(network.Listen),
		html.EscapeString(lang.t("localURL")),
		html.EscapeString(network.LocalURL),
		html.EscapeString(lang.t("publicURL")),
		html.EscapeString(emptyDash(cfg.PublicBaseURL)),
		html.EscapeString(lang.t("lan")),
		html.EscapeString(strings.Join(network.LANURLs, ", ")),
		html.EscapeString(lang.t("configuration")),
		html.EscapeString(lang.t("configFile")),
		html.EscapeString(s.configPath),
		html.EscapeString(lang.t("defaultCWD")),
		html.EscapeString(cfg.MobileDefaults.CWD),
		html.EscapeString(lang.t("defaultInstance")),
		html.EscapeString(cfg.MobileDefaults.InstanceID),
		html.EscapeString(lang.t("httpServiceTest")),
		html.EscapeString(lang.t("testHTTP")),
		html.EscapeString(lang.t("httpTestHint")),
		html.EscapeString(lang.t("notTested")),
	)
	writeHTML(w, pageShell(lang, "status", "status", body, `<script>`+statusJS(lang)+`</script>`))
}

func (s *Server) writePairingConsole(w http.ResponseWriter, lang uiLang, baseURLs []string) {
	var cards strings.Builder
	var browserCards strings.Builder
	for _, baseURL := range baseURLs {
		pairURL := baseURL + "/api/mobile-pair?code=" + url.QueryEscape(s.mobilePairCode())
		deepLink := "easycodex://pair?u=" + url.QueryEscape(pairURL)
		qrURL := "/api/pairing/qr.svg?data=" + url.QueryEscape(deepLink)
		fmt.Fprintf(&cards, `
<article class="pair-card">
  <div class="qr-frame"><img src="%s" alt="Pairing QR"></div>
  <div class="pair-meta">
    <span class="badge">%s</span>
    <h3>%s</h3>
    <p class="pair-hint">%s</p>
    <label>%s</label>
    <code>%s</code>
    <label>%s</label>
    <code>%s</code>
  </div>
</article>`,
			html.EscapeString(qrURL),
			networkBadge(lang, baseURL),
			html.EscapeString(lang.t("androidScanPair")),
			html.EscapeString(lang.t("androidScanPairHint")),
			html.EscapeString(lang.t("agentBaseURL")),
			html.EscapeString(baseURL),
			html.EscapeString(lang.t("qrAppLink")),
			html.EscapeString(deepLink))

		browserURL := baseURL + "/terminal#baseUrl=" + url.QueryEscape(baseURL) + "&token=" + url.QueryEscape(s.configSnapshot().Token)
		browserQRURL := "/api/pairing/qr.svg?data=" + url.QueryEscape(browserURL)
		fmt.Fprintf(&browserCards, `
<article class="pair-card">
  <div class="qr-frame"><img src="%s" alt="Browser Terminal QR"></div>
  <div class="pair-meta">
    <span class="badge">%s</span>
    <h3>%s</h3>
    <p class="pair-hint">%s</p>
    <label>%s</label>
    <a class="link-field" href="%s">%s</a>
    <label>%s</label>
    <code>%s</code>
  </div>
</article>`,
			html.EscapeString(browserQRURL),
			networkBadge(lang, baseURL),
			html.EscapeString(lang.t("browserScanOpen")),
			html.EscapeString(lang.t("browserPairHint")),
			html.EscapeString(lang.t("pcBrowserURL")),
			html.EscapeString(browserURL),
			html.EscapeString(baseURL+"/terminal"),
			html.EscapeString(lang.t("qrFullLink")),
			html.EscapeString(browserURL))
	}

	body := fmt.Sprintf(`
<section class="hero">
  <div>
    <p class="eyebrow">%s</p>
    <h1>%s</h1>
    <p class="lead">%s</p>
  </div>
  <img class="hero-mark" src="/assets/easycodex.svg" alt="">
</section>
<section class="panel pair-section"><h2>%s</h2><div class="pair-grid">%s</div></section>
<section class="panel pair-section"><h2>%s</h2><div class="pair-grid">%s</div></section>`,
		html.EscapeString(lang.t("pairing")),
		html.EscapeString(lang.t("pairingTitle")),
		html.EscapeString(lang.t("pairingLead")),
		html.EscapeString(lang.t("androidPairTitle")),
		cards.String(),
		html.EscapeString(lang.t("browserPairTitle")),
		browserCards.String())
	writeHTML(w, pageShell(lang, "pairing", "pairing", body, ""))
}

func (s *Server) easycodexIcon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	_, _ = w.Write([]byte(easycodexSVG()))
}

func writeHTML(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(body))
}

func (s *Server) updateUILanguageFromSettings(r *http.Request) uiLang {
	cfg := s.configSnapshot()
	if raw := r.URL.Query().Get("lang"); raw != "" {
		lang := normalizeUILang(raw)
		if cfg.UILanguage != string(lang) {
			cfg.UILanguage = string(lang)
			if err := config.Save(s.configPath, cfg); err == nil {
				s.setConfig(cfg)
			}
		}
		return lang
	}
	return normalizeUILang(cfg.UILanguage)
}

func pageShell(lang uiLang, titleKey, active, body, script string) string {
	return pageShellWithChrome(lang, titleKey, active, body, script, true)
}

func pageShellWithChrome(lang uiLang, titleKey, active, body, script string, chrome bool) string {
	nav := func(id, href, label string) string {
		class := ""
		if id == active {
			class = ` class="active"`
		}
		return fmt.Sprintf(`<a%s href="%s">%s</a>`, class, href, label)
	}
	title := lang.t(titleKey)
	header := ""
	if chrome {
		header = `<header class="topbar">
  <a class="brand" href="/pairing"><img src="/assets/easycodex.svg" alt=""><span>EasyCodex</span></a>
  <nav>` + nav("pairing", "/pairing", html.EscapeString(lang.t("navPairing"))) + nav("connections", "/connections", html.EscapeString(lang.t("connections"))) + nav("settings", "/settings", html.EscapeString(lang.t("settings"))) + nav("status", "/status", html.EscapeString(lang.t("status"))) + `<span class="version-badge">v` + html.EscapeString(AppVersion) + `</span><a class="github-link" href="https://github.com/laomoi-cpu/EasyCodex" target="_blank" rel="noreferrer">` + html.EscapeString(lang.t("github")) + `</a></nav>
</header>`
	}
	return `<!doctype html>
<html lang="` + html.EscapeString(string(lang)) + `">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>` + html.EscapeString(machinePageTitle(localMachineName(), title)) + `</title>
<link rel="icon" href="/assets/easycodex.svg">
<style>` + consoleCSS() + `</style>
</head>
<body class="page-` + html.EscapeString(active) + `">
` + header + `
<main>` + body + `</main>
` + script + `
</body>
</html>`
}

func machinePageTitle(machineName, pageTitle string) string {
	appTitle := "EasyCodex"
	machineName = strings.TrimSpace(machineName)
	if machineName != "" {
		appTitle = machineName + " - " + appTitle
	}
	pageTitle = strings.TrimSpace(pageTitle)
	if pageTitle != "" {
		return appTitle + " " + pageTitle
	}
	return appTitle
}

func localMachineName() string {
	name, err := os.Hostname()
	if err != nil {
		return ""
	}
	return name
}

func connectionsPageHTML(lang uiLang) string {
	body := fmt.Sprintf(`
<section class="hero compact">
  <div>
    <p class="eyebrow">%s</p>
    <h1>%s</h1>
    <p class="lead">%s</p>
  </div>
  <div id="connectionsState" class="status-card muted">%s</div>
</section>
<section class="panel">
  <div class="panel-title-row">
    <h2>%s</h2>
    <button type="button" class="secondary" id="refreshConnections">%s</button>
  </div>
  <div class="table-wrap">
    <table class="connection-table">
      <thead>
        <tr>
          <th>%s</th>
          <th>%s</th>
          <th>%s</th>
          <th>%s</th>
          <th>%s</th>
          <th>%s</th>
        </tr>
      </thead>
      <tbody id="connectionsBody"></tbody>
    </table>
  </div>
</section>`,
		html.EscapeString(lang.t("connectedTerminals")),
		html.EscapeString(lang.t("connectionsTitle")),
		html.EscapeString(lang.t("connectionsLead")),
		html.EscapeString(lang.t("loading")),
		html.EscapeString(lang.t("connectionsList")),
		html.EscapeString(lang.t("refresh")),
		html.EscapeString(lang.t("terminal")),
		html.EscapeString(lang.t("typeLabel")),
		html.EscapeString(lang.t("address")),
		html.EscapeString(lang.t("lastSeen")),
		html.EscapeString(lang.t("lastRequest")),
		html.EscapeString(lang.t("count")))
	script := `<script>` + connectionsJS(lang) + `</script>`
	return pageShell(lang, "connections", "connections", body, script)
}

func statusJS(lang uiLang) string {
	return jsI18N(lang) + `
const statusTestButton = document.getElementById('runNetworkTests');
const statusTestState = document.getElementById('networkTestState');
const statusTestResults = document.getElementById('networkTestResults');
function setNetworkTestState(text){ statusTestState.textContent = text; }
function renderNetworkTestRows(items, pending){
  statusTestResults.innerHTML = '';
  if(!items.length){
    statusTestResults.innerHTML = '<div class="muted-text">' + escapeHtml(i18n.noURLs) + '</div>';
    return;
  }
  items.forEach(item => {
    const row = document.createElement('div');
    row.className = 'test-row ' + (pending ? '' : (item.ok ? 'ok' : 'err'));
    const status = pending ? i18n.testing : (item.ok ? i18n.ok : i18n.failed);
    const latency = pending ? '-' : (item.latencyMs + ' ms');
    const error = pending ? '' : (item.error || item.service || '');
    row.innerHTML = '<strong>' + escapeHtml(item.label || '-') + '</strong>' +
      '<code title="' + escapeAttr(item.url || '') + '">' + escapeHtml(item.url || '-') + '</code>' +
      '<span>' + escapeHtml(status) + '</span>' +
      '<span>' + escapeHtml(latency) + '</span>' +
      (error ? '<div class="muted-text" style="grid-column:1/-1">' + escapeHtml(error) + '</div>' : '');
    statusTestResults.appendChild(row);
  });
}
async function runNetworkTests(){
  statusTestButton.disabled = true;
  setNetworkTestState(i18n.testingHTTP);
  renderNetworkTestRows([], true);
  try{
    const res = await fetch('/api/network-tests');
    const payload = await res.json();
    if(!payload.ok) throw new Error(payload.error || i18n.networkTestFailed);
    const data = payload.data || {};
    renderNetworkTestRows(data.results || [], false);
    const okCount = (data.results || []).filter(item => item.ok).length;
    setNetworkTestState(format(i18n.endpointOK, {ok: okCount, total: ((data.results || []).length)}));
  }catch(err){
    setNetworkTestState(err.message);
  }finally{
    statusTestButton.disabled = false;
  }
}
function escapeHtml(v){ return String(v).replace(/[&<>"']/g, ch=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch])); }
function escapeAttr(v){ return escapeHtml(v); }
function format(text, values){ return String(text).replace(/\{(\w+)\}/g, (_, key)=>values[key] ?? ''); }
statusTestButton.addEventListener('click', runNetworkTests);
`
}

func terminalPageHTML(lang uiLang) string {
	body := fmt.Sprintf(`
<section class="terminal-page">
  <section id="connectPanel" class="panel terminal-connect">
    <div>
      <p class="eyebrow">%s</p>
      <h1>%s</h1>
      <p class="lead">%s</p>
    </div>
    <form id="connectForm" class="connect-form">
      <label><span>%s</span><input id="baseUrl" autocomplete="off" placeholder="http://192.168.x.x:8765"></label>
      <label><span>%s</span><input id="browserToken" autocomplete="off" placeholder="easycodex-dev-token"></label>
      <button type="submit">%s</button>
    </form>
  </section>

  <section id="terminalApp" class="terminal-app" hidden>
    <aside class="terminal-sidebar">
      <div class="terminal-toolbar">
        <button id="newSession" type="button">+</button>
        <button id="refreshSessions" type="button" class="secondary">%s</button>
      </div>
      <div id="paneList" class="pane-list"></div>
    </aside>
    <section class="terminal-workbench">
      <div class="terminal-statusbar">
        <button id="connectionStatus" type="button" class="status-pill">%s</button>
        <button id="editConnection" type="button" class="secondary icon-button" aria-label="%s" title="%s">⚙</button>
      </div>
      <pre id="terminalOutput" class="terminal-output">%s</pre>
      <div id="keyPanel" class="key-panel" hidden>
        <button data-key="enter" type="button">Enter</button>
        <button data-key="ctrlc" type="button">Ctrl+C</button>
        <button data-key="shifttab" type="button">S+Tab</button>
        <button data-key="shiftpgup" type="button">S+PgUp</button>
        <button data-key="shiftpgdn" type="button">S+PgDn</button>
        <button data-key="space" type="button">Space</button>
        <button data-key="up" type="button">Up</button>
        <button data-key="down" type="button">Down</button>
        <button data-key="esc" type="button">Esc</button>
      </div>
      <form id="sendForm" class="send-row">
        <input id="commandInput" autocomplete="off" placeholder="%s">
        <button id="toggleKeys" type="button" class="secondary">Keys</button>
        <button type="submit">%s</button>
      </form>
    </section>
  </section>
</section>

<dialog id="paneDialog" class="pane-dialog">
  <h2 id="dialogTitle">%s</h2>
  <dl id="dialogDetails" class="kv"></dl>
  <div class="dialog-actions">
    <button id="dialogClose" type="button" class="secondary">%s</button>
    <button id="dialogDelete" type="button" class="danger">%s</button>
    <button id="dialogClone" type="button">%s</button>
  </div>
</dialog>
<dialog id="connectionDialog" class="pane-dialog">
  <h2>%s</h2>
  <div class="connect-form">
    <label><span>%s</span><input id="dialogBaseUrl" autocomplete="off" placeholder="http://192.168.x.x:8765"></label>
    <label><span>%s</span><input id="dialogToken" autocomplete="off" placeholder="easycodex-dev-token"></label>
  </div>
  <div class="dialog-actions">
    <button id="connectionCancel" type="button" class="secondary">%s</button>
    <button id="connectionSave" type="button">%s</button>
  </div>
</dialog>`,
		html.EscapeString(lang.t("browserTerminal")),
		html.EscapeString(lang.t("connectTitle")),
		html.EscapeString(lang.t("connectHint")),
		html.EscapeString(lang.t("agentBaseURL")),
		html.EscapeString(lang.t("token")),
		html.EscapeString(lang.t("connect")),
		html.EscapeString(lang.t("refresh")),
		html.EscapeString(lang.t("offline")),
		html.EscapeString(lang.t("serverSettings")),
		html.EscapeString(lang.t("serverSettings")),
		html.EscapeString(lang.t("terminalPrompt")),
		html.EscapeString(lang.t("messagePlaceholder")),
		html.EscapeString(lang.t("send")),
		html.EscapeString(lang.t("sessionPrefix")),
		html.EscapeString(lang.t("close")),
		html.EscapeString(lang.t("delete")),
		html.EscapeString(lang.t("clone")),
		html.EscapeString(lang.t("serverSettings")),
		html.EscapeString(lang.t("agentBaseURL")),
		html.EscapeString(lang.t("token")),
		html.EscapeString(lang.t("cancel")),
		html.EscapeString(lang.t("save")))
	script := `<script>` + terminalJS(lang) + `</script>`
	return pageShellWithChrome(lang, "terminal", "terminal", body, script, false)
}

func settingsPageHTML(lang uiLang) string {
	body := fmt.Sprintf(`
<section class="hero compact">
  <div>
    <p class="eyebrow">%s</p>
    <h1>%s</h1>
    <p class="lead">%s</p>
  </div>
  <div id="saveState" class="status-card muted">%s</div>
</section>
<form id="settingsForm" class="settings-layout">
  <section class="panel">
    <h2>%s</h2>
    <div class="field-grid">
      <label><span>%s</span><input id="listen" autocomplete="off" placeholder="0.0.0.0:8765"></label>
      <label><span>%s</span><input id="token" autocomplete="off"></label>
      <label><span>%s</span><input id="publicBaseUrl" autocomplete="off" placeholder="http://100.x.y.z:8765"></label>
      <label><span>%s</span><input id="root" autocomplete="off"></label>
      <label><span>%s</span><input id="timeout" type="number" min="1" max="120"></label>
      <label><span>%s</span><input id="version" readonly></label>
      <label><span>%s</span><select id="uiLanguage"><option value="zh">%s</option><option value="en">%s</option></select></label>
    </div>
    <label class="check-row"><input id="regenToken" type="checkbox"><span>%s</span></label>
    <label class="check-row"><input id="lanPromptShown" type="checkbox"><span>%s</span></label>
    <label class="check-row"><input id="closeGui" type="checkbox"><span>%s</span></label>
  </section>

  <section class="panel">
    <h2>%s</h2>
    <div id="updateState" class="update-state muted">%s</div>
    <div class="update-progress"><div id="updateProgressBar"></div></div>
    <div id="updateProgressText" class="muted-text">%s</div>
    <label class="check-row update-option"><input id="useGitHubProxy" type="checkbox" checked><span>%s</span></label>
    <a id="releaseLink" class="link-field update-link" href="#" target="_blank" rel="noreferrer" hidden>%s</a>
    <div class="dialog-actions update-actions">
      <button type="button" class="secondary" id="checkUpdate">%s</button>
      <button type="button" id="applyUpdate" disabled>%s</button>
    </div>
  </section>

  <section class="panel">
    <div class="panel-title-row">
      <h2>%s</h2>
      <button type="button" class="secondary" id="addInstance">%s</button>
    </div>
    <div id="instances" class="instance-list"></div>
  </section>

  <section class="panel">
    <h2>%s</h2>
    <div class="field-grid">
      <label><span>%s</span><select id="defaultInstance"></select></label>
      <label><span>%s</span><input id="defaultCwd" autocomplete="off" placeholder="D:\mgame"></label>
    </div>
    <label><span>%s</span><textarea id="defaultCommand" rows="5" spellcheck="false"></textarea></label>
  </section>

  <section class="panel">
    <h2>%s</h2>
    <div id="autoLaunch" class="choice-list"></div>
  </section>

  <div class="actionbar">
    <button type="button" class="secondary" id="reload">%s</button>
    <button type="submit">%s</button>
  </div>
</form>`,
		html.EscapeString(lang.t("agentSettings")),
		html.EscapeString(lang.t("configureTitle")),
		html.EscapeString(lang.t("configureLead")),
		html.EscapeString(lang.t("loading")),
		html.EscapeString(lang.t("network")),
		html.EscapeString(lang.t("listenAddress")),
		html.EscapeString(lang.t("agentToken")),
		html.EscapeString(lang.t("publicBaseURL")),
		html.EscapeString(lang.t("easyRoot")),
		html.EscapeString(lang.t("commandTimeout")),
		html.EscapeString(lang.t("currentVersion")),
		html.EscapeString(lang.t("uiLanguage")),
		html.EscapeString(lang.t("chinese")),
		html.EscapeString(lang.t("english")),
		html.EscapeString(lang.t("regenToken")),
		html.EscapeString(lang.t("lanPrompt")),
		html.EscapeString(lang.t("closeGui")),
		html.EscapeString(lang.t("versionUpdate")),
		html.EscapeString(lang.t("updateInitial")),
		html.EscapeString(lang.t("updateNoRunning")),
		html.EscapeString(lang.t("githubProxy")),
		html.EscapeString(lang.t("openRelease")),
		html.EscapeString(lang.t("checkUpdate")),
		html.EscapeString(lang.t("applyUpdate")),
		html.EscapeString(lang.t("instances")),
		html.EscapeString(lang.t("addInstance")),
		html.EscapeString(lang.t("mobileDefaults")),
		html.EscapeString(lang.t("defaultInstance")),
		html.EscapeString(lang.t("workingDirectory")),
		html.EscapeString(lang.t("commandArguments")),
		html.EscapeString(lang.t("autoLaunch")),
		html.EscapeString(lang.t("reload")),
		html.EscapeString(lang.t("saveSettings")))
	script := `<script>const currentUILanguage = "` + html.EscapeString(string(lang)) + `";</script><script>` + settingsJS(lang) + `</script>`
	return pageShell(lang, "settings", "settings", body, script)
}

func consoleCSS() string {
	return `
:root{color-scheme:light;--bg:#eef1f5;--text:#18212f;--muted:#667085;--panel:#fff;--line:#d7dce5;--accent:#0f8b8d;--accent2:#f59e0b;--ink:#101828;--shadow:0 14px 36px rgba(16,24,40,.10)}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--text);font:14px/1.5 "Segoe UI",Arial,sans-serif}code,input,textarea,select{font-family:Consolas,"Cascadia Mono",monospace}
.topbar{height:64px;display:flex;align-items:center;justify-content:space-between;padding:0 28px;background:rgba(255,255,255,.9);border-bottom:1px solid var(--line);position:sticky;top:0;z-index:2;backdrop-filter:blur(12px)}
.brand{display:flex;align-items:center;gap:10px;color:var(--ink);text-decoration:none;font-weight:700;font-size:17px}.brand img{width:34px;height:34px}
nav{display:flex;gap:6px;align-items:center}nav a{color:#475467;text-decoration:none;padding:8px 12px;border-radius:7px}nav a.active,nav a:hover{background:#e7f4f4;color:#075f63}.version-badge{display:inline-flex;align-items:center;height:30px;padding:0 9px;border-radius:999px;background:#eef2ff;color:#3730a3;font-size:12px;font-weight:800}nav a.github-link{background:#2563eb;color:#fff;font-weight:700}nav a.github-link:hover{background:#1d4ed8;color:#fff}
main{max-width:1180px;margin:0 auto;padding:28px}.hero{display:flex;justify-content:space-between;align-items:center;gap:28px;margin-bottom:22px}.hero.compact{align-items:flex-end}.eyebrow{text-transform:uppercase;letter-spacing:.08em;color:var(--accent);font-weight:700;font-size:12px;margin:0 0 6px}.hero h1{margin:0;max-width:780px;font-size:34px;line-height:1.12;letter-spacing:0}.lead{max-width:820px;color:var(--muted);font-size:16px;margin:10px 0 0}.hero-mark{width:126px;height:126px;flex:0 0 auto}
.panel,.pair-card,.status-card{background:var(--panel);border:1px solid var(--line);border-radius:8px;box-shadow:var(--shadow)}.panel{padding:22px}.panel h2{margin:0 0 16px;font-size:17px}.panel-grid{display:grid;gap:16px}.panel-grid.two{grid-template-columns:repeat(2,minmax(0,1fr))}
.pair-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(340px,1fr));gap:16px}.pair-card{display:grid;grid-template-columns:190px minmax(0,1fr);gap:18px;padding:18px}.qr-frame{display:grid;place-items:center;border:1px solid var(--line);background:#fafafa;border-radius:8px;aspect-ratio:1}.qr-frame img{width:166px;height:166px}.pair-meta{min-width:0}.pair-meta h3{margin:2px 0 6px;font-size:18px}.pair-hint{margin:0 0 10px;color:var(--muted);font-size:13px;line-height:1.5}.pair-meta label{display:block;color:var(--muted);font-size:12px;margin:12px 0 4px}.pair-meta code,.kv dd,.link-field{display:block;word-break:break-all;background:#f5f7fa;border:1px solid #e4e7ec;border-radius:6px;padding:9px;color:#253244}.link-field{color:#1d4ed8;text-decoration:none;font-weight:700}.link-field:hover{text-decoration:underline}
.badge{display:inline-flex;align-items:center;height:24px;padding:0 9px;border-radius:999px;background:#e7f4f4;color:#075f63;font-weight:700;font-size:12px}.table-wrap{overflow:auto;border:1px solid var(--line);border-radius:8px}.connection-table{width:100%;border-collapse:collapse;min-width:860px;background:#fff}.connection-table th,.connection-table td{padding:11px 12px;border-bottom:1px solid var(--line);text-align:left;vertical-align:top}.connection-table th{background:#f8fafc;color:#475467;font-size:12px;text-transform:uppercase;letter-spacing:.04em}.connection-table tr:last-child td{border-bottom:0}.connection-table code{display:block;max-width:260px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.muted-text{color:var(--muted);font-size:12px}.test-list{display:grid;gap:8px;margin-top:12px}.test-row{display:grid;grid-template-columns:90px minmax(0,1fr) 90px 90px;gap:10px;align-items:center;padding:10px;border:1px solid var(--line);border-radius:7px;background:#f8fafc}.test-row strong{color:#18212f}.test-row code{white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.test-row.ok{border-color:#86efac;background:#ecfdf3}.test-row.err{border-color:#fca5a5;background:#fef3f2}.settings-layout{display:grid;grid-template-columns:1fr 1fr;gap:16px}.settings-layout .panel:nth-child(3),.settings-layout .panel:nth-child(4){grid-column:span 1}.field-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:14px}label span{display:block;color:#344054;font-weight:600;margin-bottom:6px}input,textarea,select{width:100%;border:1px solid #cfd6df;border-radius:7px;padding:10px 11px;background:#fff;color:var(--text);font-size:14px}textarea{resize:vertical}.check-row{display:flex;gap:10px;align-items:center;margin-top:16px}.check-row input{width:auto}.check-row span{margin:0;font-weight:500}
.update-state{border:1px solid var(--line);border-radius:7px;background:#f8fafc;padding:12px;min-height:68px;color:#344054}.update-state.ok{background:#ecfdf3;border-color:#abefc6;color:#067647}.update-state.work{background:#eff6ff;border-color:#bfdbfe;color:#1d4ed8}.update-state.err{background:#fef3f2;border-color:#fecdca;color:#b42318}.update-progress{height:9px;border-radius:999px;background:#e5e7eb;overflow:hidden;margin-top:12px}.update-progress div{height:100%;width:0;background:#2563eb;transition:width .18s ease}.update-option{margin-top:12px}.update-link{margin-top:10px}.update-actions{margin-top:14px}button:disabled{opacity:.55;cursor:not-allowed}
.panel-title-row{display:flex;align-items:center;justify-content:space-between;gap:12px}.instance-row{display:grid;grid-template-columns:1fr 1fr 1fr auto;gap:10px;align-items:end;margin-top:10px}.choice-list{display:grid;gap:10px}.choice{display:flex;align-items:center;gap:10px;padding:10px;border:1px solid var(--line);border-radius:7px}.choice input{width:auto}.actionbar{grid-column:1/-1;display:flex;justify-content:flex-end;gap:10px;padding:14px 0 4px}
button{border:0;border-radius:7px;background:var(--accent);color:#fff;font-weight:700;padding:10px 16px;cursor:pointer}button:hover{filter:brightness(.96)}button.secondary{background:#fff;color:#344054;border:1px solid #cfd6df}.remove{background:#fff4ed;color:#b54708;border:1px solid #fed7aa}.status-card{padding:16px;min-width:220px}.status-card.muted{color:var(--muted)}.status-dot{display:inline-block;width:9px;height:9px;border-radius:50%;background:#12b76a;margin-right:8px}.status-card small{display:block;color:var(--muted);margin-top:4px}.kv{display:grid;grid-template-columns:145px minmax(0,1fr);gap:10px;margin:0}.kv dt{color:var(--muted)}.kv dd{margin:0}
.pair-section{margin-top:16px}.pair-section h2{margin-bottom:16px}.terminal-page{display:grid;gap:16px}.terminal-connect{display:grid;grid-template-columns:minmax(0,1fr) 360px;gap:22px;align-items:end}.connect-form{display:grid;gap:12px}.terminal-app{display:grid;grid-template-columns:280px minmax(0,1fr);gap:14px;height:calc(100vh - 122px);min-height:620px}.terminal-sidebar,.terminal-workbench{background:#fff;border:1px solid var(--line);border-radius:8px;box-shadow:var(--shadow);min-height:0}.terminal-sidebar{display:flex;flex-direction:column;padding:12px}.terminal-toolbar{display:grid;grid-template-columns:48px 1fr;gap:8px;margin-bottom:10px}.pane-list{display:grid;gap:8px;overflow:auto}.pane-item{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:8px;align-items:center;padding:9px;border:1px solid var(--line);border-radius:7px;background:#f8fafc;text-align:left;color:#18212f}.pane-item.active{background:#dbeafe;border-color:#93c5fd}.pane-item.selected{outline:2px solid #2563eb}.pane-main{display:block;min-width:0}.pane-title{display:block;font-weight:700;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.pane-meta{display:block;color:var(--muted);font-size:12px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.pane-menu{padding:7px 9px}.terminal-workbench{display:grid;grid-template-rows:auto minmax(0,1fr) auto auto;padding:12px;gap:10px}.terminal-statusbar{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:8px}.status-pill{background:#e5e7eb;color:#374151;border:1px solid #d1d5db}.status-pill.ok{background:#d1fae5;color:#065f46;border-color:#86efac}.status-pill.work{background:#dbeafe;color:#1d4ed8;border-color:#93c5fd}.status-pill.err{background:#fee2e2;color:#991b1b;border-color:#fca5a5}.terminal-output{margin:0;overflow:auto;background:#0b1220;color:#e6edf3;border-radius:8px;padding:14px;font:13px/1.45 Consolas,"Cascadia Mono",monospace;white-space:pre-wrap;word-break:break-word}.send-row{display:grid;grid-template-columns:minmax(0,1fr) 54px 92px;gap:8px}.send-row input{height:44px}.key-panel{display:grid;grid-template-columns:repeat(9,minmax(0,1fr));gap:6px}.key-panel[hidden]{display:none!important}.key-panel button{padding:7px 6px;font-size:12px}.pane-dialog{border:1px solid var(--line);border-radius:8px;box-shadow:var(--shadow);max-width:560px;width:calc(100% - 28px);padding:18px}.pane-dialog h2{margin:0 0 14px}.dialog-actions{display:flex;justify-content:flex-end;gap:8px;margin-top:16px}.danger{background:#fee2e2!important;color:#991b1b!important;border:1px solid #fca5a5!important}
.pane-last{display:block;color:#0f8b8d;font-size:12px;font-weight:700;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.icon-button{width:38px;padding:0;display:inline-grid;place-items:center;font-size:18px}.page-terminal{background:#0b1220;overflow:hidden}.page-terminal .topbar{display:none}.page-terminal main{max-width:none;height:100dvh;padding:12px 12px 18px;background:#0b1220}.page-terminal .terminal-page{height:100%;display:block}.page-terminal .terminal-app{height:calc(100dvh - 30px);min-height:0;grid-template-columns:320px minmax(0,1fr);gap:10px}.page-terminal .terminal-app[hidden],.page-terminal .terminal-connect[hidden]{display:none!important}.page-terminal .terminal-sidebar{background:#111827;border-color:#263244;box-shadow:none}.page-terminal .terminal-toolbar{grid-template-columns:40px 1fr}.page-terminal .pane-list{gap:7px}.page-terminal .pane-item{background:#182235;border-color:#263244;color:#e5e7eb;padding:10px;border-radius:7px}.page-terminal .pane-item.active{background:#12313d;border-color:#0f8b8d}.page-terminal .pane-item.selected{outline:2px solid #38bdf8}.page-terminal .pane-title{color:#f8fafc}.page-terminal .pane-meta{color:#94a3b8}.page-terminal .pane-last{color:#67e8f9}.page-terminal .terminal-workbench{position:relative;border:0;box-shadow:none;background:#0b1220;padding:0 0 10px;display:grid;grid-template-rows:minmax(0,1fr) auto auto}.page-terminal .terminal-output{border:1px solid #263244;border-radius:7px;padding-top:44px;font-size:var(--terminal-font-size,14px);line-height:1.45}.page-terminal .terminal-statusbar{position:absolute;top:8px;right:8px;z-index:5;display:flex;gap:6px;grid-template-columns:none}.page-terminal .terminal-statusbar button{height:30px;padding:0 10px;font-size:12px}.page-terminal .terminal-statusbar .icon-button{width:32px;padding:0;font-size:16px}.page-terminal .send-row{padding-bottom:8px}.page-terminal .terminal-connect{position:absolute;inset:0;z-index:10;min-height:0;display:grid;grid-template-columns:minmax(280px,420px);align-content:center;justify-content:center;border:0;box-shadow:none;background:#0b1220;padding:18px}.page-terminal .terminal-connect>div,.page-terminal .terminal-connect>form{background:#f8fafc}.page-terminal .terminal-connect>div{padding:22px 22px 0;border-radius:8px 8px 0 0}.page-terminal .terminal-connect>form{padding:14px 22px 22px;border-radius:0 0 8px 8px}
@media(max-width:760px){.topbar{padding:0 14px}.brand span{display:none}nav{gap:2px}nav a{padding:7px 8px;font-size:12px}main{padding:12px}.hero{display:block}.hero-mark{display:none}.settings-layout,.panel-grid.two{grid-template-columns:1fr}.field-grid{grid-template-columns:1fr}.pair-card{grid-template-columns:1fr}.instance-row{grid-template-columns:1fr}.actionbar{position:sticky;bottom:0;background:var(--bg);padding:12px 0}.test-row{grid-template-columns:1fr}.terminal-connect{grid-template-columns:1fr}.terminal-app{grid-template-columns:1fr;height:calc(100vh - 88px);min-height:0}.terminal-sidebar{max-height:128px;padding:8px}.pane-list{display:flex;overflow:auto}.pane-item{min-width:180px}.terminal-workbench{min-height:0}.terminal-output{font-size:12px;padding:10px}.send-row{grid-template-columns:minmax(0,1fr) 48px 74px}.key-panel{grid-template-columns:repeat(3,minmax(0,1fr))}}
@media(max-width:760px){.page-terminal{background:#0b1220;overflow:auto}.page-terminal .topbar{display:none}.page-terminal main{max-width:none;min-height:100dvh;padding:0;background:#0b1220}.page-terminal .terminal-page{min-height:100dvh;display:block}.page-terminal .terminal-connect{min-height:100dvh;display:grid;grid-template-columns:1fr;align-content:center;border:0;border-radius:0;box-shadow:none;padding:18px;background:#f8fafc}.page-terminal .terminal-connect h1{font-size:24px}.page-terminal .terminal-connect .lead{font-size:13px}.page-terminal .terminal-app{min-height:100dvh;display:flex;flex-direction:column;gap:0;background:#0b1220}.page-terminal .terminal-sidebar{border:0;border-radius:0;box-shadow:none;max-height:none;min-height:0;padding:6px;background:#f8fafc;display:grid;grid-template-columns:auto minmax(0,1fr);gap:6px;align-items:center}.page-terminal .terminal-toolbar{display:flex;gap:5px;margin:0;min-width:max-content}.page-terminal .terminal-toolbar button{height:30px;padding:0 8px;font-size:11px;flex:0 0 auto}.page-terminal #newSession{width:32px;padding:0}.page-terminal #refreshSessions{width:58px;padding:0}.page-terminal .pane-list{display:flex;gap:5px;min-width:0;overflow-x:auto;overflow-y:hidden;padding:1px 2px 3px;scrollbar-width:thin;-webkit-overflow-scrolling:touch}.page-terminal .pane-item{min-width:118px;max-width:160px;padding:5px 7px;border-radius:7px;flex:0 0 auto}.page-terminal .pane-title{font-size:11px}.page-terminal .pane-meta{display:none}.page-terminal .pane-menu{padding:2px 4px}.page-terminal .terminal-workbench{flex:1;min-height:0;border:0;border-radius:0;box-shadow:none;display:flex;flex-direction:column;gap:6px;padding:7px;background:#0b1220}.page-terminal .terminal-statusbar{display:grid;grid-template-columns:minmax(0,1fr) 40px;gap:6px;order:0}.page-terminal .terminal-statusbar button{height:30px;padding:0 8px;font-size:11px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}.page-terminal .terminal-output{min-height:62dvh;max-height:none;border-radius:0;padding:10px;font-size:12px;line-height:1.42;background:#0b1220;overflow:auto;flex:1}.page-terminal .key-panel{grid-template-columns:repeat(3,minmax(0,1fr));gap:6px;background:#111827;padding:7px;border-radius:7px;order:2}.page-terminal .key-panel button{height:30px;padding:0 4px;font-size:11px}.page-terminal .send-row{position:sticky;bottom:0;z-index:4;grid-template-columns:minmax(0,1fr) 42px 58px;gap:6px;order:3;background:#0b1220;padding:6px 0 7px}.page-terminal .send-row input{height:42px;border-radius:7px;font-size:14px}.page-terminal .send-row button{height:42px;padding:0 8px;font-size:12px}.page-terminal .pane-dialog{width:calc(100% - 18px);padding:14px}}`
}

func connectionsJS(lang uiLang) string {
	return jsI18N(lang) + `
const $ = id => document.getElementById(id);
function setState(text, kind='muted'){ const el=$('connectionsState'); el.className='status-card '+kind; el.textContent=text; }
async function loadConnections(){
  setState(i18n.loading);
  const res = await fetch('/api/connections');
  const payload = await res.json();
  if (!payload.ok) throw new Error(payload.error || 'Load failed');
  const items = (payload.data && payload.data.connections) || [];
  renderConnections(items);
  setState(items.length + ' ' + i18n.terminal.toLowerCase() + '(s)');
}
function renderConnections(items){
  const body = $('connectionsBody');
  body.innerHTML = '';
  if (!items.length) {
    const row = document.createElement('tr');
    row.innerHTML = '<td colspan="6" class="muted-text">' + escapeHtml(i18n.connectionsEmpty) + '</td>';
    body.appendChild(row);
    return;
  }
  items.forEach(item => {
    const row = document.createElement('tr');
    row.innerHTML = '<td><strong>' + escapeHtml(item.name || '-') + '</strong><div class="muted-text">' + escapeHtml(shortId(item.id)) + '</div></td>' +
      '<td><span class="badge">' + escapeHtml(item.kind || '-') + '</span></td>' +
      '<td><code title="' + escapeAttr(item.userAgent || '') + '">' + escapeHtml(item.remoteAddr || '-') + '</code><div class="muted-text">' + escapeHtml(item.userAgent || '-') + '</div></td>' +
      '<td>' + escapeHtml(item.lastSeen || '-') + '<div class="muted-text">First: ' + escapeHtml(item.firstSeen || '-') + '</div></td>' +
      '<td><code>' + escapeHtml((item.lastMethod || '') + ' ' + (item.lastPath || '')) + '</code></td>' +
      '<td>' + String(item.requests || 0) + '</td>';
    body.appendChild(row);
  });
}
function shortId(value){ value = String(value || ''); return value.length > 28 ? value.slice(0, 28) + '...' : value; }
function escapeHtml(v){ return String(v).replace(/[&<>"']/g, ch=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch])); }
function escapeAttr(v){ return escapeHtml(v); }
$('refreshConnections').onclick = () => loadConnections().catch(err => setState(err.message, 'error'));
loadConnections().catch(err => setState(err.message, 'error'));
setInterval(() => loadConnections().catch(err => setState(err.message, 'error')), 3000);
`
}

func terminalJS(lang uiLang) string {
	return jsI18N(lang) + `
const $ = id => document.getElementById(id);
const store = window.localStorage;
const ansiColors = [0x0b1220,0xdc2626,0x16a34a,0xd97706,0x2563eb,0xc026d3,0x0891b2,0xe6edf3,0x64748b,0xef4444,0x22c55e,0xf59e0b,0x60a5fa,0xe879f9,0x22d3ee,0xffffff];
const state = {
  baseUrl: store.getItem('easycodex.baseUrl') || location.origin,
  token: store.getItem('easycodex.token') || '',
  clientId: browserClientId(),
  instanceId: 'main',
  defaults: {instanceId:'main', cwd:'D:\\mgame', command:['cmd.exe','/k','cd /d D:\\mgame && codex --dangerously-bypass-approvals-and-sandbox']},
  panes: [], paneId: '', snapshotHash: '', pollTimer: 0, pollToken: 0, selectedPane: null
};

function browserClientId(){
  let id = store.getItem('easycodex.clientId') || '';
  if (!id) {
    id = 'browser:' + (window.crypto && crypto.randomUUID ? crypto.randomUUID() : String(Date.now()) + '-' + Math.random().toString(16).slice(2));
    store.setItem('easycodex.clientId', id);
  }
  return id;
}
function initFromHash(){
  if (!location.hash || location.hash.length < 2) return;
  const params = new URLSearchParams(location.hash.slice(1));
  const baseUrl = params.get('baseUrl');
  const token = params.get('token');
  if (baseUrl) state.baseUrl = trimSlash(baseUrl);
  if (token) state.token = token;
  saveConnectionFields();
  history.replaceState(null, '', location.pathname);
}
function trimSlash(value){ return String(value || '').trim().replace(/\/+$/, ''); }
function saveConnectionFields(){
  state.baseUrl = trimSlash($('baseUrl').value || state.baseUrl || location.origin);
  state.token = String($('browserToken').value || state.token || '').trim();
  store.setItem('easycodex.baseUrl', state.baseUrl);
  store.setItem('easycodex.token', state.token);
  $('baseUrl').value = state.baseUrl;
  $('browserToken').value = state.token;
}
function openConnectionDialog(){
  $('dialogBaseUrl').value = state.baseUrl || location.origin;
  $('dialogToken').value = state.token || '';
  const dialog = $('connectionDialog');
  if (dialog.showModal) dialog.showModal(); else dialog.setAttribute('open', 'open');
}
function closeConnectionDialog(){
  const dialog = $('connectionDialog');
  if (dialog.close) dialog.close(); else dialog.removeAttribute('open');
}
async function saveConnectionDialog(){
  state.baseUrl = trimSlash($('dialogBaseUrl').value || location.origin);
  state.token = String($('dialogToken').value || '').trim();
  $('baseUrl').value = state.baseUrl;
  $('browserToken').value = state.token;
  saveConnectionFields();
  closeConnectionDialog();
  stopPolling();
  await connect();
}
function showConnect(show){
  $('connectPanel').hidden = !show;
  $('terminalApp').hidden = show;
}
function setStatus(text, kind){
  const button = $('connectionStatus');
  button.textContent = text;
  button.className = 'status-pill ' + (kind || '');
}
async function api(path, options, auth){
  options = options || {};
  const headers = {'Accept':'application/json', 'X-EasyCodex-Client-ID': state.clientId, 'X-EasyCodex-Client-Kind': 'browser', 'X-EasyCodex-Client-Name': 'Browser Terminal'};
  if (auth !== false) headers['Authorization'] = 'Bearer ' + state.token;
  if (options.body !== undefined) headers['Content-Type'] = 'application/json';
  const res = await fetch(state.baseUrl + path, {
    method: options.method || 'GET',
    headers,
    body: options.body === undefined ? undefined : JSON.stringify(options.body)
  });
  let payload = {};
  try { payload = await res.json(); } catch (err) { throw new Error(i18n.invalidJSON); }
  if (!res.ok || !payload.ok) throw new Error(payload.error || ('HTTP ' + res.status));
  return payload.data || {};
}
async function connect(){
  saveConnectionFields();
  setStatus(i18n.connecting, 'work');
  await api('/api/health', {}, false);
  const cfg = await api('/api/config', {}, true);
  state.defaults = cfg.defaults || state.defaults;
  state.instanceId = state.defaults.instanceId || 'main';
  showConnect(false);
  setStatus(i18n.alreadyConnected, 'ok');
  await loadSessions();
}
async function loadSessions(){
  setStatus(i18n.loadingSessions, 'work');
  const data = await api('/api/instances/' + encodeURIComponent(state.instanceId) + '/sessions', {}, true);
  state.panes = data.panes || [];
  renderPanes();
  const current = state.panes.find(p => p.paneId === state.paneId);
  if (current) {
    selectPane(current.paneId);
  } else if (state.panes.length) {
    selectPane(state.panes[0].paneId);
  } else {
    stopPolling();
    state.paneId = '';
    $('terminalOutput').textContent = i18n.noPanes;
    setStatus(i18n.noPanes, '');
  }
}
async function refreshPaneList(){
  const data = await api('/api/instances/' + encodeURIComponent(state.instanceId) + '/sessions', {}, true);
  state.panes = data.panes || [];
  renderPanes();
  fitTerminalFont();
}
function renderPanes(){
  const box = $('paneList');
  box.innerHTML = '';
  state.panes.forEach(pane => {
    const row = document.createElement('button');
    row.type = 'button';
    row.className = 'pane-item' + (pane.isActive ? ' active' : '') + (pane.paneId === state.paneId ? ' selected' : '');
    const main = document.createElement('span');
    main.className = 'pane-main';
    const title = document.createElement('span');
    title.className = 'pane-title';
    title.textContent = (pane.isActive ? '* ' : '') + pane.paneId + ' ' + safeTitle(pane);
    const last = document.createElement('span');
    last.className = 'pane-last';
    last.textContent = pane.lastInput ? i18n.lastInputPrefix + pane.lastInput : i18n.noLastInput;
    const meta = document.createElement('span');
    meta.className = 'pane-meta';
    meta.textContent = displayCwd(pane.cwd);
    main.appendChild(title);
    main.appendChild(last);
    main.appendChild(meta);
    const menu = document.createElement('span');
    menu.className = 'pane-menu';
    menu.textContent = '...';
    row.appendChild(main);
    row.appendChild(menu);
    row.onclick = event => {
      if (event.target === menu) showPaneDetails(pane); else selectPane(pane.paneId);
    };
    row.ondblclick = () => showPaneDetails(pane);
    box.appendChild(row);
  });
}
function selectPane(paneId){
  state.paneId = paneId;
  state.snapshotHash = '';
  state.pollToken++;
  renderPanes();
  fitTerminalFont();
  $('terminalOutput').textContent = format(i18n.loadingPane, {id: paneId});
  pollSnapshot(state.pollToken);
}
async function pollSnapshot(token){
  if (token !== state.pollToken || !state.paneId) return;
  let path = '/api/instances/' + encodeURIComponent(state.instanceId) + '/panes/' + encodeURIComponent(state.paneId) + '/snapshot?lines=180&escapes=1';
  if (state.snapshotHash) path += '&since=' + encodeURIComponent(state.snapshotHash);
  try {
    const data = await api(path, {}, true);
    if (token !== state.pollToken) return;
    state.snapshotHash = data.hash || state.snapshotHash;
    if (data.changed) renderTerminal(data.text || '');
    setStatus(i18n.alreadyConnected, 'ok');
    state.pollTimer = setTimeout(() => pollSnapshot(token), 1000);
  } catch (err) {
    setStatus(i18n.snapshotFailed + ': ' + err.message, 'err');
    state.pollTimer = setTimeout(() => pollSnapshot(token), 3000);
  }
}
function stopPolling(){
  state.pollToken++;
  if (state.pollTimer) clearTimeout(state.pollTimer);
  state.pollTimer = 0;
}
async function sendCommand(enter){
  const input = $('commandInput');
  const text = input.value;
  if (!state.paneId) { setStatus(i18n.selectPaneFirst, 'err'); return; }
  if (!text && !enter) return;
  await sendRaw(text, enter);
  if (enter) input.value = '';
}
async function sendRaw(text, enter){
  if (!state.paneId) { setStatus(i18n.selectPaneFirst, 'err'); return; }
  setStatus(i18n.sending, 'work');
  const body = {textBase64: utf8Base64(text || ''), noPaste: true, enter: !!enter};
  await api('/api/instances/' + encodeURIComponent(state.instanceId) + '/panes/' + encodeURIComponent(state.paneId) + '/send', {method:'POST', body}, true);
  markPaneInput(text);
  state.snapshotHash = '';
  pollSnapshot(state.pollToken);
  setStatus(i18n.sent, 'ok');
  refreshPaneList().catch(() => {});
}
function markPaneInput(text){
  const summary = summarizeInput(text, 20);
  if (!summary) return;
  const pane = state.panes.find(item => item.paneId === state.paneId);
  if (!pane) return;
  pane.lastInput = summary;
  pane.lastInputAt = new Date().toISOString();
  renderPanes();
}
function summarizeInput(text, limit){
  const clean = String(text || '').replace(/[\x00-\x1f\x7f]/g, ' ').trim().replace(/\s+/g, ' ');
  if (!clean) return '';
  const chars = Array.from(clean);
  return chars.length > limit ? chars.slice(0, limit).join('') + '...' : clean;
}
async function spawnSession(cwd){
  setStatus(i18n.startingCodex, 'work');
  const body = {cwd: cwd || state.defaults.cwd, command: state.defaults.command || []};
  const data = await api('/api/instances/' + encodeURIComponent(state.instanceId) + '/spawn', {method:'POST', body}, true);
  await loadSessions();
  if (data.paneId) selectPane(data.paneId);
}
async function deletePane(pane){
  if (!confirm(format(i18n.deletePaneConfirm, {id: pane.paneId}))) return;
  setStatus(format(i18n.deletingPane, {id: pane.paneId}), 'work');
  await api('/api/instances/' + encodeURIComponent(state.instanceId) + '/panes/' + encodeURIComponent(pane.paneId), {method:'DELETE'}, true);
  if (pane.paneId === state.paneId) {
    stopPolling();
    state.paneId = '';
    $('terminalOutput').textContent = 'Pane ' + pane.paneId + ' deleted.';
  }
  await loadSessions();
}
function showPaneDetails(pane){
  state.selectedPane = pane;
  $('dialogTitle').textContent = i18n.sessionPrefix + pane.paneId;
  const details = $('dialogDetails');
  details.innerHTML = '';
  addDetail(details, i18n.dialogTitle, safeTitle(pane));
  addDetail(details, i18n.dialogPane, pane.paneId);
  addDetail(details, i18n.dialogWindowTab, (pane.windowId || 0) + ' / ' + (pane.tabId || 0));
  addDetail(details, i18n.dialogWorkspace, pane.workspace || '-');
  addDetail(details, i18n.dialogWorkingDir, displayCwd(pane.cwd));
  addDetail(details, i18n.dialogActive, pane.isActive ? i18n.yes : i18n.no);
  $('paneDialog').showModal();
}
function addDetail(box, name, value){
  const dt = document.createElement('dt');
  dt.textContent = name;
  const dd = document.createElement('dd');
  dd.textContent = value || '-';
  box.appendChild(dt);
  box.appendChild(dd);
}
function safeTitle(pane){ return pane.title || pane.cwd || ''; }
function activePane(){
  return state.panes.find(item => item.paneId === state.paneId) || null;
}
function fitTerminalFont(){
  const output = $('terminalOutput');
  const pane = activePane();
  const cols = pane && pane.size && Number(pane.size.cols) > 0 ? Number(pane.size.cols) : 120;
  const width = output ? Math.max(0, output.clientWidth - 28) : 0;
  if (!width || !cols) return;
  const size = Math.max(13, Math.min(16, width / cols / 0.62));
  output.style.setProperty('--terminal-font-size', size.toFixed(1) + 'px');
}
function displayCwd(cwd){
  const value = spawnCwdFromValue(cwd);
  return value || '-';
}
function spawnCwdFromValue(cwd){
  if (!cwd) return '';
  if (!cwd.startsWith('file:')) return cwd;
  try {
    let path = decodeURIComponent(new URL(cwd).pathname || '');
    if (path.length >= 3 && path[0] === '/' && path[2] === ':') path = path.slice(1);
    return path.replace(/\//g, '\\');
  } catch (err) {
    return cwd;
  }
}
function utf8Base64(text){
  const bytes = new TextEncoder().encode(text);
  let binary = '';
  bytes.forEach(byte => binary += String.fromCharCode(byte));
  return btoa(binary);
}
function renderTerminal(text){
  const output = $('terminalOutput');
  output.innerHTML = ansiToHTML(text);
  output.scrollTop = output.scrollHeight;
}
function ansiToHTML(text){
  let html = '', fg = '', bg = '', start = 0, index = 0;
  while (index < text.length) {
    if (text.charCodeAt(index) === 27) {
      const end = ansiSequenceEnd(text, index);
      if (end > index) {
        html += styledRun(text.slice(start, index), fg, bg);
        if (isSgr(text, index, end)) {
          const next = applySgr(text.slice(index + 2, end - 1), fg, bg);
          fg = next.fg; bg = next.bg;
        }
        index = end; start = index; continue;
      }
    }
    index++;
  }
  html += styledRun(text.slice(start), fg, bg);
  return html;
}
function styledRun(text, fg, bg){
  if (!text) return '';
  let style = '';
  if (fg) style += 'color:' + fg + ';';
  if (bg) style += 'background-color:' + bg + ';';
  const safe = escapeHtml(text);
  return style ? '<span style="' + style + '">' + safe + '</span>' : safe;
}
function ansiSequenceEnd(text, esc){
  if (esc + 1 >= text.length) return -1;
  const next = text[esc + 1];
  if (next === '[') {
    for (let i = esc + 2; i < text.length; i++) {
      const code = text.charCodeAt(i);
      if (code >= 64 && code <= 126) return i + 1;
    }
    return text.length;
  }
  if (next === ']' || next === 'P' || next === '_' || next === '^' || next === 'X') return stringControlEnd(text, esc + 2);
  if ('()*+-./#'.includes(next)) return Math.min(text.length, esc + 3);
  return Math.min(text.length, esc + 2);
}
function stringControlEnd(text, start){
  for (let i = start; i < text.length; i++) {
    if (text.charCodeAt(i) === 7) return i + 1;
    if (text.charCodeAt(i) === 27 && i + 1 < text.length && text[i + 1] === '\\') return i + 2;
  }
  return text.length;
}
function isSgr(text, esc, end){ return esc + 2 < end && text[esc + 1] === '[' && text[end - 1] === 'm'; }
function applySgr(params, fg, bg){
  const codes = params.replace(/:/g, ';').split(';').map(x => x.trim()).filter(Boolean).map(x => parseInt(x, 10) || 0);
  if (!codes.length) return {fg:'', bg:''};
  for (let i = 0; i < codes.length; i++) {
    const code = codes[i];
    if (code === 0) { fg = ''; bg = ''; }
    else if (code === 39) fg = '';
    else if (code === 49) bg = '';
    else if (code >= 30 && code <= 37) fg = ansiColor(code - 30);
    else if (code >= 40 && code <= 47) bg = ansiColor(code - 40);
    else if (code >= 90 && code <= 97) fg = ansiColor(code - 90 + 8);
    else if (code >= 100 && code <= 107) bg = ansiColor(code - 100 + 8);
    else if ((code === 38 || code === 48) && i + 2 < codes.length) {
      const isFg = code === 38;
      const mode = codes[++i];
      let color = '';
      if (mode === 5 && i + 1 < codes.length) color = xtermColor(codes[++i]);
      else if (mode === 2 && i + 3 < codes.length) color = rgb(codes[++i], codes[++i], codes[++i]);
      if (isFg) fg = color; else bg = color;
    }
  }
  return {fg, bg};
}
function ansiColor(index){ return index >= 0 && index < ansiColors.length ? '#' + ansiColors[index].toString(16).padStart(6, '0') : ''; }
function xtermColor(index){
  if (index >= 0 && index < 16) return ansiColor(index);
  if (index >= 16 && index <= 231) {
    const v = index - 16, r = Math.floor(v / 36), g = Math.floor(v / 6) % 6, b = v % 6;
    return rgb(channel(r), channel(g), channel(b));
  }
  if (index >= 232 && index <= 255) {
    const level = 8 + (index - 232) * 10;
    return rgb(level, level, level);
  }
  return '';
}
function channel(v){ return v === 0 ? 0 : 55 + v * 40; }
function rgb(r,g,b){ return 'rgb(' + clamp(r) + ',' + clamp(g) + ',' + clamp(b) + ')'; }
function clamp(v){ return Math.max(0, Math.min(255, v | 0)); }
function escapeHtml(value){ return String(value).replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch])); }
function format(text, values){ return String(text).replace(/\{(\w+)\}/g, (_, key)=>values[key] ?? ''); }

initFromHash();
$('baseUrl').value = state.baseUrl;
$('browserToken').value = state.token;
showConnect(!state.token);
$('connectForm').addEventListener('submit', event => { event.preventDefault(); connect().catch(err => setStatus(err.message, 'err')); });
$('connectionStatus').onclick = () => connect().catch(err => setStatus(err.message, 'err'));
$('editConnection').onclick = () => openConnectionDialog();
$('refreshSessions').onclick = () => loadSessions().catch(err => setStatus(err.message, 'err'));
$('newSession').onclick = () => spawnSession().catch(err => setStatus(err.message, 'err'));
$('sendForm').addEventListener('submit', event => { event.preventDefault(); sendCommand(true).catch(err => setStatus(err.message, 'err')); });
$('toggleKeys').onclick = () => setKeyPanel($('keyPanel').hidden);
function setKeyPanel(show){
  $('keyPanel').hidden = !show;
  $('toggleKeys').textContent = show ? i18n.hide : i18n.keys;
}
const specialKeyMap = {enter:['',true], ctrlc:['\u0003',false], tab:['\t',false], shifttab:['\u001B[Z',false], shiftpgup:['\u001B[5;2~',false], shiftpgdn:['\u001B[6;2~',false], space:[' ',false], up:['\u001B[A',false], down:['\u001B[B',false], left:['\u001B[D',false], right:['\u001B[C',false], pgup:['\u001B[5~',false], pgdn:['\u001B[6~',false], home:['\u001B[H',false], end:['\u001B[F',false], insert:['\u001B[2~',false], delete:['\u001B[3~',false], backspace:['\u007f',false], esc:['\u001B',false]};
function sendSpecialKey(name){
  const value = specialKeyMap[name];
  if (!value) return false;
  sendRaw(value[0], value[1]).catch(err => setStatus(err.message, 'err'));
  return true;
}
document.querySelectorAll('[data-key]').forEach(button => button.onclick = () => {
  sendSpecialKey(button.dataset.key);
});
function terminalShortcutFromEvent(event){
  if (event.defaultPrevented || event.isComposing) return '';
  if (!captureTerminalKeys(event.target)) return '';
  if (event.ctrlKey && !event.altKey && !event.metaKey && !event.shiftKey && String(event.key).toLowerCase() === 'c') return 'ctrlc';
  if (event.ctrlKey || event.altKey || event.metaKey) return '';
  if (event.key === 'Enter') return 'enter';
  if (event.key === 'Escape') return 'esc';
  if (event.key === 'Tab') return event.shiftKey ? 'shifttab' : 'tab';
  if (event.key === ' ') return 'space';
  if (event.key === 'ArrowUp') return 'up';
  if (event.key === 'ArrowDown') return 'down';
  if (event.key === 'ArrowLeft') return 'left';
  if (event.key === 'ArrowRight') return 'right';
  if (event.key === 'PageUp') return event.shiftKey ? 'shiftpgup' : 'pgup';
  if (event.key === 'PageDown') return event.shiftKey ? 'shiftpgdn' : 'pgdn';
  if (event.key === 'Home') return 'home';
  if (event.key === 'End') return 'end';
  if (event.key === 'Insert') return 'insert';
  if (event.key === 'Delete') return 'delete';
  if (event.key === 'Backspace') return 'backspace';
  return '';
}
function captureTerminalKeys(target){
  if ($('terminalApp').hidden || $('connectPanel').hidden === false) return false;
  if (document.querySelector('dialog[open]')) return false;
  if (!target) return true;
  if (target.isContentEditable) return false;
  const tag = String(target.tagName || '').toLowerCase();
  return !['input','textarea','select','button'].includes(tag);
}
document.addEventListener('keydown', event => {
  const key = terminalShortcutFromEvent(event);
  if (!key) return;
  event.preventDefault();
  sendSpecialKey(key);
});
$('dialogClose').onclick = () => $('paneDialog').close();
$('dialogDelete').onclick = () => { const pane = state.selectedPane; $('paneDialog').close(); if (pane) deletePane(pane).catch(err => setStatus(err.message, 'err')); };
$('dialogClone').onclick = () => { const pane = state.selectedPane; $('paneDialog').close(); if (pane) spawnSession(displayCwd(pane.cwd) === '-' ? state.defaults.cwd : displayCwd(pane.cwd)).catch(err => setStatus(err.message, 'err')); };
$('connectionCancel').onclick = () => closeConnectionDialog();
$('connectionSave').onclick = () => saveConnectionDialog().catch(err => setStatus(err.message, 'err'));
window.addEventListener('resize', fitTerminalFont);
setKeyPanel(false);
if (state.token) connect().catch(err => { showConnect(true); setStatus(err.message, 'err'); });
`
}

func settingsJS(lang uiLang) string {
	return jsI18N(lang) + `
let currentConfig = null;
let currentVersion = 'dev';
let updateInfo = null;
let updatePollTimer = 0;
const $ = id => document.getElementById(id);
function setState(text, kind='muted'){ const el=$('saveState'); el.className='status-card '+kind; el.textContent=text; }
function setUpdateState(text, kind='muted'){ const el=$('updateState'); el.className='update-state '+kind; el.textContent=text; }
function setUpdateProgress(percent, text){
  $('updateProgressBar').style.width=Math.max(0, Math.min(100, percent||0))+'%';
  $('updateProgressText').textContent=text||i18n.updateNoRunning;
}
function lines(value){ return value.split(/\r?\n/).map(x=>x.trim()).filter(Boolean); }
function fill(){
  const c=currentConfig;
  $('listen').value=c.listen||''; $('token').value=c.token||''; $('publicBaseUrl').value=c.publicBaseUrl||''; $('root').value=c.root||'';
  $('version').value=currentVersion||'dev';
  $('uiLanguage').value=c.uiLanguage||currentUILanguage||'en';
  $('timeout').value=c.commandTimeoutSeconds||5; $('regenToken').checked=!!c.regenerateTokenOnStart; $('lanPromptShown').checked=!!c.lanListenPromptShown; $('closeGui').checked=!!c.closeLaunchedGuiOnExit;
  $('defaultCwd').value=(c.mobileDefaults&&c.mobileDefaults.cwd)||'';
  $('defaultCommand').value=((c.mobileDefaults&&c.mobileDefaults.command)||[]).join('\n');
  renderInstances(c.instances||[]); renderDefaults(); renderAutoLaunch();
}
function renderInstances(items){
  const box=$('instances'); box.innerHTML='';
  items.forEach((it, index)=>{
    const row=document.createElement('div'); row.className='instance-row';
    row.innerHTML=` + "`" + `<label><span>${escapeHtml(i18n.idLabel)}</span><input data-field="id" value="${escapeAttr(it.id||'')}"></label>
      <label><span>${escapeHtml(i18n.nameLabel)}</span><input data-field="name" value="${escapeAttr(it.name||'')}"></label>
      <label><span>${escapeHtml(i18n.weztermClass)}</span><input data-field="class" value="${escapeAttr(it.class||'')}"></label>
      <button type="button" class="remove">${escapeHtml(i18n.remove)}</button>` + "`" + `;
    row.querySelector('.remove').onclick=()=>{ currentConfig.instances.splice(index,1); fill(); };
    row.querySelectorAll('input').forEach(input=>input.oninput=()=>{ currentConfig.instances[index][input.dataset.field]=input.value; renderDefaults(); renderAutoLaunch(); });
    box.appendChild(row);
  });
}
function renderDefaults(){
  const select=$('defaultInstance'); const selected=(currentConfig.mobileDefaults&&currentConfig.mobileDefaults.instanceId)||'';
  select.innerHTML='';
  (currentConfig.instances||[]).forEach(it=>{
    const option=document.createElement('option'); option.value=it.id||''; option.textContent=(it.name||it.id||i18n.instanceFallback)+' ('+(it.id||'')+')';
    option.selected=option.value===selected; select.appendChild(option);
  });
}
function renderAutoLaunch(){
  const box=$('autoLaunch'); const selected=new Set(currentConfig.autoLaunch||[]); box.innerHTML='';
  (currentConfig.instances||[]).forEach(it=>{
    const label=document.createElement('label'); label.className='choice';
    label.innerHTML=` + "`" + `<input type="checkbox" value="${escapeAttr(it.id||'')}" ${selected.has(it.id)?'checked':''}><span>${escapeHtml(it.name||it.id||i18n.instanceFallback)} <code>${escapeHtml(it.id||'')}</code></span>` + "`" + `;
    box.appendChild(label);
  });
}
function collect(){
  const instances=[...document.querySelectorAll('.instance-row')].map(row=>({
    id: row.querySelector('[data-field=id]').value.trim(),
    name: row.querySelector('[data-field=name]').value.trim(),
    class: row.querySelector('[data-field=class]').value.trim()
  }));
  return {
    listen:$('listen').value.trim(), token:$('token').value.trim(), publicBaseUrl:$('publicBaseUrl').value.trim(), uiLanguage:$('uiLanguage').value, root:$('root').value.trim(),
    commandTimeoutSeconds:parseInt($('timeout').value,10)||5,
    regenerateTokenOnStart:$('regenToken').checked,
    lanListenPromptShown:$('lanPromptShown').checked,
    closeLaunchedGuiOnExit:$('closeGui').checked,
    instances,
    autoLaunch:[...document.querySelectorAll('#autoLaunch input:checked')].map(x=>x.value),
    mobileDefaults:{ instanceId:$('defaultInstance').value, cwd:$('defaultCwd').value.trim(), command:lines($('defaultCommand').value) }
  };
}
async function load(){
  setState(i18n.loading);
  const res=await fetch('/api/settings'); const payload=await res.json(); if(!payload.ok) throw new Error(payload.error);
  currentConfig=payload.data.config; currentVersion=payload.data.version||'dev'; fill(); setState(i18n.configPrefix+payload.data.configPath);
  maybePromptLANListen();
}
async function save(event){
  event.preventDefault(); setState(i18n.saving);
  await saveSettingsPayload(collect());
}
async function saveSettingsPayload(payloadConfig, options){
  options=options||{};
  const res=await fetch('/api/settings',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payloadConfig)});
  const payload=await res.json(); if(!payload.ok){ setState(payload.error,'error'); return; }
  currentConfig=payload.data.config; currentVersion=payload.data.version||currentVersion; fill();
  const restart=payload.data.restartRequired ? i18n.restartFieldsPrefix+payload.data.restartFields.join(', ') : '';
  setState(i18n.configSavedPrefix+payload.data.configPath+'.'+restart);
  if(options.restartIfNeeded && payload.data.restartRequired && (payload.data.restartFields||[]).includes('listen')){
    await restartAgentAfterListenChange();
  }
}
function maybePromptLANListen(){
  if(!currentConfig || currentConfig.lanListenPromptShown || isLANListen(currentConfig.listen)) return;
  const nextListen='0.0.0.0:'+listenPort(currentConfig.listen);
  const ok=confirm(format(i18n.lanConfirm, {listen:(currentConfig.listen||'127.0.0.1:8765'), next:nextListen}));
  currentConfig.lanListenPromptShown=true;
  if(ok) currentConfig.listen=nextListen;
  fill();
  saveSettingsPayload(collect(), {restartIfNeeded: ok}).catch(err=>setState(err.message,'error'));
}
function isLANListen(listen){
  const value=String(listen||'').trim().toLowerCase();
  return value.startsWith('0.0.0.0:') || value.startsWith(':') || value.startsWith('[::]:') || value === '0.0.0.0';
}
function listenPort(listen){
  const match=String(listen||'').match(/:(\d+)$/);
  return match ? match[1] : '8765';
}
async function restartAgentAfterListenChange(){
  setState(i18n.restartListenSaving,'muted');
  const res=await fetch('/api/restart',{method:'POST'});
  const payload=await res.json(); if(!payload.ok) throw new Error(payload.error);
  setState(i18n.restartListenDone,'muted');
}
function renderUpdate(info){
  updateInfo=info;
  $('applyUpdate').disabled=!info.canUpdate;
  if(info.releaseUrl){
    $('releaseLink').href=info.releaseUrl; $('releaseLink').hidden=false;
  } else {
    $('releaseLink').hidden=true;
  }
  const published=info.publishedAt ? '\n'+i18n.published+': '+info.publishedAt : '';
  const pkg=info.packageName ? '\n'+i18n.packageLabel+': '+info.packageName+(info.packageKind==='patch'?' ('+i18n.smallUpdate+')':'') : '';
  const text=i18n.current+': '+(info.currentVersion||'dev')+'\n'+i18n.latest+': '+(info.latestVersion||'unknown')+'\n'+(info.message||'')+pkg+published;
  setUpdateState(text, info.canUpdate?'ok':(info.upToDate?'work':'muted'));
  setUpdateProgress(0, info.packageKind==='patch'?i18n.readyPatch:i18n.readyUpdate);
}
async function checkUpdate(){
  $('checkUpdate').disabled=true; $('applyUpdate').disabled=true; setUpdateState(i18n.checkingRelease,'work');
  try{
    const res=await fetch('/api/update/check'); const payload=await res.json(); if(!payload.ok) throw new Error(payload.error);
    renderUpdate(payload.data);
  }catch(err){
    updateInfo=null; setUpdateState(err.message,'err');
  }finally{
    $('checkUpdate').disabled=false;
  }
}
async function applyUpdate(){
  if(!updateInfo||!updateInfo.canUpdate) return;
  $('checkUpdate').disabled=true; $('applyUpdate').disabled=true; setUpdateState(i18n.startingUpdate,'work'); setUpdateProgress(0,i18n.startingUpdate);
  try{
    const body={useGitHubProxy:$('useGitHubProxy').checked};
    const res=await fetch('/api/update/apply',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)}); const payload=await res.json(); if(!payload.ok) throw new Error(payload.error);
    pollUpdateStatus();
  }catch(err){
    setUpdateState(err.message,'err'); $('checkUpdate').disabled=false; $('applyUpdate').disabled=false;
  }
}
async function pollUpdateStatus(){
  if(updatePollTimer) clearTimeout(updatePollTimer);
  try{
    const res=await fetch('/api/update/status'); const payload=await res.json(); if(!payload.ok) throw new Error(payload.error);
    const job=payload.data||{};
    const detail=job.totalBytes>0 ? ' '+formatBytes(job.bytes)+' / '+formatBytes(job.totalBytes) : '';
    setUpdateProgress(job.percent||0, (job.message||job.phase||i18n.updating)+detail);
    setUpdateState((job.message||i18n.updating)+(job.error?'\n'+job.error:''), job.error?'err':(job.done&&job.ok?'ok':'work'));
    if(job.active){
      updatePollTimer=setTimeout(pollUpdateStatus, 350);
    }else{
      $('checkUpdate').disabled=false;
      $('applyUpdate').disabled=!(updateInfo&&updateInfo.canUpdate);
      if(job.done&&job.ok) setUpdateState(i18n.updatePrepared,'ok');
    }
  }catch(err){
    setUpdateState(err.message,'err');
    updatePollTimer=setTimeout(pollUpdateStatus, 1200);
  }
}
function formatBytes(value){
  value=Number(value||0);
  if(value>=1048576) return (value/1048576).toFixed(1)+' MB';
  if(value>=1024) return (value/1024).toFixed(1)+' KB';
  return value+' B';
}
function escapeHtml(v){ return String(v).replace(/[&<>"']/g, ch=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch])); }
function escapeAttr(v){ return escapeHtml(v); }
function format(text, values){ return String(text).replace(/\{(\w+)\}/g, (_, key)=>values[key] ?? ''); }
$('settingsForm').addEventListener('submit', save);
$('reload').onclick=()=>load().catch(err=>setState(err.message,'error'));
$('addInstance').onclick=()=>{ currentConfig.instances.push({id:i18n.work,name:i18n.work,class:'easycodex'}); fill(); };
$('checkUpdate').onclick=()=>checkUpdate();
$('applyUpdate').onclick=()=>applyUpdate();
$('uiLanguage').onchange=()=>{ location.href=location.pathname+'?lang='+encodeURIComponent($('uiLanguage').value); };
load().catch(err=>setState(err.message,'error'));`
}

func easycodexSVG() string {
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64">
<defs><linearGradient id="g" x1="9" y1="7" x2="55" y2="57" gradientUnits="userSpaceOnUse"><stop stop-color="#0F8B8D"/><stop offset=".58" stop-color="#2563EB"/><stop offset="1" stop-color="#F59E0B"/></linearGradient></defs>
<rect x="6" y="6" width="52" height="52" rx="13" fill="url(#g)"/>
<path d="M20 24l-7 8 7 8" fill="none" stroke="#fff" stroke-width="4.6" stroke-linecap="round" stroke-linejoin="round"/>
<path d="M44 24l7 8-7 8" fill="none" stroke="#fff" stroke-width="4.6" stroke-linecap="round" stroke-linejoin="round"/>
<path d="M28 43l8-22" fill="none" stroke="#fff" stroke-width="4.8" stroke-linecap="round"/>
</svg>`
}

func networkBadge(lang uiLang, baseURL string) string {
	if strings.Contains(baseURL, "127.0.0.1") || strings.Contains(baseURL, "localhost") {
		return html.EscapeString(lang.t("localPC"))
	}
	if strings.HasPrefix(baseURL, "https://") || strings.Contains(baseURL, ".ts.net") || strings.Contains(baseURL, "100.") {
		return html.EscapeString(lang.t("public"))
	}
	return html.EscapeString(lang.t("wifiLAN"))
}

func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func timeNow() string {
	return time.Now().Format("2006-01-02 15:04:05")
}
