const terminalOutput = document.querySelector("#terminalOutput");
const commandForm = document.querySelector("#commandForm");
const commandInput = document.querySelector("#commandInput");
const historyList = document.querySelector("#historyList");
const historyTable = document.querySelector("#historyTable");
const historyFilterInput = document.querySelector("#historyFilterInput");
const historyTotal = document.querySelector("#historyTotal");
const historyFailed = document.querySelector("#historyFailed");
const jobsList = document.querySelector("#jobsList");
const jobsBoard = document.querySelector("#jobsBoard");
const jobIdInput = document.querySelector("#jobIdInput");
const sessionStatus = document.querySelector("#sessionStatus");
const latencyStatus = document.querySelector("#latencyStatus");
const commandCount = document.querySelector("#commandCount");
const searchInput = document.querySelector("#searchInput");
const compactToggle = document.querySelector("#compactToggle");
const errorToggle = document.querySelector("#errorToggle");
const autoJobsToggle = document.querySelector("#autoJobsToggle");

const state = {
  sessionId: newSessionId(),
  history: [],
  historyIndex: -1,
  count: 0,
  failed: 0,
  closed: false,
  settings: {
    compact: false,
    echoErrors: true,
    autoJobs: false
  }
};

commandForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const command = commandInput.value.trim();
  if (!command || state.closed) {
    return;
  }
  commandInput.value = "";
  await runCommand(command);
});

document.querySelectorAll("[data-command]").forEach((button) => {
  button.addEventListener("click", () => runCommand(button.dataset.command));
});

document.querySelector("#sampleButton").addEventListener("click", () => runCommand("pwd"));
document.querySelector("#focusButton").addEventListener("click", () => commandInput.focus());
document.querySelector("#clearButton").addEventListener("click", clearTerminal);
document.querySelector("#newSessionButton").addEventListener("click", newSession);
document.querySelector("#copyHistoryButton").addEventListener("click", copyHistory);
document.querySelector("#copyHistorySideButton").addEventListener("click", copyHistory);
document.querySelector("#clearHistoryButton").addEventListener("click", clearHistory);
document.querySelector("#resetSettingsButton").addEventListener("click", resetSettings);

document.querySelectorAll("[data-view-target]").forEach((button) => {
  button.addEventListener("click", () => showView(button.dataset.viewTarget));
});

document.querySelectorAll("[data-job-action]").forEach((button) => {
  button.addEventListener("click", () => {
    const id = jobIdInput.value.trim();
    if (id) {
      runCommand(`${button.dataset.jobAction} ${id}`);
    }
  });
});

commandInput.addEventListener("keydown", (event) => {
  if (event.key === "ArrowUp") {
    event.preventDefault();
    moveHistory(-1);
  }
  if (event.key === "ArrowDown") {
    event.preventDefault();
    moveHistory(1);
  }
});

searchInput.addEventListener("input", () => {
  const query = searchInput.value.toLowerCase();
  document.querySelectorAll(".entry").forEach((entry) => {
    entry.hidden = query !== "" && !entry.textContent.toLowerCase().includes(query);
  });
});

historyFilterInput.addEventListener("input", renderHistory);
compactToggle.addEventListener("change", () => updateSetting("compact", compactToggle.checked));
errorToggle.addEventListener("change", () => updateSetting("echoErrors", errorToggle.checked));
autoJobsToggle.addEventListener("change", () => updateSetting("autoJobs", autoJobsToggle.checked));

async function runCommand(command) {
  appendEntry(command);
  pushHistory(command);
  setBusy(true);
  const start = performance.now();

  try {
    const response = await fetch("/api/execute", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ sessionId: state.sessionId, command })
    });
    const result = await response.json();
    const elapsed = Math.round(performance.now() - start);
    latencyStatus.textContent = `${elapsed}ms`;
    appendResult(result);
    updateJobs(command, result);
    updateHistoryResult(command, result);
    if (state.settings.autoJobs && command !== "jobs" && result.keepRunning !== false) {
      await refreshJobs();
    }
    state.closed = result.keepRunning === false;
    sessionStatus.textContent = state.closed ? "exited" : (result.ok ? "ready" : "error");
    commandInput.disabled = state.closed;
  } catch (error) {
    appendResult({ ok: false, stderr: error.message, keepRunning: true });
    sessionStatus.textContent = "error";
  } finally {
    setBusy(false);
    commandInput.focus();
  }
}

