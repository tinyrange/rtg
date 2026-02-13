// WASI shim for RTG2 playground
// Provides VirtualFS and all 14 wasi_snapshot_preview1 imports

export class WASIExit extends Error {
  constructor(code) {
    super(`WASI exit: ${code}`);
    this.code = code;
  }
}

export class VirtualFS {
  constructor() {
    this.files = new Map();    // path -> Uint8Array
    this.dirs = new Set(["."]);
    this.nextFD = 4;           // 0=stdin, 1=stdout, 2=stderr, 3=preopened root
    this.openFDs = new Map();
    // fd 3 is the preopened root directory "."
    this.openFDs.set(3, { path: ".", offset: 0, isDir: true, writable: false, buffer: null });
  }

  addFile(path, content) {
    // Normalize path: strip leading ./
    path = normalizePath(path);
    this.files.set(path, typeof content === "string" ? new TextEncoder().encode(content) : content);
    // Ensure parent directories exist
    const parts = path.split("/");
    for (let i = 1; i < parts.length; i++) {
      this.dirs.add(parts.slice(0, i).join("/"));
    }
  }

  readFile(path) {
    path = normalizePath(path);
    return this.files.get(path) || null;
  }

  open(path, oflags) {
    path = normalizePath(path);
    const isDir = (oflags & 2) !== 0;    // OFLAGS_DIRECTORY
    const creat = (oflags & 1) !== 0;    // OFLAGS_CREAT
    const trunc = (oflags & 8) !== 0;    // OFLAGS_TRUNC

    if (isDir) {
      if (!this.dirs.has(path)) return -1; // ENOENT
      const fd = this.nextFD++;
      this.openFDs.set(fd, { path, offset: 0, isDir: true, writable: false, buffer: null });
      return fd;
    }

    if (this.files.has(path)) {
      const fd = this.nextFD++;
      const existing = this.files.get(path);
      this.openFDs.set(fd, {
        path,
        offset: 0,
        isDir: false,
        writable: creat || trunc,
        buffer: (creat && trunc) ? new Uint8Array(0) : existing,
      });
      if (creat && trunc) {
        this.files.set(path, new Uint8Array(0));
      }
      return fd;
    }

    if (creat) {
      this.files.set(path, new Uint8Array(0));
      // Ensure parent dirs exist
      const parts = path.split("/");
      for (let i = 1; i < parts.length; i++) {
        this.dirs.add(parts.slice(0, i).join("/"));
      }
      const fd = this.nextFD++;
      this.openFDs.set(fd, { path, offset: 0, isDir: false, writable: true, buffer: new Uint8Array(0) });
      return fd;
    }

    return -1; // ENOENT
  }

  close(fd) {
    const entry = this.openFDs.get(fd);
    if (!entry) return false;
    // Flush buffer to files map for writable files
    if (entry.writable && entry.buffer) {
      this.files.set(entry.path, entry.buffer);
    }
    this.openFDs.delete(fd);
    return true;
  }

  write(fd, data) {
    const entry = this.openFDs.get(fd);
    if (!entry || entry.isDir) return -1;
    // Append to buffer
    const newBuf = new Uint8Array(entry.buffer.length + data.length);
    newBuf.set(entry.buffer);
    newBuf.set(data, entry.buffer.length);
    entry.buffer = newBuf;
    entry.offset = newBuf.length;
    // Also update in files map
    this.files.set(entry.path, newBuf);
    return data.length;
  }

  read(fd, length) {
    const entry = this.openFDs.get(fd);
    if (!entry || entry.isDir) return null;
    const content = this.files.get(entry.path) || entry.buffer;
    const available = Math.min(length, content.length - entry.offset);
    if (available <= 0) return new Uint8Array(0);
    const result = content.slice(entry.offset, entry.offset + available);
    entry.offset += available;
    return result;
  }

