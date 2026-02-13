import { VirtualFS, createWASI, WASIExit } from "./wasi.js";
import { STD_LIBRARY } from "./std-library.js";

// --- State ---
let userFiles = {};       // { path: content }
let activeFile = null;    // currently open file path
let compiledWasm = null;  // Uint8Array of compiled output
let compiledTarget = null; // target the last compile was for
let compilerModule = null; // cached WebAssembly.Module for the compiler
let saveTimer = null;
let dirty = true;         // files changed since last compile

// --- DOM ---
const editor = document.getElementById("editor");
const editorFilename = document.getElementById("editor-filename");
const editorReadonlyBadge = document.getElementById("editor-readonly-badge");
const editorDirty = document.getElementById("editor-dirty");
const btnDeleteFile = document.getElementById("btn-delete-file");
const fileTree = document.getElementById("file-tree");
const outputContent = document.getElementById("output-content");
const statusEl = document.getElementById("status");
const btnCompile = document.getElementById("btn-compile");
const btnRun = document.getElementById("btn-run");
const btnDownload = document.getElementById("btn-download");
const btnNewFile = document.getElementById("btn-new-file");
const btnClearOutput = document.getElementById("btn-clear-output");
const targetSelect = document.getElementById("target-select");
const panelOutput = document.getElementById("panel-output");
const irViewer = document.getElementById("ir-viewer");
const irContent = document.getElementById("ir-content");
const irFuncList = document.getElementById("ir-func-list");
const irFuncSearch = document.getElementById("ir-func-search");

// --- Init ---
function init() {
  loadUserFiles();
  renderFileTree();
  openFile("user/main.go");
  setupEditorEvents();
  setupButtons();
  loadCompilerModule();
  updateDirtyIndicator();
}

// --- LocalStorage ---
function loadUserFiles() {
  try {
    const stored = localStorage.getItem("rtg2-user-files");
    if (stored) {
      userFiles = JSON.parse(stored);
      return;
    }
  } catch {}
  // Default
  userFiles = {
    "user/main.go": `package main

import "fmt"

func main() {
\tfmt.Println("Hello from RTG2!")
}
`,
  };
  saveUserFiles();
}

function saveUserFiles() {
  localStorage.setItem("rtg2-user-files", JSON.stringify(userFiles));
}

// --- Dirty tracking ---
function markDirty() {
  dirty = true;
  updateDirtyIndicator();
}

function markClean() {
  dirty = false;
  updateDirtyIndicator();
}

function updateDirtyIndicator() {
  editorDirty.classList.toggle("hidden", !dirty);
}

// --- File Tree ---
function renderFileTree() {
  fileTree.innerHTML = "";

  // User files section
  const userSection = buildTreeSection("user", userFiles, false);
  fileTree.appendChild(userSection);

  // Std library section
  const stdSection = buildTreeSection("std", STD_LIBRARY, true);
  fileTree.appendChild(stdSection);
}

function buildTreeSection(rootName, filesObj, readOnly) {
  const tree = {};
  for (const path of Object.keys(filesObj)) {
    const parts = path.split("/");
    let node = tree;
    for (let i = 0; i < parts.length; i++) {
      const p = parts[i];
      if (i === parts.length - 1) {
        node[p] = null;
      } else {
        if (!node[p]) node[p] = {};
        node = node[p];
      }
    }
  }

  const rootNode = tree[rootName];
  if (!rootNode) return document.createDocumentFragment();
  return renderTreeNode(rootName, rootNode, rootName, readOnly, !readOnly);
}