function appendEntry(command) {
  const entry = document.createElement("div");
  entry.className = "entry";
  entry.innerHTML = `
    <div class="entry-command">
      <span class="entry-prompt">gosh</span>
      <span class="entry-cwd">~/projects/go-shell &gt;</span>
      <span class="entry-text"></span>
    </div>
  `;
  entry.querySelector(".entry-text").textContent = command;
  terminalOutput.append(entry);
  scrollTerminal();
}

function appendResult(result) {
  const entry = terminalOutput.lastElementChild;
  const chip = document.createElement("span");
  chip.className = `status-chip ${result.ok ? "success" : "error"}`;
  chip.textContent = result.ok ? "OK" : "ERR";
  entry.querySelector(".entry-command").prepend(chip);

  if (result.stdout) {
    entry.append(outputBlock(result.stdout, "stdout"));
  }
  if (state.settings.echoErrors && (result.stderr || result.error)) {
    entry.append(outputBlock(result.stderr || result.error, "stderr"));
  }
  if (!result.stdout && !result.stderr && !result.error) {
    entry.append(outputBlock("(no output)", "stdout"));
  }
  scrollTerminal();
}

function outputBlock(text, kind) {
  const block = document.createElement("pre");
  block.className = `entry-output ${kind}`;
  block.textContent = text.trimEnd();
  return block;
}

function pushHistory(command) {
  const record = {
    command,
    ok: true,
    stdout: "",
    stderr: "",
    at: new Date()
  };
  state.history.push(record);
  state.historyIndex = state.history.length;
  state.count += 1;
  commandCount.textContent = `${state.count} commands`;
  historyTotal.textContent = String(state.count);

  const item = document.createElement("li");
  item.textContent = command;
  item.addEventListener("click", () => {
    commandInput.value = command;
    commandInput.focus();
  });
  historyList.prepend(item);
  renderHistory();
}

function moveHistory(direction) {
  if (state.history.length === 0) {
    return;
  }
  state.historyIndex = Math.max(0, Math.min(state.history.length, state.historyIndex + direction));
  commandInput.value = state.history[state.historyIndex]?.command || "";
}

function updateJobs(command, result) {
  if (command !== "jobs") {
    return;
  }
  const lines = result.stdout.trim().split(/\r?\n/).filter(Boolean);
  renderJobs(lines);
}

function renderJobs(lines) {
  jobsList.innerHTML = "";
  jobsBoard.innerHTML = "";
  if (lines.length === 0) {
    jobsList.innerHTML = '<div class="empty-state">No background jobs reported.</div>';
    jobsBoard.innerHTML = '<div class="empty-state">No background jobs reported.</div>';
    return;
  }
  lines.forEach((line) => {
    const parsed = parseJobLine(line);
    const row = document.createElement("div");
    row.className = "empty-state";
    row.textContent = line;
    jobsList.append(row);

    const card = document.createElement("div");
    card.className = "job-card";
    card.innerHTML = `
      <div class="job-id"></div>
      <div class="job-command"></div>
      <div class="job-status"></div>
    `;
    card.querySelector(".job-id").textContent = parsed.id ? `#${parsed.id}` : "#";
    card.querySelector(".job-command").textContent = parsed.command || line;
    card.querySelector(".job-status").textContent = parsed.status || "job";
    card.addEventListener("click", () => {
      if (parsed.id) {
        jobIdInput.value = parsed.id;
      }
    });
    jobsBoard.append(card);
  });
}

function parseJobLine(line) {
  const match = line.match(/^\[(\d+)\]\s+(\S+)\s+(.*)$/);
  if (!match) {
    return { id: "", status: "", command: line };
  }
  return { id: match[1], status: match[2], command: match[3] };
}

