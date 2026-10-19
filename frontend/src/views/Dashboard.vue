<template>
  <div class="dashboard-shell">
    <div class="page-header">
      <div>
        <p class="eyebrow">Job hunt workspace</p>
        <h2>Your JobHunter dashboard</h2>
      </div>
      <div class="header-actions">
        <button @click="refreshAll" class="btn btn-secondary" :disabled="loading">Refresh</button>
        <button @click="runHuntNow" class="btn btn-primary" :disabled="runningHunt">
          {{ runningHunt ? 'Running...' : 'Run Hunt Now' }}
        </button>
        <button @click="showPersonalisation = !showPersonalisation" class="btn btn-success">
          Personalisation
        </button>
      </div>
    </div>

    <div class="status-grid">
      <div class="glass-card stat-card">
        <span class="stat-label">Cron status</span>
        <strong>{{ dashboard?.scheduler_status?.running ? 'Running' : 'Unknown' }}</strong>
        <small>{{ dashboard?.scheduler_status?.last_run_message || 'Waiting for first run.' }}</small>
      </div>
      <div class="glass-card stat-card">
        <span class="stat-label">Allowed sources</span>
        <strong>{{ sources.length }}</strong>
        <small>{{ sources.length ? 'Published job boards being checked.' : 'Add public ATS job boards to begin.' }}</small>
      </div>
      <div class="glass-card stat-card">
        <span class="stat-label">Drafts</span>
        <strong>{{ drafts.length }}</strong>
        <small>{{ pendingDrafts }} pending review, {{ matchedJobs.length }} matched jobs.</small>
      </div>
      <div class="glass-card stat-card">
        <span class="stat-label">AICC</span>
        <strong>{{ dashboard?.integrations?.aicc_key ? 'Configured' : 'Missing key' }}</strong>
        <small>Used for resume profiling and draft generation.</small>
      </div>
    </div>

    <section v-if="showPersonalisation" class="glass-card personalisation-panel">
      <div class="section-header">
        <div>
          <h3>Personalisation</h3>
          <p>Edit your profile, upload resume text/docx, and choose company career pages.</p>
        </div>
        <button @click="showPersonalisation = false" class="btn btn-secondary">Close</button>
      </div>

      <div class="two-column">
        <form @submit.prevent="saveProfile" class="form-stack">
          <label for="summary">Profile summary</label>
          <textarea id="summary" v-model="profile.profile_summary" class="input-field textarea" placeholder="Backend engineer with Go, Vue, distributed systems..."></textarea>

          <label for="roles">Target roles</label>
          <input id="roles" v-model="profile.target_roles" class="input-field" placeholder="Backend Engineer, Full Stack Engineer" />

          <label for="skills">Skills</label>
          <input id="skills" v-model="profile.skills" class="input-field" placeholder="Go, Vue, MySQL, Docker, LLM apps" />

          <label for="location">Location preference</label>
          <input id="location" v-model="profile.location" class="input-field" placeholder="Remote, Bengaluru, US timezone..." />

          <button type="submit" class="btn btn-primary">Save Profile</button>
        </form>

        <div class="form-stack">
          <form @submit.prevent="uploadResume" class="upload-box">
            <h4>Resume</h4>
            <p>Upload `.pdf`, `.txt`, `.md`, or `.docx` for extraction. Scanned image resumes still need OCR.</p>
            <input type="file" @change="handleFileUpload" class="input-field" accept=".pdf,.doc,.docx,.txt,.md,.png,.jpg,.jpeg" />
            <button type="submit" class="btn btn-success" :disabled="!selectedFile || uploadingResume">
              {{ uploadingResume ? 'Uploading...' : 'Upload Resume' }}
            </button>
            <small v-if="resumeMessage">{{ resumeMessage }}</small>
          </form>

          <form @submit.prevent="discoverSources" class="upload-box">
            <h4>Find a company job board</h4>
            <p>Paste its public Greenhouse, Lever, or Ashby board URL. General career pages are not a reliable live source.</p>
            <div class="inline-form">
              <input v-model="sourceQuery" class="input-field" placeholder="https://jobs.lever.co/company" />
              <button type="submit" class="btn btn-primary" :disabled="discovering">Find</button>
            </div>
          </form>

          <div class="candidate-list">
            <button
              v-for="candidate in candidates"
              :key="candidate.url"
              @click="addSource(candidate.url, candidate.name)"
              class="source-candidate"
              :disabled="!candidate.provider"
              type="button"
            >
              <span>
                <strong>{{ candidate.name }}</strong>
                <small>{{ candidate.url }}</small>
              </span>
              <em>{{ candidate.status }}</em>
            </button>
          </div>
        </div>
      </div>
    </section>

    <div class="content-grid">
      <section class="glass-card">
        <div class="section-header">
          <div>
            <h3>Allowed sources</h3>
            <p>Career pages this account is tracking.</p>
          </div>
        </div>

        <form @submit.prevent="addManualSource" class="inline-form">
          <input v-model="newSourceUrl" class="input-field" type="url" placeholder="https://jobs.ashbyhq.com/company" required />
          <input v-model="newSourceName" class="input-field" placeholder="Company name" />
          <button type="submit" class="btn btn-primary">Add</button>
        </form>

        <div v-if="sources.length === 0" class="empty-state">
          No sources yet. Add a public Greenhouse, Lever, or Ashby board URL above.
        </div>
        <ul v-else class="source-list">
          <li v-for="source in sources" :key="source.ID">
            <span>
              <strong>{{ source.Name || source.URL }}</strong>
              <a :href="source.URL" target="_blank" rel="noreferrer">{{ source.URL }}</a>
            </span>
            <em>{{ source.Provider || 'job board' }}</em>
          </li>
        </ul>
      </section>

      <section class="glass-card">
        <div class="section-header">
          <div>
            <h3>Profile signal</h3>
            <p>Used to match and draft applications.</p>
          </div>
        </div>
        <div class="profile-summary">
          <p v-if="profile.profile_summary">{{ profile.profile_summary }}</p>
          <p v-else class="empty-state">No profile summary yet. Add it manually or upload a text/docx resume.</p>
          <dl>
            <div><dt>Roles</dt><dd>{{ profile.target_roles || 'Not set' }}</dd></div>
            <div><dt>Skills</dt><dd>{{ profile.skills || 'Not set' }}</dd></div>
            <div><dt>Location</dt><dd>{{ profile.location || 'Not set' }}</dd></div>
          </dl>
        </div>
      </section>
    </div>

    <section class="glass-card drafts-section">
      <div class="section-header">
        <div>
          <h3>Filtered matched jobs</h3>
          <p>Jobs kept after resume/profile matching. Drafts are generated from this filtered set.</p>
        </div>
      </div>

      <div v-if="matchedJobs.length === 0" class="empty-state">
        No matched jobs yet. Upload/extract your resume, add sources, then click Run Hunt Now.
      </div>

      <div class="match-grid">
        <article v-for="job in matchedJobs" :key="job.ID" class="match-card">
          <div>
            <div class="draft-topline">
              <h4>{{ job.JobTitle || `Job ID: ${job.JobID}` }}</h4>
              <span class="score-badge">{{ job.MatchScore }}%</span>
            </div>
            <a v-if="job.ApplyURL || job.JobURL" :href="job.ApplyURL || job.JobURL" target="_blank" rel="noreferrer">Open official listing / application</a>
            <p v-if="job.Location">{{ job.Location }}</p>
            <p>{{ job.MatchReason }}</p>
            <small v-if="job.LastSeenAt">Confirmed on source board: {{ new Date(job.LastSeenAt).toLocaleString() }}</small>
          </div>
          <button
            v-if="job.SourceURL"
            @click="addSource(job.SourceURL, job.Company || job.SourceURL)"
            type="button"
            class="btn btn-secondary"
          >
            Add Source
          </button>
        </article>
      </div>
    </section>

    <section class="glass-card drafts-section">
      <div class="section-header">
        <div>
          <h3>Your job drafts</h3>
          <p>Each draft includes a role-focused resume. Open the employer’s application page, review and upload the resume there, then record that you submitted it.</p>
        </div>
      </div>

      <div v-if="drafts.length === 0" class="empty-state">
        No drafts yet. Add sources and click Run Hunt Now to fetch openings.
      </div>

      <div class="draft-grid">
        <article v-for="draft in drafts" :key="draft.ID" class="draft-card">
          <div>
            <div class="draft-topline">
              <h4>{{ draft.JobTitle || `Job ID: ${draft.JobID}` }}</h4>
              <span :class="['badge', draft.Status]">{{ draft.Status }}</span>
            </div>
            <a v-if="draft.ApplyURL || draft.JobURL" :href="draft.ApplyURL || draft.JobURL" target="_blank" rel="noreferrer">Open official application page</a>
            <p v-if="draft.Location">{{ draft.Location }}</p>
            <small v-if="draft.LastSeenAt">Last confirmed listed: {{ new Date(draft.LastSeenAt).toLocaleString() }}</small>
            <details class="draft-details">
              <summary>Review tailored resume</summary>
              <pre class="resume-preview">{{ draft.TailoredResume || 'Resume draft is not available. Upload a text-based resume and run discovery again.' }}</pre>
              <button v-if="draft.TailoredResume" @click="downloadResume(draft)" type="button" class="btn btn-secondary">Download resume draft (.txt)</button>
            </details>
            <details class="draft-details">
              <summary>Review application message</summary>
              <p class="draft-content">{{ draft.DraftContent }}</p>
            </details>
            <p v-if="!draft.JobActive" class="closed-job">This listing is no longer confirmed active. Verify it on the employer site.</p>
          </div>
          <div class="actions" v-if="draft.Status === 'pending'">
            <button @click="updateStatus(draft.ID, 'applied')" class="btn btn-success" :disabled="!draft.JobActive || !(draft.ApplyURL || draft.JobURL)">I submitted the application</button>
            <button @click="updateStatus(draft.ID, 'rejected')" class="btn btn-danger">Dismiss</button>
          </div>
        </article>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { apiFetch } from '../api';
