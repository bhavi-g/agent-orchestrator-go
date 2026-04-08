/* ================================================================
   GLOBALS
   ================================================================ */
let _pollId = null;

/* ================================================================
   UTILITIES
   ================================================================ */
function esc(s) {
    if (s == null) return '';
    const d = document.createElement('div');
    d.textContent = String(s);
    return d.innerHTML;
}
function shortId(id) { return id ? id.substring(0, 8) : '—'; }
function fmtTime(ts) {
    if (!ts) return '—';
    const d = new Date(ts);
    if (isNaN(d)) return ts;
    return d.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
}
function fmtDuration(start, end) {
    if (!start || !end) return '—';
    const ms = new Date(end) - new Date(start);
    if (ms < 1000) return ms + 'ms';
    return (ms / 1000).toFixed(1) + 's';
}
function tryParseJSON(s) {
    if (typeof s !== 'string') return s;
    try { return JSON.parse(s); } catch { return s; }
}
function prettyJSON(obj) {
    if (obj == null) return '';
    if (typeof obj === 'string') obj = tryParseJSON(obj);
    try { return JSON.stringify(obj, null, 2); } catch { return String(obj); }
}
function statusColor(s) {
    if (!s) return 'pending';
    const u = s.toUpperCase();
    if (u === 'COMPLETED' || u === 'SUCCEEDED') return 'ok';
    if (u === 'FAILED') return 'fail';
    if (u === 'RUNNING') return 'run';
    return 'pending';
}
function statusBadge(s) {
    if (!s) return '';
    const u = s.toUpperCase();
    const map = {
        COMPLETED: ['badge-success', 'green', 'Completed'],
        SUCCEEDED: ['badge-success', 'green', 'Succeeded'],
        FAILED:    ['badge-error',   'red',   'Failed'],
        RUNNING:   ['badge-info',    'blue',  'Running'],
        PENDING:   ['badge-neutral',  '',     'Pending'],
    };
    const entry = map[u] || ['badge-neutral', '', u];
    const cls = entry[0], dot = entry[1], label = entry[2];
    const dotHtml = dot ? `<span class="badge-dot ${dot}"></span>` : '';
    return `<span class="badge ${cls}">${dotHtml}${esc(label)}</span>`;
}
function jsonBlock(obj) {
    if (obj == null) return '<pre class="json-block">—</pre>';
    try { return `<pre class="json-block">${esc(prettyJSON(obj))}</pre>`; }
    catch { return `<pre class="json-block">${esc(String(obj))}</pre>`; }
}

/* ================================================================
   SIDEBAR
   ================================================================ */
function groupByTime(runs) {
    const now   = new Date();
    const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    const yest  = new Date(today - 86400000);
    const week  = new Date(today - 7 * 86400000);
    const g = { today: [], yesterday: [], week: [], older: [] };
    runs.forEach(r => {
        const d = r.created_at ? new Date(r.created_at) : new Date(0);
        if (d >= today)     g.today.push(r);
        else if (d >= yest) g.yesterday.push(r);
        else if (d >= week) g.week.push(r);
        else                g.older.push(r);
    });
    return g;
}

async function loadSidebar(activeId) {
    const el = document.getElementById('sidebar-history');
    if (!el) return;
    try {
        const runs = await API.listRuns();
        if (!runs || !runs.length) {
            el.innerHTML = '<div class="history-empty">No analyses yet</div>';
            return;
        }
        const groups = groupByTime(runs);
        const labels = { today: 'Today', yesterday: 'Yesterday', week: 'Previous 7 Days', older: 'Older' };
        let html = '';
        for (const [key, label] of Object.entries(labels)) {
            if (!groups[key].length) continue;
            html += '<div class="history-section">';
            html += `<span class="history-label">${label}</span>`;
            groups[key].forEach(r => {
                const dot    = statusColor(r.status);
                const active = r.run_id === activeId ? ' active' : '';
                const title  = esc(r.goal || shortId(r.run_id));
                html += `<div class="history-item${active}" data-id="${esc(r.run_id)}">`;
                html += `<span class="history-dot ${dot}"></span>`;
                html += `<span>${title}</span>`;
                html += '</div>';
            });
            html += '</div>';
        }
        el.innerHTML = html;
        el.querySelectorAll('.history-item').forEach(item => {
            item.addEventListener('click', () => {
                location.hash = '#/run?id=' + encodeURIComponent(item.dataset.id);
            });
        });
    } catch {
        el.innerHTML = '<div class="history-empty">Could not load history</div>';
    }
}

