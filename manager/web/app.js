const state = {
  config: null,
  editor: "visual",
  dirtyVisual: false,
  dirtyRaw: false,
  frameLoaded: false,
  settingsOpen: false,
  renameProfileID: "",
  deleteProfileID: "",
  migrationEnabled: false,
  migrationOpen: false,
  draggedDeviceRow: null,
  draggedDeviceStartIndex: -1,
};

const elements = {
  serviceState: document.querySelector("#service-state"),
  serviceStateText: document.querySelector("#service-state-text"),
  refreshButton: document.querySelector("#refresh-button"),
  settingsButton: document.querySelector("#settings-button"),
  settingsModal: document.querySelector("#settings-modal"),
  settingsDialog: document.querySelector(".settings-dialog"),
  modalSaveButton: document.querySelector("#modal-save-button"),
  modalSaveButtonText: document.querySelector("#modal-save-button-text"),
  saveButton: document.querySelector("#save-button"),
  saveButtonText: document.querySelector("#save-button-text"),
  configFileTabs: document.querySelector("#config-file-tabs"),
  configFileAdd: document.querySelector("#config-file-add"),
  configFilePopover: document.querySelector("#config-file-popover"),
  configFileName: document.querySelector("#config-file-name"),
  configFileCreateButton: document.querySelector("#config-file-create-button"),
  profileRenameModal: document.querySelector("#profile-rename-modal"),
  profileRenameInput: document.querySelector("#profile-rename-input"),
  profileRenameSave: document.querySelector("#profile-rename-save"),
  profileDeleteModal: document.querySelector("#profile-delete-modal"),
  profileDeleteName: document.querySelector("#profile-delete-name"),
  profileDeleteFile: document.querySelector("#profile-delete-file"),
  profileDeleteConfirm: document.querySelector("#profile-delete-confirm"),
  currentConfigName: document.querySelector("#current-config-name"),
  jobsList: document.querySelector("#jobs-list"),
  jobsEmpty: document.querySelector("#jobs-empty"),
  jobSettingsList: document.querySelector("#job-settings-list"),
  rawYAML: document.querySelector("#raw-yaml"),
  targetCount: document.querySelector("#target-count"),
  jobCount: document.querySelector("#job-count"),
  summaryInterval: document.querySelector("#summary-interval"),
  backupCount: document.querySelector("#backup-count"),
  backupDirectory: document.querySelector("#backup-directory"),
  backupEnabled: document.querySelector("#backup-enabled"),
  backupRetention: document.querySelector("#backup-retention"),
  saveBackupSettingsButton: document.querySelector("#save-backup-settings"),
  backupRecordCount: document.querySelector("#backup-record-count"),
  backupList: document.querySelector("#backup-list"),
  backupEmpty: document.querySelector("#backup-empty"),
  modifiedTime: document.querySelector("#modified-time"),
  frame: document.querySelector("#prometheus-frame"),
  frameLoading: document.querySelector("#frame-loading"),
  reloadFrameButton: document.querySelector("#reload-frame-button"),
  migrationBanner: document.querySelector("#migration-banner"),
  migrationDetailsButton: document.querySelector("#migration-details-button"),
  migrationModal: document.querySelector("#migration-modal"),
  migrationSourcePath: document.querySelector("#migration-source-path"),
  migrationTargetPath: document.querySelector("#migration-target-path"),
  migrationCommand: document.querySelector("#migration-command"),
  migrationCopyButton: document.querySelector("#migration-copy-button"),
  toastRegion: document.querySelector("#toast-region"),
};

document.addEventListener("DOMContentLoaded", () => {
  bindEvents();
  loadAll();
});

function bindEvents() {
  document.querySelectorAll("[data-view-button]").forEach((button) => {
    button.addEventListener("click", () => setView(button.dataset.viewButton));
  });
  document.querySelectorAll("[data-editor-tab]").forEach((button) => {
    button.addEventListener("click", () => setEditor(button.dataset.editorTab));
  });
  elements.refreshButton.addEventListener("click", refreshConfig);
  elements.settingsButton.addEventListener("click", () => setSettingsOpen(!state.settingsOpen));
  elements.saveButton.addEventListener("click", saveConfig);
  elements.modalSaveButton.addEventListener("click", saveConfig);
  elements.configFileTabs.addEventListener("click", handleProfileInteraction);
  elements.configFileAdd.addEventListener("click", () => setProfilePopoverOpen(elements.configFilePopover.hidden));
  elements.configFileCreateButton.addEventListener("click", createProfile);
  elements.configFileName.addEventListener("keydown", (event) => {
    if (event.key === "Enter") {
      event.preventDefault();
      createProfile();
    }
  });
  elements.profileRenameSave.addEventListener("click", renameProfile);
  elements.profileRenameInput.addEventListener("keydown", (event) => {
    if (event.key === "Enter") {
      event.preventDefault();
      renameProfile();
    }
  });
  elements.profileDeleteConfirm.addEventListener("click", deleteProfile);
  document.querySelectorAll("[data-profile-rename-close]").forEach((button) => {
    button.addEventListener("click", () => setRenameProfileOpen(false));
  });
  document.querySelectorAll("[data-profile-delete-close]").forEach((button) => {
    button.addEventListener("click", () => setDeleteProfileOpen(false));
  });
  elements.saveBackupSettingsButton.addEventListener("click", saveBackupSettings);
  document.querySelectorAll("[data-settings-close]").forEach((button) => {
    button.addEventListener("click", () => setSettingsOpen(false));
  });
  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape" && state.settingsOpen) {
      setSettingsOpen(false);
    }
    if (event.key === "Escape") {
      closeLabelPopovers();
      setProfilePopoverOpen(false);
      setRenameProfileOpen(false);
      setDeleteProfileOpen(false);
      setMigrationOpen(false);
    }
  });
  document.addEventListener("click", (event) => {
    if (!event.target.closest(".label-popover") && !event.target.closest("[data-action='toggle-label-popover']")) {
      closeLabelPopovers();
    }
    if (!event.target.closest(".config-file-create")) {
      setProfilePopoverOpen(false);
    }
  });
  elements.reloadFrameButton.addEventListener("click", reloadFrame);
  elements.migrationDetailsButton.addEventListener("click", () => setMigrationOpen(true));
  elements.migrationCopyButton.addEventListener("click", copyMigrationCommand);
  document.querySelectorAll("[data-migration-close]").forEach((button) => {
    button.addEventListener("click", () => setMigrationOpen(false));
  });
  elements.frame.addEventListener("load", () => elements.frameLoading.classList.add("is-hidden"));

  document.querySelector("[data-editor-body='visual']").addEventListener("input", (event) => {
    if (event.target.closest(".backup-card") || event.target.closest(".device-entry-row") || event.target.closest(".label-popover")) {
      return;
    }
    state.dirtyVisual = true;
  });
  elements.rawYAML.addEventListener("input", () => {
    state.dirtyRaw = true;
  });
  elements.rawYAML.addEventListener("keydown", handleEditorTabKey);
  elements.jobsList.addEventListener("click", handleJobAction);
  elements.jobsList.addEventListener("input", handleJobsInput);
  elements.jobsList.addEventListener("keydown", handleJobKeydown);
  elements.jobsList.addEventListener("pointerdown", handleDevicePointerDown);
  elements.jobsList.addEventListener("pointermove", handleDevicePointerMove);
  elements.jobsList.addEventListener("pointerup", handleDevicePointerEnd);
  elements.jobsList.addEventListener("pointercancel", handleDevicePointerEnd);
  elements.backupList.addEventListener("click", handleBackupAction);
}

