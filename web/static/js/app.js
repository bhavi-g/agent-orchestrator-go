/* ================================================================
   SVG ICONS
   ================================================================ */
const ICO = {
    grid:    '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/></svg>',
    list:    '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="8" y1="6" x2="21" y2="6"/><line x1="8" y1="12" x2="21" y2="12"/><line x1="8" y1="18" x2="21" y2="18"/><circle cx="4" cy="6" r="1" fill="currentColor"/><circle cx="4" cy="12" r="1" fill="currentColor"/><circle cx="4" cy="18" r="1" fill="currentColor"/></svg>',
    plus:    '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>',
    arrow:   '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><polyline points="15 18 9 12 15 6"/></svg>',
    chevron: '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><polyline points="9 6 15 12 9 18"/></svg>',
    play:    '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><polygon points="5 3 19 12 5 21 5 3"/></svg>',
    clock:   '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>',
    check:   '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><polyline points="20 6 9 17 4 12"/></svg>',
    x:       '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>',
    bolt:    '<svg width="24" height="24" viewBox="0 0 24 24" fill="currentColor"><path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z"/></svg>',
};

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
    return d.toLocaleString(undefined, { month:'short', day:'numeric', hour:'2-digit', minute:'2-digit', second:'2-digit' });
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
function pct(v) {
    if (v == null) return '—';
    return (v * 100).toFixed(0) + '%';
}

/* ================================================================
   COMPONENTS
   ================================================================ */
function statusBadge(s) {
    if (!s) return '';
    const upper = s.toUpperCase();
    const map = {
        COMPLETED: { cls: 'badge-success', dot: 'green', label: 'Completed' },
        SUCCEEDED: { cls: 'badge-success', dot: 'green', label: 'Succeeded' },
        FAILED:    { cls: 'badge-error',   dot: 'red',   label: 'Failed' },
        RUNNING:   { cls: 'badge-info',    dot: 'blue pulse', label: 'Running' },
        PENDING:   { cls: 'badge-neutral',  dot: '',      label: 'Pending' },
    };
    const m = map[upper] || { cls: 'badge-neutral', dot: '', label: upper };
    const dot = m.dot ? `<span class="badge-dot ${m.dot}"></span>` : '';
    return `<span class="badge ${m.cls}">${dot}${esc(m.label)}</span>`;
}

function confidenceBadge(level) {
    if (!level) return '';
    const l = level.toLowerCase();
    const map = { high: 'badge-success', medium: 'badge-warning', low: 'badge-error' };
    return `<span class="badge ${map[l] || 'badge-neutral'}">${esc(level)}</span>`;
}

function metricCard(label, value, color, sub) {
    return `<div class="metric-card ${color}">
        <div class="metric-label">${esc(label)}</div>
        <div class="metric-value">${value}</div>
        ${sub ? `<div class="metric-sub">${esc(sub)}</div>` : ''}
    </div>`;
}

function loadingHTML() {
    return '<div class="loading"><div class="spinner"></div></div>';
}
function emptyHTML(msg) {
    return `<div class="empty-state"><div class="empty-icon">&#9744;</div><div>${esc(msg)}</div></div>`;
}
function jsonBlock(obj) {
    return `<pre class="json-block">${esc(prettyJSON(obj))}</pre>`;
}

/* ================================================================
   ROUTER
   ================================================================ */
function getRoute() {
    const hash = location.hash.slice(1) || '/';
    const qIdx = hash.indexOf('?');
    const path = qIdx >= 0 ? hash.slice(0, qIdx) : hash;
    const params = new URLSearchParams(qIdx >= 0 ? hash.slice(qIdx + 1) : '');
    return { path, params };
}

function route() {
    const { path, params } = getRoute();
    updateNav(path);
    const app = document.getElementById('app');

    switch (path) {
        case '/':
        case '/dashboard':
            renderDashboard(app);
            break;
        case '/runs':
            renderRuns(app);
            break;
        case '/run':
            renderRunDetail(app, params.get('id'));
            break;
        case '/new':
            renderNewRun(app);
            break;
        default:
            app.innerHTML = emptyHTML('Page not found');
    }
}

