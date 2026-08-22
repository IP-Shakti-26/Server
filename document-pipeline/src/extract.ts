import fs from "fs";
import path from "path";
// pdf-parse ships a CJS default export; require() avoids the callable-type error
// eslint-disable-next-line @typescript-eslint/no-require-imports
const pdfParse = require("pdf-parse") as (buf: Buffer) => Promise<{ text: string }>;
import { ExtractedDocument } from "./types";

// ─── Paths ────────────────────────────────────────────────────────────────────

const RAW_DOCS_DIR = path.resolve(__dirname, "../raw_docs");
const EXTRACTED_DIR = path.resolve(__dirname, "../extracted");

// ─── Thresholds ───────────────────────────────────────────────────────────────

const MIN_PAGE_CHARS = 50;       // pages below this are considered blank
const MIN_DOC_CHARS  = 500;      // docs below this are likely scanned PDFs

// ─── Helpers ──────────────────────────────────────────────────────────────────

function ensureDir(dir: string): void {
  if (!fs.existsSync(dir)) {
    fs.mkdirSync(dir, { recursive: true });
  }
}

async function extractPdf(filePath: string): Promise<ExtractedDocument> {
  const buffer = fs.readFileSync(filePath);
  const data   = await pdfParse(buffer);

  // pdf-parse gives the full text; split on form feed to get per-page text
  const rawPages = data.text.split("\f");

  const pages = rawPages
    .map((text: string, idx: number) => ({ page_num: idx + 1, text: text.trim() }))
    .filter((p: { page_num: number; text: string }) => p.text.length >= MIN_PAGE_CHARS);

  return {
    filename: path.basename(filePath),
    pages,
  };
}

// ─── Main ─────────────────────────────────────────────────────────────────────

async function main(): Promise<void> {
  ensureDir(EXTRACTED_DIR);

  // Collect all PDF files from raw_docs/
  const allFiles = fs.readdirSync(RAW_DOCS_DIR);
  const pdfFiles = allFiles.filter((f) => f.toLowerCase().endsWith(".pdf"));

  if (pdfFiles.length === 0) {
    console.error(
      `[ERROR] No PDF files found in ${RAW_DOCS_DIR}.\n` +
      `Place your source documents there before running extract.`
    );
    process.exit(1);
  }

  console.log(`\nFound ${pdfFiles.length} PDF(s) in raw_docs/\n${"─".repeat(50)}`);

  for (const filename of pdfFiles) {
    const filePath = path.join(RAW_DOCS_DIR, filename);
    const stem     = path.basename(filename, path.extname(filename));
    const outPath  = path.join(EXTRACTED_DIR, `${stem}.json`);

    console.log(`\nExtracting: ${filename}`);

    try {
      const doc = await extractPdf(filePath);

      // ── Scanned PDF warning ────────────────────────────────────────────────
      const totalChars = doc.pages.reduce((sum, p) => sum + p.text.length, 0);
      if (totalChars < MIN_DOC_CHARS) {
        console.warn(
          `  ⚠  WARNING: Only ${totalChars} characters extracted from "${filename}".\n` +
          `     This may be a scanned/image-only PDF. ` +
          `Consider running OCR before ingestion.`
        );
      }

      // ── Write JSON output ──────────────────────────────────────────────────
      fs.writeFileSync(outPath, JSON.stringify(doc, null, 2), "utf-8");

      console.log(`  → ${doc.pages.length} page(s) extracted  →  ${outPath}`);
    } catch (err) {
      // Per-file error: log and continue with remaining files
      const message = err instanceof Error ? err.message : String(err);
      console.error(`  [ERROR] Failed to extract "${filename}": ${message}`);
    }
  }

  console.log(`\n${"─".repeat(50)}\nExtraction complete.\n`);
}

main();
