import fs from "fs";
import path from "path";
import { Chunk } from "./types";
import { CORPUS_MANIFEST } from "./manifest";

// ─── Paths ────────────────────────────────────────────────────────────────────

const EXTRACTED_DIR = path.resolve(__dirname, "../extracted");
const CHUNKS_DIR    = path.resolve(__dirname, "../chunks");

// ─── Chunking Parameters ──────────────────────────────────────────────────────

const TARGET_CHARS  = 3200;  // ≈ 800 tokens
const OVERLAP_CHARS = 400;   // ≈ 100-token overlap
const MIN_CHUNK     = 100;   // discard tiny trailing fragments

// ─── Section Detection ────────────────────────────────────────────────────────

const SECTION_PATTERNS: RegExp[] = [
  /Section\s+\d+[A-Za-z]?\([a-z]\)/,
  /Section\s+\d+[A-Za-z]?/,
  /Rule\s+\d+/,
  /Chapter\s+[IVXLC\d]+/,
  /Article\s+\d+/,
  /Schedule\s+[IVXLC\d]+/,
];

function detectSectionRef(text: string): string {
  const head = text.slice(0, 300);
  for (const pat of SECTION_PATTERNS) {
    const m = head.match(pat);
    if (m) return m[0];
  }
  return "unknown";
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

function ensureDir(dir: string): void {
  if (!fs.existsSync(dir)) fs.mkdirSync(dir, { recursive: true });
}

// ─── Core Chunker ─────────────────────────────────────────────────────────────
// Takes a flat string and produces chunk objects. No memory surprise here.

function chunkText(fullText: string, stem: string): Chunk[] {
  const meta   = CORPUS_MANIFEST[`${stem}.pdf`];
  const chunks: Chunk[] = [];
  let start = 0;
  let idx   = 0;

  while (start < fullText.length) {
    let end = Math.min(start + TARGET_CHARS, fullText.length);

    // Snap to paragraph boundary in the second half of the window
    if (end < fullText.length) {
      const mid   = start + Math.floor(TARGET_CHARS / 2);
      const slice = fullText.slice(mid, end);
      const brk   = slice.lastIndexOf("\n\n");
      if (brk !== -1) end = mid + brk + 2;
    }

    const text = fullText.slice(start, end).trim();

    if (text.length >= MIN_CHUNK) {
      chunks.push({
        chunk_id:    `${stem}_chunk_${String(idx).padStart(4, "0")}`,
        text,
        doc_title:    meta.doc_title,
        domain:       meta.domain,
        jurisdiction: meta.jurisdiction,
        authority:    meta.authority,
        section_ref:  detectSectionRef(text),
        source_url:   meta.source_url,
        retrieved_at: meta.retrieved_at,
        char_start:   start,
        char_end:     start + text.length,
      });
      idx++;
    }

    // Advance — guaranteed forward progress
    const next = end - OVERLAP_CHARS;
    start = next > start ? next : end;
  }

  return chunks;
}

// ─── Process One File ─────────────────────────────────────────────────────────

function processFile(jsonFile: string): number {
  const stem        = path.basename(jsonFile, ".json");
  const inPath      = path.join(EXTRACTED_DIR, jsonFile);
  const outPath     = path.join(CHUNKS_DIR, `${stem}_chunks.json`);

  // Read & parse — 1-2 MB files are fine for JSON.parse
  // eslint-disable-next-line prefer-const
  let raw  = JSON.parse(fs.readFileSync(inPath, "utf-8")) as {
    pages: { page_num: number; text: string }[];
  };

  // Build full text — free raw immediately after
  const fullText = raw.pages.map((p) => `[Page ${p.page_num}]\n${p.text}`).join("\n\n");
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  (raw as any) = null;

  const chunks = chunkText(fullText, stem);

  // Write chunks as JSON
  fs.writeFileSync(outPath, JSON.stringify(chunks, null, 2), "utf-8");

  return chunks.length;
}

// ─── Main ─────────────────────────────────────────────────────────────────────

function main(): void {
  ensureDir(CHUNKS_DIR);

  const jsonFiles = fs
    .readdirSync(EXTRACTED_DIR)
    .filter((f) => f.endsWith(".json"));

  if (jsonFiles.length === 0) {
    console.error(`[ERROR] No JSON files in ${EXTRACTED_DIR}. Run "npm run extract" first.`);
    process.exit(1);
  }

  console.log(`\nChunking ${jsonFiles.length} file(s)\n${"─".repeat(50)}`);

  let total = 0;

  for (const jsonFile of jsonFiles) {
    const stem        = path.basename(jsonFile, ".json");
    const manifestKey = `${stem}.pdf`;

    if (!CORPUS_MANIFEST[manifestKey]) {
      console.warn(`  ⚠  "${manifestKey}" not in CORPUS_MANIFEST — skipping.`);
      continue;
    }

    console.log(`\nProcessing: ${jsonFile}`);

    try {
      const count = processFile(jsonFile);
      console.log(`  → ${count} chunks written`);
      total += count;
    } catch (err) {
      console.error(`  [ERROR] ${err instanceof Error ? err.message : err}`);
    }
  }

  console.log(`\n${"─".repeat(50)}\nTotal chunks: ${total}\n`);
}

main();
