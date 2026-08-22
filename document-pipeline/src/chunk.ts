import fs from "fs";
import path from "path";
import { Chunk, ExtractedDocument } from "./types";
import { CORPUS_MANIFEST } from "./manifest";

// ─── Paths ────────────────────────────────────────────────────────────────────

const EXTRACTED_DIR = path.resolve(__dirname, "../extracted");
const CHUNKS_DIR    = path.resolve(__dirname, "../chunks");

// ─── Chunking Parameters ─────────────────────────────────────────────────────

const TARGET_CHARS  = 3200;   // ≈ 800 tokens
const OVERLAP_CHARS = 400;    // ≈ 100 token overlap
const MIN_CHUNK     = 100;    // discard tiny trailing fragments

// ─── Section Detection ────────────────────────────────────────────────────────

const SECTION_PATTERNS: RegExp[] = [
  /Section\s+\d+[A-Za-z]?\([a-z]\)/,   // e.g. "Section 3(p)"
  /Section\s+\d+[A-Za-z]?/,            // e.g. "Section 12A"
  /Rule\s+\d+/,                         // e.g. "Rule 7"
  /Chapter\s+[IVXLC\d]+/,              // e.g. "Chapter IV"
  /Article\s+\d+/,                      // e.g. "Article 27"
  /Schedule\s+[IVXLC\d]+/,             // e.g. "Schedule III"
];

function detectSectionRef(text: string): string {
  const head = text.slice(0, 300);
  for (const pattern of SECTION_PATTERNS) {
    const match = head.match(pattern);
    if (match) return match[0];
  }
  return "unknown";
}

// ─── Chunker ──────────────────────────────────────────────────────────────────

function chunkText(fullText: string, stem: string): Chunk[] {
  const meta = CORPUS_MANIFEST[`${stem}.pdf`];
  const chunks: Chunk[] = [];

  let start = 0;
  let index = 0;

  while (start < fullText.length) {
    // Candidate end = start + target window
    let end = Math.min(start + TARGET_CHARS, fullText.length);

    // Try to break on a paragraph boundary within the second half of the window
    if (end < fullText.length) {
      const midpoint  = start + Math.floor(TARGET_CHARS / 2);
      const searchSlice = fullText.slice(midpoint, end);
      const lastBreak   = searchSlice.lastIndexOf("\n\n");

      if (lastBreak !== -1) {
        // Snap end to the paragraph break (keep the double newline in current chunk)
        end = midpoint + lastBreak + 2;
      }
    }

    const text = fullText.slice(start, end).trim();

    if (text.length >= MIN_CHUNK) {
      const chunk_id = `${stem}_chunk_${String(index).padStart(4, "0")}`;

      chunks.push({
        chunk_id,
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

      index++;
    }

    // Advance with overlap so context bleeds across chunks
    start = end - OVERLAP_CHARS;

    // Safety guard: if we didn't advance, force forward to avoid infinite loop
    if (start <= (end - TARGET_CHARS - 1)) {
      start = end;
    }
  }

  return chunks;
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

function ensureDir(dir: string): void {
  if (!fs.existsSync(dir)) fs.mkdirSync(dir, { recursive: true });
}

function buildFullText(doc: ExtractedDocument): string {
  return doc.pages
    .map((p) => `[Page ${p.page_num}]\n${p.text}`)
    .join("\n\n");
}

// ─── Main ─────────────────────────────────────────────────────────────────────

async function main(): Promise<void> {
  ensureDir(CHUNKS_DIR);

  const jsonFiles = fs
    .readdirSync(EXTRACTED_DIR)
    .filter((f) => f.endsWith(".json"));

  if (jsonFiles.length === 0) {
    console.error(
      `[ERROR] No JSON files found in ${EXTRACTED_DIR}.\n` +
      `Run "npm run extract" first.`
    );
    process.exit(1);
  }

  console.log(`\nChunking ${jsonFiles.length} extracted file(s)\n${"─".repeat(50)}`);

  let totalChunks = 0;

  for (const jsonFile of jsonFiles) {
    const stem         = path.basename(jsonFile, ".json");
    const manifestKey  = `${stem}.pdf`;

    // ── Manifest guard ─────────────────────────────────────────────────────
    if (!CORPUS_MANIFEST[manifestKey]) {
      console.warn(
        `  ⚠  WARNING: "${manifestKey}" not found in CORPUS_MANIFEST — skipping.`
      );
      continue;
    }

    const inPath  = path.join(EXTRACTED_DIR, jsonFile);
    const outPath = path.join(CHUNKS_DIR, `${stem}_chunks.json`);

    try {
      const raw: ExtractedDocument = JSON.parse(fs.readFileSync(inPath, "utf-8"));
      const fullText = buildFullText(raw);
      const chunks   = chunkText(fullText, stem);

      fs.writeFileSync(outPath, JSON.stringify(chunks, null, 2), "utf-8");

      console.log(`  ${jsonFile}: ${chunks.length} chunks  →  ${outPath}`);
      totalChunks += chunks.length;
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      console.error(`  [ERROR] Failed to chunk "${jsonFile}": ${message}`);
    }
  }

  console.log(`\n${"─".repeat(50)}\nTotal chunks across corpus: ${totalChunks}\n`);
}

main();
