const terminalOutput = document.querySelector("#terminalOutput");
const commandForm = document.querySelector("#commandForm");
const commandInput = document.querySelector("#commandInput");
const historyList = document.querySelector("#historyList");
const jobsList = document.querySelector("#jobsList");
const sessionStatus = document.querySelector("#sessionStatus");
const latencyStatus = document.querySelector("#latencyStatus");
const commandCount = document.querySelector("#commandCount");
const searchInput = document.querySelector("#searchInput");

const state = {
  history: [],
  historyIndex: -1,
  count: 0,
  closed: false
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
document.querySelector("#newSessionButton").addEventListener("click", () => window.location.reload());
document.querySelector("#copyHistoryButton").addEventListener("click", copyHistory);

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

async function runCommand(command) {
  appendEntry(command);
  pushHistory(command);
  setBusy(true);
  const start = performance.now();

  try {
    const response = await fetch("/api/execute", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ command })
    });
    const result = await response.json();
    const elapsed = Math.round(performance.now() - start);
    latencyStatus.textContent = `${elapsed}ms`;
    appendResult(result);
    updateJobs(command, result);
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
  if (result.stderr || result.error) {
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
  state.history.push(command);
  state.historyIndex = state.history.length;
  state.count += 1;
  commandCount.textContent = `${state.count} commands`;

  const item = document.createElement("li");
  item.textContent = command;
  item.addEventListener("click", () => {
    commandInput.value = command;
    commandInput.focus();
  });
  historyList.prepend(item);
}

function moveHistory(direction) {
  if (state.history.length === 0) {
    return;
  }
  state.historyIndex = Math.max(0, Math.min(state.history.length, state.historyIndex + direction));
  commandInput.value = state.history[state.historyIndex] || "";
}

function updateJobs(command, result) {
  if (command !== "jobs" || !result.stdout) {
    return;
  }
  jobsList.innerHTML = "";
  const lines = result.stdout.trim().split(/\r?\n/).filter(Boolean);
  if (lines.length === 0) {
    jobsList.innerHTML = '<div class="empty-state">No background jobs reported.</div>';
    return;
  }
  lines.forEach((line) => {
    const row = document.createElement("div");
    row.className = "empty-state";
    row.textContent = line;
    jobsList.append(row);
  });
}

function clearTerminal() {
  terminalOutput.innerHTML = "";
  const line = document.createElement("div");
  line.className = "welcome-line";
  line.innerHTML = '<span class="status-chip success">READY</span><span>Terminal cleared.</span>';
  terminalOutput.append(line);
}

async function copyHistory() {
  const text = state.history.join("\n");
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