import { auth, authReady } from '../firebase';

const dashboard = ref<any>(null);
const drafts = ref<any[]>([]);
const sources = ref<any[]>([]);
const matchedJobs = ref<any[]>([]);
const candidates = ref<any[]>([]);
const loading = ref(false);
const runningHunt = ref(false);
const discovering = ref(false);
const uploadingResume = ref(false);
const showPersonalisation = ref(false);
const selectedFile = ref<File | null>(null);
const resumeMessage = ref('');
const sourceQuery = ref('');
const newSourceUrl = ref('');
const newSourceName = ref('');

const profile = reactive({
  profile_summary: '',
  target_roles: '',
  skills: '',
  location: '',
  cron_frequency: 'daily',
});

const pendingDrafts = computed(() => drafts.value.filter(draft => draft.Status === 'pending').length);

const firebaseParams = async () => {
  await authReady;
  const firebaseUID = auth.currentUser?.uid;
  if (!firebaseUID) throw new Error('Please log in again.');
  return new URLSearchParams({ firebase_uid: firebaseUID });
};

const applyProfile = (user: any) => {
  profile.profile_summary = user?.profile_summary || '';
  profile.target_roles = user?.target_roles || '';
  profile.skills = user?.skills || '';
  profile.location = user?.location || '';
  profile.cron_frequency = user?.cron_frequency || 'daily';
};