function renderTreeNode(name, children, fullPath, readOnly, startOpen) {
  if (children === null) {
    const div = document.createElement("div");
    div.className = "tree-item px-3 py-0.5";
    div.style.paddingLeft = (fullPath.split("/").length * 12 + 12) + "px";
    div.textContent = name;
    div.dataset.path = fullPath;
    div.dataset.readonly = readOnly;
    div.addEventListener("click", () => openFile(fullPath));
    return div;
  }

  const container = document.createElement("div");

  const header = document.createElement("div");
  header.className = "tree-item px-3 py-0.5 font-medium text-gray-300";
  header.style.paddingLeft = (fullPath.split("/").length * 12) + "px";
  const arrow = document.createElement("span");
  arrow.className = "inline-block w-4 text-center text-gray-500 transition-transform";
  arrow.textContent = startOpen ? "\u25BE" : "\u25B8";
  header.appendChild(arrow);
  header.appendChild(document.createTextNode(" " + name));

  const childrenDiv = document.createElement("div");
  childrenDiv.className = "dir-children" + (startOpen ? " open" : "");

  header.addEventListener("click", () => {
    const isOpen = childrenDiv.classList.toggle("open");
    arrow.textContent = isOpen ? "\u25BE" : "\u25B8";
  });

  const entries = Object.entries(children).sort((a, b) => {
    const aIsDir = a[1] !== null;
    const bIsDir = b[1] !== null;
    if (aIsDir !== bIsDir) return aIsDir ? -1 : 1;
    return a[0].localeCompare(b[0]);
  });

  for (const [childName, childNode] of entries) {
    childrenDiv.appendChild(renderTreeNode(childName, childNode, fullPath + "/" + childName, readOnly, false));
  }

  container.appendChild(header);
  container.appendChild(childrenDiv);
  return container;
}

// --- Editor ---
function openFile(path) {
  saveCurrentFile();

  activeFile = path;
  editorFilename.textContent = path;

  const isUserFile = path.startsWith("user/");

  if (isUserFile) {
    editor.value = userFiles[path] || "";
    editor.readOnly = false;
    editorReadonlyBadge.classList.add("hidden");
    btnDeleteFile.classList.toggle("hidden", path === "user/main.go");
  } else {
    editor.value = STD_LIBRARY[path] || "";
    editor.readOnly = true;
    editorReadonlyBadge.classList.remove("hidden");
    btnDeleteFile.classList.add("hidden");
  }

  for (const el of fileTree.querySelectorAll(".tree-item")) {
    el.classList.toggle("active", el.dataset.path === path);
  }
}

function saveCurrentFile() {
  if (activeFile && activeFile.startsWith("user/") && !editor.readOnly) {
    userFiles[activeFile] = editor.value;
    if (saveTimer) clearTimeout(saveTimer);
    saveUserFiles();
  }
}

function setupEditorEvents() {
  editor.addEventListener("keydown", (e) => {
    if (e.key === "Tab") {
      e.preventDefault();
      const start = editor.selectionStart;
      const end = editor.selectionEnd;
      editor.value = editor.value.substring(0, start) + "\t" + editor.value.substring(end);
      editor.selectionStart = editor.selectionEnd = start + 1;
      markDirty();
    }
  });

  editor.addEventListener("input", () => {
    if (activeFile && activeFile.startsWith("user/")) {
      markDirty();
      if (saveTimer) clearTimeout(saveTimer);
      saveTimer = setTimeout(() => {
        userFiles[activeFile] = editor.value;
        saveUserFiles();
      }, 500);
    }
  });
}

function setupButtons() {
  btnCompile.addEventListener("click", compile);
  btnRun.addEventListener("click", run);
  btnClearOutput.addEventListener("click", () => { outputContent.innerHTML = ""; });

  btnDownload.addEventListener("click", downloadBinary);

  targetSelect.addEventListener("change", () => {
    // Target change means recompile is needed
    const target = targetSelect.value;
    const canRun = target === "wasi/wasm32";
    btnRun.classList.toggle("hidden", !canRun);
    markDirty();
    compiledWasm = null;
    btnRun.disabled = true;
    btnDownload.disabled = true;
    btnDownload.classList.add("hidden");
    showIRViewer(false);
  });

  btnNewFile.addEventListener("click", () => {
    const name = prompt("File path (e.g. user/lib.go):");
    if (!name) return;
    const path = name.startsWith("user/") ? name : "user/" + name;
    if (userFiles[path]) {
      openFile(path);
      return;
    }
    userFiles[path] = "package main\n";
    saveUserFiles();
    markDirty();
    renderFileTree();
    openFile(path);
  });

  btnDeleteFile.addEventListener("click", () => {
    if (!activeFile || !activeFile.startsWith("user/") || activeFile === "user/main.go") return;
    if (!confirm("Delete " + activeFile + "?")) return;
    delete userFiles[activeFile];
    saveUserFiles();
    markDirty();
    renderFileTree();
    openFile("user/main.go");
  });
}