function updateNav(path) {
    document.querySelectorAll('.nav-item').forEach(el => {
        const href = el.getAttribute('href').slice(1); // strip #
        const active = (path === href) ||
            (href === '/' && (path === '/' || path === '/dashboard')) ||
            (href === '/runs' && path === '/run');
        el.classList.toggle('active', active);
    });
}

/* ================================================================
   HEALTH CHECK
   ================================================================ */
async function checkHealth() {
    const ok = await API.checkHealth();
    const dot = document.querySelector('.health-dot');
    const txt = document.querySelector('.health-text');
    if (dot) { dot.className = 'health-dot ' + (ok ? 'ok' : 'err'); }
    if (txt) { txt.textContent = ok ? 'System healthy' : 'Disconnected'; }
}

/* ================================================================
   DASHBOARD PAGE
   ================================================================ */
async function renderDashboard(app) {
    app.innerHTML = loadingHTML();
    try {
        const [metrics, runs] = await Promise.all([
            API.getMetrics().catch(() => null),
            API.listRuns().catch(() => []),
        ]);

        const m = metrics || {};
        const recent = (runs || []).slice(0, 8);
        const totalRuns = (runs || []).length;
        const successCount = (runs || []).filter(r => r.status === 'COMPLETED').length;
        const successRate = totalRuns > 0 ? (successCount / totalRuns) : 0;

        app.innerHTML = `<div class="fade-in">
            <div class="page-header">
                <div><h1>Dashboard</h1><div class="subtitle">Overview of your agent orchestrator</div></div>
                <a href="#/new" class="btn btn-primary">${ICO.plus} New Analysis</a>
            </div>

            <div class="metrics-grid">
                ${metricCard('Total Runs', totalRuns, 'purple', 'all time')}
                ${metricCard('Success Rate', pct(successRate), 'green', `${successCount} of ${totalRuns}`)}
                ${metricCard('Hallucination Rate', pct(m.hallucination_rate || 0), 'red', m.hallucination_rate > 0 ? 'needs attention' : 'clean')}
                ${metricCard('Evidence Coverage', pct(m.evidence_coverage || 0), 'blue', m.evidence_coverage >= 1 ? 'fully covered' : 'partial')}
            </div>

            <div class="card">
                <div class="card-title">Recent Runs</div>
                ${recent.length ? runsTable(recent) : emptyHTML('No runs yet. Start your first analysis!')}
            </div>
        </div>`;
        bindTableRows(app);
    } catch (e) {
        app.innerHTML = emptyHTML('Failed to load dashboard: ' + e.message);
    }
}

/* ================================================================
   RUNS LIST PAGE
   ================================================================ */
let currentFilter = 'ALL';

async function renderRuns(app) {
    app.innerHTML = loadingHTML();
    try {
        const runs = await API.listRuns().catch(() => []);
        renderRunsContent(app, runs || []);
    } catch (e) {
        app.innerHTML = emptyHTML('Failed to load runs: ' + e.message);
    }
}

function renderRunsContent(app, allRuns) {
    const filtered = currentFilter === 'ALL' ? allRuns
        : allRuns.filter(r => r.status === currentFilter);

    const filters = ['ALL', 'COMPLETED', 'FAILED', 'RUNNING'];

    app.innerHTML = `<div class="fade-in">
        <div class="page-header">
            <div><h1>All Runs</h1><div class="subtitle">${allRuns.length} total runs</div></div>
            <a href="#/new" class="btn btn-primary">${ICO.plus} New Analysis</a>
        </div>

        <div class="filter-bar">
            ${filters.map(f => `<button class="filter-btn ${currentFilter === f ? 'active' : ''}" data-filter="${f}">
                ${f === 'ALL' ? 'All' : f.charAt(0) + f.slice(1).toLowerCase()}
            </button>`).join('')}
        </div>

        <div class="card" style="padding:0; overflow:hidden;">
            ${filtered.length ? runsTable(filtered) : emptyHTML('No matching runs')}
        </div>
    </div>`;

    // Filter buttons
    app.querySelectorAll('.filter-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            currentFilter = btn.dataset.filter;
            renderRunsContent(app, allRuns);
        });
    });
    bindTableRows(app);
}

