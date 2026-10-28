<template>
  <div class="dashboard-shell">
    <div class="page-header">
      <div>
        <p class="eyebrow">Job hunt workspace</p>
        <h2>Your JobHunter dashboard</h2>
      </div>
      <div class="header-actions">
        <button @click="exportJobs" class="btn btn-secondary" :disabled="matchedJobs.length === 0">Export job tracker</button>
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
        <span class="stat-label">Legacy drafts</span>
        <strong>{{ drafts.length }}</strong>
        <small>{{ pendingDrafts }} ready for your review and completion.</small>
      </div>
      <div class="glass-card stat-card">
        <span class="stat-label">AICC</span>
        <strong>{{ dashboard?.integrations?.aicc_key ? 'Configured' : 'Missing key' }}</strong>
        <small>Used for resume profile extraction.</small>
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

          <h4>Stored applicant details</h4>
          <p>Check extracted values and fill any blanks. These details are saved for reuse.</p>
          <div class="applicant-fields">
            <label>Application email<input :value="profile.email" class="input-field" readonly autocomplete="email" /></label>
            <label>First name<input v-model="profile.applicant_first_name" class="input-field" autocomplete="given-name" /></label>
            <label>Last name<input v-model="profile.applicant_last_name" class="input-field" autocomplete="family-name" /></label>
            <label>Application phone<input v-model="profile.applicant_phone" class="input-field" autocomplete="tel" /></label>
            <label>Country<input v-model="profile.applicant_country" class="input-field" autocomplete="country-name" /></label>
            <label>City<input v-model="profile.applicant_city" class="input-field" autocomplete="address-level2" /></label>
            <label>State / region<input v-model="profile.applicant_region" class="input-field" autocomplete="address-level1" /></label>
            <label>Postal code<input v-model="profile.applicant_postal_code" class="input-field" autocomplete="postal-code" /></label>
            <label>Street address<input v-model="profile.applicant_address" class="input-field" autocomplete="street-address" /></label>
            <label>LinkedIn URL<input v-model="profile.applicant_linkedin" class="input-field" autocomplete="url" /></label>
            <label>Website / portfolio<input v-model="profile.applicant_website" class="input-field" autocomplete="url" /></label>
            <label>GitHub URL<input v-model="profile.applicant_github" class="input-field" autocomplete="url" /></label>
            <label class="projects-field">Project highlights<textarea v-model="profile.applicant_projects" class="input-field textarea" placeholder="Add projects you want considered in relevant application answers."></textarea><small>JobHunter is prefilled after resume upload. Drafts include it only when relevant or when the application asks for project experience.</small></label>
            <label>Work authorization<input v-model="profile.work_authorization" class="input-field" placeholder="Enter your own answer" /></label>
            <label>Visa sponsorship<input v-model="profile.visa_sponsorship" class="input-field" placeholder="Enter your own answer" /></label>
            <label>Notice period<input v-model="profile.notice_period" class="input-field" /></label>
            <label>Experience<input v-model="profile.applicant_years_experience" class="input-field" placeholder="2.2 years" /></label>
            <label>Salary expectation<input v-model="profile.salary_expectation" class="input-field" /></label>
          </div>

          <button type="submit" class="btn btn-primary">Save Profile</button>
        </form>

        <div class="form-stack">
          <form @submit.prevent="uploadResume" class="upload-box">
            <h4>Resume</h4>
            <p>Upload `.pdf`, `.txt`, `.md`, or `.docx` for extraction. Resume text is sent to the configured AICC/Gemini provider to extract profile fields. Scanned image resumes still need OCR.</p>
            <input type="file" @change="handleFileUpload" class="input-field" accept=".pdf,.docx,.txt,.md" />
            <button type="submit" class="btn btn-success" :disabled="!selectedFile || uploadingResume">
              {{ uploadingResume ? 'Uploading...' : 'Upload Resume' }}
            </button>
            <small v-if="resumeMessage">{{ resumeMessage }}</small>
          </form>

          <div class="upload-box resume-scraper-box">
            <h4>Resume Scraper & AI Parser</h4>
            <p>Paste or edit your resume text directly to scrape, extract, and auto-populate your skills, roles, and profile details without needing to upload a file.</p>
            <textarea
              v-model="resumeScraperText"
              class="input-field textarea"
              style="min-height: 120px; font-family: monospace; font-size: 0.85rem;"
              placeholder="Paste raw resume text here (experience, skills, contact details, projects)..."
            ></textarea>
            <div style="margin-top: 8px; display: flex; gap: 8px; align-items: center; flex-wrap: wrap;">
              <button
                type="button"
                class="btn btn-primary"
                :disabled="!resumeScraperText.trim() || scrapingResume"
                @click="scrapeResume"
              >
                {{ scrapingResume ? 'Scraping & Extracting…' : '⚡ Scrape & Auto-fill Profile' }}
              </button>
              <button
                v-if="profile.resume_text && profile.resume_text !== resumeScraperText"
                type="button"
                class="btn btn-secondary"
                @click="resumeScraperText = profile.resume_text"
              >
                Load Saved Resume Text
              </button>
            </div>
            <small v-if="scrapeMessage" :class="{ 'error-msg': scrapeError, 'success-msg': !scrapeError }">
              {{ scrapeMessage }}
            </small>
          </div>

          <form @submit.prevent="discoverSources" class="upload-box">
            <h4>Find a company job board</h4>
          <p>Paste a public Greenhouse, Lever, or Ashby board URL. General career pages are not a reliable live source.</p>
            <div class="inline-form">
              <input v-model="sourceQuery" class="input-field" placeholder="https://jobs.lever.co/company" />
              <button type="submit" class="btn btn-primary" :disabled="discovering">Find</button>
            </div>
          </form>

          <div class="upload-box">
            <h4>Public job feeds</h4>
          <p>Himalayas filters to India; Jobicy adds APAC remote roles; RemoteOK and Arbeitnow cover worldwide remote positions. Your profile location boosts India-based jobs in matching.</p>
            <div class="inline-form feed-buttons">
              <button type="button" class="btn btn-secondary" @click="addSource('https://himalayas.app/jobs/api/search?country=IN&q=software%20engineer', 'Himalayas · India')">Add Himalayas India feed</button>
              <button type="button" class="btn btn-secondary" @click="addSource('https://jobicy.com/api/v2/remote-jobs?geo=apac&count=200', 'Jobicy · APAC remote')">Add Jobicy APAC remote feed</button>
              <button type="button" class="btn btn-secondary" @click="addSource('https://remoteok.com/api', 'RemoteOK · Worldwide remote')">Add RemoteOK feed</button>
              <button type="button" class="btn btn-secondary" @click="addSource('https://www.arbeitnow.com/api/job-board-api', 'Arbeitnow · Europe/Remote')">Add Arbeitnow feed</button>
            </div>
          </div>

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
          <p>Greenhouse ranks first, followed by Lever and Ashby; aggregator feeds rank lower. Prepare a tailored resume and application note, then review and submit on the employer’s site.</p>
        </div>
      </div>

      <!-- Quick Application Autofill Kit -->
      <div class="autofill-drawer" :class="{ open: showAutofillKit }">
        <div class="autofill-header" @click="showAutofillKit = !showAutofillKit">
          <span>📋 <strong>Application Autofill Kit</strong> (1-click copy answers while filling employer ATS forms)</span>
          <button type="button" class="btn btn-sm btn-ghost">{{ showAutofillKit ? 'Hide ▲' : 'Show Kit ▼' }}</button>
        </div>
        <div v-if="showAutofillKit" class="autofill-chips-grid">
          <div class="autofill-chip" v-if="applicantFullName" @click="copyToClipboard(applicantFullName)">
            <small>Full Name</small>
            <span>{{ applicantFullName }} 📋</span>
          </div>
          <div class="autofill-chip" v-if="profile.email" @click="copyToClipboard(profile.email)">
            <small>Email</small>
            <span>{{ profile.email }} 📋</span>
          </div>
          <div class="autofill-chip" v-if="profile.applicant_phone" @click="copyToClipboard(profile.applicant_phone)">
            <small>Phone</small>
            <span>{{ profile.applicant_phone }} 📋</span>
          </div>
          <div class="autofill-chip" v-if="profile.applicant_linkedin" @click="copyToClipboard(profile.applicant_linkedin)">
            <small>LinkedIn</small>
            <span>{{ profile.applicant_linkedin }} 📋</span>
          </div>
          <div class="autofill-chip" v-if="profile.applicant_github" @click="copyToClipboard(profile.applicant_github)">
            <small>GitHub</small>
            <span>{{ profile.applicant_github }} 📋</span>
          </div>
          <div class="autofill-chip" v-if="profile.applicant_website" @click="copyToClipboard(profile.applicant_website)">
            <small>Portfolio</small>
            <span>{{ profile.applicant_website }} 📋</span>
          </div>
          <div class="autofill-chip" v-if="profile.notice_period" @click="copyToClipboard(profile.notice_period)">
            <small>Notice Period</small>
            <span>{{ profile.notice_period }} 📋</span>
          </div>
          <div class="autofill-chip" v-if="profile.applicant_years_experience" @click="copyToClipboard(profile.applicant_years_experience)">
            <small>Experience</small>
            <span>{{ profile.applicant_years_experience }} 📋</span>
          </div>
          <div class="autofill-chip" v-if="profile.salary_expectation" @click="copyToClipboard(profile.salary_expectation)">
            <small>Salary Expected</small>
            <span>{{ profile.salary_expectation }} 📋</span>
          </div>
          <div class="autofill-chip" v-if="profile.applicant_projects" @click="copyToClipboard(profile.applicant_projects)">
            <small>Projects</small>
            <span>Click to copy highlights 📋</span>
          </div>
          <div class="autofill-chip chip-all" @click="copyFullAutofillKit">
            <small>Full Profile</small>
            <span>Copy All Answers 🚀</span>
          </div>
        </div>
      </div>

      <!-- Live Search & Filter Bar -->
      <div class="jobs-filter-bar">
        <div class="search-input-wrap">
          <input
            v-model="jobSearchQuery"
            type="text"
            class="input-field search-input"
            placeholder="🔍 Search role, company, skill (e.g. Go, Remote, Engineer)..."
          />
        </div>
        <div class="filter-controls-wrap">
          <select v-model="jobStatusFilter" class="input-field select-filter">
            <option value="all">All statuses ({{ matchedJobs.length }})</option>
            <option value="not_applied">Unapplied only ({{ notAppliedCount }})</option>
            <option value="applied">Applied ({{ appliedCount }})</option>
          </select>
          <select v-model="jobScoreFilter" class="input-field select-filter">
            <option :value="0">All Match Scores</option>
            <option :value="50">Score ≥ 50%</option>
            <option :value="70">Score ≥ 70% (Top Fit)</option>
            <option :value="80">Score ≥ 80% (Best Fit)</option>
          </select>
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            :disabled="recalculatingMatches"
            @click="recalculateMatches"
            title="Recalculate match scores based on your technical profile"
          >
            {{ recalculatingMatches ? 'Evaluating…' : '⚡ Re-evaluate Matches' }}
          </button>
        </div>
      </div>

      <div v-if="matchedJobs.length === 0" class="empty-state">
        No matched jobs yet. Upload/extract your resume, add sources, then click Run Hunt Now.
      </div>
      <div v-else-if="filteredMatchedJobs.length === 0" class="empty-state">
        No jobs match your current search/filter. <button class="btn btn-sm btn-ghost" @click="jobSearchQuery = ''; jobStatusFilter = 'all'; jobScoreFilter = 0">Reset filters</button>
      </div>

      <div class="match-grid">
        <article v-for="job in filteredMatchedJobs" :key="job.ID" class="match-card">
          <div>
            <div class="draft-topline">
              <h4>{{ job.JobTitle || `Job ID: ${job.JobID}` }}</h4>
              <span class="score-badge">{{ job.MatchScore }}%</span>
            </div>
            <a v-if="job.ApplyURL || job.JobURL" :href="job.ApplyURL || job.JobURL" target="_blank" rel="noreferrer">Open official listing / application ↗</a>
            <p v-if="job.Company"><strong>{{ job.Company }}</strong></p>
            <small v-if="job.Provider || job.SourceName">Source: {{ job.Provider || job.SourceName }}</small>
            <p v-if="job.Location">{{ job.Location }}</p>
            <p>{{ job.MatchReason }}</p>
            <p v-if="job.RequiredSkills" class="skill-hits"><strong>Skills asked:</strong> {{ job.RequiredSkills }}</p>
            <p v-if="job.MatchedSkills" class="skill-hits"><strong>Your matching skills:</strong> {{ job.MatchedSkills }}</p>
            <small v-if="formatJobDate(job.LastSeenAt)">Confirmed active: {{ formatJobDate(job.LastSeenAt) }}</small>
            <small v-else>Confirmed active listing</small>
          </div>

          <div class="match-actions">
            <span :class="['badge', job.Status === 'applied' ? 'applied' : 'pending']">{{ job.Status === 'applied' ? 'Applied' : 'Not applied' }}</span>
            <button @click="setMatchStatus(job, job.Status === 'applied' ? 'not_applied' : 'applied')" type="button" :class="['btn', job.Status === 'applied' ? 'btn-secondary' : 'btn-success']" :disabled="updatingMatchId === job.ID || (!job.JobActive && job.Status !== 'applied')">
              {{ updatingMatchId === job.ID ? 'Saving...' : job.Status === 'applied' ? 'Undo applied' : 'Mark applied' }}
            </button>
            <button
              v-if="draftForJob(job.JobID)"
              @click="openApplicationPack(draftForJob(job.JobID), job)"
              type="button"
              class="btn btn-success"
            >
              📄 View LaTeX Resume Pack
            </button>
            <button
              v-else
              @click="prepareDraft(job)"
              type="button"
              class="btn btn-primary"
              :disabled="preparingDraftId === job.ID || !job.JobActive"
            >
              {{ preparingDraftId === job.ID ? 'Tailoring LaTeX Resume…' : '⚡ Prepare Application (LaTeX Resume)' }}
            </button>
            <button v-if="job.SourceURL" @click="addSource(job.SourceURL, job.Company || job.SourceURL)" type="button" class="btn btn-secondary">Add Source</button>
          </div>
        </article>
      </div>
    </section>

    <section class="glass-card tracking-section">
      <div class="section-header">
        <div>
          <h3>Application tracker</h3>
          <p>Keep your job list ordered by resume fit. “Applied” is your own tracker status; it is not verified by the employer.</p>
        </div>
        <button @click="exportJobs" class="btn btn-primary" :disabled="matchedJobs.length === 0">Download Excel workbook</button>
      </div>
      <div class="tracking-overview">
        <div class="tracker-chart" :style="chartStyle" role="img" :aria-label="`${appliedCount} applied, ${notAppliedCount} not applied`">
          <div class="tracker-chart-center"><strong>{{ matchedJobs.length }}</strong><span>matched</span></div>
        </div>
        <div class="tracker-legend">
          <p><i class="applied-dot"></i><strong>{{ appliedCount }}</strong> applied</p>
          <p><i class="not-applied-dot"></i><strong>{{ notAppliedCount }}</strong> not applied</p>
          <small>The counts update when you change a job’s status.</small>
        </div>
      </div>
    </section>

    <section class="glass-card drafts-section">
      <div class="section-header">
        <div>
          <h3>Your job drafts</h3>
          <p>Review and edit each resume and application note, then submit it through the employer’s official application page.</p>
        </div>
        <div style="display: flex; gap: 8px;">
          <button
            @click="cleanupIrrelevantDrafts"
            class="btn btn-sm btn-danger"
            :disabled="cleaningDrafts"
            title="Purge drafts for non-technical roles or jobs that no longer match"
          >
            {{ cleaningDrafts ? 'Purging…' : '🗑️ Purge Irrelevant Drafts' }}
          </button>
        </div>
      </div>

      <div v-if="drafts.length === 0" class="empty-state">
        No application packs yet. Choose “Prepare application” on a matched job above.
      </div>

      <div class="draft-grid">
        <article v-for="draft in drafts" :key="draft.ID" class="draft-card">
          <div>
            <div class="draft-topline">
              <h4>{{ draft.JobTitle || `Job ID: ${draft.JobID}` }}</h4>
              <span :class="['badge', draft.Status]">{{ draft.Status }}</span>
            </div>
            <a v-if="draft.ApplyURL || draft.JobURL" :href="draft.ApplyURL || draft.JobURL" target="_blank" rel="noreferrer">Open official application page ↗</a>
            <p v-if="draft.Company"><strong>{{ draft.Company }}</strong><span v-if="draft.SourceName"> · via {{ draft.SourceName }}</span></p>
            <small v-if="draft.Provider">Source: {{ draft.Provider }}</small>
            <p v-if="draft.Location">{{ draft.Location }}</p>
            <small v-if="formatJobDate(draft.LastSeenAt)">Last confirmed listed: {{ formatJobDate(draft.LastSeenAt) }}</small>
            <small v-else>Confirmed active listing</small>

            <button
              type="button"
              class="btn btn-primary btn-block"
              style="margin-top: 12px; width: 100%; display: flex; align-items: center; justify-content: center; gap: 6px;"
              @click="openApplicationPack(draft)"
            >
              📄 View LaTeX Resume & Application Pack
            </button>
            <p v-if="!draft.JobActive" class="closed-job" style="margin-top: 8px;">This listing is no longer confirmed active. Verify it on the employer site.</p>
          </div>
          <div class="actions" style="margin-top: 10px;">
            <button v-if="draft.Status === 'pending'" @click="updateStatus(draft.ID, 'applied')" class="btn btn-success" :disabled="!draft.JobActive || !(draft.ApplyURL || draft.JobURL)">I submitted</button>
            <button @click="deleteDraft(draft.ID)" class="btn btn-danger">Delete</button>
          </div>
        </article>
      </div>
    </section>

    <!-- Global Application Pack & Tailored Resume Modal -->
    <div v-if="activeDraftModal" class="modal-backdrop" @click.self="closeApplicationPack">
      <div class="modal-card">
        <div class="modal-header">
          <div>
            <div style="display: flex; align-items: center; gap: 8px; margin-bottom: 4px;">
              <span class="score-badge" v-if="activeJobModal?.MatchScore">{{ activeJobModal.MatchScore }}% Match</span>
              <span class="badge applied" v-if="activeDraftModal.Status === 'applied'">Applied</span>
            </div>
            <h3>{{ activeDraftModal.JobTitle || activeJobModal?.JobTitle || 'Job Application Pack' }}</h3>
            <p v-if="activeDraftModal.Company || activeJobModal?.Company" class="modal-subheading">
              <strong>{{ activeDraftModal.Company || activeJobModal?.Company }}</strong>
              <span v-if="activeDraftModal.Location || activeJobModal?.Location"> · {{ activeDraftModal.Location || activeJobModal?.Location }}</span>
            </p>
          </div>
          <div style="display: flex; gap: 8px; align-items: center;">
            <a
              v-if="activeDraftModal.ApplyURL || activeDraftModal.JobURL || activeJobModal?.ApplyURL || activeJobModal?.JobURL"
              :href="activeDraftModal.ApplyURL || activeDraftModal.JobURL || activeJobModal?.ApplyURL || activeJobModal?.JobURL"
              target="_blank"
              rel="noreferrer"
              class="btn btn-success btn-sm"
              style="text-decoration: none;"
            >
              🚀 Official Career Page ↗
            </a>
            <button type="button" class="btn btn-secondary btn-sm" @click="closeApplicationPack">✕ Close</button>
          </div>
        </div>

        <div class="modal-nav">
          <button
            type="button"
            :class="['tab-btn', modalActiveTab === 'resume' ? 'active' : '']"
            @click="modalActiveTab = 'resume'"
          >
            📄 Tailored LaTeX Resume (.tex)
          </button>
          <button
            type="button"
            :class="['tab-btn', modalActiveTab === 'message' ? 'active' : '']"
            @click="modalActiveTab = 'message'"
          >
            ✉️ Application Note / Cover Letter
          </button>
          <button
            type="button"
            :class="['tab-btn', modalActiveTab === 'autofill' ? 'active' : '']"
            @click="modalActiveTab = 'autofill'"
          >
            📋 1-Click Form Autofill Answers
          </button>
        </div>

        <div class="modal-body">
          <!-- Tab 1: Tailored LaTeX Resume -->
          <div v-if="modalActiveTab === 'resume'" class="modal-tab-content">
            <div class="tab-toolbar" style="background: rgba(99, 102, 241, 0.08); border-radius: 8px; padding: 10px 14px; margin-bottom: 12px; display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: 10px;">
              <div>
                <strong>📄 Factual LaTeX Resume Code</strong>
                <span style="font-size: 0.85rem; color: var(--text-secondary); margin-left: 8px;">100% factual experience styled like your approved resume with JD skills prioritized. Ready to copy or compile.</span>
              </div>
              <div class="toolbar-actions" style="display: flex; gap: 6px; flex-wrap: wrap;">
                <button type="button" class="btn btn-sm btn-primary" @click="copyToClipboard(activeDraftModal.TailoredResume)">📋 Copy LaTeX Code</button>
                <button type="button" class="btn btn-sm btn-success" @click="openInCompiler('latex.to')">🌐 Open in LaTeX.to ↗</button>
                <button type="button" class="btn btn-sm btn-secondary" @click="openInCompiler('overleaf')">🌐 Overleaf ↗</button>
                <button type="button" class="btn btn-sm btn-secondary" @click="openInCompiler('latexbase')">🌐 LaTeXBase ↗</button>
                <button type="button" class="btn btn-sm btn-secondary" @click="downloadResume(activeDraftModal)">💾 Download .tex</button>
                <button type="button" class="btn btn-sm btn-secondary" :disabled="regeneratingDraft" @click="regenerateCurrentDraft">{{ regeneratingDraft ? 'Regenerating…' : '⚡ Regenerate LaTeX' }}</button>
                <button type="button" class="btn btn-sm btn-secondary" @click="saveDraft(activeDraftModal)">💾 Save</button>
              </div>
            </div>
            <textarea
              v-model="activeDraftModal.TailoredResume"
              class="input-field textarea full-editor"
              style="font-family: 'JetBrains Mono', 'Fira Code', 'Courier New', monospace; font-size: 0.85rem; line-height: 1.5; min-height: 440px; white-space: pre;"
              spellcheck="false"
              placeholder="\documentclass..."
            ></textarea>
          </div>

          <!-- Tab 2: Application Note -->
          <div v-if="modalActiveTab === 'message'" class="modal-tab-content">
            <div class="tab-toolbar">
              <span>Personalized application message highlighting your verified experience.</span>
              <div class="toolbar-actions">
                <button type="button" class="btn btn-sm btn-primary" @click="copyToClipboard(activeDraftModal.DraftContent)">📋 Copy Message</button>
                <button type="button" class="btn btn-sm btn-secondary" @click="saveDraft(activeDraftModal)">💾 Save Edits</button>
              </div>
            </div>
            <textarea
              v-model="activeDraftModal.DraftContent"
              class="input-field textarea full-editor"
              placeholder="Application message..."
            ></textarea>
          </div>

          <!-- Tab 3: Form Autofill Answers -->
          <div v-if="modalActiveTab === 'autofill'" class="modal-tab-content">
            <div class="tab-toolbar">
              <span>Click any field to copy answers directly into the employer's application form.</span>
              <button type="button" class="btn btn-sm btn-primary" @click="copyFullAutofillKit">🚀 Copy All Answers</button>
            </div>
            <div class="autofill-grid-modal">
              <div class="modal-field-card" @click="copyToClipboard(applicantFullName)">
                <label>Full Name</label>
                <div>{{ applicantFullName || 'Not set' }} <span>📋 Copy</span></div>
              </div>
              <div class="modal-field-card" @click="copyToClipboard(profile.email)">
                <label>Email Address</label>
                <div>{{ profile.email || 'Not set' }} <span>📋 Copy</span></div>
              </div>
              <div class="modal-field-card" @click="copyToClipboard(profile.applicant_phone)">
                <label>Phone Number</label>
                <div>{{ profile.applicant_phone || 'Not set' }} <span>📋 Copy</span></div>
              </div>
              <div class="modal-field-card" @click="copyToClipboard(profile.location)">
                <label>Current Location</label>
                <div>{{ profile.location || 'Not set' }} <span>📋 Copy</span></div>
              </div>
              <div class="modal-field-card" @click="copyToClipboard(profile.applicant_years_experience)">
                <label>Total Experience</label>
                <div>{{ profile.applicant_years_experience || 'Not set' }} <span>📋 Copy</span></div>
              </div>
              <div class="modal-field-card" @click="copyToClipboard(profile.notice_period)">
                <label>Notice Period</label>
                <div>{{ profile.notice_period || 'Not set' }} <span>📋 Copy</span></div>
              </div>
              <div class="modal-field-card" @click="copyToClipboard(profile.salary_expectation)">
                <label>Salary Expectation</label>
                <div>{{ profile.salary_expectation || 'Not set' }} <span>📋 Copy</span></div>
              </div>
              <div class="modal-field-card" @click="copyToClipboard(profile.applicant_linkedin)">
                <label>LinkedIn</label>
                <div>{{ profile.applicant_linkedin || 'Not set' }} <span>📋 Copy</span></div>
              </div>
              <div class="modal-field-card" @click="copyToClipboard(profile.applicant_github)">
                <label>GitHub</label>
                <div>{{ profile.applicant_github || 'Not set' }} <span>📋 Copy</span></div>
              </div>
              <div class="modal-field-card" @click="copyToClipboard(profile.applicant_website)">
                <label>Portfolio / Website</label>
                <div>{{ profile.applicant_website || 'Not set' }} <span>📋 Copy</span></div>
              </div>
              <div class="modal-field-card span-all" @click="copyToClipboard(profile.applicant_projects)">
                <label>Featured Project Highlights</label>
                <div>{{ profile.applicant_projects || 'Not set' }} <span>📋 Copy</span></div>
              </div>
            </div>
          </div>
        </div>

        <div class="modal-footer">
          <a
            v-if="activeDraftModal.ApplyURL || activeDraftModal.JobURL || activeJobModal?.ApplyURL || activeJobModal?.JobURL"
            :href="activeDraftModal.ApplyURL || activeDraftModal.JobURL || activeJobModal?.ApplyURL || activeJobModal?.JobURL"
            target="_blank"
            rel="noreferrer"
            class="btn btn-success"
            style="font-weight: 700; padding: 10px 18px;"
          >
            🚀 Open Official Career Page / Apply ↗
          </a>
          <button
            type="button"
            class="btn btn-primary"
            @click="copyToClipboard(activeDraftModal.TailoredResume)"
          >
            📋 Copy LaTeX Code
          </button>
          <button
            type="button"
            class="btn btn-success"
            @click="openInCompiler('latex.to')"
          >
            🌐 Open in LaTeX.to ↗
          </button>
          <button
            type="button"
            class="btn btn-secondary"
            @click="downloadResume(activeDraftModal)"
          >
            💾 Download .tex
          </button>
          <button
            type="button"
            class="btn btn-primary"
            v-if="activeDraftModal.Status === 'pending'"
            @click="markDraftApplied(activeDraftModal); closeApplicationPack();"
          >
            ✅ I Submitted This Application
          </button>
          <button
            type="button"
            class="btn btn-danger"
            @click="deleteDraft(activeDraftModal.ID)"
          >
            🗑️ Delete
          </button>
          <button type="button" class="btn btn-secondary" @click="closeApplicationPack">Close</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { apiFetch } from '../api';