// --- Target helpers ---
function getOutputFilename(target) {
  switch (target) {
    case "wasi/wasm32": return "output.wasm";
    case "windows/386": return "output.exe";
    case "c/64": case "c/32": case "c/16": return "output.c";
    case "ir": return "output.ir";
    default: return "output";
  }
}

function getDownloadFilename(target) {
  switch (target) {
    case "wasi/wasm32": return "program.wasm";
    case "linux/amd64": return "program-linux-amd64";
    case "linux/386": return "program-linux-386";
    case "darwin/arm64": return "program-darwin-arm64";
    case "windows/386": return "program.exe";
    case "c/64": return "program-c64.c";
    case "c/32": return "program-c32.c";
    case "c/16": return "program-c16.c";
    case "ir": return "program.ir";
    default: return "program";
  }
}

function getMimeType(target) {
  if (target === "ir") return "text/plain";
  if (target.startsWith("c/")) return "text/x-csrc";
  if (target === "wasi/wasm32") return "application/wasm";
  return "application/octet-stream";
}

// --- Compiler Module Loading ---
async function loadCompilerModule() {
  setStatus("Loading compiler...");
  try {
    const response = await fetch("compiler.wasm");
    if (!response.ok) {
      setStatus("Failed to load compiler.wasm");
      return;
    }
    compilerModule = await WebAssembly.compileStreaming(fetch("compiler.wasm"));
    setStatus("Ready");
  } catch (e) {
    setStatus("Error loading compiler: " + e.message);
  }
}

// --- Compile ---
async function compile() {
  saveCurrentFile();

  if (!compilerModule) {
    appendOutput("Compiler not loaded yet\n", "stderr");
    return;
  }

  const target = targetSelect.value;
  const outputFile = getOutputFilename(target);

  outputContent.innerHTML = "";
  compiledWasm = null;
  btnRun.disabled = true;
  btnDownload.disabled = true;
  btnDownload.classList.add("hidden");
  setStatus("Compiling...");
  btnCompile.disabled = true;

  const t0 = performance.now();

  try {
    const fs = new VirtualFS();

    // Add std library files to VirtualFS
    for (const [path, content] of Object.entries(STD_LIBRARY)) {
      fs.addFile(path, content);
    }

    // Add all user files
    for (const [path, content] of Object.entries(userFiles)) {
      fs.addFile(path, content);
    }

    const entryFile = "user/main.go";
    const isIR = target === "ir";
    const args = ["rtg", "-T", target];
    if (!isIR) args.push("-size-analysis", "size-analysis.json");
    args.push("-o", outputFile, entryFile);

    const wasi = createWASI(fs, args, {
      onStdout: (data) => appendOutput(new TextDecoder().decode(data), "stdout"),
      onStderr: (data) => appendOutput(new TextDecoder().decode(data), "stderr"),
    });

    const instance = await WebAssembly.instantiate(compilerModule, wasi.imports);
    wasi.setMemory(instance.exports.memory);

    let exitCode = 0;
    try {
      instance.exports._start();
    } catch (e) {
      if (e instanceof WASIExit) {
        exitCode = e.code;
      } else {
        throw e;
      }
    }

    const elapsed = ((performance.now() - t0) / 1000).toFixed(2);

    if (exitCode !== 0) {
      appendOutput(`\nCompilation failed (exit ${exitCode}) in ${elapsed}s\n`, "stderr");
      setStatus("Compilation failed");
      return;
    }

    const output = fs.readFile(outputFile);
    if (output && output.length > 0) {
      compiledWasm = output;
      compiledTarget = target;
      markClean();

      if (isIR) {
        const irText = new TextDecoder().decode(output);
        appendOutput(`IR generated (${irText.split("\n").length} lines) in ${elapsed}s\n`, "stdout");
        renderIRViewer(irText);
        showIRViewer(true);
        btnRun.disabled = true;
        btnDownload.disabled = false;
        btnDownload.classList.remove("hidden");
      } else {
        showIRViewer(false);
        const canRun = target === "wasi/wasm32";
        btnRun.disabled = !canRun;
        btnDownload.disabled = false;
        btnDownload.classList.remove("hidden");

        appendOutput(`Compiled ${output.length} bytes in ${elapsed}s\n`, "stdout");

        // Read and display size analysis
        try {
          const sizeData = fs.readFile("size-analysis.json");
          if (sizeData && sizeData.length > 0) {
            const json = new TextDecoder().decode(sizeData);
            const analysis = JSON.parse(json);
            renderSizeAnalysis(analysis);
          }
        } catch (e) {
          // Size analysis is optional, don't fail compile on error
        }
      }

      setStatus(`Compiled (${elapsed}s)`);
    } else {
      appendOutput(`No output produced (${elapsed}s)\n`, "stderr");
      setStatus("Compilation failed");
    }
  } catch (e) {
    const elapsed = ((performance.now() - t0) / 1000).toFixed(2);
    appendOutput("Error: " + e.message + "\n" + e.stack + "\n", "stderr");
    setStatus(`Compilation error (${elapsed}s)`);
  } finally {
    btnCompile.disabled = false;
  }
}

