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
const workspaceNameInput = document.querySelector("#workspaceNameInput");
const workspaceList = document.querySelector("#workspaceList");
const streamButton = document.querySelector("#streamButton");
const streamForm = document.querySelector("#streamForm");
const streamInput = document.querySelector("#streamInput");
const stopStreamButton = document.querySelector("#stopStreamButton");
const exportWorkspacesButton = document.querySelector("#exportWorkspacesButton");
const importWorkspacesButton = document.querySelector("#importWorkspacesButton");
const exportEncryptedWorkspacesButton = document.querySelector("#exportEncryptedWorkspacesButton");
const importEncryptedWorkspacesButton = document.querySelector("#importEncryptedWorkspacesButton");
const workspaceImportInput = document.querySelector("#workspaceImportInput");
const workspaceEncryptedImportInput = document.querySelector("#workspaceEncryptedImportInput");

const storageKey = "gosh.workspaces.v1";
const tokenStorageKey = "gosh.ui.token";
const encryptedArchiveVersion = 1;

const state = {
  workspaces: loadWorkspaces(),
  activeWorkspaceId: "",
  stream: null,
  syncTimer: null,
  historyIndex: -1,
  settings: {
    compact: false,
    echoErrors: true,
    autoJobs: false
  }
};

if (state.workspaces.length === 0) {
  state.workspaces = [createWorkspace("Default")];
  saveWorkspaces();
}
state.activeWorkspaceId = state.workspaces[0].id;

commandForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const command = commandInput.value.trim();
  const workspace = activeWorkspace();
  if (!command || workspace?.closed) {
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
streamButton.addEventListener("click", startInteractiveStream);
streamForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  await sendStreamInput(streamInput.value + "\n");
  streamInput.value = "";
});
stopStreamButton.addEventListener("click", () => {
  stopInteractiveStream();
});
workspaceNameInput.addEventListener("change", renameActiveWorkspace);
document.querySelector("#copyHistoryButton").addEventListener("click", copyHistory);
document.querySelector("#copyHistorySideButton").addEventListener("click", copyHistory);
document.querySelector("#clearHistoryButton").addEventListener("click", clearHistory);
document.querySelector("#resetSettingsButton").addEventListener("click", resetSettings);
exportWorkspacesButton.addEventListener("click", exportWorkspaces);
importWorkspacesButton.addEventListener("click", () => workspaceImportInput.click());
workspaceImportInput.addEventListener("change", importWorkspaces);
exportEncryptedWorkspacesButton.addEventListener("click", exportEncryptedWorkspaces);
importEncryptedWorkspacesButton.addEventListener("click", () => workspaceEncryptedImportInput.click());
workspaceEncryptedImportInput.addEventListener("change", importEncryptedWorkspaces);

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

const resizeObserver = new ResizeObserver(() => resizeInteractiveStream());
resizeObserver.observe(terminalOutput);