  readdir(path) {
    path = normalizePath(path);
    const entries = [];
    const prefix = path === "." ? "" : path + "/";

    // Collect direct children (files)
    for (const filePath of this.files.keys()) {
      const rel = prefix ? (filePath.startsWith(prefix) ? filePath.slice(prefix.length) : null) : filePath;
      if (rel !== null && rel.length > 0 && !rel.includes("/")) {
        entries.push({ name: rel, type: 4 }); // 4 = regular file
      }
    }

    // Collect direct children (dirs)
    for (const dirPath of this.dirs) {
      if (dirPath === path) continue;
      const rel = prefix ? (dirPath.startsWith(prefix) ? dirPath.slice(prefix.length) : null) : dirPath;
      if (rel !== null && rel.length > 0 && !rel.includes("/")) {
        entries.push({ name: rel, type: 3 }); // 3 = directory
      }
    }

    return entries;
  }

  mkdir(path) {
    path = normalizePath(path);
    this.dirs.add(path);
    // Ensure parent directories exist
    const parts = path.split("/");
    for (let i = 1; i < parts.length; i++) {
      this.dirs.add(parts.slice(0, i).join("/"));
    }
  }

  unlink(path) {
    path = normalizePath(path);
    return this.files.delete(path);
  }

  rmdir(path) {
    path = normalizePath(path);
    return this.dirs.delete(path);
  }
}

function normalizePath(p) {
  // Strip leading ./ and /
  while (p.startsWith("./")) p = p.slice(2);
  while (p.startsWith("/")) p = p.slice(1);
  return p;
}

// WASI error codes
const ESUCCESS = 0;
const EBADF = 8;
const ENOENT = 44;
const EEXIST = 20;