import { auth, authReady } from '../firebase';
import { deleteFirebaseResumeUpload, deletePreviousFirebaseResume, uploadResumeToFirebase } from '../resumeStorage';

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
const updatingMatchId = ref<number | null>(null);
const preparingDraftId = ref<number | null>(null);
const showAutofillKit = ref(false);
const jobSearchQuery = ref('');
const jobStatusFilter = ref<'all' | 'not_applied' | 'applied'>('all');
const jobScoreFilter = ref<number>(0);

const activeDraftModal = ref<any>(null);
const activeJobModal = ref<any>(null);
const modalActiveTab = ref<'resume' | 'message' | 'autofill'>('resume');
const cleaningDrafts = ref(false);
const recalculatingMatches = ref(false);

const profile = reactive({
  email: '',
  resume_url: '',
  resume_text: '',
  profile_summary: '',
  target_roles: '',
  skills: '',
  location: '',
  cron_frequency: 'daily',
  applicant_first_name: '',
  applicant_last_name: '',
  applicant_phone: '',
  applicant_address: '',
  applicant_city: '',
  applicant_region: '',
  applicant_postal_code: '',
  applicant_country: '',
  applicant_linkedin: '',
  applicant_website: '',
  applicant_github: '',
  applicant_projects: '',
  work_authorization: '',
  visa_sponsorship: '',
  notice_period: '',
  applicant_years_experience: '',
  salary_expectation: '',
});