/* ================================================================
   SHARED TABLE
   ================================================================ */
function runsTable(runs) {
    return `<div class="table-wrap"><table>
        <thead><tr>
            <th>Run ID</th><th>Goal</th><th>Status</th><th>Steps</th><th>Created</th>
        </tr></thead>
        <tbody>
        ${runs.map(r => `<tr data-id="${esc(r.run_id)}">
            <td class="td-id">${esc(shortId(r.run_id))}</td>
            <td class="td-goal">${esc(r.goal || '—')}</td>
            <td>${statusBadge(r.status)}</td>
            <td>${r.current_step_index != null ? r.current_step_index + 1 : '—'}</td>
            <td class="td-time">${fmtTime(r.created_at)}</td>
        </tr>`).join('')}
        </tbody>
    </table></div>`;
}

function bindTableRows(container) {
    container.querySelectorAll('tbody tr[data-id]').forEach(tr => {
        tr.addEventListener('click', () => {
            location.hash = '#/run?id=' + encodeURIComponent(tr.dataset.id);
        });
    });
}

/* ================================================================
   RUN DETAIL PAGE
   ================================================================ */
async function renderRunDetail(app, id) {
    if (!id) { app.innerHTML = emptyHTML('No run ID specified'); return; }
    app.innerHTML = loadingHTML();

    try {
        const [run, steps, tools] = await Promise.all([
            API.getRun(id),
            API.getRunSteps(id).catch(() => []),
            API.getRunToolCalls(id).catch(() => []),
        ]);

        // Find report from last analyzer step
        let report = null;
        const stepsArr = steps || [];
        for (let i = stepsArr.length - 1; i >= 0; i--) {
            const parsed = tryParseJSON(stepsArr[i].output);
            if (parsed && typeof parsed === 'object' && parsed.error_summary) {
                report = parsed;
                break;
            }
        }

        const toolsArr = tools || [];

        app.innerHTML = `<div class="fade-in">
            <a href="#/runs" class="back-link">${ICO.arrow} Back to Runs</a>
            <div class="run-header">
                <div class="run-header-left">
                    <h1>${statusBadge(run.status)} <span class="text-mono">${esc(shortId(run.run_id))}</span></h1>
                    <div class="run-meta">
                        <span class="run-meta-item">${ICO.clock} ${fmtTime(run.created_at)}</span>
                        <span class="run-meta-item">Duration: ${fmtDuration(run.created_at, run.completed_at)}</span>
                        <span class="run-meta-item">Steps: ${stepsArr.length}</span>
                        <span class="run-meta-item">Tool Calls: ${toolsArr.length}</span>
                    </div>
                    ${run.goal ? `<div class="text-sm text-muted" style="margin-top:4px;">Goal: ${esc(run.goal)}</div>` : ''}
                </div>
                <div class="run-actions">
                    <button class="btn btn-ghost btn-sm" id="btn-replay">${ICO.play} Replay</button>
                </div>
            </div>

            <div class="tabs">
                <button class="tab active" data-tab="report">Report</button>
                <button class="tab" data-tab="steps">Steps (${stepsArr.length})</button>
                <button class="tab" data-tab="tools">Tool Calls (${toolsArr.length})</button>
            </div>

            <div class="tab-content active" id="tab-report">
                ${report ? renderReport(report) : emptyHTML('No structured report available')}
            </div>
            <div class="tab-content" id="tab-steps">
                ${stepsArr.length ? renderTimeline(stepsArr) : emptyHTML('No steps recorded')}
            </div>
            <div class="tab-content" id="tab-tools">
                ${toolsArr.length ? renderToolCalls(toolsArr) : emptyHTML('No tool calls recorded')}
            </div>
        </div>`;

        // Tab switching
        app.querySelectorAll('.tab').forEach(tab => {
            tab.addEventListener('click', () => {
                app.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
                app.querySelectorAll('.tab-content').forEach(tc => tc.classList.remove('active'));
                tab.classList.add('active');
                document.getElementById('tab-' + tab.dataset.tab).classList.add('active');
            });
        });

        // Accordion toggle
        app.querySelectorAll('.accordion-header').forEach(hdr => {
            hdr.addEventListener('click', () => {
                hdr.closest('.accordion-item').classList.toggle('open');
            });
        });

        // Replay button
        document.getElementById('btn-replay').addEventListener('click', async () => {
            const btn = document.getElementById('btn-replay');
            btn.disabled = true;
            btn.textContent = 'Replaying...';
            try {
                const result = await API.replayRun(id);
                alert('Replay complete!\n\nStatus: ' + (result.status || 'OK'));
            } catch (e) {
                alert('Replay failed: ' + e.message);
            }
            btn.disabled = false;
            btn.innerHTML = `${ICO.play} Replay`;
        });

        // Auto-refresh for running runs
        if (run.status === 'RUNNING') {
            const refreshId = setInterval(async () => {
                const latest = await API.getRun(id).catch(() => null);
                if (latest && latest.status !== 'RUNNING') {
                    clearInterval(refreshId);
                    renderRunDetail(app, id);
                }
            }, 3000);
        }
    } catch (e) {
        app.innerHTML = emptyHTML('Failed to load run: ' + e.message);
    }
}