// --- Run ---
async function run() {
  // Auto-compile if dirty
  if (dirty || !compiledWasm || compiledTarget !== "wasi/wasm32") {
    await compile();
    if (!compiledWasm || compiledTarget !== "wasi/wasm32") return;
  }

  setStatus("Running...");
  btnRun.disabled = true;

  if (outputContent.textContent) {
    appendOutput("\n--- Program Output ---\n", "stdout");
  }

  const t0 = performance.now();

  try {
    const fs = new VirtualFS();
    const args = ["program"];

    const wasi = createWASI(fs, args, {
      onStdout: (data) => appendOutput(new TextDecoder().decode(data), "stdout"),
      onStderr: (data) => appendOutput(new TextDecoder().decode(data), "stderr"),
    });

    const module = await WebAssembly.compile(compiledWasm);
    const instance = await WebAssembly.instantiate(module, wasi.imports);
    wasi.setMemory(instance.exports.memory);

    let exitCode = 0;
    try {
      instance.exports._start();
    } catch (e) {
      if (e instanceof WASIExit) {
        exitCode = e.code;
      } else {
        throw e;
      }
    }

    const elapsed = ((performance.now() - t0) / 1000).toFixed(2);
    appendOutput(`\nExit code: ${exitCode} (${elapsed}s)\n`, exitCode === 0 ? "stdout" : "stderr");
    setStatus(`Done (exit ${exitCode}, ${elapsed}s)`);
  } catch (e) {
    appendOutput("Runtime error: " + e.message + "\n" + e.stack + "\n", "stderr");
    setStatus("Runtime error");
  } finally {
    btnRun.disabled = false;
  }
}

