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

function parseArgs(argv) {
  const out = {
    platform: "",
    compilerOut: "build/web_stage1.wasm",
    smokeOut: "build/web_smoke.wasm",
  };

  for (let i = 0; i < argv.length; i++) {
    const key = argv[i];
    const value = argv[i + 1];
    if (!value) {
      fail(`missing value for ${key}`);
    }
    if (key === "--platform") {
      out.platform = value;
      i++;
      continue;
    }
    if (key === "--compiler-out") {
      out.compilerOut = value;
      i++;
      continue;
    }
    if (key === "--smoke-out") {
      out.smokeOut = value;
      i++;
      continue;
    }
    fail(`unknown argument ${key}`);
  }

  if (!out.compilerOut) fail("required argument --compiler-out is missing");
  if (!out.smokeOut) fail("required argument --smoke-out is missing");
  return out;
}

function loadStdLibrary() {
  const stdPath = path.join(repoRoot, "web", "std-library.js");
  const src = fs.readFileSync(stdPath, "utf8");
  const marker = "export const STD_LIBRARY = ";
  const start = src.indexOf(marker);
  if (start < 0) {
    fail("web/std-library.js does not contain STD_LIBRARY export");
  }
  const jsonStart = start + marker.length;
  const jsonEnd = src.lastIndexOf(";");
  if (jsonEnd <= jsonStart) {
    fail("failed to parse STD_LIBRARY JSON payload");
  }
  try {
    return JSON.parse(src.slice(jsonStart, jsonEnd));
  } catch (err) {
    fail(`failed to parse STD_LIBRARY: ${err.message}`);
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

async function main() {
  const args = parseArgs(process.argv.slice(2));

  const tmpDir = path.join(repoRoot, "build", "web-wasm-selfhost-node");
  fs.mkdirSync(tmpDir, { recursive: true });
  const wasiModulePath = path.join(tmpDir, "wasi.mjs");
  fs.copyFileSync(path.join(repoRoot, "web", "wasi.js"), wasiModulePath);

  const wasiModule = await import(pathToFileURL(wasiModulePath).href);
  const { VirtualFS, createWASI, WASIExit } = wasiModule;
  if (!VirtualFS || !createWASI || !WASIExit) {
    fail("failed to load WASI helpers from web/wasi.js");
  }

  const stdLibrary = loadStdLibrary();
  const compilerWasmPath = path.join(repoRoot, "web", "compiler.wasm");
  const compilerWasm = fs.readFileSync(compilerWasmPath);
  const compilerModule = await WebAssembly.compile(compilerWasm);

  const addStdLib = (fsView) => {
    for (const [filePath, content] of Object.entries(stdLibrary)) {
      fsView.addFile(filePath, content);
    }
  };

  const fsStage1 = new VirtualFS();
  addStdLib(fsStage1);
  const stage1 = await runWasiModule(
    compilerModule,
    fsStage1,
    ["rtg", "-T", "wasi/wasm32", "-o", args.compilerOut, "std/compiler"],
    { createWASI, WASIExit }
  );
  if (stage1.exitCode !== 0) {
    fail(`webpage WASM compiler exited with ${stage1.exitCode}\n${(stage1.stdout + stage1.stderr).trim()}`);
  }

  const compilerOutBytes = fsStage1.readFile(args.compilerOut);
  if (!compilerOutBytes || compilerOutBytes.length === 0) {
    fail(`WASM compiler did not produce ${args.compilerOut}`);
  }

  const generatedCompilerModule = await WebAssembly.compile(compilerOutBytes);
  const fsStage2 = new VirtualFS();
  addStdLib(fsStage2);
  fsStage2.addFile("tests/types_int.go", fs.readFileSync(path.join(repoRoot, "tests", "types_int.go"), "utf8"));
  const stage2 = await runWasiModule(
    generatedCompilerModule,
    fsStage2,
    ["rtg", "-T", "wasi/wasm32", "-o", args.smokeOut, "tests/types_int.go"],
    { createWASI, WASIExit }
  );
  if (stage2.exitCode !== 0) {
    fail(`generated WASM compiler exited with ${stage2.exitCode}\n${(stage2.stdout + stage2.stderr).trim()}`);
  }

  const smokeOutBytes = fsStage2.readFile(args.smokeOut);
  if (!smokeOutBytes || smokeOutBytes.length === 0) {
    fail(`generated compiler did not produce ${args.smokeOut}`);
  }

  const smokeProgramModule = await WebAssembly.compile(smokeOutBytes);
  const fsRun = new VirtualFS();
  const run = await runWasiModule(smokeProgramModule, fsRun, ["program"], { createWASI, WASIExit });
  if (run.exitCode !== 0) {
    fail(`generated WASM smoke program exited with ${run.exitCode}\n${(run.stdout + run.stderr).trim()}`);
  }
  const smokeOutput = (run.stdout + run.stderr).trim();
  if (!smokeOutput.includes("PASS")) {
    fail(`generated WASM smoke output missing PASS\n${smokeOutput}`);
  }

  console.log(
    `PASS: webpage WASM compiler self-compiled and ran (${compilerOutBytes.length} bytes)${
      args.platform ? ` on ${args.platform}` : ""
    }`
  );
}

main().catch((err) => {
  fail(err && err.stack ? err.stack : String(err));
});