async function runCommand(command) {
  const workspace = activeWorkspace();
  if (!workspace) {
    return;
  }
  appendEntry(command);
  pushHistory(command);
  setBusy(true);
  const start = performance.now();

  try {
    const response = await fetch("/api/execute", {
      method: "POST",
      headers: apiHeaders(),
      body: JSON.stringify({ sessionId: workspace.sessionId, command })
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
    workspace.closed = result.keepRunning === false;
    saveWorkspaces();
    sessionStatus.textContent = workspace.closed ? "exited" : (result.ok ? "ready" : "error");
    commandInput.disabled = workspace.closed;
  } catch (error) {
    appendResult({ ok: false, stderr: error.message, keepRunning: true });
    sessionStatus.textContent = "error";
  } finally {
    setBusy(false);
    commandInput.focus();
  }
}

function appendEntry(command, persist = true) {
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
  if (persist) {
    rememberTranscript("command", command);
  }
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
    rememberTranscript("stdout", result.stdout);
  }
  if (state.settings.echoErrors && (result.stderr || result.error)) {
    const text = result.stderr || result.error;
    entry.append(outputBlock(text, "stderr"));
    rememberTranscript("stderr", text);
  }
  if (!result.stdout && !result.stderr && !result.error) {
    entry.append(outputBlock("(no output)", "stdout"));
    rememberTranscript("stdout", "(no output)");
  }
  scrollTerminal();
}

function outputBlock(text, kind) {
  const block = document.createElement("pre");
  block.className = `entry-output ${kind}`;
  block.append(renderTerminalText(text.trimEnd()));
  return block;
}

function renderTerminalText(text) {
  if (usesTerminalScreen(text)) {
    return renderTerminalScreen(text);
  }
  const fragment = document.createDocumentFragment();
  const state = { classes: [] };
  const normalized = normalizeTerminalControls(text);
  let index = 0;
  for (const match of normalized.matchAll(/\x1b\[([?0-9;]*)([A-Za-z])/g)) {
    appendTerminalSpan(fragment, normalized.slice(index, match.index), state.classes);
    applyControlSequence(match[1], match[2], state);
    index = match.index + match[0].length;
  }
  appendTerminalSpan(fragment, normalized.slice(index), state.classes);
  return fragment;
}

function usesTerminalScreen(text) {
  return /\x1b\[(?:\?[0-9]+[hl]|[0-9;]*[HfABCDJK])/.test(text) || text.includes("\r");
}

function createTerminalScreen(cols = 120, rows = 80) {
  return {
    cols,
    rows,
    cursorRow: 0,
    cursorCol: 0,
    lines: Array.from({ length: rows }, () => [])
  };
}

function renderTerminalScreen(text) {
  const screen = createTerminalScreen();
  let index = 0;
  while (index < text.length) {
    if (text[index] === "\x1b" && text[index + 1] === "[") {
      const sequence = readControlSequence(text, index);
      if (sequence) {
        applyScreenControl(sequence.params, sequence.command, screen);
        index = sequence.end;
        continue;
      }
    }
    writeTerminalCharacter(screen, text[index]);
    index += 1;
  }
  const fragment = document.createDocumentFragment();
  appendTerminalSpan(fragment, screenText(screen), []);
  return fragment;
}

function readControlSequence(text, start) {
  const match = /^\x1b\[([?0-9;]*)([A-Za-z])/.exec(text.slice(start));
  if (!match) {
    return null;
  }
  return {
    params: match[1],
    command: match[2],
    end: start + match[0].length
  };
}

function applyScreenControl(params, command, screen) {
  const values = params.replace("?", "").split(";").filter(Boolean).map((value) => Number(value));
  if (params === "?1049" && (command === "h" || command === "l")) {
    clearTerminalScreen(screen);
    return;
  }
  if (command === "H" || command === "f") {
    screen.cursorRow = Math.max(0, Math.min(screen.rows - 1, (values[0] || 1) - 1));
    screen.cursorCol = Math.max(0, Math.min(screen.cols - 1, (values[1] || 1) - 1));
    return;
  }
  if (command === "A") {
    moveTerminalCursor(screen, -(values[0] || 1), 0);
    return;
  }
  if (command === "B") {
    moveTerminalCursor(screen, values[0] || 1, 0);
    return;
  }
  if (command === "C") {
    moveTerminalCursor(screen, 0, values[0] || 1);
    return;
  }
  if (command === "D") {
    moveTerminalCursor(screen, 0, -(values[0] || 1));
    return;
  }
  if (command === "J") {
    clearTerminalScreen(screen);
    return;
  }
  if (command === "K") {
    screen.lines[screen.cursorRow] = screen.lines[screen.cursorRow].slice(0, screen.cursorCol);
  }
}

function moveTerminalCursor(screen, rowDelta, colDelta) {
  screen.cursorRow = Math.max(0, Math.min(screen.rows - 1, screen.cursorRow + rowDelta));
  screen.cursorCol = Math.max(0, Math.min(screen.cols - 1, screen.cursorCol + colDelta));
}

function clearTerminalScreen(screen) {
  screen.lines = Array.from({ length: screen.rows }, () => []);
  screen.cursorRow = 0;
  screen.cursorCol = 0;
}

function writeTerminalCharacter(screen, character) {
  if (character === "\n") {
    screen.cursorRow = Math.min(screen.rows - 1, screen.cursorRow + 1);
    screen.cursorCol = 0;
    return;
  }
  if (character === "\r") {
    screen.cursorCol = 0;
    return;
  }
  if (character === "\b") {
    screen.cursorCol = Math.max(0, screen.cursorCol - 1);
    return;
  }
  const line = screen.lines[screen.cursorRow];
  while (line.length < screen.cursorCol) {
    line.push(" ");
  }
  line[screen.cursorCol] = character;
  screen.cursorCol += 1;
  if (screen.cursorCol >= screen.cols) {
    screen.cursorCol = 0;
    screen.cursorRow = Math.min(screen.rows - 1, screen.cursorRow + 1);
  }
}

function screenText(screen) {
  const lines = screen.lines.map((line) => line.join("").replace(/\s+$/g, ""));
  while (lines.length > 0 && lines[lines.length - 1] === "") {
    lines.pop();
  }
  return lines.join("\n");
}

function normalizeTerminalControls(text) {
  return text
    .replace(/\x1b\[\?1049[hl]/g, "\n")
    .replace(/\x1bc/g, "\n")
    .replace(/\r([^\n])/g, "\n$1")
    .replace(/\x1b\[[0-9;]*[JK]/g, "\n");
}

function applyControlSequence(params, command, state) {
  if (command !== "m") {
    return;
  }
  state.classes = parseSGR(params, state.classes);
}

function parseSGR(params, currentClasses) {
  const codes = params === "" ? [0] : params.split(";").map((value) => Number(value || "0"));
  let classes = [...currentClasses];
  codes.forEach((code) => {
    if (code === 0) {
      classes = [];
      return;
    }
    if (code === 1) {
      classes = upsertClass(classes, "ansi-bold");
      return;
    }
    if (code === 2) {
      classes = upsertClass(classes, "ansi-dim");
      return;
    }
    if (code === 4) {
      classes = upsertClass(classes, "ansi-underline");
      return;
    }
    if (code === 22) {
      classes = classes.filter((value) => value !== "ansi-bold" && value !== "ansi-dim");
      return;
    }
    if (code === 24) {
      classes = classes.filter((value) => value !== "ansi-underline");
      return;
    }
    if (code === 39) {
      classes = classes.filter((value) => !value.startsWith("ansi-fg-"));
      return;
    }
    if ((code >= 30 && code <= 37) || (code >= 90 && code <= 97)) {
      classes = classes.filter((value) => !value.startsWith("ansi-fg-"));
      classes.push(`ansi-fg-${code}`);
    }
  });
  return classes;
}

function upsertClass(classes, className) {
  return classes.includes(className) ? classes : [...classes, className];
}

function appendTerminalSpan(fragment, text, classes) {
  if (!text) {
    return;
  }
  const span = document.createElement("span");
  span.className = classes.join(" ");
  span.textContent = text;
  fragment.append(span);
}

function pushHistory(command) {
  const workspace = activeWorkspace();
  const record = {
    command,
    ok: true,
    stdout: "",
    stderr: "",
    at: new Date()
  };
  workspace.history = [...workspace.history, record];
  state.historyIndex = workspace.history.length;
  workspace.count += 1;
  commandCount.textContent = `${workspace.count} commands`;
  historyTotal.textContent = String(workspace.count);

  historyList.prepend(historyListItem(command));
  saveWorkspaces();
  renderHistory();
}

function moveHistory(direction) {
  const workspace = activeWorkspace();
  if (!workspace || workspace.history.length === 0) {
    return;
  }
  state.historyIndex = Math.max(0, Math.min(workspace.history.length, state.historyIndex + direction));
  commandInput.value = workspace.history[state.historyIndex]?.command || "";
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
      headers: apiHeaders(),
      body: JSON.stringify({ sessionId: activeWorkspace()?.sessionId || "default", command: "jobs" })
    });
    const result = await response.json();
    updateJobs("jobs", result);
  } catch {
    renderJobs([]);
  }
}

function updateHistoryResult(command, result) {
  const workspace = activeWorkspace();
  const record = workspace?.history[workspace.history.length - 1];
  if (!record || record.command !== command) {
    return;
  }
  record.ok = Boolean(result.ok);
  record.stdout = result.stdout || "";
  record.stderr = result.stderr || result.error || "";
  if (!record.ok) {
    workspace.failed += 1;
  }
  historyFailed.textContent = String(workspace.failed);
  saveWorkspaces();
  renderHistory();
}

function renderHistory() {
  const workspace = activeWorkspace();
  const query = historyFilterInput.value.toLowerCase();
  historyTable.innerHTML = "";
  const records = (workspace?.history || []).filter((record) => record.command.toLowerCase().includes(query));
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
  const workspace = activeWorkspace();
  if (!workspace) {
    return;
  }
  workspace.history = [];
  state.historyIndex = -1;
  workspace.count = 0;
  workspace.failed = 0;
  historyList.innerHTML = "";
  historyTotal.textContent = "0";
  historyFailed.textContent = "0";
  commandCount.textContent = "0 commands";
  saveWorkspaces();
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
  const workspace = createWorkspace(`Workspace ${state.workspaces.length + 1}`);
  state.workspaces = [workspace, ...state.workspaces];
  state.activeWorkspaceId = workspace.id;
  saveWorkspaces();
  renderWorkspaces();
  loadWorkspace(workspace);
}

function loadWorkspaces() {
  try {
    const raw = localStorage.getItem(storageKey);
    if (!raw) {
      return [];
    }
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) {
      return [];
    }
    return parsed.map((workspace) => ({
      id: workspace.id || newSessionId(),
      sessionId: workspace.sessionId || newSessionId(),
      name: workspace.name || "Workspace",
      history: Array.isArray(workspace.history) ? workspace.history : [],
      count: Number(workspace.count) || 0,
      failed: Number(workspace.failed) || 0,
      closed: Boolean(workspace.closed),
      transcript: Array.isArray(workspace.transcript) ? workspace.transcript : []
    }));
  } catch {
    return [];
  }
}

function saveWorkspaces() {
  localStorage.setItem(storageKey, JSON.stringify(state.workspaces));
  queueWorkspaceSync();
}

function renderWorkspaces() {
  workspaceList.innerHTML = "";
  state.workspaces.forEach((workspace) => {
    const row = document.createElement("div");
    row.className = `workspace-item ${workspace.id === state.activeWorkspaceId ? "active" : ""}`;
    row.innerHTML = `
      <button type="button" class="workspace-open"></button>
      <button type="button" class="workspace-remove" aria-label="Remove workspace">x</button>
    `;
    row.querySelector(".workspace-open").textContent = workspace.name;
    row.querySelector(".workspace-open").addEventListener("click", () => {
      state.activeWorkspaceId = workspace.id;
      renderWorkspaces();
      loadWorkspace(workspace);
    });
    row.querySelector(".workspace-remove").addEventListener("click", () => removeWorkspace(workspace.id));
    workspaceList.append(row);
  });
}

function removeWorkspace(id) {
  if (state.workspaces.length === 1) {
    return;
  }
  state.workspaces = state.workspaces.filter((workspace) => workspace.id !== id);
  if (state.activeWorkspaceId === id) {
    state.activeWorkspaceId = state.workspaces[0].id;
    loadWorkspace(state.workspaces[0]);
  }
  saveWorkspaces();
  renderWorkspaces();
}

function renameActiveWorkspace() {
  const workspace = activeWorkspace();
  if (!workspace) {
    return;
  }
  workspace.name = workspaceNameInput.value.trim() || "Workspace";
  saveWorkspaces();
  renderWorkspaces();
}

function rememberTranscript(kind, text) {
  const workspace = activeWorkspace();
  if (!workspace) {
    return;
  }
  workspace.transcript = [...workspace.transcript, { kind, text }].slice(-200);
  saveWorkspaces();
}

function createWorkspace(name) {
  return {
    id: newSessionId(),
    sessionId: newSessionId(),
    name,
    history: [],
    count: 0,
    failed: 0,
    closed: false,
    transcript: []
  };
}

function activeWorkspace() {
  return state.workspaces.find((workspace) => workspace.id === state.activeWorkspaceId);
}

function loadWorkspace(workspace) {
  sessionStatus.textContent = "ready";
  workspaceNameInput.value = workspace.name;
  commandInput.disabled = false;
  terminalOutput.innerHTML = "";
  if (workspace.transcript.length === 0) {
    appendWelcome("Go Shell UI is connected to the local shell engine.");
  } else {
    workspace.transcript.forEach((item) => {
      if (item.kind === "command") {
        appendEntry(item.text, false);
      } else {
        terminalOutput.append(outputBlock(item.text, item.kind));
      }
    });
  }
  historyList.innerHTML = "";
  [...workspace.history].reverse().forEach((record) => historyList.append(historyListItem(record.command)));
  commandCount.textContent = `${workspace.count} commands`;
  historyTotal.textContent = String(workspace.count);
  historyFailed.textContent = String(workspace.failed);
  renderJobs([]);
  renderHistory();
  commandInput.focus();
}

function historyListItem(command) {
  const item = document.createElement("li");
  item.textContent = command;
  item.addEventListener("click", () => {
    commandInput.value = command;
    commandInput.focus();
  });
  return item;
}

async function startInteractiveStream() {
  const workspace = activeWorkspace();
  if (!workspace) {
    return;
  }
  if (state.stream) {
    state.stream.close();
  }
  const size = terminalSize();
  const response = await fetch("/api/pty/start", {
    method: "POST",
    headers: apiHeaders(),
    body: JSON.stringify({ sessionId: workspace.sessionId, command: "", cols: size.cols, rows: size.rows })
  });
  const result = await response.json();
  if (!result.ok) {
    appendEntry("interactive stream", true);
    appendResult({ ok: false, stderr: result.error, keepRunning: true });
    return;
  }
  appendWelcome(result.pty ? "Interactive PTY stream started." : "Interactive stream started.");
  const events = new EventSource(streamURL(workspace.sessionId));
  state.stream = events;
  sessionStatus.textContent = "streaming";
  resizeInteractiveStream();
  events.addEventListener("output", (event) => appendStreamOutput(event.data));
  events.addEventListener("close", () => {
    events.close();
    if (state.stream === events) {
      state.stream = null;
      sessionStatus.textContent = "ready";
    }
  });
  events.onerror = () => {
    events.close();
    if (state.stream === events) {
      state.stream = null;
      sessionStatus.textContent = "error";
    }
  };
  streamInput.focus();
}

async function sendStreamInput(data) {
  const workspace = activeWorkspace();
  if (!workspace || !state.stream || data.trim() === "") {
    return;
  }
  await fetch("/api/pty/input", {
    method: "POST",
    headers: apiHeaders(),
    body: JSON.stringify({ sessionId: workspace.sessionId, data })
  });
}

async function stopInteractiveStream() {
  const workspace = activeWorkspace();
  if (state.stream) {
    state.stream.close();
    state.stream = null;
  }
  if (workspace) {
    await fetch("/api/pty/stop", {
      method: "POST",
      headers: apiHeaders(),
      body: JSON.stringify({ sessionId: workspace.sessionId, data: "" })
    }).catch(() => {});
  }
  sessionStatus.textContent = "ready";
}

function appendStreamOutput(text) {
  const block = outputBlock(text, "stdout");
  terminalOutput.append(block);
  rememberTranscript("stdout", text);
  scrollTerminal();
}

function appendWelcome(text) {
  const line = document.createElement("div");
  line.className = "welcome-line";
  line.innerHTML = '<span class="status-chip success">READY</span><span></span>';
  line.querySelector("span:last-child").textContent = text;
  terminalOutput.append(line);
  scrollTerminal();
}

function newSessionId() {
  const bytes = new Uint32Array(2);
  crypto.getRandomValues(bytes);
  return `ui-${Date.now().toString(36)}-${bytes[0].toString(36)}${bytes[1].toString(36)}`;
}

function authToken() {
  const params = new URLSearchParams(window.location.search);
  const token = params.get("token") || params.get("access_token");
  if (token) {
    localStorage.setItem(tokenStorageKey, token);
    return token;
  }
  return localStorage.getItem(tokenStorageKey) || "";
}

function apiHeaders() {
  const headers = { "Content-Type": "application/json" };
  const token = authToken();
  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }
  return headers;
}

function streamURL(sessionId) {
  const params = new URLSearchParams({ sessionId });
  const token = authToken();
  if (token) {
    params.set("token", token);
  }
  return `/api/pty/stream?${params.toString()}`;
}

function queueWorkspaceSync() {
  clearTimeout(state.syncTimer);
  state.syncTimer = setTimeout(syncWorkspacesToServer, 250);
}

async function syncWorkspacesToServer() {
  try {
    await fetch("/api/workspaces", {
      method: "PUT",
      headers: apiHeaders(),
      body: JSON.stringify({ workspaces: state.workspaces })
    });
  } catch {
    // localStorage remains the fallback cache when server persistence is unavailable.
  }
}

async function hydrateWorkspacesFromServer() {
  try {
    const response = await fetch("/api/workspaces", { headers: apiHeaders() });
    if (!response.ok) {
      return;
    }
    const payload = await response.json();
    if (!Array.isArray(payload.workspaces) || payload.workspaces.length === 0) {
      queueWorkspaceSync();
      return;
    }
    state.workspaces = payload.workspaces.map((workspace) => ({
      id: workspace.id || newSessionId(),
      sessionId: workspace.sessionId || newSessionId(),
      name: workspace.name || "Workspace",
      history: Array.isArray(workspace.history) ? workspace.history : [],
      count: Number(workspace.count) || 0,
      failed: Number(workspace.failed) || 0,
      closed: Boolean(workspace.closed),
      transcript: Array.isArray(workspace.transcript) ? workspace.transcript : []
    }));
    state.activeWorkspaceId = state.workspaces[0].id;
    localStorage.setItem(storageKey, JSON.stringify(state.workspaces));
    renderWorkspaces();
    loadWorkspace(activeWorkspace());
  } catch {
    // The cached browser workspace is already loaded.
  }
}

function terminalSize() {
  const style = window.getComputedStyle(terminalOutput);
  const width = terminalOutput.clientWidth - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight);
  const height = terminalOutput.clientHeight - parseFloat(style.paddingTop) - parseFloat(style.paddingBottom);
  return {
    cols: Math.max(20, Math.floor(width / 8)),
    rows: Math.max(8, Math.floor(height / 18))
  };
}