const resumeScraperText = ref('');
const scrapingResume = ref(false);
const scrapeMessage = ref('');
const scrapeError = ref(false);

const pendingDrafts = computed(() => drafts.value.filter(draft => draft.Status === 'pending').length);
const appliedCount = computed(() => matchedJobs.value.filter(job => job.Status === 'applied').length);
const notAppliedCount = computed(() => matchedJobs.value.length - appliedCount.value);
const chartStyle = computed(() => {
  const applied = matchedJobs.value.length ? appliedCount.value / matchedJobs.value.length * 100 : 0;
  return { background: `conic-gradient(#6ee7b7 0 ${applied}%, #c4b5fd ${applied}% 100%)` };
});

const applicantFullName = computed(() => {
  return `${profile.applicant_first_name || ''} ${profile.applicant_last_name || ''}`.trim();
});

const filteredMatchedJobs = computed(() => {
  let list = matchedJobs.value;

  if (jobStatusFilter.value === 'applied') {
    list = list.filter((j: any) => j.Status === 'applied');
  } else if (jobStatusFilter.value === 'not_applied') {
    list = list.filter((j: any) => j.Status !== 'applied');
  }

  if (jobScoreFilter.value > 0) {
    list = list.filter((j: any) => (j.MatchScore || 0) >= jobScoreFilter.value);
  }

  const q = jobSearchQuery.value.trim().toLowerCase();
  if (q) {
    list = list.filter((j: any) => {
      const title = (j.JobTitle || '').toLowerCase();
      const comp = (j.Company || '').toLowerCase();
      const loc = (j.Location || '').toLowerCase();
      const skills = (String(j.MatchedSkills || '') + ' ' + String(j.RequiredSkills || '')).toLowerCase();
      return title.includes(q) || comp.includes(q) || loc.includes(q) || skills.includes(q);
    });
  }

  return list;
});