async function loadAll() {
  await Promise.all([loadConfig(), loadStatus(), loadMigrationStatus()]);
}

async function loadMigrationStatus() {
  try {
    const response = await requestJSON("/api/migration");
    const migration = response.data || {};
    state.migrationEnabled = migration.enabled === true;
    if (!state.migrationEnabled) {
      setMigrationOpen(false);
      elements.migrationBanner.hidden = true;
      elements.migrationSourcePath.textContent = "";
      elements.migrationTargetPath.textContent = "";
      elements.migrationCommand.textContent = "";
      return;
    }
    elements.migrationBanner.hidden = !migration.migrationRequired;
    elements.migrationSourcePath.textContent = migration.sourceDirectory || "";
    elements.migrationTargetPath.textContent = migration.targetDirectory || "";
    elements.migrationCommand.textContent = migration.command || "";
  } catch (_error) {
    state.migrationEnabled = false;
    setMigrationOpen(false);
    elements.migrationBanner.hidden = true;
  }
}

function setMigrationOpen(open) {
  const shouldOpen = Boolean(open && state.migrationEnabled);
  state.migrationOpen = shouldOpen;
  elements.migrationModal.hidden = !shouldOpen;
  elements.migrationModal.setAttribute("aria-hidden", String(!shouldOpen));
  document.body.classList.toggle("profile-modal-open", shouldOpen);
}

async function copyMigrationCommand() {
  const command = elements.migrationCommand.textContent.trim();
  if (!command) {
    return;
  }
  try {
    await navigator.clipboard.writeText(command);
    showToast("命令已复制", "请停止两个应用后在 SSH 中执行。", "success");
  } catch (_error) {
    showToast("复制失败", "请手动选择并复制迁移命令。", "error");
  }
}

async function loadConfig(profileID = "") {
  setRefreshing(true);
  try {
    const query = profileID ? `?profile=${encodeURIComponent(profileID)}` : "";
    const response = await requestJSON(`/api/config${query}`);
    state.config = response.data;
    state.dirtyVisual = false;
    state.dirtyRaw = false;
    renderConfig();
  } catch (error) {
    showToast("读取配置失败", error.message, "error");
  } finally {
    setRefreshing(false);
  }
}

async function loadStatus() {
  elements.serviceState.classList.remove("is-ready", "is-down");
  elements.serviceStateText.textContent = "检查中";
  try {
    const response = await requestJSON("/api/status");
    const ready = Boolean(response.data && response.data.ready);
    elements.serviceState.classList.add(ready ? "is-ready" : "is-down");
    elements.serviceStateText.textContent = ready ? "运行正常" : "服务未就绪";
  } catch (_error) {
    elements.serviceState.classList.add("is-down");
    elements.serviceStateText.textContent = "状态未知";
  }
}

async function refreshConfig() {
  if ((state.dirtyVisual || state.dirtyRaw) && !window.confirm("当前有未保存修改，确定重新读取磁盘配置吗？")) {
    return;
  }
  await loadConfig(state.config?.profile || "");
  await loadStatus();
  showToast("配置已刷新", `已重新读取 ${currentProfileFileName()}。`, "success");
}

function renderConfig() {
  if (!state.config) {
    return;
  }
  const global = state.config.global || {};
  document.querySelector("#global-scrape-interval").value = global.scrapeInterval || "";
  document.querySelector("#global-evaluation-interval").value = global.evaluationInterval || "";
  document.querySelector("#global-scrape-timeout").value = global.scrapeTimeout || "";
  elements.rawYAML.value = state.config.raw || "";
  renderProfiles();
  renderJobs();
  renderJobSettings();
  renderSummary();
  elements.modifiedTime.textContent = state.config.modified
    ? `更新于 ${formatTime(state.config.modified)}`
    : "修改时间未知";
}

function renderSummary() {
  const jobs = state.config.jobs || [];
  const targets = jobs.reduce((total, job) => {
    return total + (job.staticConfigs || []).reduce((subtotal, config) => {
      return subtotal + devicesForConfig(config).filter((device) => !isLocalPrometheusTarget(job, device)).length;
    }, 0);
  }, 0);
  const backups = state.config.backups || [];
  elements.targetCount.textContent = String(targets).padStart(2, "0");
  elements.jobCount.textContent = String(jobs.length).padStart(2, "0");
  elements.summaryInterval.textContent = state.config.global?.scrapeInterval || "默认";
  elements.backupCount.textContent = String(backups.length).padStart(2, "0");
  renderBackups();
}

function renderBackups() {
  const settings = state.config.backupSettings || { enabled: true, retention: 20 };
  const backups = state.config.backups || [];
  elements.backupDirectory.textContent = state.config.backupDirectory || "未配置";
  elements.backupDirectory.title = state.config.backupDirectory || "未配置";
  elements.backupEnabled.checked = settings.enabled !== false;
  elements.backupRetention.value = settings.retention || 20;
  elements.backupRecordCount.textContent = `${backups.length} 份`;
  elements.backupList.innerHTML = backups.map(backupTemplate).join("");
  elements.backupEmpty.hidden = backups.length !== 0;
}

function backupTemplate(backup) {
  return `
    <article class="backup-record">
      <div class="backup-record-copy">
        <strong title="${escapeAttribute(backup.name)}">${escapeHTML(backup.name)}</strong>
        <small>${formatTime(backup.modified)} · ${formatBytes(backup.size)}</small>
      </div>
      <button class="remove-button" type="button" data-backup-name="${escapeAttribute(backup.name)}" aria-label="删除备份 ${escapeAttribute(backup.name)}" title="删除备份">×</button>
    </article>`;
}

function updateBackupState(data) {
  if (!data || !state.config) {
    return;
  }
  if (data.backups) {
    state.config.backups = data.backups;
  }
  if (data.backupSettings) {
    state.config.backupSettings = data.backupSettings;
  }
  if (data.backupDirectory) {
    state.config.backupDirectory = data.backupDirectory;
  }
  renderSummary();
}

async function saveBackupSettings() {
  const retention = Number(elements.backupRetention.value);
  if (!Number.isInteger(retention) || retention < 1 || retention > 100) {
    showToast("备份设置无效", "保留份数必须是 1 到 100 之间的整数。", "error");
    return;
  }
  elements.saveBackupSettingsButton.disabled = true;
  try {
    const response = await requestJSON("/api/backups/settings", {
      method: "PUT",
      body: JSON.stringify({ enabled: elements.backupEnabled.checked, retention }),
    });
    updateBackupState(response.data);
    showToast("备份设置已保存", `最多保留 ${retention} 份备份。`, "success");
  } catch (error) {
    showToast("备份设置未保存", error.message, "error");
  } finally {
    elements.saveBackupSettingsButton.disabled = false;
  }
}