async function resizeInteractiveStream() {
  const workspace = activeWorkspace();
  if (!workspace || !state.stream) {
    return;
  }
  const size = terminalSize();
  await fetch("/api/pty/resize", {
    method: "POST",
    headers: apiHeaders(),
    body: JSON.stringify({ sessionId: workspace.sessionId, cols: size.cols, rows: size.rows })
  }).catch(() => {});
}

function clearTerminal() {
  terminalOutput.innerHTML = "";
  const workspace = activeWorkspace();
  if (workspace) {
    workspace.transcript = [];
    saveWorkspaces();
  }
  appendWelcome("Terminal cleared.");
}

async function copyHistory() {
  const text = (activeWorkspace()?.history || []).map((record) => record.command).join("\n");
  if (navigator.clipboard && text) {
    await navigator.clipboard.writeText(text);
  }
}

function exportWorkspaces() {
  const payload = {
    exportedAt: new Date().toISOString(),
    workspaces: state.workspaces
  };
  downloadWorkspaceArchive(payload, "gosh-workspaces", "json");
}

async function exportEncryptedWorkspaces() {
  const passphrase = window.prompt("Archive passphrase");
  if (!passphrase) {
    return;
  }
  try {
    const payload = {
      exportedAt: new Date().toISOString(),
      workspaces: state.workspaces
    };
    const archive = await encryptWorkspaceArchive(payload, passphrase);
    downloadWorkspaceArchive(archive, "gosh-workspaces-encrypted", "json");
  } catch (error) {
    appendEntry("encrypt workspaces", true);
    appendResult({ ok: false, stderr: error.message, keepRunning: true });
  }
}