const copyFullAutofillKit = async () => {
  const parts = [
    applicantFullName.value ? `Name: ${applicantFullName.value}` : '',
    profile.email ? `Email: ${profile.email}` : '',
    profile.applicant_phone ? `Phone: ${profile.applicant_phone}` : '',
    profile.applicant_linkedin ? `LinkedIn: ${profile.applicant_linkedin}` : '',
    profile.applicant_github ? `GitHub: ${profile.applicant_github}` : '',
    profile.applicant_website ? `Portfolio: ${profile.applicant_website}` : '',
    profile.notice_period ? `Notice Period: ${profile.notice_period}` : '',
    profile.applicant_years_experience ? `Experience: ${profile.applicant_years_experience}` : '',
    profile.salary_expectation ? `Expected Compensation: ${profile.salary_expectation}` : '',
    profile.applicant_projects ? `Project Highlights:\n${profile.applicant_projects}` : '',
  ].filter(Boolean).join('\n');
  await copyToClipboard(parts);
};

const firebaseParams = async () => {
  await authReady;
  const firebaseUID = auth.currentUser?.uid;
  if (!firebaseUID) throw new Error('Please log in again.');
  return new URLSearchParams({ firebase_uid: firebaseUID });
};

const applyProfile = (user: any) => {
  profile.email = user?.email || '';
  profile.resume_url = user?.resume_url || '';
  profile.resume_text = user?.resume_text || '';
  if (!resumeScraperText.value && user?.resume_text) {
    resumeScraperText.value = user.resume_text;
  }
  profile.profile_summary = user?.profile_summary || '';
  profile.target_roles = user?.target_roles || '';
  profile.skills = user?.skills || '';
  profile.location = user?.location || '';
  profile.cron_frequency = user?.cron_frequency || 'daily';
  profile.applicant_first_name = user?.applicant_first_name || '';
  profile.applicant_last_name = user?.applicant_last_name || '';
  profile.applicant_phone = user?.applicant_phone || '';
  profile.applicant_address = user?.applicant_address || '';
  profile.applicant_city = user?.applicant_city || '';
  profile.applicant_region = user?.applicant_region || '';
  profile.applicant_postal_code = user?.applicant_postal_code || '';
  profile.applicant_country = user?.applicant_country || '';
  profile.applicant_linkedin = user?.applicant_linkedin || '';
  profile.applicant_website = user?.applicant_website || '';
  profile.applicant_github = user?.applicant_github || '';
  profile.applicant_projects = user?.applicant_projects || '';
  profile.work_authorization = user?.work_authorization || '';
  profile.visa_sponsorship = user?.visa_sponsorship || '';
  profile.notice_period = user?.notice_period || '';
  profile.applicant_years_experience = user?.applicant_years_experience || '';
  profile.salary_expectation = user?.salary_expectation || '';
};

