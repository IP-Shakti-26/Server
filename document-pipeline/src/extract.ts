import fs from "fs";
import path from "path";
import { createRequire } from "module";
import { pathToFileURL } from "url";
import { getDocument, GlobalWorkerOptions } from "pdfjs-dist/legacy/build/pdf.mjs";
import type { TextItem } from "pdfjs-dist/types/src/display/api";
import { ExtractedDocument } from "./types";

// On Windows, Node's ESM loader requires file:// URLs (not raw absolute paths)
const _require = createRequire(import.meta.url);
const workerPath = _require.resolve("pdfjs-dist/legacy/build/pdf.worker.mjs");
GlobalWorkerOptions.workerSrc = pathToFileURL(workerPath).href;

// ─── Paths ────────────────────────────────────────────────────────────────────

const RAW_DOCS_DIR  = path.resolve(__dirname, "../raw_docs");
const EXTRACTED_DIR = path.resolve(__dirname, "../extracted");

// ─── Thresholds ───────────────────────────────────────────────────────────────

const MIN_PAGE_CHARS = 50;   // pages below this are considered blank
const MIN_DOC_CHARS  = 500;  // docs below this are likely scanned PDFs

// ─── Helpers ──────────────────────────────────────────────────────────────────

function ensureDir(dir: string): void {
  if (!fs.existsSync(dir)) fs.mkdirSync(dir, { recursive: true });
}

async function extractPdf(filePath: string): Promise<ExtractedDocument> {
  const data   = new Uint8Array(fs.readFileSync(filePath));
  const loadingTask = getDocument({ data, useWorkerFetch: false, isEvalSupported: false });
  const pdfDoc = await loadingTask.promise;

  const pages: { page_num: number; text: string }[] = [];

  for (let pageNum = 1; pageNum <= pdfDoc.numPages; pageNum++) {
    const page        = await pdfDoc.getPage(pageNum);
    const textContent = await page.getTextContent();

    // Concatenate text items, preserving line breaks via transform y-position
    let lastY: number | null = null;
    let pageText = "";

    for (const item of textContent.items) {
      if (!("str" in item)) continue;
      const textItem = item as TextItem;
      const y = textItem.transform[5];

      if (lastY !== null && Math.abs(y - lastY) > 2) {
        pageText += "\n";
      }
      pageText += textItem.str;
      lastY = y;
    }

    const trimmed = pageText.trim();
    if (trimmed.length >= MIN_PAGE_CHARS) {
      pages.push({ page_num: pageNum, text: trimmed });
    }
  }

  return {
    filename: path.basename(filePath),
    pages,
  };
}

// ─── Main ─────────────────────────────────────────────────────────────────────

async function main(): Promise<void> {
  ensureDir(EXTRACTED_DIR);

  // Collect all PDF files from raw_docs/ (case-insensitive extension match)
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

      // ── Scanned PDF warning ──────────────────────────────────────────────
      const totalChars = doc.pages.reduce((sum, p) => sum + p.text.length, 0);
      if (totalChars < MIN_DOC_CHARS) {
        console.warn(
          `  ⚠  WARNING: Only ${totalChars} chars extracted from "${filename}".\n` +
          `     This may be a scanned/image-only PDF. Consider running OCR first.`
        );
      }

      // ── Write JSON output ────────────────────────────────────────────────
      fs.writeFileSync(outPath, JSON.stringify(doc, null, 2), "utf-8");
      console.log(`  → ${doc.pages.length} page(s) extracted  →  ${outPath}`);
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      console.error(`  [ERROR] Failed to extract "${filename}": ${message}`);
    }
  }

  console.log(`\n${"─".repeat(50)}\nExtraction complete.\n`);
}

main();