async function encryptWorkspaceArchive(payload, passphrase) {
  const salt = crypto.getRandomValues(new Uint8Array(16));
  const iv = crypto.getRandomValues(new Uint8Array(12));
  const key = await deriveArchiveKey(passphrase, salt);
  const encoded = new TextEncoder().encode(JSON.stringify(payload));
  const encrypted = await crypto.subtle.encrypt({ name: "AES-GCM", iv }, key, encoded);
  return {
    format: "gosh-workspace-archive",
    version: encryptedArchiveVersion,
    kdf: "PBKDF2-SHA-256",
    iterations: 250000,
    cipher: "AES-GCM",
    salt: base64Encode(salt),
    iv: base64Encode(iv),
    data: base64Encode(new Uint8Array(encrypted))
  };
}

async function importWorkspaces(event) {
  const file = event.target.files?.[0];
  event.target.value = "";
  if (!file) {
    return;
  }
  try {
    const payload = JSON.parse(await file.text());
    replaceWorkspacesFromArchive(payload);
    appendWelcome("Workspace archive imported.");
  } catch (error) {
    appendEntry("import workspaces", true);
    appendResult({ ok: false, stderr: error.message, keepRunning: true });
  }
}

async function importEncryptedWorkspaces(event) {
  const file = event.target.files?.[0];
  event.target.value = "";
  if (!file) {
    return;
  }
  const passphrase = window.prompt("Archive passphrase");
  if (!passphrase) {
    return;
  }
  try {
    const archive = JSON.parse(await file.text());
    const payload = await decryptWorkspaceArchive(archive, passphrase);
    replaceWorkspacesFromArchive(payload);
    appendWelcome("Encrypted workspace archive imported.");
  } catch (error) {
    appendEntry("decrypt workspaces", true);
    appendResult({ ok: false, stderr: error.message, keepRunning: true });
  }
}