export function createWASI(fs, args, callbacks = {}) {
  const { onStdout, onStderr } = callbacks;
  let memory = null;

  function setMemory(mem) {
    memory = mem;
  }

  function getMemView() {
    return new DataView(memory.buffer);
  }

  function getMemU8() {
    return new Uint8Array(memory.buffer);
  }

  function readString(ptr, len) {
    return new TextDecoder().decode(new Uint8Array(memory.buffer, ptr, len));
  }

  function writeString(ptr, str) {
    const bytes = new TextEncoder().encode(str);
    new Uint8Array(memory.buffer).set(bytes, ptr);
    return bytes.length;
  }

  // Encode args as null-terminated C strings
  const encodedArgs = args.map(a => new TextEncoder().encode(a + "\0"));

  const imports = {
    fd_write(fd, iovs, iovsLen, nwrittenPtr) {
      const view = getMemView();
      const mem = getMemU8();
      let totalWritten = 0;

      for (let i = 0; i < iovsLen; i++) {
        const bufPtr = view.getUint32(iovs + i * 8, true);
        const bufLen = view.getUint32(iovs + i * 8 + 4, true);
        const data = mem.slice(bufPtr, bufPtr + bufLen);

        if (fd === 1) {
          if (onStdout) onStdout(data);
        } else if (fd === 2) {
          if (onStderr) onStderr(data);
        } else {
          const written = fs.write(fd, data);
          if (written < 0) return EBADF;
        }
        totalWritten += bufLen;
      }

      view.setUint32(nwrittenPtr, totalWritten, true);
      return ESUCCESS;
    },

    fd_read(fd, iovs, iovsLen, nreadPtr) {
      const view = getMemView();
      const mem = getMemU8();
      let totalRead = 0;

      for (let i = 0; i < iovsLen; i++) {
        const bufPtr = view.getUint32(iovs + i * 8, true);
        const bufLen = view.getUint32(iovs + i * 8 + 4, true);

        if (fd === 0) {
          // stdin: return EOF
          break;
        }

        const data = fs.read(fd, bufLen);
        if (data === null) return EBADF;
        mem.set(data, bufPtr);
        totalRead += data.length;
        if (data.length < bufLen) break;
      }

      view.setUint32(nreadPtr, totalRead, true);
      return ESUCCESS;
    },

    fd_close(fd) {
      if (fd <= 2) return ESUCCESS; // don't close stdio
      return fs.close(fd) ? ESUCCESS : EBADF;
    },

    // path_open has 2 i64 BigInt params (rights_base, rights_inheriting)
    path_open(dirfd, dirflags, pathPtr, pathLen, oflags, rights_base, rights_inheriting, fdflags, fdOut) {
      const pathStr = readString(pathPtr, pathLen);
      const fd = fs.open(pathStr, oflags);
      if (fd < 0) return ENOENT;
      const view = getMemView();
      view.setUint32(fdOut, fd, true);
      return ESUCCESS;
    },

    args_sizes_get(argcPtr, argvBufSizePtr) {
      const view = getMemView();
      view.setUint32(argcPtr, args.length, true);
      let totalSize = 0;
      for (const a of encodedArgs) totalSize += a.length;
      view.setUint32(argvBufSizePtr, totalSize, true);
      return ESUCCESS;
    },

    args_get(argvPtr, argvBufPtr) {
      const view = getMemView();
      const mem = getMemU8();
      let bufOffset = argvBufPtr;
      for (let i = 0; i < encodedArgs.length; i++) {
        view.setUint32(argvPtr + i * 4, bufOffset, true);
        mem.set(encodedArgs[i], bufOffset);
        bufOffset += encodedArgs[i].length;
      }
      return ESUCCESS;
    },

    environ_sizes_get(countPtr, sizePtr) {
      const view = getMemView();
      view.setUint32(countPtr, 0, true);
      view.setUint32(sizePtr, 0, true);
      return ESUCCESS;
    },

    environ_get(_environPtr, _environBufPtr) {
      return ESUCCESS;
    },

    proc_exit(code) {
      throw new WASIExit(code);
    },

    path_create_directory(dirfd, pathPtr, pathLen) {
      const pathStr = readString(pathPtr, pathLen);
      fs.mkdir(pathStr);
      return ESUCCESS;
    },

    path_remove_directory(dirfd, pathPtr, pathLen) {
      const pathStr = readString(pathPtr, pathLen);
      fs.rmdir(pathStr);
      return ESUCCESS;
    },

    path_unlink_file(dirfd, pathPtr, pathLen) {
      const pathStr = readString(pathPtr, pathLen);
      fs.unlink(pathStr);
      return ESUCCESS;
    },

    // fd_readdir has i64 BigInt cookie param
    fd_readdir(fd, bufPtr, bufLen, cookie, bufusedPtr) {
      const entry = fs.openFDs.get(fd);
      if (!entry || !entry.isDir) return EBADF;

      const entries = fs.readdir(entry.path);
      const view = getMemView();
      const mem = getMemU8();
      let offset = 0;
      const cookieNum = Number(cookie);

      for (let i = cookieNum; i < entries.length; i++) {
        const e = entries[i];
        const nameBytes = new TextEncoder().encode(e.name);
        const entrySize = 24 + nameBytes.length; // d_next(8) + d_ino(8) + d_namlen(4) + d_type(1) + name

        if (offset + entrySize > bufLen) break;

        const base = bufPtr + offset;
        // d_next (8 bytes LE) - next cookie value
        view.setUint32(base, i + 1, true);
        view.setUint32(base + 4, 0, true);
        // d_ino (8 bytes LE) - inode, just use index
        view.setUint32(base + 8, i + 1, true);
        view.setUint32(base + 12, 0, true);
        // d_namlen (4 bytes LE)
        view.setUint32(base + 16, nameBytes.length, true);
        // d_type (1 byte) - 3=dir, 4=file
        mem[base + 20] = e.type;
        // Pad bytes 21-23 may exist but name starts at offset 24 per WASI spec
        // Actually per the os_wasi.go parsing, name starts at offset+24
        // Zero out padding
        mem[base + 21] = 0;
        mem[base + 22] = 0;
        mem[base + 23] = 0;
        // name
        mem.set(nameBytes, base + 24);
        offset += entrySize;
      }

      view.setUint32(bufusedPtr, offset, true);
      return ESUCCESS;
    },

    fd_prestat_get(fd, bufPtr) {
      if (fd === 3) {
        const view = getMemView();
        // tag = 0 (__wasi_preopentype_dir)
        view.setUint32(bufPtr, 0, true);
        // name_len = 1 (for ".")
        view.setUint32(bufPtr + 4, 1, true);
        return ESUCCESS;
      }
      return EBADF; // no more preopened dirs
    },

    fd_prestat_dir_name(fd, pathPtr, pathLen) {
      if (fd === 3) {
        const mem = getMemU8();
        mem[pathPtr] = 0x2E; // "."
        return ESUCCESS;
      }
      return EBADF;
    },
  };

  return {
    imports: { wasi_snapshot_preview1: imports },
    setMemory,
  };
}
