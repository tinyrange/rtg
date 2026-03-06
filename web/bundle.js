#!/usr/bin/env node
// Assembles playground.html: a single self-contained HTML file
// with all JS inlined and compiler.wasm base64-embedded.

const fs = require("fs");
const path = require("path");

const webDir = __dirname;
const rootDir = path.dirname(webDir);
const exampleManifest = require(path.join(webDir, "examples-manifest.js"));

// --- Step 1: Bundle std library ---
function walkGo(dir, base) {
  const result = {};
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const rel = base ? base + "/" + entry.name : entry.name;
    if (entry.isDirectory()) {
      Object.assign(result, walkGo(path.join(dir, entry.name), rel));
    } else if (entry.name.endsWith(".go")) {
      result[rel] = fs.readFileSync(path.join(dir, entry.name), "utf-8");
    }
  }
  return result;
}

const lib = walkGo(path.join(rootDir, "std"), "std");
const xlib = walkGo(path.join(rootDir, "x"), "x");
const stdLibJS = "const STD_LIBRARY = " + JSON.stringify(lib) + ";";
const xLibJS = "const X_LIBRARY = " + JSON.stringify(xlib) + ";";
console.log("Bundled " + Object.keys(lib).length + " std library files");
console.log("Bundled " + Object.keys(xlib).length + " x library files");

function buildExampleLibrary() {
  return exampleManifest.map((spec) => {
    const files = {};
    for (const mapping of spec.files || []) {
      files[mapping.to] = fs.readFileSync(path.join(rootDir, mapping.from), "utf8");
    }
    return {
      defaultTarget: spec.defaultTarget || "wasi/wasm32",
      smoke: spec.smoke !== false,
      openPath: spec.openPath || Object.keys(files)[0] || "user/main.go",
      files,
    };
  });
}

const examples = buildExampleLibrary();
const examplesLibJS = "const EXAMPLE_LIBRARY = " + JSON.stringify(examples) + ";";
console.log("Bundled " + examples.length + " examples");

// Also write the ES module version for dev mode
fs.writeFileSync(
  path.join(webDir, "std-library.js"),
  "// Auto-generated - do not edit\nexport const STD_LIBRARY = " + JSON.stringify(lib) + ";\n"
);
fs.writeFileSync(
  path.join(webDir, "x-library.js"),
  "// Auto-generated - do not edit\nexport const X_LIBRARY = " + JSON.stringify(xlib) + ";\n"
);
fs.writeFileSync(
  path.join(webDir, "examples-library.js"),
  "// Auto-generated - do not edit\nexport const EXAMPLE_LIBRARY = " + JSON.stringify(examples) + ";\n"
);

// --- Step 2: Read source files ---
let html = fs.readFileSync(path.join(webDir, "index.html"), "utf-8");
const wasiJS = fs.readFileSync(path.join(webDir, "wasi.js"), "utf-8");
const playgroundJS = fs.readFileSync(path.join(webDir, "playground.js"), "utf-8");
const compilerWasm = fs.readFileSync(path.join(webDir, "compiler.wasm"));

// --- Step 3: Base64-encode compiler.wasm ---
const wasmB64 = compilerWasm.toString("base64");
console.log("Encoded compiler.wasm: " + (wasmB64.length / 1024).toFixed(0) + " KB base64");

// --- Step 4: Strip ES module syntax ---
function stripImports(code) {
  return code
    .replace(/^import\s+\{[^}]*\}\s+from\s+["'][^"']*["'];?\s*$/gm, "")
    .replace(/^export\s+(class|function|const|let|var)\s/gm, "$1 ");
}

// --- Step 5: Build combined script ---
const script = [
  "// --- Embedded compiler.wasm (base64) ---",
  'const COMPILER_WASM_B64 = "' + wasmB64 + '";',
  "",
  "// --- wasi.js ---",
  stripImports(wasiJS),
  "",
  "// --- std-library.js ---",
  stdLibJS,
  "",
  "// --- x-library.js ---",
  xLibJS,
  "",
  "// --- examples-library.js ---",
  examplesLibJS,
  "",
  "// --- playground.js ---",
  stripImports(playgroundJS),
].join("\n");

// --- Step 6: Patch HTML ---

// Replace script tag with inline script
html = html.replace(
  /\s*<!--SCRIPTS-->\s*<script type="module" src="playground\.js"><\/script>/,
  "\n  <script>\n" + script + "\n  </script>"
);

// Patch loadCompilerModule to decode from base64 instead of fetching
html = html.replace(
  'compilerModule = await WebAssembly.compileStreaming(fetch("compiler.wasm"));',
  [
    "const b64 = COMPILER_WASM_B64;",
    "    const binStr = atob(b64);",
    "    const bytes = new Uint8Array(binStr.length);",
    "    for (let i = 0; i < binStr.length; i++) bytes[i] = binStr.charCodeAt(i);",
    "    compilerModule = await WebAssembly.compile(bytes);",
  ].join("\n")
);

// Remove the now-unused fetch response check
html = html.replace(
  /const response = await fetch\("compiler\.wasm"\);\s*if \(!response\.ok\) \{\s*setStatus\("Failed to load compiler\.wasm"\);\s*return;\s*\}\s*/,
  ""
);

// --- Step 7: Write output ---
fs.writeFileSync(path.join(webDir, "playground.html"), html);
const sizeKB = (Buffer.byteLength(html) / 1024).toFixed(0);
console.log("Generated playground.html (" + sizeKB + " KB)");