const refreshAll = async () => {
  loading.value = true;
  try {
    // Ensure user record is registered/linked in backend on every load
    if (auth.currentUser?.uid) {
      apiFetch('/api/users/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: auth.currentUser.email || '', firebase_uid: auth.currentUser.uid }),
      }).catch(() => {});
    }

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

const scrapeResume = async () => {
  if (!resumeScraperText.value.trim()) return;
  scrapingResume.value = true;
  scrapeMessage.value = '';
  scrapeError.value = false;
  try {
    const res = await apiFetch('/api/users/resume/scrape', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ resume_text: resumeScraperText.value }),
    });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(body.error || 'Failed to scrape resume');
    if (body.applicant_profile) applyProfile(body.applicant_profile);
    if (body.profile_summary) profile.profile_summary = body.profile_summary;
    scrapeMessage.value = body.warning || '✅ Resume successfully scraped! Skills, roles, and profile details auto-filled.';
    await refreshAll();
  } catch (err: any) {
    scrapeError.value = true;
    scrapeMessage.value = err.message || 'Scrape failed.';
  } finally {
    scrapingResume.value = false;
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
  let uploadedStoragePath = '';
  let profileSaved = false;
  try {
    const previousResumeURL = profile.resume_url;
    uploadedStoragePath = await uploadResumeToFirebase(selectedFile.value);
    const params = await firebaseParams();
    const formData = new FormData();
    formData.append('resume', selectedFile.value);
    formData.append('firebase_uid', params.get('firebase_uid') || '');
    formData.append('storage_path', uploadedStoragePath);

    const res = await apiFetch('/api/users/resume', { method: 'POST', body: formData });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(body.error || 'Resume upload failed.');
    profileSaved = true;
    profile.resume_url = `firebase-storage://${uploadedStoragePath}`;
    try { await deletePreviousFirebaseResume(previousResumeURL); } catch { /* Keep the newly saved resume if old-file cleanup fails. */ }
    resumeMessage.value = body.warning || `Resume stored securely in Firebase Storage; extracted ${body.extracted_chars || 0} characters into your profile.`;
    if (body.applicant_profile) applyProfile(body.applicant_profile);
    if (body.profile_summary) profile.profile_summary = body.profile_summary;
    await refreshAll();
  } catch (error: any) {
    if (uploadedStoragePath && !profileSaved) {
      await deleteFirebaseResumeUpload(uploadedStoragePath).catch(() => undefined);
    }
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

const saveDraft = async (draft: any) => {
  const res = await apiFetch(`/api/drafts/${draft.ID}`, { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ DraftContent: draft.DraftContent, TailoredResume: draft.TailoredResume }) });
  if (!res.ok) { alert('Failed to save draft edits.'); return; }
  alert('Draft saved. Review it before applying on the employer site.');
};