const refreshAll = async () => {
  loading.value = true;
  try {
    const params = await firebaseParams();
    const [dashboardRes, draftsRes, sourcesRes] = await Promise.all([
      apiFetch(`/api/dashboard?${params.toString()}`),
      apiFetch(`/api/drafts?${params.toString()}`),
      apiFetch(`/api/sources?${params.toString()}`),
    ]);
    if (dashboardRes.ok) {
      dashboard.value = await dashboardRes.json();
      applyProfile(dashboard.value.user);
      matchedJobs.value = dashboard.value.matched_jobs || [];
    }
    if (draftsRes.ok) drafts.value = await draftsRes.json();
    if (sourcesRes.ok) sources.value = await sourcesRes.json();
  } catch (error: any) {
    alert(error.message || 'Failed to refresh dashboard.');
  } finally {
    loading.value = false;
  }
};

const saveProfile = async () => {
  const params = await firebaseParams();
  const firebaseUID = params.get('firebase_uid');
  const res = await apiFetch('/api/users/profile', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ firebase_uid: firebaseUID, ...profile }),
  });
  if (!res.ok) {
    alert('Failed to save profile.');
    return;
  }
  applyProfile(await res.json());
  await refreshAll();
};

const handleFileUpload = (event: Event) => {
  const target = event.target as HTMLInputElement;
  selectedFile.value = target.files?.[0] || null;
};

