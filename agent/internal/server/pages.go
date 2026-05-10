package server

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"
	"time"

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
	writeHTML(w, settingsPageHTML())
}

func (s *Server) statusPage(w http.ResponseWriter, r *http.Request) {
	if !isLocalRequest(r) {
		writeError(w, http.StatusForbidden, fmt.Errorf("status page is only available from localhost"))
		return
	}
	cfg := s.configSnapshot()
	network := netinfo.Inspect(cfg.Listen)
	body := fmt.Sprintf(`
<section class="hero compact">
  <div>
    <p class="eyebrow">Agent Status</p>
    <h1>EasyCodex is running</h1>
    <p class="lead">Local control plane for PC WezTerm sessions and the Android companion.</p>
  </div>
  <div class="status-card">
    <span class="status-dot"></span>
    <strong>Online</strong>
    <small>%s</small>
  </div>
</section>
<section class="panel-grid two">
  <article class="panel">
    <h2>Network</h2>
    <dl class="kv">
      <dt>Listen</dt><dd>%s</dd>
      <dt>Local URL</dt><dd>%s</dd>
      <dt>LAN</dt><dd>%s</dd>
    </dl>
  </article>
  <article class="panel">
    <h2>Configuration</h2>
    <dl class="kv">
      <dt>Config file</dt><dd>%s</dd>
      <dt>Default cwd</dt><dd>%s</dd>
      <dt>Default instance</dt><dd>%s</dd>
    </dl>
  </article>
</section>`,
		html.EscapeString(timeNow()),
		html.EscapeString(network.Listen),
		html.EscapeString(network.LocalURL),
		html.EscapeString(strings.Join(network.LANURLs, ", ")),
		html.EscapeString(s.configPath),
		html.EscapeString(cfg.MobileDefaults.CWD),
		html.EscapeString(cfg.MobileDefaults.InstanceID),
	)
	writeHTML(w, pageShell("Status", "status", body, ""))
}

func (s *Server) writePairingConsole(w http.ResponseWriter, baseURLs []string) {
	var cards strings.Builder
	for _, baseURL := range baseURLs {
		pairURL := baseURL + "/api/mobile-pair?code=" + url.QueryEscape(s.mobilePairCode())
		deepLink := "easycodex://pair?u=" + url.QueryEscape(pairURL)
		qrURL := "/api/pairing/qr.svg?data=" + url.QueryEscape(deepLink)
		fmt.Fprintf(&cards, `
<article class="pair-card">
  <div class="qr-frame"><img src="%s" alt="Pairing QR"></div>
  <div class="pair-meta">
    <span class="badge">%s</span>
    <label>Phone Base URL</label>
    <code>%s</code>
    <label>Pair Link</label>
    <code>%s</code>
  </div>
</article>`, html.EscapeString(qrURL), networkBadge(baseURL), html.EscapeString(baseURL), html.EscapeString(deepLink))
	}

	body := fmt.Sprintf(`
<section class="hero">
  <div>
    <p class="eyebrow">Android Pairing</p>
    <h1>Scan once, then control Codex from your phone.</h1>
    <p class="lead">Use the QR code that matches the same Wi-Fi network as the phone. If none work, open Settings and set listen to <code>0.0.0.0:8765</code>, then allow Windows Firewall.</p>
  </div>
  <img class="hero-mark" src="/assets/easycodex.svg" alt="">
</section>
<section class="pair-grid">%s</section>`, cards.String())
	writeHTML(w, pageShell("Pairing", "pairing", body, ""))
}

func (s *Server) easycodexIcon(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	_, _ = w.Write([]byte(easycodexSVG()))
}

func writeHTML(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(body))
}