async function handleBackupAction(event) {
  const button = event.target.closest("[data-backup-name]");
  if (!button) {
    return;
  }
  const name = button.dataset.backupName;
  if (!window.confirm(`确定删除备份“${name}”吗？此操作不可恢复。`)) {
    return;
  }
  button.disabled = true;
  try {
    const response = await requestJSON(`/api/backups/${encodeURIComponent(name)}`, { method: "DELETE" });
    updateBackupState(response.data);
    showToast("备份已删除", name, "success");
  } catch (error) {
    button.disabled = false;
    showToast("备份删除失败", error.message, "error");
  }
}

function setSettingsOpen(open) {
  if (!open && state.editor === "raw") {
    const previousOpen = state.settingsOpen;
    setEditor("visual");
    if (state.editor === "raw") {
      state.settingsOpen = previousOpen;
      return false;
    }
  }
  state.settingsOpen = open;
  elements.settingsButton.setAttribute("aria-expanded", String(open));
  elements.settingsModal.hidden = !open;
  elements.settingsModal.setAttribute("aria-hidden", String(!open));
  elements.settingsModal.classList.toggle("is-open", open);
  document.body.classList.toggle("settings-open", open);
  if (open) {
    window.requestAnimationFrame(() => elements.settingsDialog.querySelector("[data-settings-close]")?.focus());
  } else {
    elements.settingsButton.focus({ preventScroll: true });
  }
  return true;
}

function renderProfiles() {
  const profiles = state.config.profiles || [];
  const selectedJobSummary = summarizeJobs(state.config.jobs || []);
  elements.configFileTabs.innerHTML = profiles.map((profile) => {
    const selected = profile.id === state.config.profile;
    const deleteDisabled = profile.active || profiles.length <= 1;
    const deleteTitle = profile.active
      ? "运行中的配置不能删除，请先应用其他配置"
      : profiles.length <= 1 ? "至少保留一个配置文件" : "删除配置";
    return `
      <article class="config-file-card${selected ? " is-selected" : ""}">
        <button class="config-file-open" type="button" role="tab" aria-selected="${selected}" data-profile-id="${escapeAttribute(profile.id)}" title="打开 ${escapeAttribute(profile.fileName)}">
          <span class="config-file-card-meta">
            <span class="config-file-format">YML</span>
            ${profile.active ? '<span class="config-file-running">运行中</span>' : ""}
          </span>
          <strong>${escapeHTML(profile.name || profile.id)}</strong>
          <span class="config-file-name">${escapeHTML(profile.fileName)}</span>
          <small>${selected ? escapeHTML(selectedJobSummary) : `更新于 ${formatTime(profile.modified)}`}</small>
        </button>
        <div class="config-file-actions">
          <button class="config-file-action" type="button" data-profile-action="rename" data-profile-id="${escapeAttribute(profile.id)}" aria-label="编辑配置名称 ${escapeAttribute(profile.name || profile.id)}" title="编辑名称">
            <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L8 18l-4 1 1-4Z"/></svg>
          </button>
          <button class="config-file-action is-danger" type="button" data-profile-action="delete" data-profile-id="${escapeAttribute(profile.id)}" aria-label="删除配置 ${escapeAttribute(profile.name || profile.id)}" title="${escapeAttribute(deleteTitle)}"${deleteDisabled ? " disabled" : ""}>
            <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 6h18M8 6V4h8v2M19 6l-1 14H6L5 6M10 10v6M14 10v6"/></svg>
          </button>
        </div>
      </article>`;
  }).join("");
  const visibleJobCount = (state.config.jobs || []).filter((job) => !isLocalCollectionJob(job)).length;
  elements.currentConfigName.textContent = `${visibleJobCount} 个设备采集任务`;
}

async function handleProfileInteraction(event) {
  const actionButton = event.target.closest("[data-profile-action]");
  if (actionButton) {
    const profile = (state.config?.profiles || []).find((item) => item.id === actionButton.dataset.profileId);
    if (!profile) {
      return;
    }
    if (actionButton.dataset.profileAction === "rename") {
      setRenameProfileOpen(true, profile);
    } else if (actionButton.dataset.profileAction === "delete") {
      setDeleteProfileOpen(true, profile);
    }
    return;
  }
  const button = event.target.closest("[data-profile-id]");
  if (!button || button.dataset.profileId === state.config?.profile) {
    return;
  }
  if ((state.dirtyVisual || state.dirtyRaw) && !window.confirm("当前配置文件有未保存修改，确定切换并放弃这些修改吗？")) {
    return;
  }
  state.dirtyVisual = false;
  state.dirtyRaw = false;
  await loadConfig(button.dataset.profileId);
}

function setRenameProfileOpen(open, profile = null) {
  if (open && profile) {
    state.renameProfileID = profile.id;
    elements.profileRenameInput.value = profile.name || profile.id;
    elements.profileRenameInput.setCustomValidity("");
  } else if (!open) {
    state.renameProfileID = "";
  }
  elements.profileRenameModal.hidden = !open;
  elements.profileRenameModal.setAttribute("aria-hidden", String(!open));
  syncProfileModalState();
  if (open) {
    window.requestAnimationFrame(() => elements.profileRenameInput.select());
  }
}

async function renameProfile() {
  const profileID = state.renameProfileID;
  const name = elements.profileRenameInput.value.trim();
  elements.profileRenameInput.setCustomValidity("");
  if (!profileID || !name) {
    elements.profileRenameInput.setCustomValidity("请输入配置名称");
    elements.profileRenameInput.reportValidity();
    return;
  }
  elements.profileRenameSave.disabled = true;
  try {
    const response = await requestJSON(`/api/config/profiles/${encodeURIComponent(profileID)}`, {
      method: "PUT",
      body: JSON.stringify({ name }),
    });
    state.config.profiles = response.data.profiles;
    renderProfiles();
    setRenameProfileOpen(false);
    showToast("配置名称已更新", `显示名称已改为“${name}”，YAML 文件名保持不变。`, "success");
  } catch (error) {
    showToast("配置名称未更新", error.message, "error");
  } finally {
    elements.profileRenameSave.disabled = false;
  }
}

function setDeleteProfileOpen(open, profile = null) {
  if (open && profile) {
    state.deleteProfileID = profile.id;
    elements.profileDeleteName.textContent = profile.name || profile.id;
    elements.profileDeleteFile.textContent = profile.fileName;
    elements.profileDeleteConfirm.disabled = false;
  } else if (!open) {
    state.deleteProfileID = "";
  }
  elements.profileDeleteModal.hidden = !open;
  elements.profileDeleteModal.setAttribute("aria-hidden", String(!open));
  syncProfileModalState();
  if (open) {
    window.requestAnimationFrame(() => elements.profileDeleteConfirm.focus());
  }
}