const uploadResume = async () => {
  if (!selectedFile.value) return;
  uploadingResume.value = true;
  resumeMessage.value = '';
  try {
    const params = await firebaseParams();
    const formData = new FormData();
    formData.append('resume', selectedFile.value);
    formData.append('firebase_uid', params.get('firebase_uid') || '');

    const res = await apiFetch('/api/users/resume', { method: 'POST', body: formData });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(body.error || 'Resume upload failed.');
    resumeMessage.value = body.warning || `Extracted ${body.extracted_chars || 0} characters into your profile signal.`;
    if (body.profile_summary) profile.profile_summary = body.profile_summary;
    await refreshAll();
  } catch (error: any) {
    resumeMessage.value = error.message;
  } finally {
    uploadingResume.value = false;
  }
};

const discoverSources = async () => {
  discovering.value = true;
  try {
    const params = new URLSearchParams();
    if (sourceQuery.value) params.set('query', sourceQuery.value);
    const res = await apiFetch(`/api/sources/discover?${params.toString()}`);
    if (res.ok) candidates.value = await res.json();
  } finally {
    discovering.value = false;
  }
};

const addSource = async (url: string, name: string) => {
  const params = await firebaseParams();
  const res = await apiFetch('/api/sources', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ firebase_uid: params.get('firebase_uid'), url, name }),
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    alert(body.error || 'Failed to add source.');
    return;
  }
  await refreshAll();
};

const addManualSource = async () => {
  await addSource(newSourceUrl.value, newSourceName.value || newSourceUrl.value);
  newSourceUrl.value = '';
  newSourceName.value = '';
};

const runHuntNow = async () => {
  runningHunt.value = true;
  try {
    await apiFetch('/api/scheduler/run', { method: 'POST' });
    setTimeout(refreshAll, 2500);
  } finally {
    setTimeout(() => {
      runningHunt.value = false;
      refreshAll();
    }, 5000);
  }
};

const updateStatus = async (id: number, status: string) => {
  const res = await apiFetch(`/api/drafts/${id}/status`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ status }),
  });
  if (res.ok) await refreshAll();
};

const downloadResume = (draft: any) => {
  const blob = new Blob([draft.TailoredResume], { type: 'text/plain;charset=utf-8' });
  const href = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = href;
  anchor.download = `resume-${String(draft.JobTitle || 'job').toLowerCase().replace(/[^a-z0-9]+/g, '-')}.txt`;
  anchor.click();
  URL.revokeObjectURL(href);
};

onMounted(async () => {
  await refreshAll();
  await discoverSources();
});
</script>