// --- Download ---
function downloadBinary() {
  if (!compiledWasm) return;
  const filename = getDownloadFilename(compiledTarget);
  const mime = getMimeType(compiledTarget);
  const blob = new Blob([compiledWasm], { type: mime });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

// --- UI Helpers ---
function appendOutput(text, type) {
  const span = document.createElement("span");
  span.className = type;
  span.textContent = text;
  outputContent.appendChild(span);
  outputContent.scrollTop = outputContent.scrollHeight;
}

function setStatus(text) {
  statusEl.textContent = text;
}

// --- Size Analysis ---
function formatSize(bytes) {
  if (bytes >= 1024) return (bytes / 1024).toFixed(1) + " KB";
  return bytes + " B";
}

function renderSizeAnalysis(data) {
  if (!data || !data.functions || data.functions.length === 0) return;

  // Aggregate by package
  const pkgs = {};
  for (const f of data.functions) {
    const pkg = f.pkg || "unknown";
    if (!pkgs[pkg]) pkgs[pkg] = { size: 0, funcs: [] };
    pkgs[pkg].size += f.size;
    pkgs[pkg].funcs.push(f);
  }

  // Sort packages by size descending
  const sorted = Object.entries(pkgs).sort((a, b) => b[1].size - a[1].size);
  const total = data.total || sorted.reduce((s, [, v]) => s + v.size, 0);

  // Build output
  const container = document.getElementById("size-analysis");
  container.replaceChildren();
  container.classList.remove("hidden");

  const header = document.createElement("div");
  header.className = "size-header";
  header.textContent = `Size: ${formatSize(total)} (${data.functions.length} functions)`;
  container.appendChild(header);

  for (const [pkg, info] of sorted) {
    const pct = total > 0 ? ((info.size / total) * 100).toFixed(1) : "0.0";

    const pkgEl = document.createElement("details");
    pkgEl.className = "size-pkg";

    const summary = el("summary", null,
      span("size-pkg-name", pkg), text(" "),
      span("size-pkg-size", `${formatSize(info.size)} (${pct}%)`));
    pkgEl.appendChild(summary);

    // Sort functions by size descending within package
    info.funcs.sort((a, b) => b.size - a.size);

    const list = document.createElement("div");
    list.className = "size-func-list";
    for (const f of info.funcs) {
      const fpct = info.size > 0 ? ((f.size / info.size) * 100).toFixed(1) : "0.0";
      const shortName = f.name.indexOf(".") >= 0 ? f.name.substring(f.name.indexOf(".") + 1) : f.name;
      const row = el("div", "size-func-row",
        span("size-func-name", shortName),
        span("size-func-size", `${formatSize(f.size)} (${fpct}%)`));
      list.appendChild(row);
    }
    pkgEl.appendChild(list);
    container.appendChild(pkgEl);
  }
}

// --- IR Viewer ---
function showIRViewer(show) {
  if (show) {
    panelOutput.classList.add("ir-active");
    irViewer.classList.remove("hidden");
    document.getElementById("size-analysis").classList.add("hidden");
  } else {
    panelOutput.classList.remove("ir-active");
    irViewer.classList.add("hidden");
    irContent.replaceChildren();
    irFuncList.replaceChildren();
    irFuncSearch.value = "";
  }
}

// --- DOM helpers ---
function el(tag, cls, ...children) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  for (const c of children) e.append(c);
  return e;
}

function span(cls, s) { return el("span", cls, s); }
function text(s) { return document.createTextNode(s); }

function renderIRViewer(irText) {
  const lines = irText.split("\n");

  // Parse function names and their line indices
  const funcs = [];
  for (let i = 0; i < lines.length; i++) {
    const m = lines[i].match(/^func\s+(\S+)/);
    if (m) funcs.push({ name: m[1], line: i });
  }

  // Render function nav buttons
  irFuncList.replaceChildren();
  for (const f of funcs) {
    const btn = el("button", "ir-func-btn", f.name);
    btn.addEventListener("click", () => {
      const target = document.getElementById("ir-line-" + f.line);
      if (target) target.scrollIntoView({ behavior: "smooth", block: "start" });
    });
    irFuncList.append(btn);
  }

  // Filter input
  irFuncSearch.oninput = () => {
    const q = irFuncSearch.value.toLowerCase();
    for (const btn of irFuncList.children) {
      btn.style.display = btn.textContent.toLowerCase().includes(q) ? "" : "none";
    }
  };

  // Render highlighted lines
  irContent.replaceChildren();
  for (let i = 0; i < lines.length; i++) {
    const div = el("div", "ir-line", ...highlightIRLine(lines[i]));
    div.id = "ir-line-" + i;
    irContent.append(div);
  }
}

