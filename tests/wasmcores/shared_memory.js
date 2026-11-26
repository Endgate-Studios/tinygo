// Checks a module imports one shared memory with a maximum.
// WebAssembly core spec 5.5.10, import section; limits flags 0x03 = shared with max.
const fs = require('fs');
const bytes = fs.readFileSync(process.argv[2]);
let i = 8;
function u32() { let r = 0, s = 0, b; do { b = bytes[i++]; r |= (b & 0x7f) << s; s += 7; } while (b & 0x80); return r >>> 0; }
function name() { const n = u32(); const s = bytes.slice(i, i + n).toString(); i += n; return s; }
let found = null;
while (i < bytes.length) {
  const id = bytes[i++], size = u32(), end = i + size;
  if (id === 2) {
    const count = u32();
    for (let k = 0; k < count; k++) {
      const mod = name(), field = name(), kind = bytes[i++];
      if (kind === 0) { u32(); } // func
      else if (kind === 1) { i++; const f = bytes[i++]; u32(); if (f & 1) u32(); } // table
      else if (kind === 2) { const flags = bytes[i++]; const min = u32(); const max = (flags & 1) ? u32() : null; found = { mod, field, flags, min, max }; }
      else if (kind === 3) { i += 2; } // global
    }
  }
  i = end;
}
if (!found) { console.log('FAIL no memory import'); process.exit(1); }
if ((found.flags & 3) !== 3) { console.log('FAIL memory not shared with a maximum', found); process.exit(1); }
console.log('ok shared memory', found.mod + '.' + found.field, 'pages', found.min, '..', found.max);