/* ================================================================
   REPORT RENDERER
   ================================================================ */
function renderReport(r) {
    const evidence = r.evidence || [];
    const nextSteps = r.next_steps || [];

    return `<div class="report-grid">
        ${r.confidence ? `<div class="confidence-hero">
            <span style="color:var(--text-secondary); font-size:0.9rem;">Confidence:</span>
            <span class="conf-badge badge ${r.confidence.toLowerCase() === 'high' ? 'badge-success' : r.confidence.toLowerCase() === 'medium' ? 'badge-warning' : 'badge-error'}" style="font-size:0.95rem; padding:6px 20px;">
                ${esc(r.confidence)}
            </span>
        </div>` : ''}

        ${r.error_summary ? `<div class="report-section">
            <div class="section-label">Error Summary</div>
            <div class="section-body">${esc(r.error_summary)}</div>
        </div>` : ''}

        ${r.root_cause ? `<div class="report-section">
            <div class="section-label">Root Cause</div>
            <div class="section-body">${esc(r.root_cause)}</div>
        </div>` : ''}

        ${evidence.length ? `<div class="report-section">
            <div class="section-label">Evidence (${evidence.length})</div>
            <div class="evidence-list">
                ${evidence.map(e => `<div class="evidence-item">
                    <div class="evidence-file">${esc(e.file || '?')}${e.line ? ` <span class="line-num">:${e.line}</span>` : ''}</div>
                    <div class="evidence-text">${esc(e.content || e.text || '')}</div>
                </div>`).join('')}
            </div>
        </div>` : ''}

        ${nextSteps.length ? `<div class="report-section">
            <div class="section-label">Recommended Next Steps</div>
            <ol class="next-steps-list">
                ${nextSteps.map(s => `<li>${esc(typeof s === 'string' ? s : s.description || JSON.stringify(s))}</li>`).join('')}
            </ol>
        </div>` : ''}
    </div>`;
}

/* ================================================================
   STEPS TIMELINE RENDERER
   ================================================================ */
