function app() {
  return {
    // ── Core state ──
    user: null,
    loading: true,
    currentView: 'dashboard',
    errorMsg: '',

    // ── Projects ──
    pinnedRepos: [],      // [{owner, name}] stored in localStorage
    allRepos: [],         // [{Owner, Name, DefaultBranch, ...}] from /api/repos
    allReposLoading: false,
    projectsSearch: '',

    // ── Repo detail ──
    selectedRepo: null,   // {owner, name}
    forms: [],
    formsLoading: false,

    // ── Form ──
    formSpec: null,
    selectedForm: '',
    values: {},
    fieldErrors: {},

    // ── Preview / submit ──
    previewFiles: [],
    openPreviewFiles: {},  // {path: bool} — tracks which preview files are expanded
    previewLoading: false,
    submitting: false,
    prURL: '',

    // ── Folder tree ──
    treeFiles: [],        // existing file paths from scoped tree API
    treeTruncated: false, // true when the API capped the result
    treeLoading: false,

    // ── UI ──
    theme: 'light',
    sidebarOpen: false,
    _previewTimer: null,

    // ── Computed ──
    get previewContentMap() {
      const m = {};
      for (const f of this.previewFiles) m[f.path] = f.content;
      return m;
    },

    get filteredRepos() {
      if (!this.projectsSearch.trim()) return this.allRepos;
      const q = this.projectsSearch.toLowerCase();
      return this.allRepos.filter(r =>
        `${r.Owner}/${r.Name}`.toLowerCase().includes(q)
      );
    },

    // Flat list of tree nodes for the Folder Tree panel.
    // Combines existing repo files (treeFiles) with generated output paths (previewFiles).
    get folderTreeItems() {
      if (!this.previewFiles.length) return [];

      const existingSet = new Set(this.treeFiles);
      const generatedPaths = this.previewFiles.map(f => f.path);
      const generatedSet = new Set(generatedPaths);

      // Restrict to files under the common ancestor of the generated paths.
      const ancestor = this._commonAncestor(generatedPaths);
      const prefix = ancestor ? ancestor + '/' : '';
      const relevantExisting = prefix
        ? this.treeFiles.filter(p => p.startsWith(prefix))
        : this.treeFiles;
      const allFilePaths = new Set([...relevantExisting, ...generatedPaths]);

      // Collect all implied directory paths.
      const dirSet = new Set();
      for (const p of allFilePaths) {
        const segs = p.split('/');
        for (let i = 1; i < segs.length; i++) {
          dirSet.add(segs.slice(0, i).join('/'));
        }
      }

      const items = [];

      // Root dir header (shows the full ancestor path as label).
      if (ancestor) {
        items.push({
          label: ancestor,
          fullPath: ancestor,
          depth: 0,
          isFolder: true,
          isRoot: true,
          isGenerated: false,
          isNew: false,
          isModified: false,
        });
      }

      // Return direct children of dirPath: subdirs first (alpha), then files (alpha).
      const getChildren = (dirPath) => {
        const pfx = dirPath ? dirPath + '/' : '';
        const children = [];
        for (const d of dirSet) {
          if (pfx && !d.startsWith(pfx)) continue;
          if (d.slice(pfx.length).includes('/')) continue;
          children.push({ isFolder: true, path: d });
        }
        for (const f of allFilePaths) {
          if (pfx && !f.startsWith(pfx)) continue;
          if (f.slice(pfx.length).includes('/')) continue;
          children.push({ isFolder: false, path: f });
        }
        return children.sort((a, b) => {
          if (a.isFolder !== b.isFolder) return a.isFolder ? -1 : 1;
          return a.path.localeCompare(b.path);
        });
      };

      const dfs = (dirPath, depth) => {
        for (const child of getChildren(dirPath)) {
          const label = child.path.split('/').pop();
          if (child.isFolder) {
            items.push({ label, fullPath: child.path, depth, isFolder: true, isRoot: false, isGenerated: false, isNew: false, isModified: false });
            dfs(child.path, depth + 1);
          } else {
            const isGenerated = generatedSet.has(child.path);
            items.push({ label, fullPath: child.path, depth, isFolder: false, isRoot: false, isGenerated, isNew: isGenerated && !existingSet.has(child.path), isModified: isGenerated && existingSet.has(child.path) });
          }
        }
      };

      dfs(ancestor, ancestor ? 1 : 0);
      return items;
    },

    // ── Lifecycle ──
    async init() {
      const savedTheme = localStorage.getItem('courtyard_theme') || 'light';
      this.theme = savedTheme;
      if (savedTheme === 'dark') document.body.classList.add('dark-theme');

      try {
        const resp = await fetch('/api/me', { credentials: 'same-origin' });
        if (resp.status === 401) {
          this.user = null;
          this.loading = false;
          return;
        }
        this.user = await resp.json();
        this.loadPinnedRepos();
      } catch (e) {
        this.errorMsg = 'Failed to connect to server.';
      } finally {
        this.loading = false;
      }
    },

    // ── Pinned repos (localStorage) ──
    loadPinnedRepos() {
      try {
        const stored = localStorage.getItem('courtyard_pinned_repos');
        this.pinnedRepos = stored ? JSON.parse(stored) : [];
      } catch (e) {
        this.pinnedRepos = [];
      }
    },

    savePinnedRepos() {
      localStorage.setItem('courtyard_pinned_repos', JSON.stringify(this.pinnedRepos));
    },

    isPinned(owner, name) {
      return this.pinnedRepos.some(r => r.owner === owner && r.name === name);
    },

    togglePin(owner, name) {
      if (this.isPinned(owner, name)) {
        this.pinnedRepos = this.pinnedRepos.filter(
          r => !(r.owner === owner && r.name === name)
        );
      } else {
        this.pinnedRepos.push({ owner, name });
      }
      this.savePinnedRepos();
    },

    // ── Navigation ──
    async navigateTo(view) {
      this.currentView = view;
      this.errorMsg = '';
      this.prURL = '';
      if (view === 'projects' && this.allRepos.length === 0) {
        await this.loadAllRepos();
      }
    },

    // ── All repos (projects page) ──
    async loadAllRepos() {
      this.allReposLoading = true;
      this.errorMsg = '';
      try {
        const resp = await fetch('/api/repos', { credentials: 'same-origin' });
        if (resp.status === 401) { window.location.href = '/auth/github/login'; return; }
        if (!resp.ok) {
          this.errorMsg = 'Failed to load repositories.';
          return;
        }
        this.allRepos = (await resp.json()) || [];
      } catch (e) {
        this.errorMsg = 'Failed to load repositories.';
      } finally {
        this.allReposLoading = false;
      }
    },

    // ── Repo detail ──
    async openRepo(owner, name) {
      this.selectedRepo = { owner, name };
      this.forms = [];
      this.formSpec = null;
      this.selectedForm = '';
      this.values = {};
      this.fieldErrors = {};
      this.previewFiles = [];
      this.prURL = '';
      this.errorMsg = '';
      this.treeFiles = [];
      this.treeTruncated = false;
      this.treeLoading = false;
      this.currentView = 'repo-detail';
      this.formsLoading = true;
      try {
        const resp = await fetch(`/api/repos/${owner}/${name}/forms`, { credentials: 'same-origin' });
        if (resp.status === 401) {
          window.location.href = '/auth/github/login';
          return;
        }
        if (!resp.ok) {
          this.errorMsg = 'Failed to load forms.';
          return;
        }
        this.forms = (await resp.json()) || [];
      } catch (e) {
        this.errorMsg = 'Failed to load forms.';
      } finally {
        this.formsLoading = false;
      }
    },

    // ── Form selection ──
    async selectForm(formName) {
      this.selectedForm = formName;
      this.formSpec = null;
      this.values = {};
      this.fieldErrors = {};
      this.previewFiles = [];
      this.prURL = '';
      this.errorMsg = '';
      this.treeFiles = [];
      this.treeTruncated = false;
      this.treeLoading = false;
      if (!formName || !this.selectedRepo) return;
      const { owner, name } = this.selectedRepo;
      try {
        const resp = await fetch(
          `/api/repos/${owner}/${name}/forms/${formName}`,
          { credentials: 'same-origin' }
        );
        if (resp.status === 401) { window.location.href = '/auth/github/login'; return; }
        if (!resp.ok) {
          this.errorMsg = 'Failed to load form schema.';
          return;
        }
        this.formSpec = await resp.json();
        for (const field of (this.formSpec.fields || [])) {
          if (field.default !== undefined && field.default !== null) {
            // Resolve built-in tokens: $_username, $_provider
            let def = field.default;
            if (def === '$_username') def = this.user?.username || '';
            else if (def === '$_provider') def = this.user?.provider || '';
            this.values[field.name] = def;
          }
        }
        this.schedulePreview();
      } catch (e) {
        this.errorMsg = 'Failed to load form schema.';
      }
    },

    // ── Preview ──
    schedulePreview() {
      clearTimeout(this._previewTimer);
      this._previewTimer = setTimeout(() => this.loadPreview(), 500);
    },

    async loadPreview() {
      if (!this.formSpec || !this.selectedRepo) return;
      const { owner, name } = this.selectedRepo;
      this.previewLoading = true;
      this.treeFiles = [];
      this.treeTruncated = false;
      try {
        const resp = await fetch(
          `/api/repos/${owner}/${name}/forms/${this.selectedForm}/preview`,
          {
            method: 'POST',
            credentials: 'same-origin',
            headers: {
              'Content-Type': 'application/json',
              'X-Requested-With': 'XMLHttpRequest',
            },
            body: JSON.stringify(this._payloadValues()),
          }
        );
        const data = await resp.json();
        if (!resp.ok) {
          if (data.errors) this.fieldErrors = data.errors;
          this.previewFiles = [];
          return;
        }
        this.fieldErrors = {};
        this.previewFiles = data.files || [];
        this.openPreviewFiles = {};
        if (this.previewFiles.length > 0) this.loadScopedTree();
      } catch (e) {
        this.errorMsg = 'Preview failed.';
      } finally {
        this.previewLoading = false;
      }
    },

    // ── Preview file collapse ──
    togglePreviewFile(path) {
      this.openPreviewFiles = {
        ...this.openPreviewFiles,
        [path]: !this.openPreviewFiles[path],
      };
    },

    // Fetch the repo subtree scoped to the common ancestor of generated output paths.
    async loadScopedTree() {
      if (!this.previewFiles.length || !this.selectedRepo) return;
      const { owner, name } = this.selectedRepo;
      const ancestor = this._commonAncestor(this.previewFiles.map(f => f.path));
      const params = ancestor
        ? `?path=${encodeURIComponent(ancestor)}&max=500`
        : '?max=500';
      this.treeLoading = true;
      try {
        const resp = await fetch(
          `/api/repos/${owner}/${name}/tree${params}`,
          { credentials: 'same-origin', headers: { 'X-Requested-With': 'XMLHttpRequest' } }
        );
        if (!resp.ok) return;
        const data = await resp.json();
        this.treeFiles = data.paths || [];
        this.treeTruncated = data.truncated || false;
      } catch (_) {
        // Tree is non-critical — silently ignore errors
      } finally {
        this.treeLoading = false;
      }
    },

    // Returns the longest common directory prefix of an array of file paths.
    _commonAncestor(paths) {
      if (!paths.length) return '';
      const dirs = paths.map(p => {
        const i = p.lastIndexOf('/');
        return i >= 0 ? p.slice(0, i) : '';
      });
      if (dirs.every(d => d === '')) return '';
      const parts = dirs[0].split('/');
      for (const dir of dirs.slice(1)) {
        const other = dir.split('/');
        let i = 0;
        while (i < parts.length && i < other.length && parts[i] === other[i]) i++;
        parts.splice(i);
      }
      return parts.join('/');
    },

    // ── Submit ──
    async submitForm() {
      if (this.submitting) return;
      this.submitting = true;
      this.prURL = '';
      this.errorMsg = '';
      this.fieldErrors = {};
      const { owner, name } = this.selectedRepo;
      try {
        const resp = await fetch(
          `/api/repos/${owner}/${name}/forms/${this.selectedForm}/submit`,
          {
            method: 'POST',
            credentials: 'same-origin',
            headers: {
              'Content-Type': 'application/json',
              'X-Requested-With': 'XMLHttpRequest',
            },
            body: JSON.stringify(this._payloadValues()),
          }
        );
        const data = await resp.json();
        if (!resp.ok) {
          if (data.errors) this.fieldErrors = data.errors;
          else this.errorMsg = data.error || 'Submission failed.';
          return;
        }
        this.prURL = data.pr_url || '';
      } catch (e) {
        this.errorMsg = 'Submission failed.';
      } finally {
        this.submitting = false;
      }
    },

    // ── Payload helper — merges form values with built-in context vars ──
    _payloadValues() {
      return {
        ...this.values,
        _username: this.user?.username || '',
        _provider: this.user?.provider || '',
      };
    },

    // Simple client-side Go template interpolation: replaces {{ .field }} with current values.
    renderGoTemplate(tmpl) {
      if (!tmpl) return '';
      // Matches {{ .field }} and {{ .field | pipe1 arg | pipe2 arg ... }}
      return tmpl.replace(/\{\{\s*\.(\w+)((?:\s*\|[^}]*)?)\s*\}\}/g, (_, key, pipeStr) => {
        const v = this.values[key];
        let val = (v !== undefined && v !== null && v !== '') ? String(v) : '';

        if (pipeStr) {
          const pipes = pipeStr.split('|').map(p => p.trim()).filter(Boolean);
          for (const pipe of pipes) {
            const fn = pipe.match(/^(\w+)/)?.[1];
            const strArgs = [...pipe.matchAll(/"([^"]*)"/g)].map(m => m[1]);
            const numArg  = pipe.match(/\s+(\d+)\s*$/)?.[1];
            switch (fn) {
              case 'lower':     val = val.toLowerCase(); break;
              case 'upper':     val = val.toUpperCase(); break;
              case 'replace':   if (strArgs.length >= 2) val = val.split(strArgs[0]).join(strArgs[1]); break;
              case 'trunc':     if (numArg) val = val.slice(0, parseInt(numArg)); break;
              case 'default':   if (!val && strArgs.length) val = strArgs[0]; break;
              case 'kebabcase': val = val.toLowerCase().replace(/[\s_]+/g, '-'); break;
              case 'camelcase': val = val.replace(/[-_\s]+(.)/g, (_, c) => c.toUpperCase()); break;
              case 'title':     val = val.replace(/\b\w/g, c => c.toUpperCase()); break;
            }
          }
        }

        return val || `{${key}}`;
      });
    },

    // ── Theme ──
    toggleTheme() {
      this.theme = this.theme === 'light' ? 'dark' : 'light';
      localStorage.setItem('courtyard_theme', this.theme);
      document.body.classList.toggle('dark-theme', this.theme === 'dark');
    },

    // ═══════════════════════════════════════════════════════════════
    // ── Studio ──────────────────────────────────────────────────────
    // ═══════════════════════════════════════════════════════════════

    // State
    studioRepo: null,            // {Owner, Name, DefaultBranch} from allRepos
    studioRepoSearch: '',
    studioSpec: '',              // raw form spec YAML kept in sync with editor
    studioTemplates: [],         // [{id, name, content}]
    studioValues: {},            // filled by the preview form
    studioSpecFields: [],        // FieldSpec[] parsed from a successful spec parse
    studioPreviewFiles: [],      // [{path, content}] from last preview
    studioPreviewLoading: false,
    studioSubmitting: false,
    studioDownloading: false,
    studioPrURL: '',
    studioError: '',
    studioSpecError: '',
    studioFieldErrors: {},
    studioHelpOpen: false,
    studioRepoOpen: false,
    _studioPreviewTimer: null,

    // Studio folder tree (mirrors repo-detail tree, scoped to selected repo)
    studioTreeFiles: [],
    studioTreeTruncated: false,
    studioTreeLoading: false,

    // CodeMirror instances (not reactive — managed imperatively)
    _studioSpecCM: null,
    _studioTemplateCMs: {},      // {id: CodeMirrorInstance}

    // Derived: repos filtered by studioRepoSearch
    get studioFilteredRepos() {
      if (!this.studioRepoSearch.trim()) return this.allRepos;
      const q = this.studioRepoSearch.toLowerCase();
      return this.allRepos.filter(r =>
        `${r.Owner}/${r.Name}`.toLowerCase().includes(q)
      );
    },

    // Flat list of tree nodes for the Studio Folder Tree panel.
    get studioFolderTreeItems() {
      if (!this.studioPreviewFiles.length) return [];

      const existingSet = new Set(this.studioTreeFiles);
      const generatedPaths = this.studioPreviewFiles.map(f => f.path);
      const generatedSet = new Set(generatedPaths);

      const ancestor = this._commonAncestor(generatedPaths);
      const prefix = ancestor ? ancestor + '/' : '';
      const relevantExisting = prefix
        ? this.studioTreeFiles.filter(p => p.startsWith(prefix))
        : this.studioTreeFiles;
      const allFilePaths = new Set([...relevantExisting, ...generatedPaths]);

      const dirSet = new Set();
      for (const p of allFilePaths) {
        const segs = p.split('/');
        for (let i = 1; i < segs.length; i++) dirSet.add(segs.slice(0, i).join('/'));
      }

      const items = [];
      if (ancestor) {
        items.push({ label: ancestor, fullPath: ancestor, depth: 0, isFolder: true, isRoot: true, isGenerated: false, isNew: false, isModified: false });
      }

      const getChildren = (dirPath) => {
        const pfx = dirPath ? dirPath + '/' : '';
        const children = [];
        for (const d of dirSet) {
          if (pfx && !d.startsWith(pfx)) continue;
          if (d.slice(pfx.length).includes('/')) continue;
          children.push({ isFolder: true, path: d });
        }
        for (const f of allFilePaths) {
          if (pfx && !f.startsWith(pfx)) continue;
          if (f.slice(pfx.length).includes('/')) continue;
          children.push({ isFolder: false, path: f });
        }
        return children.sort((a, b) => {
          if (a.isFolder !== b.isFolder) return a.isFolder ? -1 : 1;
          return a.path.localeCompare(b.path);
        });
      };

      const dfs = (dirPath, depth) => {
        for (const child of getChildren(dirPath)) {
          const label = child.path.split('/').pop();
          if (child.isFolder) {
            items.push({ label, fullPath: child.path, depth, isFolder: true, isRoot: false, isGenerated: false, isNew: false, isModified: false });
            dfs(child.path, depth + 1);
          } else {
            const isGenerated = generatedSet.has(child.path);
            items.push({ label, fullPath: child.path, depth, isFolder: false, isRoot: false, isGenerated, isNew: isGenerated && !existingSet.has(child.path), isModified: isGenerated && existingSet.has(child.path) });
          }
        }
      };

      dfs(ancestor, ancestor ? 1 : 0);
      return items;
    },

    // File paths that will be committed — shown as a tree after repo selection.
    get studioCommitTree() {
      if (!this.studioRepo) return [];
      let specName = 'my-form';
      try {
        if (typeof jsyaml !== 'undefined' && this.studioSpec.trim()) {
          const parsed = jsyaml.load(this.studioSpec);
          if (parsed && parsed.name) specName = String(parsed.name);
        }
      } catch (_) {}
      const paths = [`.courtyard/forms/${specName}.yaml`];
      for (const t of this.studioTemplates) {
        const name = (t.name || '').trim().replace(/^\/+/, '');
        if (name && !name.includes('..')) {
          paths.push(`.courtyard/templates/${specName}/${name}`);
        }
      }
      return paths;
    },

    // Check if a repo has .courtyard config by comparing against forms already
    // known in the repos that were opened (lightweight heuristic — no extra API call).
    studioRepoHasCourtyard(repo) {
      // We mark repos via studioKnownCourtyardRepos populated lazily.
      return (this._studioxKnownCourtyard || []).includes(`${repo.Owner}/${repo.Name}`);
    },

    // ── Open Studio view ──
    async openStudio() {
      this.currentView = 'studio';
      this.errorMsg = '';
      this.studioError = '';
      this.studioPrURL = '';
      this.studioTreeFiles = [];
      this.studioTreeTruncated = false;
      this.studioTreeLoading = false;
      this.studioRepoOpen = false;
      // Ensure repo list is loaded for the selector.
      if (this.allRepos.length === 0) await this.loadAllRepos();
      // Detect which repos have .courtyard/ configs by calling ListForms for repos
      // already opened during the session (stored lazily in _studioxKnownCourtyard).
      if (!this._studioxKnownCourtyard) this._studioxKnownCourtyard = [];
      // Use setTimeout so x-show has applied display:block before CodeMirror measures.
      setTimeout(() => this._initStudioSpecEditor(), 50);
    },

    // ── Detect courtyard repos (lazy, called after allRepos loads) ──
    async _detectCourtyardRepos() {
      if (!this.allRepos.length) return;
      if (!this._studioxKnownCourtyard) this._studioxKnownCourtyard = [];
      // Sample a batch of repos concurrently (max 10 to stay light).
      const batch = this.allRepos.slice(0, 10);
      await Promise.all(batch.map(async repo => {
        const key = `${repo.Owner}/${repo.Name}`;
        if (this._studioxKnownCourtyard.includes(key)) return;
        try {
          const resp = await fetch(`/api/repos/${repo.Owner}/${repo.Name}/forms`, {
            credentials: 'same-origin',
          });
          if (resp.ok) {
            const forms = await resp.json();
            if (forms && forms.length > 0) {
              this._studioxKnownCourtyard.push(key);
            }
          }
        } catch (_) { /* ignore */ }
      }));
    },

    // ── Spec editor (CodeMirror) ──
    _initStudioSpecEditor() {
      const el = document.getElementById('studio-spec-editor');
      if (!el || this._studioSpecCM) return;
      if (typeof CodeMirror === 'undefined') return;
      this._studioSpecCM = CodeMirror.fromTextArea(el, {
        mode: 'yaml',
        lineNumbers: true,
        tabSize: 2,
        indentWithTabs: false,
        lineWrapping: true,
        autofocus: true,
      });
      // Seed with starter scaffold if blank.
      if (!this.studioSpec) {
        const scaffold = this._studioSpecScaffold();
        this._studioSpecCM.setValue(scaffold);
        this.studioSpec = scaffold;
      } else {
        this._studioSpecCM.setValue(this.studioSpec);
      }
      // refresh() re-measures layout — required after display:none → display:block.
      this._studioSpecCM.refresh();
      this._studioSpecCM.on('change', () => {
        this.studioSpec = this._studioSpecCM.getValue();
        this._scheduleStudioParse();
      });
    },

    _studioSpecScaffold() {
      return [
        'name: my-form',
        'description: "A short description of what this form does"',
        'targetBranch: main',
        'branchName: "courtyard/{{ .team }}/{{ .name }}"',
        'commitMessage: "feat({{ .team }}): add {{ .name }}"',
        'outputPath: "services/{{ .team }}"',
        '',
        'fields:',
        '  - name: team',
        '    type: enum',
        '    label: Team',
        '    required: true',
        '    options: [platform, backend, frontend]',
        '',
        '  - name: name',
        '    type: string',
        '    label: Service Name',
        '    required: true',
        '    validation: "^[a-z][a-z0-9-]{1,39}$"',
        '',
        'templates:',
        '  service.yaml.tmpl: {}',
      ].join('\n');
    },

    // ── Template editors ──
    addStudioTemplate() {
      const id = Date.now();
      const n = this.studioTemplates.length + 1;
      this.studioTemplates.push({ id, name: `template-${n}.yaml.tmpl`, content: '' });
      setTimeout(() => this._initTemplateEditor(id), 50);
    },

    removeStudioTemplate(id) {
      if (this._studioTemplateCMs[id]) {
        this._studioTemplateCMs[id].toTextArea();
        delete this._studioTemplateCMs[id];
      }
      this.studioTemplates = this.studioTemplates.filter(t => t.id !== id);
      this._scheduleStudioPreview();
    },

    _initTemplateEditor(id) {
      const el = document.getElementById(`studio-tmpl-${id}`);
      if (!el || this._studioTemplateCMs[id]) return;
      if (typeof CodeMirror === 'undefined') return;
      const cm = CodeMirror.fromTextArea(el, {
        mode: 'yaml',
        lineNumbers: true,
        tabSize: 2,
        indentWithTabs: false,
        lineWrapping: true,
      });
      this._studioTemplateCMs[id] = cm;
      cm.refresh();
      cm.on('change', () => {
        const tpl = this.studioTemplates.find(t => t.id === id);
        if (tpl) tpl.content = cm.getValue();
        this._scheduleStudioPreview();
      });
    },

    // ── Select repo ──
    selectStudioRepo(repo) {
      this.studioRepo = repo;
      this.studioRepoSearch = '';
      this.studioRepoOpen = false;
      this.studioPrURL = '';
      this.studioError = '';
      this.studioTreeFiles = [];
      this.studioTreeTruncated = false;
      // Re-fetch scoped tree for the new repo if preview files exist.
      if (this.studioPreviewFiles.length > 0) this.loadStudioScopedTree();
    },

    // Fetch repo subtree scoped to the common ancestor of studio output paths.
    async loadStudioScopedTree() {
      if (!this.studioPreviewFiles.length || !this.studioRepo) return;
      const { Owner, Name } = this.studioRepo;
      const ancestor = this._commonAncestor(this.studioPreviewFiles.map(f => f.path));
      const params = ancestor
        ? `?path=${encodeURIComponent(ancestor)}&max=500`
        : '?max=500';
      this.studioTreeLoading = true;
      try {
        const resp = await fetch(
          `/api/repos/${Owner}/${Name}/tree${params}`,
          { credentials: 'same-origin', headers: { 'X-Requested-With': 'XMLHttpRequest' } }
        );
        if (!resp.ok) return;
        const data = await resp.json();
        this.studioTreeFiles = data.paths || [];
        this.studioTreeTruncated = data.truncated || false;
      } catch (_) {
        // Non-critical — tree stays empty
      } finally {
        this.studioTreeLoading = false;
      }
    },

    // ── Spec parse (debounced) ──
    _scheduleStudioParse() {
      clearTimeout(this._studioPreviewTimer);
      this._studioPreviewTimer = setTimeout(() => this._parseAndPreviewStudio(), 500);
    },

    _scheduleStudioPreview() {
      clearTimeout(this._studioPreviewTimer);
      this._studioPreviewTimer = setTimeout(() => this._parseAndPreviewStudio(), 500);
    },

    async _parseAndPreviewStudio() {
      if (!this.studioSpec.trim()) return;

      // Client-side pre-check: spec must be a YAML object, not a scalar/array.
      // This catches mid-typing states (e.g. just "kop") without hitting the server.
      if (typeof jsyaml !== 'undefined') {
        try {
          const quick = jsyaml.load(this.studioSpec);
          if (quick === null || typeof quick !== 'object' || Array.isArray(quick)) {
            this.studioSpecError = 'Form spec must be a YAML mapping (object), not a scalar or list.';
            return;
          }
        } catch (yamlErr) {
          this.studioSpecError = String(yamlErr.message || yamlErr);
          return;
        }
      }
      // Build values snapshot from studioValues.
      const payload = {
        formSpec: this.studioSpec,
        templates: this._studioTemplatesMap(),
        values: this.studioValues,
      };
      this.studioPreviewLoading = true;
      this.studioSpecError = '';
      try {
        const resp = await fetch('/api/studio/preview', {
          method: 'POST',
          credentials: 'same-origin',
          headers: {
            'Content-Type': 'application/json',
            'X-Requested-With': 'XMLHttpRequest',
          },
          body: JSON.stringify(payload),
        });
        const data = await resp.json();
        if (resp.status === 422) {
          if (data.errors) {
            this.studioFieldErrors = data.errors;
            this._parseStudioFieldsClientSide();
          } else {
            // Keep previous preview files visible; only surface the spec error.
            this.studioSpecError = data.error || 'Invalid spec';
          }
          return;
        }
        if (!resp.ok) {
          this.studioSpecError = data.error || 'Preview failed';
          return;
        }
        this.studioFieldErrors = {};
        this.studioSpecError = '';
        this.studioPreviewFiles = data.files || [];
        this._parseStudioFieldsClientSide();
        this.studioTreeFiles = [];
        this.studioTreeTruncated = false;
        if (this.studioPreviewFiles.length > 0) this.loadStudioScopedTree();
      } catch (e) {
        this.studioSpecError = 'Preview request failed';
      } finally {
        this.studioPreviewLoading = false;
      }
    },

    // Lightweight client-side extraction of fields from the spec YAML for
    // rendering preview-form inputs. Uses jsyaml if available (loaded from CDN).
    _parseStudioFieldsClientSide() {
      try {
        if (typeof jsyaml === 'undefined') return;
        const parsed = jsyaml.load(this.studioSpec);
        const fields = parsed && parsed.fields ? parsed.fields : [];
        this.studioSpecFields = fields;
        // Seed defaults for new fields.
        for (const f of fields) {
          if (!(f.name in this.studioValues)) {
            let def = f.default !== undefined && f.default !== null ? f.default : '';
            if (def === '$_username') def = this.user?.username || '';
            else if (def === '$_provider') def = this.user?.provider || '';
            this.studioValues[f.name] = def;
          }
        }
      } catch (_) {
        this.studioSpecFields = [];
      }
    },

    onStudioValueChange() {
      this._scheduleStudioPreview();
    },

    // ── Commit ──
    async studioCommit() {
      if (this.studioSubmitting) return;
      if (!this.studioRepo) {
        this.studioError = 'Select a target repository first.';
        return;
      }
      this.studioSubmitting = true;
      this.studioPrURL = '';
      this.studioError = '';
      try {
        const resp = await fetch('/api/studio/commit', {
          method: 'POST',
          credentials: 'same-origin',
          headers: {
            'Content-Type': 'application/json',
            'X-Requested-With': 'XMLHttpRequest',
          },
          body: JSON.stringify({
            owner: this.studioRepo.Owner,
            repo: this.studioRepo.Name,
            formSpec: this.studioSpec,
            templates: this._studioTemplatesMap(),
          }),
        });
        const data = await resp.json();
        if (!resp.ok) {
          this.studioError = data.error || 'Commit failed.';
          return;
        }
        this.studioPrURL = data.pr_url || '';
      } catch (e) {
        this.studioError = 'Commit request failed.';
      } finally {
        this.studioSubmitting = false;
      }
    },

    // ── Download ──
    async studioDownload() {
      if (this.studioDownloading) return;
      this.studioDownloading = true;
      this.studioError = '';
      try {
        const resp = await fetch('/api/studio/download', {
          method: 'POST',
          credentials: 'same-origin',
          headers: {
            'Content-Type': 'application/json',
            'X-Requested-With': 'XMLHttpRequest',
          },
          body: JSON.stringify({
            formSpec: this.studioSpec,
            templates: this._studioTemplatesMap(),
          }),
        });
        if (!resp.ok) {
          const data = await resp.json().catch(() => ({}));
          this.studioError = data.error || 'Download failed.';
          return;
        }
        // Trigger browser file download from the blob.
        const blob = await resp.blob();
        const cd = resp.headers.get('Content-Disposition') || '';
        const nameMatch = cd.match(/filename="?([^"]+)"?/);
        const filename = nameMatch ? nameMatch[1] : 'courtyard-config.zip';
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = filename;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);
      } catch (e) {
        this.studioError = 'Download request failed.';
      } finally {
        this.studioDownloading = false;
      }
    },

    // ── Helper: build templates map from studioTemplates array ──
    _studioTemplatesMap() {
      const map = {};
      for (const t of this.studioTemplates) {
        if (t.name.trim()) map[t.name.trim()] = t.content;
      }
      return map;
    },
  };
}