/* ================================================================
   ROUTER
   ================================================================ */
function getRoute() {
    const hash = location.hash.slice(1) || '/';
    const qIdx = hash.indexOf('?');
    const path   = qIdx >= 0 ? hash.slice(0, qIdx) : hash;
    const params = new URLSearchParams(qIdx >= 0 ? hash.slice(qIdx + 1) : '');
    return { path, params };
}

function route() {
    if (_pollId) { clearInterval(_pollId); _pollId = null; }
    const { path, params } = getRoute();
    const id  = params.get('id');
    const app = document.getElementById('app');
    loadSidebar(id || null);
    switch (path) {
        case '/':
        case '/dashboard':
        case '/new':
            renderWelcome(app);
            break;
        case '/run':
            if (id) renderConversation(app, id);
            else    renderWelcome(app);
            break;
        default:
            renderWelcome(app);
    }
}

/* ================================================================
   WELCOME SCREEN
   ================================================================ */
function renderWelcome(app) {
    app.innerHTML = `
    <div class="welcome fade-in">
        <div class="welcome-icon">
            <svg width="26" height="26" viewBox="0 0 24 24" fill="currentColor"><path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z"/></svg>
        </div>
        <h1 class="welcome-title">Agent Orchestrator</h1>
        <p class="welcome-sub">Point it at a folder of logs. It reads every file, traces the root cause, and tells you exactly what broke.</p>
        <div class="welcome-actions">
            <button class="welcome-demo-btn" id="welcome-demo">
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polygon points="5 3 19 12 5 21 5 3"/></svg>
                Try Demo
            </button>
            <span class="welcome-or">or paste a path / drag files below</span>
        </div>
        <div class="suggestions">
            <div class="suggestion">
                <strong>OOM / Panic crash</strong>
                <span>Out-of-memory, nil pointer, segfault</span>
            </div>
            <div class="suggestion">
                <strong>Service failures</strong>
                <span>HTTP 5xx, timeouts, connection drops</span>
            </div>
            <div class="suggestion">
                <strong>Database errors</strong>
                <span>Query failures, lock timeouts, deadlocks</span>
            </div>
            <div class="suggestion">
                <strong>Any log directory</strong>
                <span>Paste the absolute path below</span>
            </div>
        </div>
    </div>`;
    app.querySelectorAll('.suggestion').forEach(card => {
        card.addEventListener('click', () => {
            const pathEl = document.getElementById('inp-path');
            if (pathEl) pathEl.focus();
        });
    });
    const welcomeDemo = document.getElementById('welcome-demo');
    if (welcomeDemo) {
        welcomeDemo.addEventListener('click', () => {
            const demoBtn = document.getElementById('btn-demo');
            if (demoBtn) demoBtn.click();
        });
    }
}

/* ================================================================
   CONVERSATION VIEW
   ================================================================ */
async function renderConversation(app, id) {
    app.innerHTML = '<div class="center-loading"><div class="spinner"></div></div>';
    try {
        const [run, steps, tools] = await Promise.all([
            API.getRun(id),
            API.getRunSteps(id).catch(() => []),
            API.getRunToolCalls(id).catch(() => []),
        ]);
        const stepsArr = steps || [];
        const toolsArr = tools || [];
        const report   = extractReport(stepsArr);
        const isLive   = run.status === 'RUNNING' || run.status === 'PENDING';

        app.innerHTML = '<div class="conversation fade-in">' +
            renderUserMsg(run) +
            renderAiMsg(run, stepsArr, toolsArr, report, isLive) +
            '</div>';

        wireEvents(app, id, run);

        if (isLive) {
            _pollId = setInterval(async () => {
                const latest = await API.getRun(id).catch(() => null);
                if (!latest) return;
                if (!['RUNNING', 'PENDING'].includes(latest.status)) {
                    clearInterval(_pollId); _pollId = null;
                    renderConversation(app, id);
                    loadSidebar(id);
                } else {
                    const newSteps = await API.getRunSteps(id).catch(() => []);
                    const counter  = document.getElementById('step-counter-' + id);
                    if (counter) counter.textContent = stepCountText(newSteps.length);
                }
            }, 2000);
        }
    } catch (e) {
        app.innerHTML = '<div class="conversation fade-in"><div class="empty-msg">Failed to load: ' + esc(e.message) + '</div></div>';
    }
}