async function deleteProfile() {
  const profileID = state.deleteProfileID;
  const profile = (state.config?.profiles || []).find((item) => item.id === profileID);
  if (!profileID || !profile) {
    return;
  }
  elements.profileDeleteConfirm.disabled = true;
  try {
    const response = await requestJSON(`/api/config/profiles/${encodeURIComponent(profileID)}`, { method: "DELETE" });
    const deletedSelectedProfile = state.config.profile === profileID;
    if (deletedSelectedProfile) {
      state.config = response.data;
      state.dirtyVisual = false;
      state.dirtyRaw = false;
      renderConfig();
    } else {
      state.config.profiles = response.data.profiles;
      renderProfiles();
    }
    setDeleteProfileOpen(false);
    showToast("配置文件已删除", `${profile.name || profile.id}（${profile.fileName}）`, "success");
  } catch (error) {
    elements.profileDeleteConfirm.disabled = false;
    showToast("配置文件未删除", error.message, "error");
  }
}

function syncProfileModalState() {
  const open = !elements.profileRenameModal.hidden || !elements.profileDeleteModal.hidden;
  document.body.classList.toggle("profile-modal-open", open);
}

function setProfilePopoverOpen(open) {
  elements.configFilePopover.hidden = !open;
  elements.configFileAdd.setAttribute("aria-expanded", String(open));
  if (open) {
    window.requestAnimationFrame(() => elements.configFileName.focus());
  }
}

async function createProfile() {
  const name = elements.configFileName.value.trim();
  elements.configFileName.setCustomValidity("");
  if (!name) {
    elements.configFileName.setCustomValidity("请输入配置名称");
    elements.configFileName.reportValidity();
    return;
  }
  if ((state.dirtyVisual || state.dirtyRaw) && !window.confirm("当前配置文件有未保存修改，创建新配置会离开当前文件，确定放弃这些修改吗？")) {
    return;
  }
  elements.configFileCreateButton.disabled = true;
  try {
    const response = await requestJSON("/api/config/profiles", {
      method: "POST",
      body: JSON.stringify({ name }),
    });
    state.config = response.data;
    state.dirtyVisual = false;
    state.dirtyRaw = false;
    elements.configFileName.value = "";
    setProfilePopoverOpen(false);
    renderConfig();
    showToast("配置文件已创建", `${currentProfileFileName()} 已打开，应用后才会设为运行配置。`, "success");
  } catch (error) {
    showToast("配置文件未创建", error.message, "error");
  } finally {
    elements.configFileCreateButton.disabled = false;
  }
}

function renderJobs() {
  const jobs = state.config.jobs || [];
  elements.jobsList.innerHTML = jobs.length === 0 ? "" : `
    <div class="jobs-workspace">
      <div class="job-panels">
        ${jobs.map((job, jobIndex) => jobTemplate(job, jobIndex)).join("")}
      </div>
    </div>`;
  elements.jobsEmpty.hidden = jobs.length !== 0;
}

function renderJobSettings() {
  const jobs = state.config.jobs || [];
  elements.jobSettingsList.innerHTML = jobs.map(jobSettingsTemplate).join("");
}

function jobSettingsTemplate(job, jobIndex) {
  const staticConfigs = job.staticConfigs || [];
  const advanced = job.advanced || staticConfigs.some((config) => config.advanced);
  const purpose = jobPurpose(job, jobIndex);
  const hidden = isLocalCollectionJob(job);
  return `
    <article class="job-settings-card" data-job-index="${jobIndex}"${hidden ? " hidden" : ""}>
      <div class="job-settings-heading">
        <div>
          <span class="job-settings-number">${String(jobIndex + 1).padStart(2, "0")}</span>
          <strong>${escapeHTML(purpose)}</strong>
        </div>
        ${advanced ? "<span>含高级字段</span>" : ""}
      </div>
      <div class="field-grid field-grid-four">
        <label class="field">
          <span>任务标签</span>
          <input type="text" value="${escapeAttribute(job.jobName || "")}" placeholder="例如：node" autocomplete="off" data-job-name-setting>
          <small>对应 Prometheus 的 job_name，通常保持默认</small>
        </label>
        <label class="field">
          <span>指标路径</span>
          <input type="text" value="${escapeAttribute(job.metricsPath || "")}" placeholder="/metrics" data-metrics-path>
        </label>
        <label class="field">
          <span>协议</span>
          <select data-scheme>
            <option value="" ${!job.scheme ? "selected" : ""}>默认</option>
            <option value="http" ${job.scheme === "http" ? "selected" : ""}>HTTP</option>
            <option value="https" ${job.scheme === "https" ? "selected" : ""}>HTTPS</option>
          </select>
        </label>
        <label class="field">
          <span>单独间隔</span>
          <input type="text" value="${escapeAttribute(job.scrapeInterval || "")}" placeholder="默认" data-scrape-interval>
        </label>
      </div>
      <label class="field job-timeout-field">
        <span>单独超时</span>
        <input type="text" value="${escapeAttribute(job.scrapeTimeout || "")}" placeholder="默认" data-scrape-timeout>
      </label>
    </article>`;
}

function jobTemplate(job, jobIndex) {
  const sourceIndex = Number.isInteger(job.sourceIndex) ? String(job.sourceIndex) : "";
  const staticConfigs = job.staticConfigs || [];
  const advanced = job.advanced || staticConfigs.some((config) => config.advanced);
  const purpose = jobPurpose(job, jobIndex);
  const hidden = isLocalCollectionJob(job);
  const primaryConfig = staticConfigs[0] || null;
  const primaryDevices = primaryConfig ? devicesForConfig(primaryConfig) : [];
  const primaryLabels = primaryConfig ? Object.keys(primaryConfig.labels || {}) : [];
  return `
    <article class="job-card${hidden ? " is-local-job" : ""}" data-job-index="${jobIndex}" data-job-name="${escapeAttribute(job.jobName || "")}" data-source-index="${sourceIndex}" data-advanced="${advanced}"${hidden ? " hidden" : ""}>
      <header class="job-card-header">
        <div class="job-identity">
          <div class="job-title-wrap">
            <strong class="job-purpose-title">${escapeHTML(purpose)}</strong>
          </div>
          <div class="job-group-summary">
            <span>${primaryConfig ? "设备分组 01" : "暂无设备分组"}</span>
            ${primaryConfig ? `<span class="device-count"><strong class="device-count-value" data-device-count-for="0">${primaryDevices.length}</strong> 台设备</span>` : ""}
          </div>
        </div>
        <div class="job-actions">
          ${advanced ? '<div class="job-badges"><span class="job-badge advanced">保留高级字段</span></div>' : ""}
          <div class="job-primary-actions">
            <button class="mini-button" type="button" data-action="add-static-config">＋ 新增分组</button>
            ${primaryConfig ? `
              <button class="mini-button label-manager-button" type="button" data-action="toggle-label-popover" data-static-index="0" aria-expanded="false">
                <span>公共标签</span><small class="label-count" data-label-count-for="0">${primaryLabels.length}</small>
              </button>
              <button class="group-remove-button" type="button" data-action="remove-static-config" data-static-index="0" data-group-title="设备分组 01" aria-label="删除设备分组" title="删除设备分组">×</button>` : ""}
          </div>
        </div>
      </header>
      <div class="job-card-body">
        ${advanced ? '<div class="advanced-notice"><strong>高级任务</strong><span>服务发现及设备别名以外的重标签字段会原样保留；需要修改它们时请使用“原始 YAML”。</span></div>' : ""}
        <div class="static-configs">
          ${staticConfigs.map((config, configIndex) => staticConfigTemplate(config, configIndex, jobIndex, job)).join("")}
        </div>
        ${staticConfigs.length === 0 ? '<div class="empty-static-actions"><span>当前任务没有静态目标分组</span></div>' : ""}
      </div>
    </article>`;
}

