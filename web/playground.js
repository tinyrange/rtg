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
    // Show download for non-wasm targets, or always after compile
    markDirty();
    compiledWasm = null;
    btnRun.disabled = true;
    btnDownload.disabled = true;
    btnDownload.classList.add("hidden");
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
    default: return "program";
  }
}

function getMimeType(target) {
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
    const args = ["rtg", "-T", target, "-size-analysis", "size-analysis.json", "-o", outputFile, entryFile];

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
  container.innerHTML = "";
  container.classList.remove("hidden");

  const header = document.createElement("div");
  header.className = "size-header";
  header.textContent = `Size: ${formatSize(total)} (${data.functions.length} functions)`;
  container.appendChild(header);

  for (const [pkg, info] of sorted) {
    const pct = total > 0 ? ((info.size / total) * 100).toFixed(1) : "0.0";

    const pkgEl = document.createElement("details");
    pkgEl.className = "size-pkg";

    const summary = document.createElement("summary");
    summary.innerHTML = `<span class="size-pkg-name">${pkg}</span> <span class="size-pkg-size">${formatSize(info.size)} (${pct}%)</span>`;
    pkgEl.appendChild(summary);

    // Sort functions by size descending within package
    info.funcs.sort((a, b) => b.size - a.size);

    const list = document.createElement("div");
    list.className = "size-func-list";
    for (const f of info.funcs) {
      const fpct = info.size > 0 ? ((f.size / info.size) * 100).toFixed(1) : "0.0";
      const shortName = f.name.indexOf(".") >= 0 ? f.name.substring(f.name.indexOf(".") + 1) : f.name;
      const row = document.createElement("div");
      row.className = "size-func-row";
      row.innerHTML = `<span class="size-func-name">${shortName}</span><span class="size-func-size">${formatSize(f.size)} (${fpct}%)</span>`;
      list.appendChild(row);
    }
    pkgEl.appendChild(list);
    container.appendChild(pkgEl);
  }
}

// --- Start ---
init();