<style scoped>
.dashboard-shell {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}
.page-header,
.section-header,
.header-actions,
.inline-form,
.draft-topline,
.actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
}
.header-actions {
  flex-wrap: wrap;
  justify-content: flex-end;
}
.eyebrow,
.stat-label,
small,
.section-header p,
.upload-box p,
.empty-state,
dt {
  color: var(--text-secondary);
}
.eyebrow,
.stat-label {
  font-size: 0.78rem;
  font-weight: 700;
  text-transform: uppercase;
}
.status-grid,
.content-grid,
.two-column,
.draft-grid {
  display: grid;
  gap: 1rem;
}
.status-grid {
  grid-template-columns: repeat(4, minmax(0, 1fr));
}
.content-grid,
.two-column {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}
.stat-card {
  display: flex;
  min-height: 150px;
  flex-direction: column;
  gap: 0.5rem;
}
.stat-card strong {
  font-size: 1.75rem;
}
.personalisation-panel {
  border-color: rgba(16, 185, 129, 0.35);
}
.form-stack,
.upload-box,
.profile-summary {
  display: flex;
  flex-direction: column;
  gap: 0.85rem;
}
.textarea {
  min-height: 140px;
  resize: vertical;
}
.inline-form .input-field {
  margin-bottom: 0;
}
.btn-secondary {
  background: rgba(148, 163, 184, 0.16);
  color: var(--text-primary);
  border: 1px solid var(--glass-border);
}
.candidate-list,
.source-list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}
.source-candidate,
.source-list li {
  display: flex;
  justify-content: space-between;
  gap: 1rem;
  width: 100%;
  padding: 0.85rem;
  border: 1px solid var(--glass-border);
  border-radius: 0.5rem;
  background: rgba(15, 23, 42, 0.45);
  color: var(--text-primary);
  text-align: left;
}
.source-candidate {
  cursor: pointer;
}
.source-candidate small,
.source-list a {
  display: block;
  color: var(--text-secondary);
  overflow-wrap: anywhere;
}
.source-list {
  padding: 0;
  list-style: none;
}
.source-list a,
.draft-card a {
  color: var(--accent-secondary);
}
.profile-summary dl {
  display: grid;
  gap: 0.75rem;
}
.profile-summary dd {
  margin: 0;
}
.draft-grid,
.match-grid {
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
}
.draft-card,
.match-card {
  display: flex;
  min-height: 260px;
  flex-direction: column;
  justify-content: space-between;
  gap: 1rem;
  padding: 1rem;
  border: 1px solid var(--glass-border);
  border-radius: 0.5rem;
  background: rgba(15, 23, 42, 0.35);
}
.match-card {
  min-height: 210px;
}
.match-card p {
  margin-top: 0.75rem;
  color: var(--text-secondary);
}
.score-badge {
  flex: 0 0 auto;
  padding: 0.3rem 0.7rem;
  border-radius: 0.5rem;
  background: rgba(16, 185, 129, 0.18);
  color: #34d399;
  font-weight: 800;
}
.badge {
  flex: 0 0 auto;
  padding: 0.25rem 0.75rem;
  border-radius: 9999px;
  font-size: 0.75rem;
  font-weight: 700;
  text-transform: uppercase;
}
.badge.pending {
  background: rgba(245, 158, 11, 0.2);
  color: #f59e0b;
}
.badge.applied {
  background: rgba(16, 185, 129, 0.2);
  color: #10b981;
}
.badge.rejected {
  background: rgba(239, 68, 68, 0.2);
  color: #ef4444;
}
.draft-content {
  max-height: 210px;
  margin-top: 1rem;
  overflow-y: auto;
  color: var(--text-secondary);
  font-size: 0.92rem;
  white-space: pre-wrap;
}
.actions button {
  flex: 1;
}
button:disabled {
  cursor: not-allowed;
  opacity: 0.65;
}
@media (max-width: 900px) {
  .page-header,
  .section-header {
    align-items: flex-start;
    flex-direction: column;
  }
  .status-grid,
  .content-grid,
  .two-column {
    grid-template-columns: 1fr;
  }
  .inline-form {
    align-items: stretch;
    flex-direction: column;
  }
}
.resume-preview { white-space: pre-wrap; max-height: 24rem; overflow: auto; padding: 1rem; background: rgba(0,0,0,.2); border-radius: .5rem; }
.draft-details { margin-top: .75rem; }
.draft-details summary { cursor: pointer; font-weight: 600; }
.closed-job { color: #ffbf69; }
</style>