function staticConfigTemplate(config, configIndex, jobIndex, job) {
  const sourceIndex = Number.isInteger(config.sourceIndex) ? String(config.sourceIndex) : "";
  const labels = Object.entries(config.labels || {}).sort(([left], [right]) => left.localeCompare(right));
  const devices = devicesForConfig(config);
  const localGroup = devices.length > 0 && devices.every((device) => isLocalPrometheusTarget(job, device));
  const primaryGroup = configIndex === 0 && !localGroup;
  return `
    <section class="static-config${localGroup ? " is-local-group" : ""}${primaryGroup ? " is-primary-group" : ""}" data-static-index="${configIndex}" data-source-index="${sourceIndex}" data-advanced="${Boolean(config.advanced)}">
      ${primaryGroup ? "" : `<div class="static-config-header">
        <div>
          <span class="static-config-title">${localGroup ? "本机目标" : `设备分组 ${String(configIndex + 1).padStart(2, "0")}`}</span>
          <span class="device-count"><strong class="device-count-value" data-device-count-for="${configIndex}">${devices.length}</strong> ${localGroup ? "个目标" : "台设备"}</span>
        </div>
        <div class="static-config-actions">
          <button class="mini-button label-manager-button" type="button" data-action="toggle-label-popover" data-static-index="${configIndex}" aria-expanded="false">
            <span>公共标签</span><small class="label-count" data-label-count-for="${configIndex}">${labels.length}</small>
          </button>
          <button class="group-remove-button" type="button" data-action="remove-static-config" data-static-index="${configIndex}" data-group-title="${localGroup ? "本机目标" : `设备分组 ${String(configIndex + 1).padStart(2, "0")}`}" aria-label="删除设备分组" title="删除设备分组">×</button>
        </div>
      </div>`}
      <div class="label-popover" role="dialog" aria-modal="true" aria-labelledby="label-manager-title-${jobIndex}-${configIndex}" hidden>
            <button class="label-manager-backdrop" type="button" data-action="close-label-popover" aria-label="关闭公共标签"></button>
            <section class="label-manager-panel">
              <header class="label-popover-heading">
                <div>
                  <strong id="label-manager-title-${jobIndex}-${configIndex}">公共标签</strong>
                  <span>应用到本分组全部目标</span>
                </div>
                <button class="label-manager-close" type="button" data-action="close-label-popover" aria-label="关闭公共标签" title="关闭">×</button>
              </header>
              <div class="label-list" aria-label="公共标签列表">
                <div class="label-list-head" aria-hidden="true"><span>标签名</span><span>标签值</span><span>操作</span></div>
                <div class="label-rows">
                  ${labels.map(([key, value]) => labelRowTemplate(key, value)).join("")}
                </div>
                <div class="label-entry-row">
                  <input type="text" placeholder="填写标签名" aria-label="新标签名" autocomplete="off" data-new-label-key>
                  <input type="text" placeholder="填写标签值" aria-label="新标签值" autocomplete="off" data-new-label-value>
                  <button class="label-add-button" type="button" data-action="commit-label" aria-label="添加填写的标签" title="添加标签">＋</button>
                </div>
              </div>
            </section>
      </div>
      ${config.advanced ? '<div class="advanced-notice"><strong>保留字段</strong><span>本组包含未展示设置，表单保存时仍会保留。</span></div>' : ""}
      <div class="device-list" aria-label="设备列表">
        <div class="device-list-head" aria-hidden="true">
          <span>设备地址</span><span>设备别名</span><span>操作</span>
        </div>
        <div class="device-rows">
          ${devices.map((device, deviceIndex) => deviceRowTemplate(device, deviceIndex, isLocalPrometheusTarget(job, device))).join("")}
        </div>
        <div class="device-entry-row">
          <label class="device-field">
            <span class="sr-only">新设备地址</span>
            <input type="text" placeholder="填写 IP:端口" aria-label="新设备地址" autocomplete="off" data-new-device-address>
          </label>
          <label class="device-field">
            <span class="sr-only">新设备别名</span>
            <input type="text" placeholder="设备别名（可选）" aria-label="新设备别名" autocomplete="off" data-new-device-alias>
          </label>
          <button class="device-add-button" type="button" data-action="commit-device" aria-label="添加填写的设备" title="添加设备">＋</button>
        </div>
      </div>
    </section>`;
}

function deviceRowTemplate(device = {}, deviceIndex = 0, localTarget = false) {
  const rowNumber = String(deviceIndex + 1).padStart(2, "0");
  return `
    <div class="device-row${localTarget ? " is-local-target" : ""}">
      <label class="device-field">
        <span class="sr-only">设备地址</span>
        <input type="text" value="${escapeAttribute(device.address || "")}" placeholder="192.168.1.20:9100" aria-label="设备地址 ${rowNumber}" data-device-address>
      </label>
      <label class="device-field">
        <span class="sr-only">设备别名</span>
        <input type="text" value="${escapeAttribute(device.alias || "")}" placeholder="例如：机房 NAS" aria-label="设备别名 ${rowNumber}" data-device-alias>
      </label>
      <div class="device-row-actions">
        ${localTarget ? '<span class="local-target-badge">本机服务</span>' : ""}
        <button class="device-drag-handle" type="button" data-device-drag-handle aria-label="拖动排序设备 ${rowNumber}" title="拖动排序，方向键也可移动">⠿</button>
        <button class="remove-button" type="button" data-action="remove-device" aria-label="移除设备 ${rowNumber}" title="移除设备">×</button>
      </div>
    </div>`;
}

function labelRowTemplate(key = "", value = "") {
  return `
    <div class="label-row">
      <input type="text" value="${escapeAttribute(key)}" placeholder="标签名" aria-label="标签名" data-label-key>
      <input type="text" value="${escapeAttribute(value)}" placeholder="标签值" aria-label="标签值" data-label-value>
      <button class="remove-button" type="button" data-action="remove-label" aria-label="删除标签" title="删除标签">×</button>
    </div>`;
}

