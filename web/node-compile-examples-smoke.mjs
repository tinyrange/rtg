#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const repoRoot = path.resolve(__dirname, "..");

function fail(message) {
  console.error(`error: ${message}`);
  process.exit(1);
}

function loadExportedJSON(relPath, exportName) {
  const fullPath = path.join(repoRoot, relPath);
  const src = fs.readFileSync(fullPath, "utf8");
  const marker = `export const ${exportName} = `;
  const start = src.indexOf(marker);
  if (start < 0) fail(`${relPath} does not contain ${exportName} export`);
  const jsonStart = start + marker.length;
  const jsonEnd = src.lastIndexOf(";");
  if (jsonEnd <= jsonStart) fail(`failed to parse ${exportName} payload from ${relPath}`);
  try {
    return JSON.parse(src.slice(jsonStart, jsonEnd));
  } catch (err) {
    fail(`failed to parse ${exportName} from ${relPath}: ${err.message}`);
  }
}

function asText(chunks) {
  return Buffer.concat(chunks).toString("utf8");
}

async function runWasiModule(module, fsView, args, hooks) {
  const stdout = [];
  const stderr = [];
  const { createWASI, WASIExit } = hooks;

  const wasi = createWASI(fsView, args, {
    onStdout: (data) => stdout.push(Buffer.from(data)),
    onStderr: (data) => stderr.push(Buffer.from(data)),
  });

  const instance = await WebAssembly.instantiate(module, wasi.imports);
  wasi.setMemory(instance.exports.memory);

  let exitCode = 0;
  try {
    instance.exports._start();
  } catch (err) {
    if (err instanceof WASIExit) {
      exitCode = err.code;
    } else {
      throw err;
    }
  }

  return {
    exitCode,
    stdout: asText(stdout),
    stderr: asText(stderr),
  };
}

function addLibrary(fsView, lib) {
  for (const [filePath, content] of Object.entries(lib)) {
    fsView.addFile(filePath, content);
  }
}

function packageEntry(pathname) {
  const slash = pathname.lastIndexOf("/");
  return slash >= 0 ? pathname.slice(0, slash) : pathname;
}

async function main() {
  const tmpDir = path.join(repoRoot, "build", "web-wasm-selfhost-node");
  fs.mkdirSync(tmpDir, { recursive: true });
  const wasiModulePath = path.join(tmpDir, "wasi.mjs");
  fs.copyFileSync(path.join(repoRoot, "web", "wasi.js"), wasiModulePath);

  const wasiModule = await import(pathToFileURL(wasiModulePath).href);
  const { VirtualFS, createWASI, WASIExit } = wasiModule;
  if (!VirtualFS || !createWASI || !WASIExit) {
    fail("failed to load WASI helpers from web/wasi.js");
  }

  const stdLibrary = loadExportedJSON("web/std-library.js", "STD_LIBRARY");
  const xLibrary = loadExportedJSON("web/x-library.js", "X_LIBRARY");
  const examples = loadExportedJSON("web/examples-library.js", "EXAMPLE_LIBRARY");

  const compilerWasmPath = path.join(repoRoot, "web", "compiler.wasm");
  const compilerWasm = fs.readFileSync(compilerWasmPath);
  const compilerModule = await WebAssembly.compile(compilerWasm);

  const fsStage1 = new VirtualFS();
  addLibrary(fsStage1, stdLibrary);
  addLibrary(fsStage1, xLibrary);
  const stage1Out = "build/examples_stage1_compiler.wasm";
  const stage1 = await runWasiModule(
    compilerModule,
    fsStage1,
    ["rtg", "-T", "wasi/wasm32", "-o", stage1Out, "std/compiler"],
    { createWASI, WASIExit }
  );
  if (stage1.exitCode !== 0) {
    fail(`webpage WASM compiler self-compile failed with ${stage1.exitCode}\n${(stage1.stdout + stage1.stderr).trim()}`);
  }
  const stage1Bytes = fsStage1.readFile(stage1Out);
  if (!stage1Bytes || stage1Bytes.length === 0) {
    fail(`self-compiled compiler did not produce ${stage1Out}`);
  }
  const generatedCompilerModule = await WebAssembly.compile(stage1Bytes);
  let compiledCount = 0;
  let skippedCount = 0;
  const failures = [];

  for (let i = 0; i < examples.length; i++) {
    const example = examples[i];
    if (example.smoke === false) {
      skippedCount++;
      continue;
    }
    const entryPath = packageEntry(example.openPath);
    const fsView = new VirtualFS();
    addLibrary(fsView, stdLibrary);
    addLibrary(fsView, xLibrary);
    addLibrary(fsView, example.files || {});

    const outputPath = `build/example_${i}.out`;
    let compile;
    try {
      compile = await runWasiModule(
        generatedCompilerModule,
        fsView,
        ["rtg", "-T", example.defaultTarget || "wasi/wasm32", "-o", outputPath, entryPath],
        { createWASI, WASIExit }
      );
    } catch (err) {
      failures.push(
        `example ${entryPath} (${example.defaultTarget}) trapped\n${err && err.stack ? err.stack : String(err)}`
      );
      continue;
    }
    if (compile.exitCode !== 0) {
      failures.push(
        `example ${entryPath} (${example.defaultTarget}) failed with ${compile.exitCode}\n${(compile.stdout + compile.stderr).trim()}`
      );
      continue;
    }
    const out = fsView.readFile(outputPath);
    if (!out || out.length === 0) {
      failures.push(`example ${entryPath} (${example.defaultTarget}) produced no output`);
      continue;
    }
    compiledCount++;
  }

  if (failures.length > 0) {
    fail(failures.join("\n\n"));
  }

  console.log(
    `PASS: webpage WASM compiler compiled ${compiledCount} bundled examples` +
      (skippedCount > 0 ? ` (skipped ${skippedCount} known-broken example)` : "")
  );
}

main().catch((err) => {
  fail(err && err.stack ? err.stack : String(err));
});