const updateStatus = async (id: number, status: string) => {
  const res = await apiFetch(`/api/drafts/${id}/status`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ status }),
  });
  if (res.ok) await refreshAll();
};

const formatJobDate = (dateStr: string | null | undefined): string => {
  if (!dateStr) return '';
  const d = new Date(dateStr);
  if (isNaN(d.getTime()) || d.getFullYear() < 2000) return '';
  return d.toLocaleString();
};

const openApplicationPack = (draft: any, job?: any) => {
  activeDraftModal.value = draft;
  activeJobModal.value = job || matchedJobs.value.find((j: any) => j.JobID === draft.JobID) || null;
  modalActiveTab.value = 'resume';
};

const closeApplicationPack = () => {
  activeDraftModal.value = null;
  activeJobModal.value = null;
};

const cleanupIrrelevantDrafts = async () => {
  if (!confirm('Purge all non-technical and irrelevant drafts from your dashboard?')) return;
  cleaningDrafts.value = true;
  try {
    const res = await apiFetch('/api/drafts/cleanup-irrelevant', { method: 'POST' });
    const data = await res.json().catch(() => ({}));
    alert(data.message || 'Irrelevant drafts cleaned up.');
    await refreshAll();
  } catch (err: any) {
    alert(err.message || 'Failed to cleanup drafts.');
  } finally {
    cleaningDrafts.value = false;
  }
};

const recalculateMatches = async () => {
  recalculatingMatches.value = true;
  try {
    const res = await apiFetch('/api/jobs/recalculate-matches', { method: 'POST' });
    const data = await res.json().catch(() => ({}));
    alert(data.message || 'Matches re-evaluated based on your profile.');
    await refreshAll();
  } catch (err: any) {
    alert(err.message || 'Failed to re-evaluate matches.');
  } finally {
    recalculatingMatches.value = false;
  }
};