function collectVisualConfig() {
  const jobs = Array.from(elements.jobsList.querySelectorAll(".job-card")).map((card) => {
    const sourceIndex = parseOptionalIndex(card.dataset.sourceIndex);
    const settingsCard = elements.jobSettingsList.querySelector(`.job-settings-card[data-job-index="${card.dataset.jobIndex}"]`);
    const staticConfigs = Array.from(card.querySelectorAll(".static-config")).map((section) => {
      const labels = {};
      const devices = Array.from(section.querySelectorAll(".device-row")).map((row) => ({
        address: row.querySelector("[data-device-address]").value.trim(),
        alias: row.querySelector("[data-device-alias]").value.trim(),
      })).filter((device) => device.address);
      section.querySelectorAll(".label-row").forEach((row) => {
        const key = row.querySelector("[data-label-key]").value.trim();
        if (key) {
          labels[key] = row.querySelector("[data-label-value]").value;
        }
      });
      return {
        ...(parseOptionalIndex(section.dataset.sourceIndex) !== null ? { sourceIndex: parseOptionalIndex(section.dataset.sourceIndex) } : {}),
        targets: devices.map((device) => device.address),
        devices,
        labels,
        advanced: section.dataset.advanced === "true",
      };
    });
    return {
      ...(sourceIndex !== null ? { sourceIndex } : {}),
      jobName: settingsCard?.querySelector("[data-job-name-setting]")?.value.trim() || card.dataset.jobName || "",
      metricsPath: settingsCard?.querySelector("[data-metrics-path]")?.value.trim() || "",
      scheme: settingsCard?.querySelector("[data-scheme]")?.value || "",
      scrapeInterval: settingsCard?.querySelector("[data-scrape-interval]")?.value.trim() || "",
      scrapeTimeout: settingsCard?.querySelector("[data-scrape-timeout]")?.value.trim() || "",
      staticConfigs,
      advanced: card.dataset.advanced === "true",
    };
  });
  return {
    global: {
      scrapeInterval: document.querySelector("#global-scrape-interval").value.trim(),
      evaluationInterval: document.querySelector("#global-evaluation-interval").value.trim(),
      scrapeTimeout: document.querySelector("#global-scrape-timeout").value.trim(),
    },
    jobs,
  };
}

function handleJobAction(event) {
  const actionButton = event.target.closest("[data-action]");
  if (!actionButton) {
    return;
  }
  const action = actionButton.dataset.action;
  const jobCard = actionButton.closest(".job-card");
  const staticConfig = actionButton.closest(".static-config")
    || (actionButton.dataset.staticIndex !== undefined
      ? jobCard?.querySelector(`.static-config[data-static-index="${actionButton.dataset.staticIndex}"]`)
      : null);
  if (!jobCard) {
    return;
  }

  if (action === "add-static-config") {
    const config = collectVisualConfig();
    const jobIndex = Number(jobCard.dataset.jobIndex);
    config.jobs[jobIndex].staticConfigs.push({ targets: [], devices: [], labels: {}, advanced: false });
    applyCollectedConfig(config);
    return;
  }
  if (action === "remove-static-config") {
    const title = actionButton.dataset.groupTitle || staticConfig?.querySelector(".static-config-title")?.textContent.trim() || "设备分组";
    if (!window.confirm(`确定删除“${title}”吗？其中的目标和公共标签都会被移除。`)) {
      return;
    }
    staticConfig.remove();
    state.dirtyVisual = true;
    syncVisualStateAndRender();
    return;
  }
  if (action === "toggle-label-popover") {
    toggleLabelPopover(actionButton, staticConfig);
    return;
  }
  if (action === "close-label-popover") {
    closeLabelPopovers();
    return;
  }
  if (action === "commit-device") {
    commitDevice(staticConfig);
    return;
  }
  if (action === "remove-device") {
    actionButton.closest(".device-row").remove();
    updateDeviceSection(staticConfig);
    state.dirtyVisual = true;
    return;
  }
  if (action === "commit-label") {
    commitLabel(staticConfig);
    return;
  }
  if (action === "remove-label") {
    actionButton.closest(".label-row").remove();
    updateLabelSection(staticConfig);
    state.dirtyVisual = true;
  }
}

function handleJobsInput(event) {
  if (event.target.closest(".device-entry-row") || event.target.closest(".label-entry-row")) {
    return;
  }
  state.dirtyVisual = true;
}

function commitDevice(section) {
  const addressInput = section.querySelector("[data-new-device-address]");
  const aliasInput = section.querySelector("[data-new-device-alias]");
  const address = addressInput.value.trim();
  addressInput.setCustomValidity("");
  if (!address) {
    addressInput.setCustomValidity("请输入设备地址");
    addressInput.reportValidity();
    return;
  }
  const duplicate = Array.from(section.querySelectorAll("[data-device-address]")).find((input) => input.value.trim() === address);
  if (duplicate) {
    addressInput.setCustomValidity("该设备地址已存在");
    addressInput.reportValidity();
    duplicate.focus();
    return;
  }
  const rows = section.querySelector(".device-rows");
  const jobName = section.closest(".job-card")?.dataset.jobName || "";
  const localTarget = isLocalPrometheusTarget({ jobName }, { address });
  rows.insertAdjacentHTML("beforeend", deviceRowTemplate({ address, alias: aliasInput.value.trim() }, rows.children.length, localTarget));
  addressInput.value = "";
  aliasInput.value = "";
  updateDeviceSection(section);
  state.dirtyVisual = true;
  addressInput.focus();
}

function toggleLabelPopover(button, section) {
  const popover = section?.querySelector(".label-popover");
  if (!popover) {
    return;
  }
  const willOpen = popover.hidden;
  closeLabelPopovers(popover);
  popover.hidden = !willOpen;
  const staticIndex = section.dataset.staticIndex;
  section.closest(".job-card").querySelectorAll(`[data-action="toggle-label-popover"][data-static-index="${staticIndex}"]`)
    .forEach((toggle) => toggle.setAttribute("aria-expanded", String(willOpen)));
  document.body.classList.toggle("label-manager-open", willOpen);
  if (willOpen) {
    window.requestAnimationFrame(() => popover.querySelector("[data-new-label-key]").focus());
  }
}

function closeLabelPopovers(excluded = null) {
  elements.jobsList.querySelectorAll(".label-popover").forEach((popover) => {
    if (popover === excluded) {
      return;
    }
    popover.hidden = true;
    const section = popover.closest(".static-config");
    const staticIndex = section?.dataset.staticIndex;
    section?.closest(".job-card").querySelectorAll(`[data-action="toggle-label-popover"][data-static-index="${staticIndex}"]`)
      .forEach((toggle) => toggle.setAttribute("aria-expanded", "false"));
  });
  document.body.classList.toggle("label-manager-open", Boolean(excluded));
}