function stepCountText(n) { return n + ' step' + (n !== 1 ? 's' : '') + ' completed'; }

function renderUserMsg(run) {
    return '<div class="msg-user"><div class="msg-user-bubble">' +
        '<div class="msg-user-goal">' + esc(run.goal || 'Log Analysis') + '</div>' +
        '<div class="msg-user-time">' + fmtTime(run.created_at) + '</div>' +
        '</div></div>';
}

function renderAiMsg(run, steps, tools, report, isLive) {
    const iconSvg = '<svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z"/></svg>';
    const icon    = '<div class="msg-ai-icon">' + iconSvg + '</div>';
    let body = '';

    if (isLive) {
        body = '<div class="thinking-box">' +
            '<div class="thinking-title" id="thinking-hdr-' + run.run_id + '">' +
            '<span class="spinner-inline"></span>' +
            ' Analyzing logs… ' +
            '<span id="step-counter-' + run.run_id + '" style="color:var(--text-muted); font-weight:400;">' +
            stepCountText(steps.length) + '</span>' +
            '</div></div>';
    } else {
        const statusIcon = run.status === 'FAILED'
            ? '<span class="badge badge-error">Failed</span>'
            : '<span class="badge badge-success">Complete</span>';
        const confHtml = (report && report.confidence_level)
            ? '<span class="conf-badge conf-' + report.confidence_level.toLowerCase() + '">' + esc(report.confidence_level) + ' confidence</span>'
            : '';
        const metaTxt = steps.length + ' steps · ' + tools.length + ' tool calls · ' + fmtDuration(run.created_at, run.completed_at);

        body = '<div class="report-header">' + statusIcon + confHtml + '<span class="report-meta">' + metaTxt + '</span></div>';

        if (run.status === 'FAILED' && !report) {
            body += '<div class="report-section"><p style="color:var(--text-muted)">The analysis did not complete. Check that the path exists and is accessible.</p></div>';
        } else if (report) {
            body += renderReport(report);
        }
        body += renderDetailToggles(run.run_id, steps, tools);
    }

    return '<div class="msg-ai">' + icon + '<div class="msg-ai-body">' + body + '</div></div>';
}

/* ================================================================
   REPORT RENDERER
   ================================================================ */
function renderReport(r) {
    const evidence  = r.supporting_evidence || [];
    const nextSteps = r.suggested_next_steps || [];
    let html = '';

    if (r.error_summary) {
        html += '<div class="report-section"><h3>Error Summary</h3><p>' + esc(r.error_summary) + '</p></div>';
    }
    if (r.suspected_root_cause && r.suspected_root_cause !== 'N/A — no issues found.') {
        html += '<div class="report-section"><h3>Root Cause</h3><p>' + esc(r.suspected_root_cause) + '</p></div>';
    }
    if (evidence.length) {
        let evHtml = '<div class="evidence-list">';
        evidence.forEach(e => {
            evHtml += '<div class="evidence-item">';
            evHtml += '<div class="evidence-file">' + esc(e.file || '?');
            if (e.line_number) evHtml += '<span class="ln"> :' + e.line_number + '</span>';
            evHtml += '</div>';
            evHtml += '<div class="evidence-text">' + esc(e.text || '') + '</div>';
            evHtml += '</div>';
        });
        evHtml += '</div>';
        html += '<div class="report-section"><h3>Evidence (' + evidence.length + ')</h3>' + evHtml + '</div>';
    }
    if (nextSteps.length) {
        let stHtml = '<ol class="steps-list">';
        nextSteps.forEach(s => {
            stHtml += '<li>' + esc(typeof s === 'string' ? s : (s.description || JSON.stringify(s))) + '</li>';
        });
        stHtml += '</ol>';
        html += '<div class="report-section"><h3>Recommended Next Steps</h3>' + stHtml + '</div>';
    }
    return html;
}

/* ================================================================
   DETAIL TOGGLES (Steps / Tool Calls / Replay)
   ================================================================ */