const deleteDraft = async (draftId: number) => {
  if (!confirm('Permanently delete this draft?')) return;
  try {
    const res = await apiFetch(`/api/drafts/${draftId}`, { method: 'DELETE' });
    if (res.ok) {
      if (activeDraftModal.value?.ID === draftId) {
        closeApplicationPack();
      }
      await refreshAll();
    }
  } catch (err: any) {
    alert(err.message || 'Failed to delete draft.');
  }
};

const prepareDraft = async (job: any) => {
  preparingDraftId.value = job.ID;
  try {
    const res = await apiFetch(`/api/jobs/matches/${job.ID}/draft`, { method: 'POST' });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(body.error || 'Could not prepare application materials.');
    await refreshAll();
    const createdDraft = body.draft || draftForJob(job.JobID) || drafts.value[0];
    if (createdDraft) {
      openApplicationPack(createdDraft, job);
    }
  } catch (error: any) {
    alert(error.message || 'Could not prepare application materials.');
  } finally {
    preparingDraftId.value = null;
  }
};

const regeneratingDraft = ref(false);
const regenerateCurrentDraft = async () => {
  if (!activeDraftModal.value) return;
  regeneratingDraft.value = true;
  try {
    const jobID = activeDraftModal.value.JobID;
    const res = await apiFetch(`/api/jobs/matches/${jobID}/draft`, { method: 'POST' });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(body.error || 'Could not regenerate LaTeX resume.');
    await refreshAll();
    const updated = body.draft || draftForJob(jobID);
    if (updated) {
      activeDraftModal.value = updated;
    }
    alert('LaTeX Resume regenerated successfully!');
  } catch (err: any) {
    alert(err.message || 'Could not regenerate LaTeX resume.');
  } finally {
    regeneratingDraft.value = false;
  }
};

const openInCompiler = async (compiler: 'latex.to' | 'latexbase' | 'overleaf') => {
  const content = activeDraftModal.value?.TailoredResume || '';
  if (content) {
    try {
      await navigator.clipboard.writeText(content);
    } catch { /* clipboard fallback */ }
  }
  if (compiler === 'latex.to') {
    window.open('https://latex.to', '_blank');
    alert('LaTeX code copied to your clipboard! In LaTeX.to, select all, paste (Cmd+V / Ctrl+V), and click Preview to render your PDF.');
  } else if (compiler === 'latexbase') {
    window.open('https://latexbase.com', '_blank');
  } else {
    window.open('https://www.overleaf.com/project', '_blank');
  }
};

const setMatchStatus = async (job: any, status: string) => {
  updatingMatchId.value = job.ID;
  try {
    const res = await apiFetch(`/api/jobs/matches/${job.ID}/status`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ status }),
    });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(body.error || 'Could not update application status.');
    job.Status = status;
  } catch (error: any) {
    alert(error.message || 'Could not update application status.');
  } finally {
    updatingMatchId.value = null;
  }
};

// ── Draft panel helpers ──────────────────────────────────────────────────────

/** Return the pre-prepared draft (if any) for a given job ID. */
const draftForJob = (jobId: number) => drafts.value.find((d: any) => d.JobID === jobId) ?? null;

/** Copy text to clipboard with a transient user-facing confirmation. */
const copyToClipboard = async (text: string) => {
  try {
    await navigator.clipboard.writeText(text);
    alert('Copied to clipboard ✓');
  } catch {
    alert('Could not access clipboard — please select and copy manually.');
  }
};

/**
 * Mark a draft as "applied" from the inline panel.
 * Mirrors the existing updateStatus() call used in the drafts section.
 */
const markDraftApplied = async (draft: any) => {
  if (!draft?.ID) return;
  const res = await apiFetch(`/api/drafts/${draft.ID}/status`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ status: 'applied' }),
  });
  if (res.ok) {
    draft.Status = 'applied';
    await refreshAll();
  } else {
    const body = await res.json().catch(() => ({}));
    alert(body.error || 'Could not mark as applied.');
  }
};

// ────────────────────────────────────────────────────────────────────────────

const exportJobs = async () => {
  try {
    const res = await apiFetch('/api/jobs/export.xlsx');
    if (!res.ok) throw new Error('Could not export job tracker.');
    const blob = await res.blob();
    const href = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = href;
    anchor.download = 'jobhunter-tracker.xlsx';
    anchor.click();
    URL.revokeObjectURL(href);
  } catch (error: any) {
    alert(error.message || 'Could not export job tracker.');
  }
};