function commitLabel(section) {
  const popover = section.querySelector(".label-popover");
  const keyInput = popover.querySelector("[data-new-label-key]");
  const valueInput = popover.querySelector("[data-new-label-value]");
  const key = keyInput.value.trim();
  keyInput.setCustomValidity("");
  if (!key) {
    keyInput.setCustomValidity("请输入标签名");
    keyInput.reportValidity();
    return;
  }
  const duplicate = Array.from(section.querySelectorAll("[data-label-key]")).find((input) => input.value.trim() === key);
  if (duplicate) {
    keyInput.setCustomValidity("该标签名已存在");
    keyInput.reportValidity();
    duplicate.focus();
    return;
  }
  const rows = section.querySelector(".label-rows");
  rows.insertAdjacentHTML("beforeend", labelRowTemplate(key, valueInput.value));
  keyInput.value = "";
  valueInput.value = "";
  updateLabelSection(section);
  state.dirtyVisual = true;
  keyInput.focus();
}

function updateLabelSection(section) {
  const rows = section.querySelectorAll(".label-row");
  const staticIndex = section.dataset.staticIndex;
  section.closest(".job-card").querySelectorAll(`[data-label-count-for="${staticIndex}"]`)
    .forEach((count) => { count.textContent = String(rows.length); });
}

function handleJobKeydown(event) {
  const section = event.target.closest(".static-config");
  if (!section) {
    return;
  }
  if (event.key === "Enter" && event.target.matches("[data-new-device-address], [data-new-device-alias]")) {
    event.preventDefault();
    commitDevice(section);
    return;
  }
  if (event.key === "Enter" && event.target.matches("[data-new-label-key], [data-new-label-value]")) {
    event.preventDefault();
    commitLabel(section);
    return;
  }
  const handle = event.target.closest("[data-device-drag-handle]");
  if (!handle || (event.key !== "ArrowUp" && event.key !== "ArrowDown")) {
    return;
  }
  event.preventDefault();
  moveDeviceRow(handle.closest(".device-row"), event.key === "ArrowUp" ? -1 : 1);
}

function moveDeviceRow(row, direction) {
  const sibling = direction < 0 ? row.previousElementSibling : row.nextElementSibling;
  if (!sibling) {
    return;
  }
  if (direction < 0) {
    row.parentElement.insertBefore(row, sibling);
  } else {
    row.parentElement.insertBefore(sibling, row);
  }
  updateDeviceSection(row.closest(".static-config"));
  state.dirtyVisual = true;
  row.querySelector("[data-device-drag-handle]").focus();
}

function handleDevicePointerDown(event) {
  const handle = event.target.closest("[data-device-drag-handle]");
  if (!handle || event.button !== 0) {
    return;
  }
  event.preventDefault();
  state.draggedDeviceRow = handle.closest(".device-row");
  state.draggedDeviceStartIndex = Array.from(state.draggedDeviceRow.parentElement.children).indexOf(state.draggedDeviceRow);
  handle.setPointerCapture(event.pointerId);
  state.draggedDeviceRow.parentElement.classList.add("is-sorting");
  state.draggedDeviceRow.classList.add("is-dragging");
}

function handleDevicePointerMove(event) {
  if (!state.draggedDeviceRow || !event.target.closest("[data-device-drag-handle]")) {
    return;
  }
  event.preventDefault();
  const rows = state.draggedDeviceRow.parentElement;
  const nextRow = Array.from(rows.querySelectorAll(".device-row:not(.is-dragging)"))
    .find((row) => event.clientY < row.getBoundingClientRect().top + row.getBoundingClientRect().height / 2);
  rows.insertBefore(state.draggedDeviceRow, nextRow || null);
}

function handleDevicePointerEnd(event) {
  if (!state.draggedDeviceRow) {
    return;
  }
  event.preventDefault();
  const section = state.draggedDeviceRow.closest(".static-config");
  const rows = state.draggedDeviceRow.parentElement;
  const endIndex = Array.from(rows.children).indexOf(state.draggedDeviceRow);
  const handle = state.draggedDeviceRow.querySelector("[data-device-drag-handle]");
  if (handle.hasPointerCapture(event.pointerId)) {
    handle.releasePointerCapture(event.pointerId);
  }
  state.draggedDeviceRow.classList.remove("is-dragging");
  rows.classList.remove("is-sorting");
  state.draggedDeviceRow = null;
  const changed = state.draggedDeviceStartIndex !== endIndex;
  state.draggedDeviceStartIndex = -1;
  updateDeviceSection(section);
  state.dirtyVisual = state.dirtyVisual || changed;
}

function updateDeviceSection(section) {
  const rows = Array.from(section.querySelectorAll(".device-row"));
  rows.forEach((row, index) => {
    const rowNumber = String(index + 1).padStart(2, "0");
    row.querySelector("[data-device-address]").setAttribute("aria-label", `设备地址 ${rowNumber}`);
    row.querySelector("[data-device-alias]").setAttribute("aria-label", `设备别名 ${rowNumber}`);
    const dragHandle = row.querySelector("[data-device-drag-handle]");
    dragHandle.setAttribute("aria-label", `拖动排序设备 ${rowNumber}`);
    const removeButton = row.querySelector("[data-action='remove-device']");
    removeButton.setAttribute("aria-label", `移除设备 ${rowNumber}`);
  });
  const staticIndex = section.dataset.staticIndex;
  section.closest(".job-card").querySelectorAll(`[data-device-count-for="${staticIndex}"]`)
    .forEach((count) => { count.textContent = String(rows.length); });
  const userTargetCount = elements.jobsList.querySelectorAll(".device-row:not(.is-local-target)").length;
  elements.targetCount.textContent = String(userTargetCount).padStart(2, "0");
}

function applyCollectedConfig(collected) {
  state.config.global = collected.global;
  state.config.jobs = collected.jobs;
  state.dirtyVisual = true;
  renderJobs();
  renderJobSettings();
  renderSummary();
}

function syncVisualStateAndRender() {
  applyCollectedConfig(collectVisualConfig());
}

function deviceCount(config) {
  if (Array.isArray(config.devices)) {
    return config.devices.length;
  }
  return (config.targets || []).length;
}

function devicesForConfig(config) {
  if (Array.isArray(config.devices) && config.devices.length > 0) {
    return config.devices;
  }
  return (config.targets || []).map((address) => ({ address, alias: "" }));
}

function isLocalPrometheusTarget(job, device) {
  if ((job?.jobName || "").trim().toLowerCase() !== "prometheus") {
    return false;
  }
  const address = String(device?.address || "").trim().toLowerCase();
  return address.startsWith("127.0.0.1:") || address.startsWith("localhost:") || address.startsWith("[::1]:");
}

function isLocalCollectionJob(job) {
  if ((job?.jobName || "").trim().toLowerCase() !== "prometheus") {
    return false;
  }
  const devices = (job.staticConfigs || []).flatMap((config) => devicesForConfig(config));
  return devices.length > 0 && devices.every((device) => isLocalPrometheusTarget(job, device));
}