func pageShell(title, active, body, script string) string {
	nav := func(id, href, label string) string {
		class := ""
		if id == active {
			class = ` class="active"`
		}
		return fmt.Sprintf(`<a%s href="%s">%s</a>`, class, href, label)
	}
	return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>EasyCodex ` + html.EscapeString(title) + `</title>
<link rel="icon" href="/assets/easycodex.svg">
<style>` + consoleCSS() + `</style>
</head>
<body>
<header class="topbar">
  <a class="brand" href="/pairing"><img src="/assets/easycodex.svg" alt=""><span>EasyCodex</span></a>
  <nav>` + nav("pairing", "/pairing", "Pairing") + nav("settings", "/settings", "Settings") + nav("status", "/status", "Status") + `</nav>
</header>
<main>` + body + `</main>
` + script + `
</body>
</html>`
}

func settingsPageHTML() string {
	body := `
<section class="hero compact">
  <div>
    <p class="eyebrow">Agent Settings</p>
    <h1>Configure PC agent and mobile defaults.</h1>
    <p class="lead">Changes are saved to the Agent config file. Mobile defaults, token, and instance list apply immediately; listen address and startup token behavior apply after restarting the Agent.</p>
  </div>
  <div id="saveState" class="status-card muted">Loading...</div>
</section>
<form id="settingsForm" class="settings-layout">
  <section class="panel">
    <h2>Network</h2>
    <div class="field-grid">
      <label><span>Listen address</span><input id="listen" autocomplete="off" placeholder="0.0.0.0:8765"></label>
      <label><span>API token</span><input id="token" autocomplete="off"></label>
      <label><span>EasyCodex root</span><input id="root" autocomplete="off"></label>
      <label><span>Command timeout seconds</span><input id="timeout" type="number" min="1" max="120"></label>
    </div>
    <label class="check-row"><input id="regenToken" type="checkbox"><span>Regenerate API token every Agent startup</span></label>
    <label class="check-row"><input id="closeGui" type="checkbox"><span>Close GUI windows launched by Agent when Agent exits</span></label>
  </section>

  <section class="panel">
    <div class="panel-title-row">
      <h2>Instances</h2>
      <button type="button" class="secondary" id="addInstance">Add Instance</button>
    </div>
    <div id="instances" class="instance-list"></div>
  </section>

  <section class="panel">
    <h2>Mobile New Session Defaults</h2>
    <div class="field-grid">
      <label><span>Default instance</span><select id="defaultInstance"></select></label>
      <label><span>Working directory</span><input id="defaultCwd" autocomplete="off" placeholder="D:\mgame"></label>
    </div>
    <label><span>Command arguments, one per line</span><textarea id="defaultCommand" rows="5" spellcheck="false"></textarea></label>
  </section>

  <section class="panel">
    <h2>Auto Launch</h2>
    <div id="autoLaunch" class="choice-list"></div>
  </section>

  <div class="actionbar">
    <button type="button" class="secondary" id="reload">Reload</button>
    <button type="submit">Save Settings</button>
  </div>
</form>`
	script := `<script>` + settingsJS() + `</script>`
	return pageShell("Settings", "settings", body, script)
}

func consoleCSS() string {
	return `
:root{color-scheme:light;--bg:#eef1f5;--text:#18212f;--muted:#667085;--panel:#fff;--line:#d7dce5;--accent:#0f8b8d;--accent2:#f59e0b;--ink:#101828;--shadow:0 14px 36px rgba(16,24,40,.10)}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--text);font:14px/1.5 "Segoe UI",Arial,sans-serif}code,input,textarea,select{font-family:Consolas,"Cascadia Mono",monospace}
.topbar{height:64px;display:flex;align-items:center;justify-content:space-between;padding:0 28px;background:rgba(255,255,255,.9);border-bottom:1px solid var(--line);position:sticky;top:0;z-index:2;backdrop-filter:blur(12px)}
.brand{display:flex;align-items:center;gap:10px;color:var(--ink);text-decoration:none;font-weight:700;font-size:17px}.brand img{width:34px;height:34px}
nav{display:flex;gap:6px}nav a{color:#475467;text-decoration:none;padding:8px 12px;border-radius:7px}nav a.active,nav a:hover{background:#e7f4f4;color:#075f63}
main{max-width:1180px;margin:0 auto;padding:28px}.hero{display:flex;justify-content:space-between;align-items:center;gap:28px;margin-bottom:22px}.hero.compact{align-items:flex-end}.eyebrow{text-transform:uppercase;letter-spacing:.08em;color:var(--accent);font-weight:700;font-size:12px;margin:0 0 6px}.hero h1{margin:0;max-width:780px;font-size:34px;line-height:1.12;letter-spacing:0}.lead{max-width:820px;color:var(--muted);font-size:16px;margin:10px 0 0}.hero-mark{width:126px;height:126px;flex:0 0 auto}
.panel,.pair-card,.status-card{background:var(--panel);border:1px solid var(--line);border-radius:8px;box-shadow:var(--shadow)}.panel{padding:22px}.panel h2{margin:0 0 16px;font-size:17px}.panel-grid{display:grid;gap:16px}.panel-grid.two{grid-template-columns:repeat(2,minmax(0,1fr))}
.pair-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(340px,1fr));gap:16px}.pair-card{display:grid;grid-template-columns:190px minmax(0,1fr);gap:18px;padding:18px}.qr-frame{display:grid;place-items:center;border:1px solid var(--line);background:#fafafa;border-radius:8px;aspect-ratio:1}.qr-frame img{width:166px;height:166px}.pair-meta{min-width:0}.pair-meta label{display:block;color:var(--muted);font-size:12px;margin:12px 0 4px}.pair-meta code,.kv dd{display:block;word-break:break-all;background:#f5f7fa;border:1px solid #e4e7ec;border-radius:6px;padding:9px;color:#253244}
.badge{display:inline-flex;align-items:center;height:24px;padding:0 9px;border-radius:999px;background:#e7f4f4;color:#075f63;font-weight:700;font-size:12px}.settings-layout{display:grid;grid-template-columns:1fr 1fr;gap:16px}.settings-layout .panel:nth-child(2),.settings-layout .panel:nth-child(3){grid-column:span 1}.field-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:14px}label span{display:block;color:#344054;font-weight:600;margin-bottom:6px}input,textarea,select{width:100%;border:1px solid #cfd6df;border-radius:7px;padding:10px 11px;background:#fff;color:var(--text);font-size:14px}textarea{resize:vertical}.check-row{display:flex;gap:10px;align-items:center;margin-top:16px}.check-row input{width:auto}.check-row span{margin:0;font-weight:500}
.panel-title-row{display:flex;align-items:center;justify-content:space-between;gap:12px}.instance-row{display:grid;grid-template-columns:1fr 1fr 1fr auto;gap:10px;align-items:end;margin-top:10px}.choice-list{display:grid;gap:10px}.choice{display:flex;align-items:center;gap:10px;padding:10px;border:1px solid var(--line);border-radius:7px}.choice input{width:auto}.actionbar{grid-column:1/-1;display:flex;justify-content:flex-end;gap:10px;padding:14px 0 4px}
button{border:0;border-radius:7px;background:var(--accent);color:#fff;font-weight:700;padding:10px 16px;cursor:pointer}button:hover{filter:brightness(.96)}button.secondary{background:#fff;color:#344054;border:1px solid #cfd6df}.remove{background:#fff4ed;color:#b54708;border:1px solid #fed7aa}.status-card{padding:16px;min-width:220px}.status-card.muted{color:var(--muted)}.status-dot{display:inline-block;width:9px;height:9px;border-radius:50%;background:#12b76a;margin-right:8px}.status-card small{display:block;color:var(--muted);margin-top:4px}.kv{display:grid;grid-template-columns:145px minmax(0,1fr);gap:10px;margin:0}.kv dt{color:var(--muted)}.kv dd{margin:0}
@media(max-width:760px){.topbar{padding:0 14px}.brand span{display:none}main{padding:18px}.hero{display:block}.hero-mark{display:none}.settings-layout,.panel-grid.two{grid-template-columns:1fr}.field-grid{grid-template-columns:1fr}.pair-card{grid-template-columns:1fr}.instance-row{grid-template-columns:1fr}.actionbar{position:sticky;bottom:0;background:var(--bg);padding:12px 0}}`
}

func settingsJS() string {
	return `
let currentConfig = null;
const $ = id => document.getElementById(id);
function setState(text, kind='muted'){ const el=$('saveState'); el.className='status-card '+kind; el.textContent=text; }
function lines(value){ return value.split(/\r?\n/).map(x=>x.trim()).filter(Boolean); }
function fill(){
  const c=currentConfig;
  $('listen').value=c.listen||''; $('token').value=c.token||''; $('root').value=c.root||'';
  $('timeout').value=c.commandTimeoutSeconds||5; $('regenToken').checked=!!c.regenerateTokenOnStart; $('closeGui').checked=!!c.closeLaunchedGuiOnExit;
  $('defaultCwd').value=(c.mobileDefaults&&c.mobileDefaults.cwd)||'';
  $('defaultCommand').value=((c.mobileDefaults&&c.mobileDefaults.command)||[]).join('\n');
  renderInstances(c.instances||[]); renderDefaults(); renderAutoLaunch();
}
function renderInstances(items){
  const box=$('instances'); box.innerHTML='';
  items.forEach((it, index)=>{
    const row=document.createElement('div'); row.className='instance-row';
    row.innerHTML=` + "`" + `<label><span>ID</span><input data-field="id" value="${escapeAttr(it.id||'')}"></label>
      <label><span>Name</span><input data-field="name" value="${escapeAttr(it.name||'')}"></label>
      <label><span>WezTerm class</span><input data-field="class" value="${escapeAttr(it.class||'')}"></label>
      <button type="button" class="remove">Remove</button>` + "`" + `;
    row.querySelector('.remove').onclick=()=>{ currentConfig.instances.splice(index,1); fill(); };
    row.querySelectorAll('input').forEach(input=>input.oninput=()=>{ currentConfig.instances[index][input.dataset.field]=input.value; renderDefaults(); renderAutoLaunch(); });
    box.appendChild(row);
  });
}
function renderDefaults(){
  const select=$('defaultInstance'); const selected=(currentConfig.mobileDefaults&&currentConfig.mobileDefaults.instanceId)||'';
  select.innerHTML='';
  (currentConfig.instances||[]).forEach(it=>{
    const option=document.createElement('option'); option.value=it.id||''; option.textContent=(it.name||it.id||'instance')+' ('+(it.id||'')+')';
    option.selected=option.value===selected; select.appendChild(option);
  });
}
function renderAutoLaunch(){
  const box=$('autoLaunch'); const selected=new Set(currentConfig.autoLaunch||[]); box.innerHTML='';
  (currentConfig.instances||[]).forEach(it=>{
    const label=document.createElement('label'); label.className='choice';
    label.innerHTML=` + "`" + `<input type="checkbox" value="${escapeAttr(it.id||'')}" ${selected.has(it.id)?'checked':''}><span>${escapeHtml(it.name||it.id||'instance')} <code>${escapeHtml(it.id||'')}</code></span>` + "`" + `;
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
    listen:$('listen').value.trim(), token:$('token').value.trim(), root:$('root').value.trim(),
    commandTimeoutSeconds:parseInt($('timeout').value,10)||5,
    regenerateTokenOnStart:$('regenToken').checked,
    closeLaunchedGuiOnExit:$('closeGui').checked,
    instances,
    autoLaunch:[...document.querySelectorAll('#autoLaunch input:checked')].map(x=>x.value),
    mobileDefaults:{ instanceId:$('defaultInstance').value, cwd:$('defaultCwd').value.trim(), command:lines($('defaultCommand').value) }
  };
}
async function load(){
  setState('Loading...');
  const res=await fetch('/api/settings'); const payload=await res.json(); if(!payload.ok) throw new Error(payload.error);
  currentConfig=payload.data.config; fill(); setState('Config: '+payload.data.configPath);
}
async function save(event){
  event.preventDefault(); setState('Saving...');
  const res=await fetch('/api/settings',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(collect())});
  const payload=await res.json(); if(!payload.ok){ setState(payload.error,'error'); return; }
  currentConfig=payload.data.config; fill();
  const restart=payload.data.restartRequired ? ' Restart Agent for: '+payload.data.restartFields.join(', ') : '';
  setState('Saved to '+payload.data.configPath+'.'+restart);
}
function escapeHtml(v){ return String(v).replace(/[&<>"']/g, ch=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch])); }
function escapeAttr(v){ return escapeHtml(v); }
$('settingsForm').addEventListener('submit', save);
$('reload').onclick=()=>load().catch(err=>setState(err.message,'error'));
$('addInstance').onclick=()=>{ currentConfig.instances.push({id:'work',name:'work',class:'easycodex'}); fill(); };
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

func networkBadge(baseURL string) string {
	if strings.Contains(baseURL, "127.0.0.1") || strings.Contains(baseURL, "localhost") {
		return "Local PC"
	}
	return "Wi-Fi / LAN"
}

func timeNow() string {
	return time.Now().Format("2006-01-02 15:04:05")
}
