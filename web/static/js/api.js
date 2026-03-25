/* ================================================================
   API Client – wraps all backend endpoints
   ================================================================ */
const API = {
    base: '',

    async _get(path) {
        const res = await fetch(this.base + path);
        if (!res.ok) throw new Error(`GET ${path}: ${res.status}`);
        return res.json();
    },

    async _post(path, body) {
        const res = await fetch(this.base + path, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        });
        if (!res.ok) {
            const txt = await res.text();
            throw new Error(txt || `POST ${path}: ${res.status}`);
        }
        return res.json();
    },

    listRuns()            { return this._get('/runs'); },
    getRun(id)            { return this._get(`/runs/${encodeURIComponent(id)}`); },
    getRunSteps(id)       { return this._get(`/runs/${encodeURIComponent(id)}/steps`); },
    getRunToolCalls(id)   { return this._get(`/runs/${encodeURIComponent(id)}/tools`); },
    createRun(taskId, input) {
        return this._post('/runs', { task_id: taskId, input: { directory: input } });
    },
    replayRun(id)         { return this._post(`/runs/${encodeURIComponent(id)}/replay`, {}); },
    getMetrics()          { return this._get('/metrics'); },
    getRunMetrics(id)     { return this._get(`/metrics/${encodeURIComponent(id)}`); },

    async checkHealth() {
        try {
            const res = await fetch(this.base + '/health');
            return res.ok;
        } catch { return false; }
    },
};