function jobPurpose(job, jobIndex = 0) {
  const jobName = String(job?.jobName || "").trim();
  if (jobName.toLowerCase() === "prometheus") {
    return "本机采集";
  }
  if (jobName.toLowerCase() === "node") {
    return "设备采集";
  }
  return jobName ? `${jobName} 采集` : `采集任务 ${String(jobIndex + 1).padStart(2, "0")}`;
}

function summarizeJobs(jobs) {
  if (jobs.length === 0) {
    return "暂无采集任务";
  }
  const names = jobs.map((job, index) => `${jobPurpose(job, index)} ${(job.scheme || "http").toUpperCase()}`);
  return names.length <= 2 ? names.join(" · ") : `${names.slice(0, 2).join(" · ")} 等 ${names.length} 项`;
}

function currentProfileFileName() {
  const profile = (state.config?.profiles || []).find((item) => item.id === state.config?.profile);
  return profile?.fileName || `${state.config?.profile || "prometheus"}.yml`;
}

async function saveConfig() {
  if (!state.config) {
    return;
  }
  setSaving(true);
  try {
    let response;
    if (state.editor === "raw") {
      response = await requestJSON("/api/config/raw", {
        method: "PUT",
        body: JSON.stringify({ profile: state.config.profile, checksum: state.config.checksum, raw: elements.rawYAML.value }),
      });
    } else {
      const visual = collectVisualConfig();
      response = await requestJSON("/api/config/basic", {
        method: "PUT",
        body: JSON.stringify({ profile: state.config.profile, checksum: state.config.checksum, ...visual }),
      });
    }
    state.config = response.data;
    state.dirtyVisual = false;
    state.dirtyRaw = false;
    renderConfig();
    await loadStatus();
    if (response.reloadOk) {
      const backupMessage = response.backup
        ? `校验通过，已生成备份 ${response.backup} 并热重载。`
        : "校验通过，未生成备份，Prometheus 已热重载。";
      showToast("配置已应用", backupMessage, "success");
    } else {
      showToast("配置已保存", response.reloadError || "Prometheus 热重载失败，请检查运行日志。", "warning");
    }
  } catch (error) {
    const message = error.details ? `${error.message}\n${error.details}` : error.message;
    showToast("配置未应用", message, "error");
    if (error.status === 409) {
      setTimeout(() => refreshConfig(), 600);
    }
  } finally {
    setSaving(false);
  }
}

function setView(viewName) {
  if (state.settingsOpen && !setSettingsOpen(false)) {
    return;
  }
  document.querySelectorAll("[data-view-button]").forEach((button) => {
    button.classList.toggle("is-active", button.dataset.viewButton === viewName);
  });
  document.querySelectorAll("[data-view]").forEach((view) => {
    view.classList.toggle("is-active", view.dataset.view === viewName);
  });
  if (viewName === "prometheus" && !state.frameLoaded) {
    state.frameLoaded = true;
    elements.frame.src = elements.frame.dataset.src;
  }
  window.scrollTo({ top: 0, behavior: "smooth" });
}

function setEditor(editorName) {
  if (editorName === state.editor) {
    return;
  }
  if (state.editor === "visual" && state.dirtyVisual) {
    if (!window.confirm("图形化表单有未保存修改。切换到 YAML 不会自动转换这些修改，确定放弃吗？")) {
      return;
    }
    state.dirtyVisual = false;
    renderConfig();
  }
  if (state.editor === "raw" && state.dirtyRaw) {
    if (!window.confirm("YAML 有未保存修改，确定放弃并切换到图形化配置吗？")) {
      return;
    }
    state.dirtyRaw = false;
    renderConfig();
  }
  state.editor = editorName;
  document.querySelectorAll("[data-editor-tab]").forEach((tab) => {
    const active = tab.dataset.editorTab === editorName;
    tab.classList.toggle("is-active", active);
    tab.setAttribute("aria-selected", String(active));
  });
  document.querySelectorAll("[data-editor-body]").forEach((body) => {
    body.classList.toggle("is-active", body.dataset.editorBody === editorName);
  });
  const saveLabel = editorName === "raw" ? "校验并应用 YAML" : "校验并应用";
  elements.saveButtonText.textContent = saveLabel;
  elements.modalSaveButtonText.textContent = saveLabel;
}

function reloadFrame() {
  if (!state.frameLoaded) {
    setView("prometheus");
    return;
  }
  elements.frameLoading.classList.remove("is-hidden");
  elements.frame.src = elements.frame.src;
}

function setSaving(saving) {
  elements.saveButton.disabled = saving;
  elements.modalSaveButton.disabled = saving;
  elements.saveButtonText.textContent = saving
    ? "正在校验…"
    : state.editor === "raw" ? "校验并应用 YAML" : "校验并应用";
  elements.modalSaveButtonText.textContent = saving
    ? "正在校验…"
    : state.editor === "raw" ? "校验并应用 YAML" : "校验并应用";
}

function setRefreshing(refreshing) {
  elements.refreshButton.disabled = refreshing;
  elements.refreshButton.classList.toggle("is-spinning", refreshing);
}

function handleEditorTabKey(event) {
  if (event.key !== "Tab") {
    return;
  }
  event.preventDefault();
  const textarea = event.target;
  const start = textarea.selectionStart;
  const end = textarea.selectionEnd;
  textarea.setRangeText("  ", start, end, "end");
  state.dirtyRaw = true;
}

async function requestJSON(url, options = {}) {
  const response = await fetch(url, {
    headers: { "Content-Type": "application/json", ...(options.headers || {}) },
    ...options,
  });
  let payload;
  try {
    payload = await response.json();
  } catch (_error) {
    payload = { error: `HTTP ${response.status}` };
  }
  if (!response.ok || !payload.ok) {
    const error = new Error(payload.error || payload.message || `HTTP ${response.status}`);
    error.status = response.status;
    error.details = payload.details || "";
    throw error;
  }
  return payload;
}

function showToast(title, message, type = "success") {
  const toast = document.createElement("div");
  toast.className = `toast ${type === "success" ? "" : `is-${type}`}`.trim();
  const content = document.createElement("div");
  const heading = document.createElement("strong");
  const body = document.createElement("p");
  heading.textContent = title;
  body.textContent = message;
  content.append(heading, body);
  toast.append(content);
  elements.toastRegion.append(toast);
  window.setTimeout(() => toast.remove(), type === "error" ? 9000 : 5200);
}

function parseOptionalIndex(value) {
  if (value === "" || value === undefined) {
    return null;
  }
  const parsed = Number(value);
  return Number.isInteger(parsed) ? parsed : null;
}

function formatTime(value) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(date);
}

function formatBytes(value) {
  if (!Number.isFinite(value) || value < 1024) {
    return `${value || 0} B`;
  }
  if (value < 1024 * 1024) {
    return `${(value / 1024).toFixed(1)} KB`;
  }
  return `${(value / (1024 * 1024)).toFixed(1)} MB`;
}

function escapeHTML(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

function escapeAttribute(value) {
  return escapeHTML(value).replaceAll("`", "&#096;");
}