function renderTimeline(steps) {
    return `<div class="timeline">
        ${steps.map(s => {
            const status = (s.status || '').toUpperCase();
            const dotCls = status === 'SUCCEEDED' ? 'success'
                         : status === 'FAILED' ? 'error'
                         : status === 'RUNNING' ? 'running' : '';
            const parsed = tryParseJSON(s.output);
            return `<div class="timeline-item">
                <div class="timeline-dot ${dotCls}"></div>
                <div class="timeline-header">
                    <span class="timeline-agent">${esc(s.step_id || s.type || 'Step')}</span>
                    ${statusBadge(s.status)}
                </div>
                <div class="timeline-meta">
                    ${s.started_at ? fmtTime(s.started_at) : ''}
                    ${s.started_at && s.finished_at ? ' &middot; ' + fmtDuration(s.started_at, s.finished_at) : ''}
                </div>
                ${parsed && typeof parsed === 'object' ? `<details style="margin-top:8px;">
                    <summary style="cursor:pointer; font-size:0.82rem; color:var(--text-muted);">Show output</summary>
                    ${jsonBlock(parsed)}
                </details>` : ''}
            </div>`;
        }).join('')}
    </div>`;
}

/* ================================================================
   TOOL CALLS ACCORDION RENDERER
   ================================================================ */
function renderToolCalls(tools) {
    return `<div>
        ${tools.map(t => {
            const dur = fmtDuration(t.started_at, t.finished_at);
            return `<div class="accordion-item">
                <button class="accordion-header">
                    <span class="accordion-chevron">${ICO.chevron}</span>
                    <span class="tool-name">${esc(t.tool_name || '?')}</span>
                    ${statusBadge(t.status)}
                    <span class="tool-meta">${dur}</span>
                </button>
                <div class="accordion-body">
                    <div class="json-label">Input</div>
                    ${jsonBlock(t.input)}
                    <div class="json-label">Output</div>
                    ${jsonBlock(t.output)}
                </div>
            </div>`;
        }).join('')}
    </div>`;
}

/* ================================================================
   NEW RUN PAGE
   ================================================================ */
function renderNewRun(app) {
    app.innerHTML = `<div class="fade-in">
        <div class="page-header">
            <div><h1>New Analysis</h1><div class="subtitle">Submit a log directory for automated analysis</div></div>
        </div>
        <div class="card form-card">
            <form id="new-run-form">
                <div class="form-group">
                    <label class="form-label">Task ID</label>
                    <input class="form-input" name="task_id" placeholder="e.g. diagnose-payment-errors" required />
                    <div class="form-hint">A short identifier for this analysis task</div>
                </div>
                <div class="form-group">
                    <label class="form-label">Log Directory</label>
                    <input class="form-input text-mono" name="input" placeholder="/absolute/path/to/logs" required />
                    <div class="form-hint">Absolute path to the directory containing log files on the server</div>
                </div>
                <button type="submit" class="btn btn-primary" id="submit-btn">${ICO.play} Start Analysis</button>
            </form>
        </div>
    </div>`;

    document.getElementById('new-run-form').addEventListener('submit', async (e) => {
        e.preventDefault();
        const form = e.target;
        const btn = document.getElementById('submit-btn');
        const taskId = form.task_id.value.trim();
        const input = form.input.value.trim();
        if (!taskId || !input) return;

        btn.disabled = true;
        btn.textContent = 'Starting...';
        try {
            const result = await API.createRun(taskId, input);
            const runId = result.run_id || result.id;
            if (runId) {
                location.hash = '#/run?id=' + encodeURIComponent(runId);
            } else {
                alert('Run created but no ID returned');
                location.hash = '#/runs';
            }
        } catch (err) {
            alert('Failed to create run: ' + err.message);
            btn.disabled = false;
            btn.innerHTML = `${ICO.play} Start Analysis`;
        }
    });
}

/* ================================================================
   INIT
   ================================================================ */
window.addEventListener('hashchange', route);
window.addEventListener('DOMContentLoaded', () => {
    checkHealth();
    setInterval(checkHealth, 30000);
    route();
});