const downloadResume = (draft: any) => {
  const content = draft?.TailoredResume || '';
  if (!content) {
    alert('No resume content found to download.');
    return;
  }
  const isLatex = content.includes('\\documentclass');
  const ext = isLatex ? 'tex' : 'txt';
  const mime = isLatex ? 'text/x-tex;charset=utf-8' : 'text/plain;charset=utf-8';
  const blob = new Blob([content], { type: mime });
  const href = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = href;
  const company = String(draft.Company || activeJobModal.value?.Company || 'company').toLowerCase().replace(/[^a-z0-9]+/g, '-');
  anchor.download = `Gaurav_Chaudhary_Resume_${company}.${ext}`;
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
.feed-buttons {
  flex-wrap: wrap;
  gap: 0.6rem;
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
.match-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: .6rem;
  margin-top: 1rem;
}
.tracking-overview {
  display: flex;
  align-items: center;
  gap: 2rem;
  padding: 1rem 0;
}
.tracker-chart {
  width: 174px;
  height: 174px;
  display: grid;
  place-items: center;
  border-radius: 50%;
}
.tracker-chart-center {
  width: 112px;
  height: 112px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: #171827;
}
.tracker-chart-center strong { font-size: 1.7rem; }
.tracker-chart-center span, .tracker-legend small { color: var(--text-secondary); }
.tracker-legend p { display: flex; align-items: center; gap: .55rem; margin: .55rem 0; }
.tracker-legend i { width: 12px; height: 12px; border-radius: 50%; }
.applied-dot { background: #6ee7b7; }
.not-applied-dot { background: #c4b5fd; }
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
  .tracking-overview { align-items: flex-start; flex-direction: column; }
}
.resume-preview { white-space: pre-wrap; max-height: 24rem; overflow: auto; padding: 1rem; background: rgba(0,0,0,.2); border-radius: .5rem; }
.draft-details { margin-top: .75rem; }
.draft-details summary { cursor: pointer; font-weight: 600; }
.closed-job { color: #ffbf69; }
.applicant-fields { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: .75rem; }
.applicant-fields label { display: grid; gap: .35rem; }
.applicant-fields .projects-field { grid-column: 1 / -1; }
@media (max-width: 700px) { .applicant-fields { grid-template-columns: 1fr; } }

.inline-draft-panel {
  margin-top: 1rem;
  padding: 0.75rem;
  border-radius: 0.5rem;
  background: rgba(15, 23, 42, 0.6);
  border: 1px solid rgba(110, 231, 183, 0.3);
}
.inline-draft-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}
.draft-ready-badge {
  font-size: 0.8rem;
  font-weight: 600;
  color: #34d399;
}
.btn-sm {
  padding: 0.3rem 0.6rem;
  font-size: 0.8rem;
}
.btn-ghost {
  background: transparent;
  color: var(--accent-secondary);
  border: 1px solid rgba(255, 255, 255, 0.15);
}
.btn-ghost:hover {
  background: rgba(255, 255, 255, 0.08);
}
.draft-copy-row,
.draft-submit-row {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin-top: 0.5rem;
}
.draft-editor {
  margin-top: 0.5rem;
  min-height: 100px;
  font-size: 0.88rem;
}
.draft-status-note {
  margin-top: 0.5rem;
  font-size: 0.85rem;
  color: var(--text-secondary);
}

.autofill-drawer {
  margin-bottom: 1.25rem;
  padding: 0.85rem;
  border-radius: 0.6rem;
  background: rgba(30, 41, 59, 0.45);
  border: 1px solid rgba(148, 163, 184, 0.2);
}
.autofill-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  cursor: pointer;
  user-select: none;
}
.autofill-chips-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 0.6rem;
  margin-top: 0.85rem;
  padding-top: 0.75rem;
  border-top: 1px dashed rgba(255, 255, 255, 0.1);
}
.autofill-chip {
  display: flex;
  flex-direction: column;
  padding: 0.45rem 0.75rem;
  border-radius: 0.45rem;
  background: rgba(15, 23, 42, 0.6);
  border: 1px solid rgba(255, 255, 255, 0.12);
  cursor: pointer;
  transition: all 0.15s ease-in-out;
}
.autofill-chip:hover {
  background: rgba(79, 70, 229, 0.25);
  border-color: #818cf8;
}
.autofill-chip small {
  font-size: 0.7rem;
  text-transform: uppercase;
  color: var(--text-secondary);
  font-weight: 700;
}
.autofill-chip span {
  font-size: 0.86rem;
  color: var(--text-primary);
  font-weight: 500;
}
.autofill-chip.chip-all {
  background: rgba(16, 185, 129, 0.2);
  border-color: #34d399;
}
.autofill-chip.chip-all span {
  color: #34d399;
  font-weight: 700;
}

.jobs-filter-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem;
  margin-bottom: 1.25rem;
  align-items: center;
}
.search-input-wrap {
  flex: 1 1 300px;
}
.search-input {
  margin-bottom: 0;
  width: 100%;
}
.filter-controls-wrap {
  display: flex;
  gap: 0.6rem;
  flex-wrap: wrap;
}
.select-filter {
  margin-bottom: 0;
  padding: 0.55rem 0.85rem;
  background: rgba(15, 23, 42, 0.7);
}

.modal-backdrop {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.78);
  backdrop-filter: blur(8px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 9999;
  padding: 1rem;
}
.modal-card {
  background: #111827;
  border: 1px solid rgba(255, 255, 255, 0.18);
  border-radius: 1rem;
  width: 100%;
  max-width: 900px;
  max-height: 92vh;
  display: flex;
  flex-direction: column;
  box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.75);
  animation: modalFadeIn 0.2s ease-out;
}
@keyframes modalFadeIn {
  from { opacity: 0; transform: scale(0.97); }
  to { opacity: 1; transform: scale(1); }
}
.modal-header {
  padding: 1.25rem 1.5rem;
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(31, 41, 55, 0.5);
}
.modal-header h3 {
  margin-top: 0.35rem;
  font-size: 1.35rem;
}
.modal-subheading {
  margin-top: 0.25rem;
  color: var(--text-secondary);
  font-size: 0.95rem;
}
.modal-nav {
  display: flex;
  background: rgba(15, 23, 42, 0.9);
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
  padding: 0 1rem;
  overflow-x: auto;
}
.tab-btn {
  padding: 0.85rem 1.25rem;
  background: transparent;
  border: none;
  border-bottom: 2px solid transparent;
  color: var(--text-secondary);
  font-weight: 600;
  cursor: pointer;
  white-space: nowrap;
  transition: all 0.2s;
}
.tab-btn:hover {
  color: var(--text-primary);
}
.tab-btn.active {
  color: #34d399;
  border-bottom-color: #34d399;
}
.modal-body {
  padding: 1.25rem 1.5rem;
  overflow-y: auto;
  flex: 1;
}
.tab-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.85rem;
  flex-wrap: wrap;
  gap: 0.5rem;
}
.tab-toolbar span {
  font-size: 0.88rem;
  color: var(--text-secondary);
}
.toolbar-actions {
  display: flex;
  gap: 0.5rem;
}
.full-editor {
  min-height: 380px;
  width: 100%;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 0.88rem;
  line-height: 1.6;
  background: #0f172a;
  border: 1px solid rgba(255, 255, 255, 0.15);
  border-radius: 0.5rem;
  padding: 1rem;
  color: #e2e8f0;
}
.autofill-grid-modal {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0.75rem;
}
.modal-field-card {
  padding: 0.85rem 1rem;
  border-radius: 0.5rem;
  background: #1f2937;
  border: 1px solid rgba(255, 255, 255, 0.1);
  cursor: pointer;
  transition: all 0.15s;
}
.modal-field-card:hover {
  border-color: #34d399;
  background: rgba(55, 65, 81, 0.8);
}
.modal-field-card label {
  font-size: 0.72rem;
  text-transform: uppercase;
  color: var(--text-secondary);
  font-weight: 700;
  display: block;
}
.modal-field-card div {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-top: 0.35rem;
  font-size: 0.92rem;
  font-weight: 500;
  color: #f3f4f6;
}
.modal-field-card div span {
  font-size: 0.78rem;
  color: #34d399;
  font-weight: 600;
}
.modal-field-card.span-all {
  grid-column: 1 / -1;
}
.modal-footer {
  padding: 1rem 1.5rem;
  border-top: 1px solid rgba(255, 255, 255, 0.1);
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 0.75rem;
  flex-wrap: wrap;
  background: rgba(17, 24, 39, 0.8);
}
</style>