async function decryptWorkspaceArchive(archive, passphrase) {
  if (archive?.format !== "gosh-workspace-archive" || archive.version !== encryptedArchiveVersion) {
    throw new Error("Unsupported encrypted workspace archive.");
  }
  const salt = base64Decode(archive.salt);
  const iv = base64Decode(archive.iv);
  const encrypted = base64Decode(archive.data);
  const key = await deriveArchiveKey(passphrase, salt);
  const decrypted = await crypto.subtle.decrypt({ name: "AES-GCM", iv }, key, encrypted);
  return JSON.parse(new TextDecoder().decode(decrypted));
}

async function runEncryptedArchiveWorkflowCheck(passphrase = "gosh-workflow-check") {
  const payload = {
    exportedAt: "check",
    workspaces: [createWorkspace("Archive Check")]
  };
  const archive = await encryptWorkspaceArchive(payload, passphrase);
  const restored = await decryptWorkspaceArchive(archive, passphrase);
  return restored.workspaces?.[0]?.name === "Archive Check";
}

async function deriveArchiveKey(passphrase, salt) {
  const material = await crypto.subtle.importKey(
    "raw",
    new TextEncoder().encode(passphrase),
    "PBKDF2",
    false,
    ["deriveKey"]
  );
  return crypto.subtle.deriveKey(
    { name: "PBKDF2", salt, iterations: 250000, hash: "SHA-256" },
    material,
    { name: "AES-GCM", length: 256 },
    false,
    ["encrypt", "decrypt"]
  );
}

