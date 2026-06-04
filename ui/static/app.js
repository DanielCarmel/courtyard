function app() {
  return {
    // State
    user: null,
    loading: true,
    repos: [],
    forms: [],
    formSpec: null,
    selectedRepo: '',
    selectedForm: '',
    values: {},
    fieldErrors: {},
    previewFiles: [],
    previewLoading: false,
    submitting: false,
    prURL: '',
    errorMsg: '',
    _previewTimer: null,

    async init() {
      try {
        const resp = await fetch('/api/me', { credentials: 'same-origin' });
        if (resp.status === 401) {
          this.user = null;
          this.loading = false;
          return;
        }
        this.user = await resp.json();
        await this.loadRepos();
      } catch (e) {
        this.errorMsg = 'Failed to connect to server.';
      } finally {
        this.loading = false;
      }
    },

    async loadRepos() {
      try {
        const resp = await fetch('/api/repos', { credentials: 'same-origin' });
        if (!resp.ok) { this.errorMsg = 'Failed to load repositories.'; return; }
        this.repos = await resp.json();
      } catch (e) {
        this.errorMsg = 'Failed to load repositories.';
      }
    },

    async onRepoChange() {
      this.forms = [];
      this.formSpec = null;
      this.selectedForm = '';
      this.values = {};
      this.previewFiles = [];
      this.prURL = '';
      this.errorMsg = '';
      if (!this.selectedRepo) return;
      const [owner, repo] = this.selectedRepo.split('/');
      try {
        const resp = await fetch(`/api/repos/${owner}/${repo}/forms`, { credentials: 'same-origin' });
        if (!resp.ok) { this.errorMsg = 'Failed to load forms.'; return; }
        this.forms = (await resp.json()) || [];
      } catch (e) {
        this.errorMsg = 'Failed to load forms.';
      }
    },

    async onFormChange() {
      this.formSpec = null;
      this.values = {};
      this.fieldErrors = {};
      this.previewFiles = [];
      this.prURL = '';
      this.errorMsg = '';
      if (!this.selectedForm || !this.selectedRepo) return;
      const [owner, repo] = this.selectedRepo.split('/');
      try {
        const resp = await fetch(`/api/repos/${owner}/${repo}/forms/${this.selectedForm}`, { credentials: 'same-origin' });
        if (!resp.ok) { this.errorMsg = 'Failed to load form schema.'; return; }
        this.formSpec = await resp.json();
        // Apply defaults.
        for (const field of (this.formSpec.fields || [])) {
          if (field.default !== undefined && field.default !== null) {
            this.values[field.name] = field.default;
          }
        }
        this.schedulePreview();
      } catch (e) {
        this.errorMsg = 'Failed to load form schema.';
      }
    },

    schedulePreview() {
      clearTimeout(this._previewTimer);
      this._previewTimer = setTimeout(() => this.loadPreview(), 500);
    },

    async loadPreview() {
      if (!this.formSpec || !this.selectedRepo) return;
      const [owner, repo] = this.selectedRepo.split('/');
      this.previewLoading = true;
      try {
        const resp = await fetch(
          `/api/repos/${owner}/${repo}/forms/${this.selectedForm}/preview`,
          {
            method: 'POST',
            credentials: 'same-origin',
            headers: { 'Content-Type': 'application/json', 'X-Requested-With': 'XMLHttpRequest' },
            body: JSON.stringify(this.values),
          }
        );
        const data = await resp.json();
        if (!resp.ok) {
          if (data.errors) { this.fieldErrors = data.errors; }
          this.previewFiles = [];
          return;
        }
        this.fieldErrors = {};
        this.previewFiles = data.files || [];
      } catch (e) {
        this.errorMsg = 'Preview failed.';
      } finally {
        this.previewLoading = false;
      }
    },

    async submitForm() {
      if (this.submitting) return;
      this.submitting = true;
      this.prURL = '';
      this.errorMsg = '';
      this.fieldErrors = {};
      const [owner, repo] = this.selectedRepo.split('/');
      try {
        const resp = await fetch(
          `/api/repos/${owner}/${repo}/forms/${this.selectedForm}/submit`,
          {
            method: 'POST',
            credentials: 'same-origin',
            headers: { 'Content-Type': 'application/json', 'X-Requested-With': 'XMLHttpRequest' },
            body: JSON.stringify(this.values),
          }
        );
        const data = await resp.json();
        if (!resp.ok) {
          if (data.errors) { this.fieldErrors = data.errors; }
          else { this.errorMsg = data.error || 'Submit failed.'; }
          return;
        }
        this.prURL = data.pr_url || '';
      } catch (e) {
        this.errorMsg = 'Submit failed. Please try again.';
      } finally {
        this.submitting = false;
      }
    },
  };
}