async function refreshJobs() {
  try {
    const response = await fetch("/api/execute", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ sessionId: state.sessionId, command: "jobs" })
    });
    const result = await response.json();
    updateJobs("jobs", result);
  } catch {
    renderJobs([]);
  }
}

function updateHistoryResult(command, result) {
  const record = state.history[state.history.length - 1];
  if (!record || record.command !== command) {
    return;
  }
  record.ok = Boolean(result.ok);
  record.stdout = result.stdout || "";
  record.stderr = result.stderr || result.error || "";
  if (!record.ok) {
    state.failed += 1;
  }
  historyFailed.textContent = String(state.failed);
  renderHistory();
}

function renderHistory() {
  const query = historyFilterInput.value.toLowerCase();
  historyTable.innerHTML = "";
  const records = state.history.filter((record) => record.command.toLowerCase().includes(query));
  if (records.length === 0) {
    historyTable.innerHTML = '<li class="empty-state">No matching commands.</li>';
    return;
  }
  [...records].reverse().forEach((record, index) => {
    const row = document.createElement("li");
    row.className = "history-row";
    row.innerHTML = `
      <span class="status-chip ${record.ok ? "success" : "error"}"></span>
      <span class="history-command"></span>
      <button class="ghost-button" type="button">Run</button>
    `;
    row.querySelector(".status-chip").textContent = record.ok ? "OK" : "ERR";
    row.querySelector(".history-command").textContent = record.command;
    row.querySelector("button").addEventListener("click", () => runCommand(record.command));
    row.style.order = String(index);
    historyTable.append(row);
  });
}

function clearHistory() {
  state.history = [];
  state.historyIndex = -1;
  state.count = 0;
  state.failed = 0;
  historyList.innerHTML = "";
  historyTotal.textContent = "0";
  historyFailed.textContent = "0";
  commandCount.textContent = "0 commands";
  renderHistory();
}

function showView(view) {
  document.querySelectorAll("[data-view]").forEach((panel) => {
    panel.classList.toggle("active", panel.dataset.view === view);
  });
  document.querySelectorAll("[data-view-target]").forEach((button) => {
    button.classList.toggle("active", button.dataset.viewTarget === view);
  });
  if (view === "jobs") {
    refreshJobs();
  }
}

function updateSetting(key, value) {
  state.settings = { ...state.settings, [key]: value };
  document.body.classList.toggle("compact-output", state.settings.compact);
}

function resetSettings() {
  state.settings = { compact: false, echoErrors: true, autoJobs: false };
  compactToggle.checked = false;
  errorToggle.checked = true;
  autoJobsToggle.checked = false;
  document.body.classList.remove("compact-output");
}

function newSession() {
  state.sessionId = newSessionId();
  state.closed = false;
  sessionStatus.textContent = "ready";
  commandInput.disabled = false;
  clearTerminal();
  clearHistory();
  renderJobs([]);
  commandInput.focus();
}

function newSessionId() {
  const bytes = new Uint32Array(2);
  crypto.getRandomValues(bytes);
  return `ui-${Date.now().toString(36)}-${bytes[0].toString(36)}${bytes[1].toString(36)}`;
}

function clearTerminal() {
  terminalOutput.innerHTML = "";
  const line = document.createElement("div");
  line.className = "welcome-line";
  line.innerHTML = '<span class="status-chip success">READY</span><span>Terminal cleared.</span>';
  terminalOutput.append(line);
}

async function copyHistory() {
  const text = state.history.map((record) => record.command).join("\n");
  if (navigator.clipboard && text) {
    await navigator.clipboard.writeText(text);
  }
}

function setBusy(isBusy) {
  sessionStatus.textContent = isBusy ? "running" : sessionStatus.textContent;
  commandInput.disabled = isBusy || state.closed;
}

function scrollTerminal() {
  terminalOutput.scrollTop = terminalOutput.scrollHeight;
}

commandInput.focus();
