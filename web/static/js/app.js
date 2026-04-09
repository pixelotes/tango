document.addEventListener('alpine:init', () => {
    Alpine.data('tangoApp', () => ({
        torrents: [],
        stats: { torrent_count: 0, speed_download: 0, speed_upload: 0, active_count: 0, waiting_count: 0, stopped_count: 0 },
        filter: 'all',
        selectedTorrent: null,
        detailTab: 'files',
        showAddModal: false,
        addForm: { uri: '', target_path: '', label: '' },
        addFile: null,
        polling: null,

        init() {
            this.fetchAll();
            this.polling = setInterval(() => this.fetchAll(), 2000);
        },

        destroy() {
            if (this.polling) clearInterval(this.polling);
        },

        async fetchAll() {
            try {
                const [tRes, sRes] = await Promise.all([
                    fetch('/api/torrents'),
                    fetch('/api/stats')
                ]);
                this.torrents = await tRes.json();
                this.stats = await sRes.json();
            } catch (e) {
                console.error('Fetch error:', e);
            }
        },

        get filteredTorrents() {
            if (this.filter === 'all') return this.torrents;
            if (this.filter.startsWith('label:')) {
                const label = this.filter.slice(6);
                return this.torrents.filter(t => t.label === label);
            }
            return this.torrents.filter(t => {
                switch (this.filter) {
                    case 'active': return t.status === 'Downloading' || t.status === 'Seeding';
                    case 'waiting': return t.status === 'Waiting';
                    case 'complete': return t.status === 'Complete' || t.status === 'Seeding';
                    case 'stopped': return t.status === 'Stopped';
                    case 'error': return t.status === 'Error';
                    default: return true;
                }
            });
        },

        filterCount(filter) {
            if (filter === 'all') return this.torrents.length;
            return this.torrents.filter(t => {
                switch (filter) {
                    case 'active': return t.status === 'Downloading' || t.status === 'Seeding';
                    case 'waiting': return t.status === 'Waiting';
                    case 'complete': return t.status === 'Complete' || t.status === 'Seeding';
                    case 'stopped': return t.status === 'Stopped';
                    case 'error': return t.status === 'Error';
                    default: return 0;
                }
            }).length;
        },

        labelCounts() {
            const counts = {};
            this.torrents.forEach(t => {
                if (t.label) counts[t.label] = (counts[t.label] || 0) + 1;
            });
            return counts;
        },

        async addTorrent() {
            if (this.addFile) {
                const fd = new FormData();
                fd.append('file', this.addFile);
                fd.append('target_path', this.addForm.target_path);
                fd.append('label', this.addForm.label);
                await fetch('/api/torrents/file', { method: 'POST', body: fd });
            } else if (this.addForm.uri) {
                await fetch('/api/torrents', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(this.addForm)
                });
            }
            this.showAddModal = false;
            this.addForm = { uri: '', target_path: '', label: '' };
            this.addFile = null;
            this.fetchAll();
        },

        async removeTorrent(id, deleteData) {
            if (!confirm(deleteData ? 'Delete torrent AND data?' : 'Remove torrent?')) return;
            await fetch(`/api/torrents/${id}?delete_data=${deleteData}`, { method: 'DELETE' });
            if (this.selectedTorrent?.id === id) this.selectedTorrent = null;
            this.fetchAll();
        },

        async startTorrent(id) {
            await fetch(`/api/torrents/${id}/start`, { method: 'POST' });
        },

        async stopTorrent(id) {
            await fetch(`/api/torrents/${id}/stop`, { method: 'POST' });
        },

        async selectTorrent(id) {
            if (this.selectedTorrent?.id === id) {
                this.selectedTorrent = null;
                return;
            }
            try {
                const res = await fetch(`/api/torrents/${id}`);
                this.selectedTorrent = await res.json();
            } catch (e) {
                console.error('Detail fetch error:', e);
            }
        },

        async refreshDetail() {
            if (!this.selectedTorrent) return;
            try {
                const res = await fetch(`/api/torrents/${this.selectedTorrent.id}`);
                this.selectedTorrent = await res.json();
            } catch (e) { /* ignore */ }
        },

        progressBarClass(t) {
            if (t.progress >= 1) return 'complete';
            if (t.status === 'Seeding') return 'seeding';
            return '';
        },

        formatBytes(n) {
            if (n == null || n === 0) return '0 B';
            const units = ['B', 'KB', 'MB', 'GB', 'TB'];
            const i = Math.floor(Math.log(n) / Math.log(1024));
            return (n / Math.pow(1024, i)).toFixed(i > 0 ? 1 : 0) + ' ' + units[i];
        },

        formatSpeed(n) {
            return this.formatBytes(n) + '/s';
        },

        formatETA(seconds) {
            if (seconds == null || seconds <= 0) return '--';
            if (seconds > 86400) return Math.floor(seconds / 86400) + 'd ' + Math.floor((seconds % 86400) / 3600) + 'h';
            if (seconds > 3600) return Math.floor(seconds / 3600) + 'h ' + Math.floor((seconds % 3600) / 60) + 'm';
            if (seconds > 60) return Math.floor(seconds / 60) + 'm ' + (seconds % 60) + 's';
            return seconds + 's';
        },

        formatPercent(p) {
            return (p * 100).toFixed(1) + '%';
        },

        formatRatio(r) {
            return r != null ? r.toFixed(2) : '0.00';
        },

        statusBadge(status) {
            const map = {
                'Downloading': 'bg-primary',
                'Seeding': 'bg-success',
                'Stopped': 'bg-secondary',
                'Waiting': 'bg-warning text-dark',
                'Complete': 'bg-info',
                'Error': 'bg-danger',
                'Removed': 'bg-dark'
            };
            return map[status] || 'bg-secondary';
        }
    }));
});