// Returns an array of DOM nodes for a single IR line.
function highlightIRLine(line) {
  if (!line) return [text("\n")];

  // Section comments: "; === ... ==="
  if (/^;\s*===/.test(line)) return [span("ir-section-comment", line)];

  // Full-line comments
  if (/^;/.test(line)) return [span("ir-comment", line)];

  // "end" keyword
  if (/^end\b/.test(line)) return [span("ir-keyword", "end")];

  // func declaration
  const funcMatch = line.match(/^(func)\s+(\S+)(.*)/);
  if (funcMatch) {
    return [span("ir-keyword", "func"), text(" "), span("ir-func-name", funcMatch[2]), ...tokenizeArgs(funcMatch[3])];
  }

  // Other declarations
  const declMatch = line.match(/^(\s*)(global|local|type|typeid|method|interface)\b(.*)/);
  if (declMatch) {
    return [text(declMatch[1]), span("ir-keyword", declMatch[2]), ...tokenizeArgs(declMatch[3])];
  }

  // Instructions: "  NNNN: opcode args"
  const instrMatch = line.match(/^(\s*)(\d{4}:)\s+(\S+)(.*)/);
  if (instrMatch) {
    return [text(instrMatch[1]), span("ir-addr", instrMatch[2]), text(" "), span("ir-opcode", instrMatch[3]), ...tokenizeArgs(instrMatch[4])];
  }

  return [text(line)];
}

// Tokenize raw text into an array of DOM nodes.
function tokenizeArgs(raw) {
  if (!raw) return [];
  const nodes = [];
  let i = 0;
  while (i < raw.length) {
    // Comment to end of line
    if (raw[i] === ";") {
      nodes.push(span("ir-comment", raw.slice(i)));
      break;
    }
    // Quoted string
    if (raw[i] === '"') {
      const end = raw.indexOf('"', i + 1);
      const s = end >= 0 ? raw.slice(i, end + 1) : raw.slice(i);
      nodes.push(span("ir-string", s));
      i += s.length;
      continue;
    }
    // key=value param
    const pm = raw.slice(i).match(/^([a-zA-Z_]\w*=\S+)/);
    if (pm) {
      nodes.push(span("ir-param", pm[0]));
      i += pm[0].length;
      continue;
    }
    // Boolean
    const bm = raw.slice(i).match(/^(true|false)\b/);
    if (bm) {
      nodes.push(span("ir-bool", bm[0]));
      i += bm[0].length;
      continue;
    }
    // Number
    const nm = raw.slice(i).match(/^-?\d+/);
    if (nm && (i === 0 || /[\s(=]/.test(raw[i - 1]))) {
      nodes.push(span("ir-number", nm[0]));
      i += nm[0].length;
      continue;
    }
    // Plain text: consume until next interesting char
    let j = i + 1;
    while (j < raw.length && !';"\n'.includes(raw[j]) && !/[a-zA-Z_\d]/.test(raw[j])) j++;
    if (j === i + 1 && /[a-zA-Z_\d]/.test(raw[i])) {
      while (j < raw.length && /[a-zA-Z_\d]/.test(raw[j])) j++;
    }
    nodes.push(text(raw.slice(i, j)));
    i = j;
  }
  return nodes;
}

// --- Resizable Panels ---
function setupResizeHandles() {
  setupResize("resize-files", "panel-files", "left");
  setupResize("resize-output", "panel-output", "right");
}

function setupResize(handleId, panelId, side) {
  const handle = document.getElementById(handleId);
  const panel = document.getElementById(panelId);
  if (!handle || !panel) return;

  let startX, startW;

  handle.addEventListener("mousedown", (e) => {
    e.preventDefault();
    startX = e.clientX;
    startW = panel.offsetWidth;
    handle.classList.add("dragging");
    document.body.classList.add("resizing");

    function onMove(e) {
      const dx = e.clientX - startX;
      const newW = side === "left" ? startW + dx : startW - dx;
      panel.style.width = Math.max(80, newW) + "px";
    }

    function onUp() {
      handle.classList.remove("dragging");
      document.body.classList.remove("resizing");
      document.removeEventListener("mousemove", onMove);
      document.removeEventListener("mouseup", onUp);
    }

    document.addEventListener("mousemove", onMove);
    document.addEventListener("mouseup", onUp);
  });
}

// --- Start ---
setupResizeHandles();
init();