function renderDetailToggles(runId, steps, tools) {
    const replaySvg = '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polygon points="5 3 19 12 5 21 5 3"/></svg>';
    let html = '<div class="detail-toggles">';
    if (steps.length) html += '<button class="detail-toggle-btn" data-panel="panel-steps-' + runId + '">Steps (' + steps.length + ')</button>';
    if (tools.length) html += '<button class="detail-toggle-btn" data-panel="panel-tools-' + runId + '">Tool Calls (' + tools.length + ')</button>';
    html += '<button class="replay-btn" id="btn-replay-' + runId + '">' + replaySvg + ' Replay</button>';
    html += '</div>';
    html += '<div class="detail-panel" id="panel-steps-' + runId + '">';
    html += steps.length ? renderTimeline(steps) : '<div class="empty-msg">No steps recorded</div>';
    html += '</div>';
    html += '<div class="detail-panel" id="panel-tools-' + runId + '">';
    html += tools.length ? renderToolCalls(tools) : '<div class="empty-msg">No tool calls recorded</div>';
    html += '</div>';
    return html;
}

function renderTimeline(steps) {
    let html = '<div class="timeline">';
    steps.forEach(s => {
        const dc     = statusColor(s.status);
        const parsed = tryParseJSON(s.output);
        html += '<div class="tl-item">';
        html += '<div class="tl-dot ' + dc + '"></div>';
        html += '<div class="tl-header"><span class="tl-name">' + esc(s.step_id || s.type || 'Step') + '</span>' + statusBadge(s.status) + '</div>';
        html += '<div class="tl-meta">';
        if (s.started_at) html += fmtTime(s.started_at);
        if (s.started_at && s.finished_at) html += ' · ' + fmtDuration(s.started_at, s.finished_at);
        html += '</div>';
        if (parsed && typeof parsed === 'object') {
            html += '<div class="tl-output"><details><summary>Show output</summary>' + jsonBlock(parsed) + '</details></div>';
        }
        html += '</div>';
    });
    html += '</div>';
    return html;
}

function renderToolCalls(tools) {
    const chevSvg = '<svg class="acc-chevron" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polyline points="9 6 15 12 9 18"/></svg>';
    let html = '<div class="accordion">';
    tools.forEach(t => {
        const dur = fmtDuration(t.started_at, t.finished_at);
        html += '<div class="acc-item">';
        html += '<button class="acc-header">' + chevSvg;
        html += '<span class="acc-name">' + esc(t.tool_name || '?') + '</span>';
        html += statusBadge(t.status);
        html += '<span class="acc-meta">' + dur + '</span></button>';
        html += '<div class="acc-body">';
        html += '<div class="json-label">Input</div>' + jsonBlock(t.input);
        html += '<div class="json-label">Output</div>' + jsonBlock(t.output);
        html += '</div></div>';
    });
    html += '</div>';
    return html;
}

/* ================================================================
   WIRE EVENTS
   ================================================================ */
function wireEvents(app, id) {
    app.querySelectorAll('.detail-toggle-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            const panel   = document.getElementById(btn.dataset.panel);
            if (!panel) return;
            const opening = !panel.classList.contains('visible');
            app.querySelectorAll('.detail-panel').forEach(p => p.classList.remove('visible'));
            app.querySelectorAll('.detail-toggle-btn').forEach(b => b.classList.remove('active'));
            if (opening) { panel.classList.add('visible'); btn.classList.add('active'); }
        });
    });

    app.querySelectorAll('.acc-header').forEach(hdr => {
        hdr.addEventListener('click', () => hdr.closest('.acc-item').classList.toggle('open'));
    });

    const replayBtn = document.getElementById('btn-replay-' + id);
    if (replayBtn) {
        replayBtn.addEventListener('click', async () => {
            replayBtn.disabled = true; replayBtn.textContent = 'Replaying…';
            try {
                await API.replayRun(id);
                alert('Replay started — it will appear in the sidebar shortly.');
                loadSidebar(id);
            } catch (e) {
                alert('Replay failed: ' + e.message);
            }
            const playSvg = '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polygon points="5 3 19 12 5 21 5 3"/></svg>';
            replayBtn.disabled = false;
            replayBtn.innerHTML = playSvg + ' Replay';
        });
    }
}

/* ================================================================
   HELPERS
   ================================================================ */
function extractReport(steps) {
    for (let i = steps.length - 1; i >= 0; i--) {
        const parsed = tryParseJSON(steps[i].output);
        if (parsed && typeof parsed === 'object' && parsed.error_summary) return parsed;
    }
    return null;
}

/* ================================================================
   INPUT HANDLING
   ================================================================ */