function replaceWorkspacesFromArchive(payload) {
  const imported = Array.isArray(payload) ? payload : payload.workspaces;
  if (!Array.isArray(imported) || imported.length === 0) {
    throw new Error("No workspaces found in archive.");
  }
  state.workspaces = imported.map((workspace) => ({
    id: workspace.id || newSessionId(),
    sessionId: workspace.sessionId || newSessionId(),
    name: workspace.name || "Workspace",
    history: Array.isArray(workspace.history) ? workspace.history : [],
    count: Number(workspace.count) || 0,
    failed: Number(workspace.failed) || 0,
    closed: Boolean(workspace.closed),
    transcript: Array.isArray(workspace.transcript) ? workspace.transcript : []
  }));
  state.activeWorkspaceId = state.workspaces[0].id;
  saveWorkspaces();
  renderWorkspaces();
  loadWorkspace(activeWorkspace());
}

function downloadWorkspaceArchive(payload, name, extension) {
  const blob = new Blob([JSON.stringify(payload, null, 2)], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = `${name}-${new Date().toISOString().slice(0, 10)}.${extension}`;
  document.body.append(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

function base64Encode(bytes) {
  let binary = "";
  bytes.forEach((byte) => {
    binary += String.fromCharCode(byte);
  });
  return btoa(binary);
}

function base64Decode(value) {
  const binary = atob(value);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes;
}

function setBusy(isBusy) {
  const workspace = activeWorkspace();
  sessionStatus.textContent = isBusy ? "running" : sessionStatus.textContent;
  commandInput.disabled = isBusy || Boolean(workspace?.closed);
}

function scrollTerminal() {
  terminalOutput.scrollTop = terminalOutput.scrollHeight;
}

commandInput.focus();
renderWorkspaces();
loadWorkspace(activeWorkspace());
hydrateWorkspacesFromServer();