function setupInput() {
    const pathEl  = document.getElementById('inp-path');
    const taskEl  = document.getElementById('inp-task');
    const sendBtn = document.getElementById('btn-send');
    const fileIn  = document.getElementById('file-input');
    const uploadBtn = document.getElementById('btn-upload');
    const demoBtn = document.getElementById('btn-demo');
    const inputBox = document.getElementById('input-box');
    if (!pathEl || !sendBtn) return;

    pathEl.addEventListener('input', () => {
        sendBtn.disabled = !pathEl.value.trim();
    });
    pathEl.addEventListener('keydown', e => {
        if (e.key === 'Enter' && !e.shiftKey && !sendBtn.disabled) { e.preventDefault(); sendBtn.click(); }
    });

    sendBtn.addEventListener('click', async () => {
        const path = pathEl.value.trim();
        if (!path) return;
        const taskRaw = taskEl.value.trim();
        const task    = taskRaw || (path.split('/').filter(x => x).pop() || 'analysis');
        sendBtn.disabled = true; pathEl.disabled = true; taskEl.disabled = true;
        try {
            const result = await API.createRun(task, path);
            const runId  = result.run_id || result.id;
            pathEl.value = ''; taskEl.value = '';
            pathEl.disabled = false; taskEl.disabled = false; sendBtn.disabled = true;
            if (runId) location.hash = '#/run?id=' + encodeURIComponent(runId);
        } catch (err) {
            alert('Failed: ' + err.message);
            pathEl.disabled = false; taskEl.disabled = false;
            sendBtn.disabled = !pathEl.value.trim();
        }
    });

    // File upload
    if (fileIn) {
        fileIn.addEventListener('change', () => {
            if (fileIn.files.length) handleFileUpload(fileIn.files, pathEl, taskEl, sendBtn);
        });
    }

    // Drag and drop
    if (inputBox) {
        inputBox.addEventListener('dragover', e => { e.preventDefault(); inputBox.classList.add('drag-over'); });
        inputBox.addEventListener('dragleave', () => inputBox.classList.remove('drag-over'));
        inputBox.addEventListener('drop', e => {
            e.preventDefault();
            inputBox.classList.remove('drag-over');
            if (e.dataTransfer.files.length) handleFileUpload(e.dataTransfer.files, pathEl, taskEl, sendBtn);
        });
    }

    // Demo button
    if (demoBtn) {
        demoBtn.addEventListener('click', async () => {
            demoBtn.disabled = true; demoBtn.textContent = 'Loading…';
            try {
                const res = await API.getDemoDir();
                pathEl.value = res.directory;
                taskEl.value = 'demo-crash-analysis';
                sendBtn.disabled = false;
                sendBtn.click();
            } catch (err) {
                alert('Demo failed: ' + err.message);
            }
            demoBtn.disabled = false; demoBtn.textContent = 'Try Demo';
        });
    }
}

async function handleFileUpload(files, pathEl, taskEl, sendBtn) {
    pathEl.value = 'Uploading ' + files.length + ' file(s)…';
    pathEl.disabled = true; sendBtn.disabled = true;
    try {
        const res = await API.uploadFiles(files);
        pathEl.value = res.directory;
        taskEl.value = 'uploaded-logs';
        pathEl.disabled = false;
        sendBtn.disabled = false;
        sendBtn.click();
    } catch (err) {
        alert('Upload failed: ' + err.message);
        pathEl.value = ''; pathEl.disabled = false; sendBtn.disabled = true;
    }
}

/* ================================================================
   HEALTH CHECK
   ================================================================ */
async function checkHealth() {
    const ok  = await API.checkHealth().catch(() => false);
    const dot = document.getElementById('health-dot');
    const txt = document.getElementById('health-text');
    if (dot) dot.className = 'health-dot ' + (ok ? 'ok' : 'err');
    if (txt) txt.textContent = ok ? 'System healthy' : 'Disconnected';
}

/* ================================================================
   INIT
   ================================================================ */
window.addEventListener('hashchange', route);
window.addEventListener('DOMContentLoaded', () => {
    document.getElementById('btn-new').addEventListener('click', () => {
        location.hash = '#/';
        setTimeout(() => { const el = document.getElementById('inp-path'); if (el) el.focus(); }, 50);
    });
    checkHealth();
    setInterval(checkHealth, 30000);
    setupInput();
    route();
});
